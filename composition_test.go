package ddgs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jcastilloa/goddgs/internal/engine"
	"github.com/jcastilloa/goddgs/internal/transport"
)

type facadeCompositionFixture struct {
	Input struct {
		Client struct {
			Proxy   string `json:"proxy"`
			Timeout int    `json:"timeout"`
			Verify  string `json:"verify"`
		} `json:"client"`
		Calls []struct {
			Backend      string `json:"backend"`
			MaxResults   int    `json:"max_results"`
			Page         int    `json:"page"`
			Query        string `json:"query"`
			Region       string `json:"region"`
			SafeSearch   string `json:"safesearch"`
			SourceMarker string `json:"source_marker"`
			TimeLimit    string `json:"timelimit"`
		} `json:"calls"`
	} `json:"input"`
	Result struct {
		Output struct {
			CacheSize        int `json:"cache_size"`
			ConstructorCalls []struct {
				Name    string `json:"name"`
				Proxy   string `json:"proxy"`
				Timeout int    `json:"timeout"`
				Verify  string `json:"verify"`
			} `json:"constructor_calls"`
			Operations []struct {
				Label   string      `json:"label"`
				Results []RawResult `json:"results"`
			} `json:"operations"`
			SearchCalls []struct {
				Name   string `json:"name"`
				Query  string `json:"query"`
				Kwargs struct {
					Page         int    `json:"page"`
					Region       string `json:"region"`
					SafeSearch   string `json:"safesearch"`
					SourceMarker string `json:"source_marker"`
					TimeLimit    string `json:"timelimit"`
				} `json:"kwargs"`
			} `json:"search_calls"`
			EagerAuto struct {
				CacheSize        int      `json:"cache_size"`
				ConstructorNames []string `json:"constructor_names"`
				InstanceNames    []string `json:"instance_names"`
			} `json:"eager_auto"`
		} `json:"output"`
	} `json:"result"`
}

func TestComposedExecutor_MatchesFrozenLazyCacheAndForwardingFixture(t *testing.T) {
	fixture := loadFacadeCompositionFixture(t, "testdata/contracts/pure/pure.facade-lazy-engine-cache-and-forwarding.json")

	client := New(
		WithProxy(fixture.Input.Client.Proxy),
		WithTimeout(0),
		WithTLSRootCAFile(fixture.Input.Client.Verify),
	)
	factory := &recordingCompositionFactory{}
	selector := compositionSelectorFunc(func(category, backend string) ([]engine.Metadata, error) {
		if category != string(textCategory) {
			t.Fatalf("category = %q, want %q", category, textCategory)
		}
		return []engine.Metadata{{Name: backend, Category: category, Provider: backend, Priority: 1}}, nil
	})
	executor := newComposedExecutor(client.config, factory, selector)

	for index, call := range fixture.Input.Calls {
		timeLimit := call.TimeLimit
		maxResults := call.MaxResults
		got, err := executor.search(context.Background(), searchRequest{
			category: textCategory,
			query:    call.Query,
			config: searchConfig{
				region:     call.Region,
				safeSearch: call.SafeSearch,
				timeLimit:  &timeLimit,
				maxResults: &maxResults,
				page:       call.Page,
				backend:    call.Backend,
				sourceKeywords: []sourceKeyword{{
					name:  "source_marker",
					value: call.SourceMarker,
				}},
			},
		})
		if err != nil {
			t.Fatalf("operation %d search: %v", index, err)
		}
		if gotWant := fixture.Result.Output.Operations[index].Results; !reflect.DeepEqual(got, gotWant) {
			t.Fatalf("operation %d results = %#v, want %#v", index, got, gotWant)
		}
	}

	if got, want := factory.names(), constructorNames(fixture); !reflect.DeepEqual(got, want) {
		t.Fatalf("constructed engines = %#v, want %#v", got, want)
	}
	if got, want := factory.requests(), searchRequests(fixture); !reflect.DeepEqual(got, want) {
		t.Fatalf("engine requests = %#v, want %#v", got, want)
	}
	if got, want := factory.configs(), expectedCompositionConfigs(t, fixture); !reflect.DeepEqual(got, want) {
		t.Fatalf("transport configs = %#v, want %#v", got, want)
	}
}

