package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type schedulerFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Backend            string   `json:"backend"`
		Category           string   `json:"category"`
		Engines            []string `json:"engines"`
		MaxResults         *int     `json:"max_results"`
		Query              string   `json:"query"`
		WaitTimeoutSeconds *float64 `json:"wait_timeout_seconds"`
		Threads            *int     `json:"threads"`
		Cases              []struct {
			Name       string `json:"name"`
			MaxResults *int   `json:"max_results"`
			Threads    *int   `json:"threads"`
		} `json:"cases"`
	} `json:"input"`
	Result struct {
		Status string          `json:"status"`
		Output json.RawMessage `json:"output"`
		Error  struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"result"`
}

func TestSourceWorkerCount_MatchesFrozenFixtures(t *testing.T) {
	fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-worker-formula.json")
	var output map[string]struct {
		MaxWorkers []int    `json:"max_workers"`
		Hrefs      []string `json:"hrefs"`
	}
	decodeSchedulerOutput(t, fixture, &output)

	engines := []ScheduledEngine{
		{Provider: "one"},
		{Provider: "two"},
		{Provider: "three"},
	}
	for _, testCase := range fixture.Input.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			got, err := sourceWorkerCount(engines, testCase.MaxResults, testCase.Threads)
			if err != nil {
				t.Fatalf("sourceWorkerCount: %v", err)
			}
			want := output[testCase.Name].MaxWorkers
			if len(want) != 1 {
				t.Fatalf("fixture max_workers = %#v, want one value", want)
			}
			if got != want[0] {
				t.Fatalf("sourceWorkerCount = %d, want %d", got, want[0])
			}
		})
	}

	for _, path := range []string{
		"../../testdata/contracts/pure/pure.scheduler-negative-max-workers.json",
		"../../testdata/contracts/pure/pure.scheduler-negative-ten-max-workers.json",
		"../../testdata/contracts/pure/pure.scheduler-negative-threads-workers.json",
	} {
		fixture := loadSchedulerFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			_, err := sourceWorkerCount(engines, fixture.Input.MaxResults, fixture.Input.Threads)
			if err == nil {
				t.Fatalf("sourceWorkerCount error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
			}
			if got := err.Error(); got != fixture.Result.Error.Message {
				t.Fatalf("sourceWorkerCount error = %q, want %q", got, fixture.Result.Error.Message)
			}
		})
	}

	floatBoundary := loadSchedulerFloatBoundaryFixture(t, "../../testdata/contracts/pure/pure.scheduler-worker-formula-float-boundary.json")
	for encoded, want := range floatBoundary {
		maxResults, err := strconv.Atoi(encoded)
		if err != nil {
			t.Fatalf("parse fixture max results %q: %v", encoded, err)
		}
		t.Run("float-boundary/"+encoded, func(t *testing.T) {
			got, err := sourceWorkerCount([]ScheduledEngine{{Provider: "one"}, {Provider: "two"}, {Provider: "three"}}, &maxResults, nil)
			if err != nil {
				t.Fatalf("sourceWorkerCount: %v", err)
			}
			if got := sourceMaximumWorkers(maxResults); got != want {
				t.Fatalf("sourceMaximumWorkers = %d, want %d", got, want)
			}
			if got != 3 {
				t.Fatalf("sourceWorkerCount = %d, want provider cap 3", got)
			}
		})
	}
}

func TestScheduler_MatchesFrozenWorkerPoolBoundFixture(t *testing.T) {
	fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-worker-pool-bound.json")
	release := make(chan struct{})
	oneStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	var secondOnce sync.Once
	var active, maximum atomic.Int32
	var started struct {
		one   atomic.Bool
		two   atomic.Bool
		three atomic.Bool
	}

	search := func(name string, mark *atomic.Bool) EngineSearch {
		return func(ctx context.Context, _ EngineRequest) ([]Result, error) {
			mark.Store(true)
			activeNow := active.Add(1)
			for {
				observed := maximum.Load()
				if activeNow <= observed || maximum.CompareAndSwap(observed, activeNow) {
					break
				}
			}
			defer active.Add(-1)
			if name == "one" {
				close(oneStarted)
				if err := waitForSchedulerSignal(ctx, secondStarted); err != nil {
					return nil, err
				}
			}
			if name == "two" {
				if err := waitForSchedulerSignal(ctx, oneStarted); err != nil {
					return nil, err
				}
				secondOnce.Do(func() { close(secondStarted) })
			}
			if err := waitForSchedulerSignal(ctx, release); err != nil {
				return nil, err
			}
			return []Result{schedulerTextResult(t, name, "https://pool-"+name+".example", "")}, nil
		}
	}
	engines := []ScheduledEngine{
		{Name: "one", Provider: "one", Search: search("one", &started.one)},
		{Name: "two", Provider: "two", Search: search("two", &started.two)},
		{Name: "three", Provider: "three", Search: search("three", &started.three)},
	}
	result := make(chan struct {
		results []map[string]any
		err     error
	}, 1)
	go func() {
		results, err := NewScheduler().Search(context.Background(), ScheduleRequest{Query: "query", MaxResults: fixture.Input.MaxResults}, engines)
		result <- struct {
			results []map[string]any
			err     error
		}{results: results, err: err}
	}()
	select {
	case <-secondStarted:
		if got := maximum.Load(); got != 2 {
			t.Fatalf("maximum active searches before release = %d, want 2", got)
		}
		close(release)
	case <-time.After(time.Second):
		t.Fatal("second source worker did not start")
	}
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("Search: %v", got.err)
		}
		var want struct {
			Hrefs     []string        `json:"hrefs"`
			MaxActive int             `json:"max_active"`
			Started   map[string]bool `json:"started"`
		}
		decodeSchedulerOutput(t, fixture, &want)
		if actual := schedulerHrefs(t, got.results); !reflect.DeepEqual(actual, want.Hrefs) {
			t.Fatalf("hrefs = %#v, want %#v", actual, want.Hrefs)
		}
		if actual := int(maximum.Load()); actual != want.MaxActive {
			t.Fatalf("maximum active searches = %d, want %d", actual, want.MaxActive)
		}
		if actual := map[string]bool{"one": started.one.Load(), "two": started.two.Load(), "three": started.three.Load()}; !reflect.DeepEqual(actual, want.Started) {
			t.Fatalf("started = %#v, want %#v", actual, want.Started)
		}
	case <-time.After(time.Second):
		t.Fatal("Search did not complete after worker release")
	}
}

func TestScheduler_MatchesFrozenFinalSliceFixtures(t *testing.T) {
	tests := []struct {
		path    string
		results []Result
	}{
		{
			path: "../../testdata/contracts/pure/pure.scheduler-zero-max-unlimited.json",
			results: []Result{
				schedulerTextResult(t, "one", "https://one.example", ""),
				schedulerTextResult(t, "two", "https://two.example", ""),
			},
		},
		{
			path:    "../../testdata/contracts/pure/pure.scheduler-positive-max-final-slice.json",
			results: schedulerThreeResults(t),
		},
		{
			path:    "../../testdata/contracts/pure/pure.scheduler-none-max-unlimited.json",
			results: schedulerThreeResults(t),
		},
		{
			path:    "../../testdata/contracts/pure/pure.scheduler-negative-one-final-slice.json",
			results: schedulerThreeResults(t),
		},
		{
			path:    "../../testdata/contracts/pure/pure.scheduler-negative-nine-final-slice.json",
			results: schedulerThreeResults(t),
		},
		{
			path: "../../testdata/contracts/pure/pure.scheduler-rank-before-final-slice.json",
			results: []Result{
				schedulerTextResult(t, "irrelevant", "https://first.example", ""),
				schedulerTextResult(t, "needle result", "https://second.example", ""),
			},
		},
	}

	for _, testCase := range tests {
		fixture := loadSchedulerFixture(t, testCase.path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			got, err := NewScheduler().Search(context.Background(), ScheduleRequest{
				Query:      sourceSchedulerQuery(fixture),
				MaxResults: fixture.Input.MaxResults,
			}, []ScheduledEngine{{
				Name:     "many",
				Provider: "many",
				Search: func(context.Context, EngineRequest) ([]Result, error) {
					return testCase.results, nil
				},
			}})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			var want []string
			decodeSchedulerOutput(t, fixture, &want)
			if actual := schedulerHrefs(t, got); !reflect.DeepEqual(actual, want) {
				t.Fatalf("hrefs = %#v, want %#v", actual, want)
			}
		})
	}
}

func TestScheduler_ForwardsFrozenCommonEngineRequestFields(t *testing.T) {
	tests := []struct {
		path string
	}{
		{path: "../../testdata/contracts/pure/pure.search-call-defaults.json"},
		{path: "../../testdata/contracts/pure/pure.search-call-explicit-zero-empty-values.json"},
		{path: "../../testdata/contracts/pure/pure.search-call-negative-page.json"},
	}
	for _, testCase := range tests {
		fixture := loadSchedulerInvocationFixture(t, testCase.path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			if len(fixture.Result.Output.Calls) != 1 {
				t.Fatalf("fixture calls = %d, want 1", len(fixture.Result.Output.Calls))
			}
			want := fixture.Result.Output.Calls[0]
			captured := make(chan EngineRequest, 1)
			_, err := NewScheduler().Search(context.Background(), ScheduleRequest{
				Query:      fixture.Input.Query,
				Region:     want.Kwargs.Region,
				SafeSearch: want.Kwargs.SafeSearch,
				TimeLimit:  want.Kwargs.TimeLimit,
				Page:       want.Kwargs.Page,
			}, []ScheduledEngine{{
				Name: "probe", Provider: "probe",
				Search: func(_ context.Context, request EngineRequest) ([]Result, error) {
					captured <- request
					return []Result{schedulerTextResult(t, "probe", "https://probe.example", "")}, nil
				},
			}})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			select {
			case got := <-captured:
				if got.Query != want.Query {
					t.Fatalf("query = %q, want %q", got.Query, want.Query)
				}
				if got.Region != want.Kwargs.Region {
					t.Fatalf("region = %q, want %q", got.Region, want.Kwargs.Region)
				}
				if got.SafeSearch != want.Kwargs.SafeSearch {
					t.Fatalf("safe search = %q, want %q", got.SafeSearch, want.Kwargs.SafeSearch)
				}
				if !optionalStringEqual(got.TimeLimit, want.Kwargs.TimeLimit) {
					t.Fatalf("time limit = %#v, want %#v", got.TimeLimit, want.Kwargs.TimeLimit)
				}
				if got.Page != want.Kwargs.Page {
					t.Fatalf("page = %d, want %d", got.Page, want.Kwargs.Page)
				}
			default:
				t.Fatal("engine did not receive source request")
			}
		})
	}
}

func TestScheduler_EngineRequestsOwnTimeLimitValues(t *testing.T) {
	timeLimit := "w"
	requests := make(chan EngineRequest, 2)
	engines := []ScheduledEngine{
		{
			Name:     "first",
			Provider: "first",
			Search: func(_ context.Context, request EngineRequest) ([]Result, error) {
				requests <- request
				return []Result{schedulerTextResult(t, "first", "https://first.example", "")}, nil
			},
		},
		{
			Name:     "second",
			Provider: "second",
			Search: func(_ context.Context, request EngineRequest) ([]Result, error) {
				requests <- request
				return []Result{schedulerTextResult(t, "second", "https://second.example", "")}, nil
			},
		},
	}

	if _, err := NewScheduler().Search(context.Background(), ScheduleRequest{
		Query:     "query",
		TimeLimit: &timeLimit,
	}, engines); err != nil {
		t.Fatalf("Search: %v", err)
	}

	first := <-requests
	second := <-requests
	if first.TimeLimit == nil || second.TimeLimit == nil {
		t.Fatalf("time limits = %#v, %#v, want values", first.TimeLimit, second.TimeLimit)
	}
	if *first.TimeLimit != timeLimit || *second.TimeLimit != timeLimit {
		t.Fatalf("time limits = %q, %q, want %q", *first.TimeLimit, *second.TimeLimit, timeLimit)
	}
	if first.TimeLimit == &timeLimit || second.TimeLimit == &timeLimit || first.TimeLimit == second.TimeLimit {
		t.Fatal("engine requests share caller or sibling time-limit storage")
	}
}

func TestScheduler_SnapshotsTimeLimitBeforeLaterEngineDispatch(t *testing.T) {
	timeLimit := "w"
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondRequest := make(chan EngineRequest, 1)
	result := make(chan error, 1)

	go func() {
		_, err := NewScheduler().Search(context.Background(), ScheduleRequest{
			Query:      "query",
			TimeLimit:  &timeLimit,
			MaxResults: intPointer(10),
		}, []ScheduledEngine{
			{
				Name:     "first",
				Provider: "shared",
				Search: func(_ context.Context, _ EngineRequest) ([]Result, error) {
					close(firstStarted)
					<-releaseFirst
					return nil, nil
				},
			},
			{
				Name:     "second",
				Provider: "shared",
				Search: func(_ context.Context, request EngineRequest) ([]Result, error) {
					secondRequest <- request
					return []Result{schedulerTextResult(t, "second", "https://second.example", "")}, nil
				},
			},
		})
		result <- err
	}()

	select {
	case <-firstStarted:
		timeLimit = "m"
		close(releaseFirst)
	case <-time.After(time.Second):
		t.Fatal("first engine did not start")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Search did not finish")
	}
	request := <-secondRequest
	if request.TimeLimit == nil || *request.TimeLimit != "w" {
		t.Fatalf("second time limit = %#v, want frozen \"w\"", request.TimeLimit)
	}
	if request.TimeLimit == &timeLimit {
		t.Fatal("second request aliases caller time-limit storage")
	}
}

func TestScheduler_SnapshotsMaxResultsBeforeLaterFinalSlice(t *testing.T) {
	maxResults := 10
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	result := make(chan struct {
		results []map[string]any
		err     error
	}, 1)

	go func() {
		results, err := NewScheduler().Search(context.Background(), ScheduleRequest{
			Query:      "query",
			MaxResults: &maxResults,
		}, []ScheduledEngine{
			{
				Name:     "first",
				Provider: "first",
				Search: func(_ context.Context, _ EngineRequest) ([]Result, error) {
					close(firstStarted)
					<-releaseFirst
					return []Result{schedulerTextResult(t, "first", "https://first.example", "")}, nil
				},
			},
			{
				Name:     "second",
				Provider: "second",
				Search: func(context.Context, EngineRequest) ([]Result, error) {
					return []Result{schedulerTextResult(t, "second", "https://second.example", "")}, nil
				},
			},
		})
		result <- struct {
			results []map[string]any
			err     error
		}{results: results, err: err}
	}()

	select {
	case <-firstStarted:
		maxResults = 1
		close(releaseFirst)
	case <-time.After(time.Second):
		t.Fatal("first engine did not start")
	}

	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("Search: %v", got.err)
		}
		want := []string{"https://first.example", "https://second.example"}
		if actual := schedulerHrefs(t, got.results); !reflect.DeepEqual(actual, want) {
			t.Fatalf("hrefs = %#v, want snapshot %#v", actual, want)
		}
	case <-time.After(time.Second):
		t.Fatal("Search did not finish")
	}
}

func TestCopyScheduleRequest_OwnsOptionalValues(t *testing.T) {
	timeLimit := "w"
	maxResults := 10
	threads := 2
	waitTimeout := 5 * time.Second

	got := copyScheduleRequest(ScheduleRequest{
		Query:       "query",
		TimeLimit:   &timeLimit,
		MaxResults:  &maxResults,
		Threads:     &threads,
		WaitTimeout: &waitTimeout,
	})

	timeLimit = "m"
	maxResults = 1
	threads = 1
	waitTimeout = 0

	if got.TimeLimit == nil || *got.TimeLimit != "w" || got.TimeLimit == &timeLimit {
		t.Fatalf("time limit = %#v, want owned \"w\"", got.TimeLimit)
	}
	if got.MaxResults == nil || *got.MaxResults != 10 || got.MaxResults == &maxResults {
		t.Fatalf("max results = %#v, want owned 10", got.MaxResults)
	}
	if got.Threads == nil || *got.Threads != 2 || got.Threads == &threads {
		t.Fatalf("threads = %#v, want owned 2", got.Threads)
	}
	if got.WaitTimeout == nil || *got.WaitTimeout != 5*time.Second || got.WaitTimeout == &waitTimeout {
		t.Fatalf("wait timeout = %#v, want owned 5s", got.WaitTimeout)
	}
}

func TestScheduler_SnapshotsEngineSliceBeforeLaterDispatch(t *testing.T) {
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	result := make(chan struct {
		results []map[string]any
		err     error
	}, 1)
	engines := []ScheduledEngine{
		{
			Name:     "first",
			Provider: "first",
			Search: func(_ context.Context, _ EngineRequest) ([]Result, error) {
				close(firstStarted)
				<-releaseFirst
				return nil, nil
			},
		},
		{
			Name:     "second",
			Provider: "second",
			Search: func(context.Context, EngineRequest) ([]Result, error) {
				return []Result{schedulerTextResult(t, "second", "https://second.example", "")}, nil
			},
		},
	}

	go func() {
		results, err := NewScheduler().Search(context.Background(), ScheduleRequest{
			Query:   "query",
			Threads: intPointer(1),
		}, engines)
		result <- struct {
			results []map[string]any
			err     error
		}{results: results, err: err}
	}()

	select {
	case <-firstStarted:
		engines[1] = ScheduledEngine{
			Name:     "mutated",
			Provider: "mutated",
			Search: func(context.Context, EngineRequest) ([]Result, error) {
				return []Result{schedulerTextResult(t, "mutated", "https://mutated.example", "")}, nil
			},
		}
		close(releaseFirst)
	case <-time.After(time.Second):
		t.Fatal("first engine did not start")
	}

	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("Search: %v", got.err)
		}
		want := []string{"https://second.example"}
		if actual := schedulerHrefs(t, got.results); !reflect.DeepEqual(actual, want) {
			t.Fatalf("hrefs = %#v, want snapshot %#v", actual, want)
		}
	case <-time.After(time.Second):
		t.Fatal("Search did not finish")
	}
}

func TestScheduler_MatchesFrozenProviderTimingFixtures(t *testing.T) {
	t.Run("not reserved on submit", func(t *testing.T) {
		fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-provider-not-reserved-on-submit.json")
		secondStarted := make(chan struct{})
		var secondOnce sync.Once
		var started struct {
			first  atomic.Bool
			second atomic.Bool
			other  atomic.Bool
		}

		engines := []ScheduledEngine{
			{
				Name: "first", Provider: "shared",
				Search: func(ctx context.Context, _ EngineRequest) ([]Result, error) {
					started.first.Store(true)
					if err := waitForSchedulerSignal(ctx, secondStarted); err != nil {
						return nil, err
					}
					return []Result{schedulerTextResult(t, "first", "https://first.example", "one")}, nil
				},
			},
			{
				Name: "second", Provider: "shared",
				Search: func(context.Context, EngineRequest) ([]Result, error) {
					started.second.Store(true)
					secondOnce.Do(func() { close(secondStarted) })
					return []Result{schedulerTextResult(t, "second", "https://second.example", "two")}, nil
				},
			},
			{
				Name: "other", Provider: "other",
				Search: func(context.Context, EngineRequest) ([]Result, error) {
					started.other.Store(true)
					return []Result{schedulerTextResult(t, "other", "https://other.example", "three")}, nil
				},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		got, err := NewScheduler().Search(ctx, ScheduleRequest{Query: "query", MaxResults: fixture.Input.MaxResults}, engines)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		var want struct {
			Hrefs   []string        `json:"hrefs"`
			Started map[string]bool `json:"started"`
		}
		decodeSchedulerOutput(t, fixture, &want)
		if actual := schedulerHrefs(t, got); !reflect.DeepEqual(actual, want.Hrefs) {
			t.Fatalf("hrefs = %#v, want %#v", actual, want.Hrefs)
		}
		if actual := map[string]bool{
			"first": started.first.Load(), "second": started.second.Load(), "other": started.other.Load(),
		}; !reflect.DeepEqual(actual, want.Started) {
			t.Fatalf("started = %#v, want %#v", actual, want.Started)
		}
	})

	t.Run("empty result does not mark provider", func(t *testing.T) {
		fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-provider-seen-after-nonempty.json")
		var started struct {
			empty   atomic.Bool
			success atomic.Bool
			skipped atomic.Bool
		}
		engines := []ScheduledEngine{
			{Name: "empty", Provider: "shared", Search: func(context.Context, EngineRequest) ([]Result, error) {
				started.empty.Store(true)
				return nil, nil
			}},
			{Name: "success", Provider: "shared", Search: func(context.Context, EngineRequest) ([]Result, error) {
				started.success.Store(true)
				return []Result{schedulerTextResult(t, "success", "https://success.example", "")}, nil
			}},
			{Name: "skipped", Provider: "shared", Search: func(context.Context, EngineRequest) ([]Result, error) {
				started.skipped.Store(true)
				return []Result{schedulerTextResult(t, "skipped", "https://skipped.example", "")}, nil
			}},
		}
		got, err := NewScheduler().Search(context.Background(), ScheduleRequest{Query: "query", MaxResults: fixture.Input.MaxResults}, engines)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		var want struct {
			Hrefs   []string        `json:"hrefs"`
			Started map[string]bool `json:"started"`
		}
		decodeSchedulerOutput(t, fixture, &want)
		if actual := schedulerHrefs(t, got); !reflect.DeepEqual(actual, want.Hrefs) {
			t.Fatalf("hrefs = %#v, want %#v", actual, want.Hrefs)
		}
		if actual := map[string]bool{
			"empty": started.empty.Load(), "success": started.success.Load(), "skipped": started.skipped.Load(),
		}; !reflect.DeepEqual(actual, want.Started) {
			t.Fatalf("started = %#v, want %#v", actual, want.Started)
		}
	})

	t.Run("engine error does not mark provider", func(t *testing.T) {
		fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-provider-error-does-not-mark-seen.json")
		var failed, recovered atomic.Bool
		engines := []ScheduledEngine{
			{Name: "failed", Provider: "shared", Search: func(context.Context, EngineRequest) ([]Result, error) {
				failed.Store(true)
				return nil, errors.New("first provider failure")
			}},
			{Name: "recovered", Provider: "shared", Search: func(context.Context, EngineRequest) ([]Result, error) {
				recovered.Store(true)
				return []Result{schedulerTextResult(t, "recovered", "https://recovered.example", "")}, nil
			}},
		}
		got, err := NewScheduler().Search(context.Background(), ScheduleRequest{Query: "query", MaxResults: fixture.Input.MaxResults}, engines)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		var want struct {
			Hrefs   []string        `json:"hrefs"`
			Started map[string]bool `json:"started"`
		}
		decodeSchedulerOutput(t, fixture, &want)
		if actual := schedulerHrefs(t, got); !reflect.DeepEqual(actual, want.Hrefs) {
			t.Fatalf("hrefs = %#v, want %#v", actual, want.Hrefs)
		}
		if actual := map[string]bool{"failed": failed.Load(), "recovered": recovered.Load()}; !reflect.DeepEqual(actual, want.Started) {
			t.Fatalf("started = %#v, want %#v", actual, want.Started)
		}
	})
}

func TestScheduler_MatchesFrozenMaxReachedDispatchFixture(t *testing.T) {
	fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-max-reached-stops-later-dispatch.json")
	var started struct {
		first  atomic.Bool
		second atomic.Bool
		third  atomic.Bool
	}
	engines := []ScheduledEngine{
		{Name: "first", Provider: "first", Search: func(context.Context, EngineRequest) ([]Result, error) {
			started.first.Store(true)
			return []Result{schedulerTextResult(t, "first", "https://limit-first.example", "")}, nil
		}},
		{Name: "second", Provider: "second", Search: func(context.Context, EngineRequest) ([]Result, error) {
			started.second.Store(true)
			return []Result{schedulerTextResult(t, "second", "https://limit-second.example", "")}, nil
		}},
		{Name: "third", Provider: "third", Search: func(context.Context, EngineRequest) ([]Result, error) {
			started.third.Store(true)
			return []Result{schedulerTextResult(t, "third", "https://limit-third.example", "")}, nil
		}},
	}
	got, err := NewScheduler().Search(context.Background(), ScheduleRequest{Query: "query", MaxResults: fixture.Input.MaxResults}, engines)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var want struct {
		Hrefs   []string        `json:"hrefs"`
		Started map[string]bool `json:"started"`
	}
	decodeSchedulerOutput(t, fixture, &want)
	if actual := schedulerHrefs(t, got); !reflect.DeepEqual(actual, want.Hrefs) {
		t.Fatalf("hrefs = %#v, want %#v", actual, want.Hrefs)
	}
	if actual := map[string]bool{"first": started.first.Load(), "second": started.second.Load(), "third": started.third.Load()}; !reflect.DeepEqual(actual, want.Started) {
		t.Fatalf("started = %#v, want %#v", actual, want.Started)
	}
}

func TestScheduler_MatchesFrozenFirstExceptionAndWaitTimeoutFixtures(t *testing.T) {
	t.Run("late result after first exception stays unaggregated", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-first-exception-late-result-not-aggregated.json")
			slowStarted := make(chan struct{})
			failureReturned := make(chan struct{})
			slowFinished := make(chan struct{})

			engines := []ScheduledEngine{
				{Name: "failing", Provider: "failing", Search: func(ctx context.Context, _ EngineRequest) ([]Result, error) {
					if err := waitForSchedulerSignal(ctx, slowStarted); err != nil {
						return nil, err
					}
					close(failureReturned)
					return nil, errors.New("boom")
				}},
				{Name: "slow", Provider: "slow", Search: func(ctx context.Context, _ EngineRequest) ([]Result, error) {
					close(slowStarted)
					if err := waitForSchedulerSignal(ctx, failureReturned); err != nil {
						return nil, err
					}
					time.Sleep(time.Nanosecond)
					close(slowFinished)
					return []Result{schedulerTextResult(t, "late", "https://late.example", "late")}, nil
				}},
			}
			_, err := NewScheduler().Search(t.Context(), ScheduleRequest{Query: "query", MaxResults: fixture.Input.MaxResults}, engines)
			if err == nil {
				t.Fatalf("Search error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
			}
			if got := err.Error(); got != fixture.Result.Error.Message {
				t.Fatalf("Search error = %q, want %q", got, fixture.Result.Error.Message)
			}
			select {
			case <-slowFinished:
			default:
				t.Fatal("Search returned before its submitted slow worker ended")
			}
		})
	})

	t.Run("completed success survives first exception", func(t *testing.T) {
		fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-completed-success-survives-first-exception.json")
		success := &scheduledFuture{engine: ScheduledEngine{Name: "success", Provider: "success"}}
		failure := &scheduledFuture{engine: ScheduledEngine{Name: "failure", Provider: "failure"}}
		pending := []*scheduledFuture{success, failure}
		completions := make(chan schedulerCompletion, len(pending))
		completions <- schedulerCompletion{
			future:  success,
			results: []Result{schedulerTextResult(t, "kept", "https://kept.example", "")},
		}
		completions <- schedulerCompletion{future: failure, err: errors.New("boom after success")}

		done, remaining, err := waitForSourceBatch(context.Background(), completions, pending, nil)
		if err != nil {
			t.Fatalf("waitForSourceBatch: %v", err)
		}
		if len(remaining) != 0 {
			t.Fatalf("remaining futures = %d, want 0", len(remaining))
		}
		aggregator, err := NewResultsAggregator([]string{"href", "image", "url", "embed_url"})
		if err != nil {
			t.Fatalf("NewResultsAggregator: %v", err)
		}
		lastError := processSourceCompletions(done, pending, aggregator, map[string]struct{}{}, nil)
		if lastError == nil || lastError.Error() != "boom after success" {
			t.Fatalf("last error = %v, want source engine error", lastError)
		}
		got, err := NewSimpleFilterRanker().Rank(resultMaps(aggregator.Extract()), "query")
		if err != nil {
			t.Fatalf("Rank: %v", err)
		}
		var want []string
		decodeSchedulerOutput(t, fixture, &want)
		if actual := schedulerHrefs(t, got); !reflect.DeepEqual(actual, want) {
			t.Fatalf("hrefs = %#v, want %#v", actual, want)
		}
	})

	t.Run("wait timeout joins but omits late result", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-wait-timeout-late-result-not-aggregated.json")
			slowStarted := make(chan struct{})
			slowFinished := make(chan struct{})
			engines := []ScheduledEngine{
				{Name: "quick", Provider: "quick", Search: func(ctx context.Context, _ EngineRequest) ([]Result, error) {
					if err := waitForSchedulerSignal(ctx, slowStarted); err != nil {
						return nil, err
					}
					return []Result{schedulerTextResult(t, "quick", "https://quick.example", "")}, nil
				}},
				{Name: "slow", Provider: "slow", Search: func(context.Context, EngineRequest) ([]Result, error) {
					close(slowStarted)
					time.Sleep(80 * time.Millisecond)
					close(slowFinished)
					return []Result{schedulerTextResult(t, "late", "https://late-timeout.example", "")}, nil
				}},
			}
			waitTimeout := time.Duration(*fixture.Input.WaitTimeoutSeconds * float64(time.Second))
			got, err := NewScheduler().Search(t.Context(), ScheduleRequest{
				Query:       "query",
				MaxResults:  fixture.Input.MaxResults,
				WaitTimeout: &waitTimeout,
			}, engines)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			var want []string
			decodeSchedulerOutput(t, fixture, &want)
			if actual := schedulerHrefs(t, got); !reflect.DeepEqual(actual, want) {
				t.Fatalf("hrefs = %#v, want %#v", actual, want)
			}
			select {
			case <-slowFinished:
			default:
				t.Fatal("Search returned before its submitted slow worker ended")
			}
		})
	})

	t.Run("wait timeout alone yields generic no-results error", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-wait-timeout-no-result-generic-error.json")
			workerExited := make(chan struct{})
			waitTimeout := time.Duration(*fixture.Input.WaitTimeoutSeconds * float64(time.Second))
			_, err := NewScheduler().Search(t.Context(), ScheduleRequest{
				Query:       "query",
				MaxResults:  fixture.Input.MaxResults,
				WaitTimeout: &waitTimeout,
			}, []ScheduledEngine{{
				Name: "slow", Provider: "slow",
				Search: func(context.Context, EngineRequest) ([]Result, error) {
					time.Sleep(80 * time.Millisecond)
					close(workerExited)
					return []Result{schedulerTextResult(t, "late", "https://timeout-only.example", "")}, nil
				},
			}})
			if err == nil {
				t.Fatalf("Search error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
			}
			if got := err.Error(); got != fixture.Result.Error.Message {
				t.Fatalf("Search error = %q, want %q", got, fixture.Result.Error.Message)
			}
			select {
			case <-workerExited:
			default:
				t.Fatal("Search returned before its submitted worker exited")
			}
		})
	})
}

func TestScheduler_ZeroTimeoutKeepsAlreadyCompletedFuture(t *testing.T) {
	fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-zero-timeout-keeps-already-completed-future.json")
	if fixture.Input.WaitTimeoutSeconds == nil {
		t.Fatal("fixture has no timeout")
	}
	timeout := time.Duration(*fixture.Input.WaitTimeoutSeconds * float64(time.Second))
	future := &scheduledFuture{engine: ScheduledEngine{Name: "done", Provider: "done"}}
	completions := make(chan schedulerCompletion, 1)
	completions <- schedulerCompletion{future: future}

	done, pending, err := waitForSourceBatch(context.Background(), completions, []*scheduledFuture{future}, &timeout)
	if err != nil {
		t.Fatalf("waitForSourceBatch: %v", err)
	}
	var want struct {
		Done    int `json:"done"`
		Pending int `json:"pending"`
	}
	decodeSchedulerOutput(t, fixture, &want)
	if got := len(done); got != want.Done {
		t.Fatalf("done = %d, want %d", got, want.Done)
	}
	if got := len(pending); got != want.Pending {
		t.Fatalf("pending = %d, want %d", got, want.Pending)
	}
}

func TestSourceBatchSnapshotObservesReadyCompletionBeforeTimeout(t *testing.T) {
	fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.scheduler-zero-timeout-keeps-already-completed-future.json")
	future := &scheduledFuture{engine: ScheduledEngine{Name: "done", Provider: "done"}}
	completions := make(chan schedulerCompletion, 1)
	completions <- schedulerCompletion{future: future}
	done := make(map[*scheduledFuture]schedulerCompletion, 1)

	if !sourceBatchSnapshot(completions, done) {
		t.Fatal("source batch snapshot did not report ready completion")
	}
	var want struct {
		Done    int `json:"done"`
		Pending int `json:"pending"`
	}
	decodeSchedulerOutput(t, fixture, &want)
	if got := len(done); got != want.Done {
		t.Fatalf("done = %d, want %d", got, want.Done)
	}
	if got := len(sourcePending([]*scheduledFuture{future}, done)); got != want.Pending {
		t.Fatalf("pending = %d, want %d", got, want.Pending)
	}
}

func TestSourceBatchCallerCancellationWinsOverReadyCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	future := &scheduledFuture{engine: ScheduledEngine{Name: "done", Provider: "done"}}
	completions := make(chan schedulerCompletion, 1)
	completions <- schedulerCompletion{future: future}

	done, pending, err := waitForSourceBatch(ctx, completions, []*scheduledFuture{future}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForSourceBatch error = %v, want context.Canceled", err)
	}
	if len(done) != 0 || len(pending) != 1 {
		t.Fatalf("done=%d pending=%d, want canceled batch untouched", len(done), len(pending))
	}
}

func TestScheduler_CallerCancellationDuringRankingWinsOverResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scheduler := NewScheduler()
	scheduler.ranker = schedulerRankerFunc(func(documents []map[string]any, _ string) ([]map[string]any, error) {
		cancel()
		return documents, nil
	})

	got, err := scheduler.Search(ctx, ScheduleRequest{Query: "query"}, []ScheduledEngine{{
		Name:     "result",
		Provider: "result",
		Search: func(context.Context, EngineRequest) ([]Result, error) {
			return []Result{schedulerTextResult(t, "result", "https://result.example", "")}, nil
		},
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search error = %v, want context.Canceled", err)
	}
	if got != nil {
		t.Fatalf("Search results = %#v, want nil after cancellation", got)
	}
}

type schedulerRankerFunc func([]map[string]any, string) ([]map[string]any, error)

func (f schedulerRankerFunc) Rank(documents []map[string]any, query string) ([]map[string]any, error) {
	return f(documents, query)
}

func TestScheduler_ClassifiesFrozenNoResultErrors(t *testing.T) {
	t.Run("empty query", func(t *testing.T) {
		fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.error-empty-query.json")
		var started atomic.Bool
		_, err := NewScheduler().Search(context.Background(), ScheduleRequest{}, []ScheduledEngine{{
			Name: "never", Provider: "never",
			Search: func(context.Context, EngineRequest) ([]Result, error) {
				started.Store(true)
				return nil, nil
			},
		}})
		if err == nil {
			t.Fatalf("Search error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
		}
		if got := err.Error(); got != fixture.Result.Error.Message {
			t.Fatalf("Search error = %q, want %q", got, fixture.Result.Error.Message)
		}
		if started.Load() {
			t.Fatal("engine started for source-invalid empty query")
		}
	})

	t.Run("timeout substring", func(t *testing.T) {
		fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.error-timeout-string-heuristic.json")
		_, err := NewScheduler().Search(context.Background(), ScheduleRequest{Query: "query"}, []ScheduledEngine{{
			Name: "timeout", Provider: "timeout",
			Search: func(context.Context, EngineRequest) ([]Result, error) {
				return nil, errors.New("operation timed out exactly")
			},
		}})
		if err == nil {
			t.Fatalf("Search error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
		}
		if got := err.Error(); got != fixture.Result.Error.Message {
			t.Fatalf("Search error = %q, want %q", got, fixture.Result.Error.Message)
		}
	})

	t.Run("no engine result", func(t *testing.T) {
		_, err := NewScheduler().Search(context.Background(), ScheduleRequest{Query: "query"}, []ScheduledEngine{{
			Name: "empty", Provider: "empty",
			Search: func(context.Context, EngineRequest) ([]Result, error) { return nil, nil },
		}})
		if err == nil || err.Error() != "No results found." {
			t.Fatalf("Search error = %v, want source no-results error", err)
		}
	})

	t.Run("uppercase timeout is generic", func(t *testing.T) {
		fixture := loadSchedulerFixture(t, "../../testdata/contracts/pure/pure.error-uppercase-timeout-string-generic.json")
		_, err := NewScheduler().Search(context.Background(), ScheduleRequest{Query: "query"}, []ScheduledEngine{{
			Name: "timeout", Provider: "timeout",
			Search: func(context.Context, EngineRequest) ([]Result, error) {
				return nil, errors.New("operation TIMED OUT exactly")
			},
		}})
		if err == nil {
			t.Fatalf("Search error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
		}
		if got := err.Error(); got != fixture.Result.Error.Message {
			t.Fatalf("Search error = %q, want %q", got, fixture.Result.Error.Message)
		}
		var sourceError *sourceSchedulerError
		if !errors.As(err, &sourceError) || sourceError.sourceType != fixture.Result.Error.Type {
			t.Fatalf("Search error type = %#v, want %s", sourceError, fixture.Result.Error.Type)
		}
	})
}

func TestScheduler_CallerCancellationStopsDispatchAndJoinsWorkers(t *testing.T) {
	t.Run("already canceled starts no engine", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var started atomic.Int32
		_, err := NewScheduler().Search(ctx, ScheduleRequest{Query: "query"}, []ScheduledEngine{{
			Name: "never", Provider: "never",
			Search: func(context.Context, EngineRequest) ([]Result, error) {
				started.Add(1)
				return nil, nil
			},
		}})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Search error = %v, want context.Canceled", err)
		}
		if got := started.Load(); got != 0 {
			t.Fatalf("started engines = %d, want 0", got)
		}
	})

	t.Run("mid-flight cancellation joins workers and halts later dispatch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		firstStarted := make(chan struct{})
		workerExited := make(chan struct{})
		var thirdStarted atomic.Bool
		engines := []ScheduledEngine{
			{Name: "first", Provider: "first", Search: func(ctx context.Context, _ EngineRequest) ([]Result, error) {
				close(firstStarted)
				<-ctx.Done()
				close(workerExited)
				return nil, ctx.Err()
			}},
			{Name: "second", Provider: "second", Search: func(ctx context.Context, _ EngineRequest) ([]Result, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}},
			{Name: "third", Provider: "third", Search: func(context.Context, EngineRequest) ([]Result, error) {
				thirdStarted.Store(true)
				return nil, nil
			}},
		}
		result := make(chan error, 1)
		go func() {
			_, err := NewScheduler().Search(ctx, ScheduleRequest{Query: "query", MaxResults: intPointer(10)}, engines)
			result <- err
		}()
		select {
		case <-firstStarted:
			cancel()
		case <-time.After(time.Second):
			t.Fatal("first engine did not start")
		}
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Search error = %v, want context.Canceled", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Search did not return after cancellation")
		}
		select {
		case <-workerExited:
		default:
			t.Fatal("Search returned before its owned worker exited")
		}
		if thirdStarted.Load() {
			t.Fatal("third engine started after cancellation")
		}
	})
}

func TestScheduler_CooperativeCancellationLeavesNoWorker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		started := make(chan struct{})
		exited := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			_, err := NewScheduler().Search(ctx, ScheduleRequest{Query: "query"}, []ScheduledEngine{{
				Name:     "cooperative",
				Provider: "cooperative",
				Search: func(ctx context.Context, _ EngineRequest) ([]Result, error) {
					close(started)
					<-ctx.Done()
					close(exited)
					return nil, ctx.Err()
				},
			}})
			result <- err
		}()

		<-started
		cancel()
		synctest.Wait()

		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("Search error = %v, want context.Canceled", err)
		}
		select {
		case <-exited:
		default:
			t.Fatal("cooperative engine worker did not exit")
		}
	})
}

func loadSchedulerFixture(t *testing.T, path string) schedulerFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var fixture schedulerFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

type schedulerInvocationFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Query string `json:"query"`
	} `json:"input"`
	Result struct {
		Output struct {
			Calls []struct {
				Query  string `json:"query"`
				Kwargs struct {
					Page       int     `json:"page"`
					Region     string  `json:"region"`
					SafeSearch string  `json:"safesearch"`
					TimeLimit  *string `json:"timelimit"`
				} `json:"kwargs"`
			} `json:"calls"`
		} `json:"output"`
	} `json:"result"`
}

func loadSchedulerInvocationFixture(t *testing.T, path string) schedulerInvocationFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture schedulerInvocationFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func loadSchedulerFloatBoundaryFixture(t *testing.T, path string) map[string]int {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture struct {
		Result struct {
			Output map[string]int `json:"output"`
		} `json:"result"`
	}
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture.Result.Output
}

func decodeSchedulerOutput(t *testing.T, fixture schedulerFixture, destination any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(fixture.Result.Output))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		t.Fatalf("decode %s output: %v", fixture.FixtureID, err)
	}
}

func schedulerTextResult(t *testing.T, title, href, body string) Result {
	t.Helper()
	result, err := NewCategoryResult("text", []Field{
		{Name: "title", Value: title},
		{Name: "href", Value: href},
		{Name: "body", Value: body},
	})
	if err != nil {
		t.Fatalf("NewCategoryResult(text): %v", err)
	}
	return result
}

func schedulerThreeResults(t *testing.T) []Result {
	t.Helper()
	return []Result{
		schedulerTextResult(t, "one", "https://one.example", ""),
		schedulerTextResult(t, "two", "https://two.example", ""),
		schedulerTextResult(t, "three", "https://three.example", ""),
	}
}

func schedulerHrefs(t *testing.T, results []map[string]any) []string {
	t.Helper()
	hrefs := make([]string, len(results))
	for index, result := range results {
		href, ok := result["href"].(string)
		if !ok {
			t.Fatalf("result %d href = %#v (%T), want string", index, result["href"], result["href"])
		}
		hrefs[index] = href
	}
	return hrefs
}

func sourceSchedulerQuery(fixture schedulerFixture) string {
	if fixture.Input.Query != "" {
		return fixture.Input.Query
	}
	return "query"
}

func waitForSchedulerSignal(ctx context.Context, signal <-chan struct{}) error {
	select {
	case <-signal:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func intPointer(value int) *int {
	return &value
}

func optionalStringEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