func TestComposedExecutor_CanceledContextDoesNotSelectOrConstruct(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	factory := &recordingCompositionFactory{}
	selector := compositionSelectorFunc(func(string, string) ([]engine.Metadata, error) {
		t.Fatal("selector called for canceled context")
		return nil, nil
	})
	executor := newComposedExecutor(defaultClientConfig(), factory, selector)

	_, err := executor.search(ctx, searchRequest{category: textCategory, query: "needle", config: defaultSearchConfig()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("search error = %v, want context.Canceled", err)
	}
	if got := factory.names(); len(got) != 0 {
		t.Fatalf("constructed engines = %#v, want none", got)
	}
}

func TestComposedExecutor_ClassifiesSchedulerFailuresAtPublicBoundary(t *testing.T) {
	tests := []struct {
		name           string
		searchError    error
		classification error
		message        string
	}{
		{
			name:           "timeout",
			searchError:    errors.New("operation timed out exactly"),
			classification: ErrTimeout,
			message:        "operation timed out exactly",
		},
		{
			name:           "generic",
			searchError:    errors.New("source failure exactly"),
			classification: ErrDDGS,
			message:        "source failure exactly",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			factory := compositionFactoryFunc(func(engine.Metadata, transport.Config) (engine.Searcher, error) {
				return compositionErrorSearcher{err: testCase.searchError}, nil
			})
			selector := compositionSelectorFunc(func(category, backend string) ([]engine.Metadata, error) {
				if category != string(textCategory) || backend != "fixture" {
					t.Fatalf("selection = (%q, %q), want (%q, %q)", category, backend, textCategory, "fixture")
				}
				return []engine.Metadata{{Name: "fixture", Category: category, Provider: "fixture", Priority: 1}}, nil
			})
			client := New()
			client.executor = newComposedExecutor(client.config, factory, selector)

			_, err := client.Text(context.Background(), "needle", WithBackend("fixture"))
			if !errors.Is(err, testCase.classification) {
				t.Fatalf("Text error %v does not classify as %v", err, testCase.classification)
			}
			var sourceError *DDGSError
			if !errors.As(err, &sourceError) {
				t.Fatalf("Text error type = %T, want *DDGSError", err)
			}
			if got := err.Error(); got != testCase.message {
				t.Fatalf("Text error = %q, want %q", got, testCase.message)
			}
		})
	}
}

func TestComposedExecutor_ConstructsAllSelectedAdaptersBeforeScheduler(t *testing.T) {
	fixture := loadFacadeCompositionFixture(t, "testdata/contracts/pure/pure.facade-lazy-engine-cache-and-forwarding.json")
	selected := fixture.Result.Output.EagerAuto.InstanceNames
	if len(selected) != 2 {
		t.Fatalf("fixture eager selected engines = %#v, want two", selected)
	}

	factory := newBlockingConstructionFactory(selected[0], selected[1])
	selector := compositionSelectorFunc(func(category, backend string) ([]engine.Metadata, error) {
		if category != string(textCategory) || backend != "auto" {
			t.Fatalf("selection = (%q, %q), want (%q, %q)", category, backend, textCategory, "auto")
		}
		return []engine.Metadata{
			{Name: selected[0], Category: category, Provider: selected[0], Priority: 1},
			{Name: selected[1], Category: category, Provider: selected[1], Priority: 1},
		}, nil
	})
	executor := newComposedExecutor(defaultClientConfig(), factory, selector)

	done := make(chan error, 1)
	go func() {
		_, err := executor.search(context.Background(), searchRequest{
			category: textCategory,
			query:    "needle",
			config:   defaultSearchConfig(),
		})
		done <- err
	}()

	select {
	case <-factory.secondConstructing:
		select {
		case <-factory.firstSearched:
			t.Fatal("scheduler started an adapter before all selected adapters were constructed")
		case <-time.After(25 * time.Millisecond):
		}
		close(factory.releaseSecond)
	case <-factory.firstSearched:
		t.Fatal("scheduler started an adapter before all selected adapters were constructed")
	case <-time.After(time.Second):
		t.Fatal("second selected adapter was not constructed")
	}

	if err := <-done; err != nil {
		t.Fatalf("search: %v", err)
	}
	if got, want := factory.names(), fixture.Result.Output.EagerAuto.ConstructorNames; !reflect.DeepEqual(got, want) {
		t.Fatalf("construction order = %#v, want %#v", got, want)
	}
}

func TestComposedExecutor_ConcurrentSearchesReuseOneCachedAdapter(t *testing.T) {
	factory := &recordingCompositionFactory{}
	selector := compositionSelectorFunc(func(category, backend string) ([]engine.Metadata, error) {
		if category != string(textCategory) || backend != "fixture" {
			return nil, errors.New("unexpected selection")
		}
		return []engine.Metadata{{Name: "fixture", Category: category, Provider: "fixture", Priority: 1}}, nil
	})
	executor := newComposedExecutor(defaultClientConfig(), factory, selector)

	const calls = 32
	var group sync.WaitGroup
	errorsByCall := make(chan error, calls)
	for range calls {
		group.Add(1)
		go func() {
			defer group.Done()
			maxResults := 1
			results, err := executor.search(context.Background(), searchRequest{
				category: textCategory,
				query:    "needle",
				config:   searchConfig{backend: "fixture", maxResults: &maxResults},
			})
			if err != nil {
				errorsByCall <- err
				return
			}
			if len(results) != 1 {
				errorsByCall <- errors.New("unexpected composition result count")
			}
		}()
	}
	group.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		t.Error(err)
	}
	if got, want := factory.names(), []string{"fixture"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("constructed engines = %#v, want %#v", got, want)
	}
}

func TestDDGS_ExtractUsesIsolatedLocalTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<h1>Fixture heading</h1><p>Hello <a href="https://target.example/path">link</a>.</p><ul><li>One</li><li>Two</li></ul>`))
	}))
	defer server.Close()

	result, err := New().Extract(t.Context(), server.URL)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if result.URL != server.URL {
		t.Fatalf("URL = %q, want %q", result.URL, server.URL)
	}
	if result.Content != "# Fixture heading\n\nHello [link](https://target.example/path).\n\n* One\n* Two" {
		t.Fatalf("Content = %#v", result.Content)
	}
}

func TestDDGS_ExtractClassifiesNon200Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := New().Extract(t.Context(), server.URL)
	if !errors.Is(err, ErrDDGS) {
		t.Fatalf("Extract error %v does not classify as ErrDDGS", err)
	}
	if want := "Failed to fetch " + server.URL + ": HTTP 503"; err.Error() != want {
		t.Fatalf("Extract error = %q, want %q", err, want)
	}
}

type compositionSelectorFunc func(category, backend string) ([]engine.Metadata, error)

func (f compositionSelectorFunc) Select(category, backend string) ([]engine.Metadata, error) {
	return f(category, backend)
}

type compositionFactoryFunc func(engine.Metadata, transport.Config) (engine.Searcher, error)

func (f compositionFactoryFunc) Create(metadata engine.Metadata, config transport.Config) (engine.Searcher, error) {
	return f(metadata, config)
}

type recordingCompositionFactory struct {
	mu        sync.Mutex
	created   []string
	configsV  []transport.Config
	requestsV []engine.SearchRequest
}

func (f *recordingCompositionFactory) Create(metadata engine.Metadata, config transport.Config) (engine.Searcher, error) {
	f.mu.Lock()
	f.created = append(f.created, metadata.Name)
	f.configsV = append(f.configsV, config)
	f.mu.Unlock()
	return compositionSearcher{name: metadata.Name, factory: f}, nil
}

func (f *recordingCompositionFactory) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.created...)
}

func (f *recordingCompositionFactory) configs() []transport.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]transport.Config(nil), f.configsV...)
}

func (f *recordingCompositionFactory) requests() []engine.SearchRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]engine.SearchRequest(nil), f.requestsV...)
}

type compositionSearcher struct {
	name    string
	factory *recordingCompositionFactory
}

type compositionErrorSearcher struct {
	err error
}

func (s compositionErrorSearcher) Search(context.Context, engine.SearchRequest) ([]engine.Result, error) {
	return nil, s.err
}

type blockingConstructionFactory struct {
	first              string
	second             string
	secondConstructing chan struct{}
	releaseSecond      chan struct{}
	firstSearched      chan struct{}

	mu      sync.Mutex
	created []string
}

func newBlockingConstructionFactory(first, second string) *blockingConstructionFactory {
	return &blockingConstructionFactory{
		first:              first,
		second:             second,
		secondConstructing: make(chan struct{}),
		releaseSecond:      make(chan struct{}),
		firstSearched:      make(chan struct{}),
	}
}

func (f *blockingConstructionFactory) Create(metadata engine.Metadata, _ transport.Config) (engine.Searcher, error) {
	f.mu.Lock()
	f.created = append(f.created, metadata.Name)
	f.mu.Unlock()

	if metadata.Name == f.second {
		close(f.secondConstructing)
		<-f.releaseSecond
	}
	return blockingConstructionSearcher{name: metadata.Name, factory: f}, nil
}

func (f *blockingConstructionFactory) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.created...)
}

type blockingConstructionSearcher struct {
	name    string
	factory *blockingConstructionFactory
}

func (s blockingConstructionSearcher) Search(_ context.Context, request engine.SearchRequest) ([]engine.Result, error) {
	if s.name == s.factory.first {
		close(s.factory.firstSearched)
	}
	return []engine.Result{mustCompositionTextResult(s.name, request.Query)}, nil
}

func (s compositionSearcher) Search(_ context.Context, request engine.SearchRequest) ([]engine.Result, error) {
	s.factory.mu.Lock()
	s.factory.requestsV = append(s.factory.requestsV, request)
	s.factory.mu.Unlock()
	return []engine.Result{mustCompositionTextResult(s.name, request.Query)}, nil
}

func mustCompositionTextResult(name, body string) engine.Result {
	result, err := engine.NewCategoryResult("text", []engine.Field{
		{Name: "title", Value: name},
		{Name: "href", Value: "https://" + name + ".example/%2520"},
		{Name: "body", Value: body},
	})
	if err != nil {
		panic(err)
	}
	return result
}

func loadFacadeCompositionFixture(t *testing.T, path string) facadeCompositionFixture {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var fixture facadeCompositionFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func constructorNames(fixture facadeCompositionFixture) []string {
	names := make([]string, len(fixture.Result.Output.ConstructorCalls))
	for index, call := range fixture.Result.Output.ConstructorCalls {
		names[index] = call.Name
	}
	return names
}

func searchRequests(fixture facadeCompositionFixture) []engine.SearchRequest {
	requests := make([]engine.SearchRequest, len(fixture.Result.Output.SearchCalls))
	for index, call := range fixture.Result.Output.SearchCalls {
		timeLimit := call.Kwargs.TimeLimit
		requests[index] = engine.SearchRequest{
			Query:      call.Query,
			Region:     call.Kwargs.Region,
			SafeSearch: call.Kwargs.SafeSearch,
			TimeLimit:  &timeLimit,
			Page:       call.Kwargs.Page,
			Parameters: []engine.SearchParameter{{Name: "source_marker", Value: call.Kwargs.SourceMarker}},
		}
	}
	return requests
}

func expectedCompositionConfigs(t *testing.T, fixture facadeCompositionFixture) []transport.Config {
	t.Helper()
	proxy := fixture.Result.Output.ConstructorCalls[0].Proxy
	want := transport.Config{
		Proxy:        &proxy,
		Timeout:      transport.WithTimeout(0),
		Verification: transport.VerifyWithPEMFile(fixture.Input.Client.Verify),
	}
	configs := make([]transport.Config, len(fixture.Result.Output.ConstructorCalls))
	for index := range configs {
		configs[index] = want
	}
	return configs
}
