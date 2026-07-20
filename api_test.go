package ddgs

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type searchInvocationFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Arguments map[string]json.RawMessage `json:"arguments"`
		Query     string                     `json:"query"`
	} `json:"input"`
	Result struct {
		Output struct {
			Calls []struct {
				Kwargs struct {
					Page       int     `json:"page"`
					Region     string  `json:"region"`
					SafeSearch string  `json:"safesearch"`
					TimeLimit  *string `json:"timelimit"`
				} `json:"kwargs"`
				Query string `json:"query"`
			} `json:"calls"`
			Selections []struct {
				Backend string `json:"backend"`
			} `json:"selections"`
		} `json:"output"`
	} `json:"result"`
}

type recordingExecutor struct {
	searchResult  []RawResult
	searchError   error
	extractResult ExtractResult
	extractError  error
	searches      []searchRequest
	extracts      []extractRequest
}

func (e *recordingExecutor) search(_ context.Context, request searchRequest) ([]RawResult, error) {
	e.searches = append(e.searches, request)
	return e.searchResult, e.searchError
}

func (e *recordingExecutor) extract(_ context.Context, request extractRequest) (ExtractResult, error) {
	e.extracts = append(e.extracts, request)
	return e.extractResult, e.extractError
}

func TestDDGS_TextMatchesFrozenSearchInvocationFixtures(t *testing.T) {
	paths, err := filepath.Glob("testdata/contracts/pure/pure.search-call-*.json")
	if err != nil {
		t.Fatalf("find search invocation fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no search invocation fixtures")
	}

	wantResults := []RawResult{{
		"title":      "mixed result",
		"statistics": map[string]any{"viewCount": 29_059},
		"images":     map[string]any{"large": "https://image.example/large"},
	}}
	for _, path := range paths {
		fixture := loadSearchInvocationFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			executor := &recordingExecutor{searchResult: wantResults}
			client := New()
			client.executor = executor

			got, err := client.Text(context.Background(), fixture.Input.Query, searchInvocationFixtureOptions(t, fixture.Input.Arguments)...)
			if err != nil {
				t.Fatalf("Text(...): %v", err)
			}
			if !reflect.DeepEqual(got, wantResults) {
				t.Fatalf("Text(...) = %#v, want %#v", got, wantResults)
			}
			if _, ok := got[0]["statistics"].(map[string]any); !ok {
				t.Fatalf("statistics type = %T, want map[string]any", got[0]["statistics"])
			}
			assertSearchInvocationFixture(t, executor.searches, fixture)
		})
	}
}

func TestDDGS_SearchOptionsPreserveUnvalidatedNegativeMaxResults(t *testing.T) {
	fixture := loadErrorFixture(t, "testdata/contracts/pure/pure.scheduler-negative-max-workers.json")
	executor := &recordingExecutor{}
	client := New()
	client.executor = executor

	_, err := client.Text(context.Background(), "query", WithMaxResults(-20))
	if err != nil {
		t.Fatalf("Text(...): %v", err)
	}
	if len(executor.searches) != 1 {
		t.Fatalf("search calls = %d, want 1", len(executor.searches))
	}
	if got := executor.searches[0].config.maxResults; got == nil || *got != -20 {
		t.Fatalf("max results = %#v, want -20", got)
	}
	if got, want := fixture.Result.Error.Type, "ValueError"; got != want {
		t.Fatalf("source scheduler error type = %q, want %q", got, want)
	}
}

func TestDDGS_SearchMethodsRouteSourceCategories(t *testing.T) {
	tests := []struct {
		name string
		call func(*DDGS, context.Context) ([]RawResult, error)
		want searchCategory
	}{
		{name: "text", call: func(d *DDGS, ctx context.Context) ([]RawResult, error) { return d.Text(ctx, "query") }, want: textCategory},
		{name: "images", call: func(d *DDGS, ctx context.Context) ([]RawResult, error) { return d.Images(ctx, "query") }, want: imagesCategory},
		{name: "news", call: func(d *DDGS, ctx context.Context) ([]RawResult, error) { return d.News(ctx, "query") }, want: newsCategory},
		{name: "videos", call: func(d *DDGS, ctx context.Context) ([]RawResult, error) { return d.Videos(ctx, "query") }, want: videosCategory},
		{name: "books", call: func(d *DDGS, ctx context.Context) ([]RawResult, error) { return d.Books(ctx, "query") }, want: booksCategory},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := []RawResult{{"category": string(tt.want)}}
			executor := &recordingExecutor{searchResult: want}
			client := New()
			client.executor = executor

			got, err := tt.call(client, context.Background())
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("search result = %#v, want %#v", got, want)
			}
			if len(executor.searches) != 1 {
				t.Fatalf("search calls = %d, want 1", len(executor.searches))
			}
			if got := executor.searches[0].category; got != tt.want {
				t.Fatalf("category = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDDGS_ExtractPreservesSourceContentKinds(t *testing.T) {
	tests := []struct {
		name       string
		options    []ExtractOption
		result     ExtractResult
		wantFormat string
	}{
		{
			name:       "default markdown string",
			result:     ExtractResult{URL: "https://example.test", Content: "# markdown"},
			wantFormat: "text_markdown",
		},
		{
			name:       "raw bytes",
			options:    []ExtractOption{WithExtractFormat("content")},
			result:     ExtractResult{URL: "https://example.test", Content: []byte("raw-bytes")},
			wantFormat: "content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &recordingExecutor{extractResult: tt.result}
			client := New()
			client.executor = executor

			got, err := client.Extract(context.Background(), "https://example.test", tt.options...)
			if err != nil {
				t.Fatalf("Extract(...): %v", err)
			}
			if !reflect.DeepEqual(got, tt.result) {
				t.Fatalf("Extract(...) = %#v, want %#v", got, tt.result)
			}
			if len(executor.extracts) != 1 {
				t.Fatalf("extract calls = %d, want 1", len(executor.extracts))
			}
			if got := executor.extracts[0].url; got != "https://example.test" {
				t.Fatalf("extract URL = %q, want original URL", got)
			}
			if got := executor.extracts[0].config.format; got != tt.wantFormat {
				t.Fatalf("extract format = %q, want %q", got, tt.wantFormat)
			}
		})
	}
}

func TestDDGS_CanceledContextSkipsExecutor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		call func(*DDGS, context.Context) error
	}{
		{
			name: "search",
			call: func(client *DDGS, ctx context.Context) error {
				_, err := client.Text(ctx, "query")
				return err
			},
		},
		{
			name: "extract",
			call: func(client *DDGS, ctx context.Context) error {
				_, err := client.Extract(ctx, "https://example.test")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &recordingExecutor{}
			client := New()
			client.executor = executor

			if err := tt.call(client, ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("operation error = %v, want context.Canceled", err)
			}
			if len(executor.searches) != 0 || len(executor.extracts) != 0 {
				t.Fatalf("executor was called: searches=%d extracts=%d", len(executor.searches), len(executor.extracts))
			}
		})
	}
}

func TestDDGS_EmptyQueryMatchesFrozenErrorContract(t *testing.T) {
	fixture := loadErrorFixture(t, "testdata/contracts/pure/pure.error-empty-query.json")
	executor := &recordingExecutor{}
	client := New()
	client.executor = executor

	_, err := client.Text(context.Background(), fixture.Input.Query)
	if !errors.Is(err, ErrDDGS) {
		t.Fatalf("Text empty query error does not classify as ErrDDGS: %v", err)
	}
	var sourceError *DDGSError
	if !errors.As(err, &sourceError) {
		t.Fatalf("Text empty query error type = %T, want *DDGSError", err)
	}
	if got, want := err.Error(), fixture.Result.Error.Message; got != want {
		t.Fatalf("Text empty query error = %q, want %q", got, want)
	}
	if len(executor.searches) != 0 {
		t.Fatalf("executor search calls = %d, want 0", len(executor.searches))
	}
}

func TestDDGSErrorClassificationsPreserveCause(t *testing.T) {
	cause := errors.New("transport cause")
	tests := []struct {
		name           string
		error          *DDGSError
		classification error
		message        string
	}{
		{
			name:           "generic",
			error:          &DDGSError{kind: ddgsErrorGeneric, message: "generic source failure", cause: cause},
			classification: ErrDDGS,
			message:        "generic source failure",
		},
		{
			name:           "timeout",
			error:          &DDGSError{kind: ddgsErrorTimeout, message: "operation timed out exactly", cause: cause},
			classification: ErrTimeout,
			message:        "operation timed out exactly",
		},
		{
			name:           "rate limit",
			error:          &DDGSError{kind: ddgsErrorRateLimit, message: "rate limited", cause: cause},
			classification: ErrRateLimit,
			message:        "rate limited",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.error.Error(); got != tt.message {
				t.Fatalf("Error() = %q, want %q", got, tt.message)
			}
			if !errors.Is(tt.error, ErrDDGS) {
				t.Fatal("error does not classify as ErrDDGS")
			}
			if !errors.Is(tt.error, tt.classification) {
				t.Fatalf("error does not classify as %v", tt.classification)
			}
			if !errors.Is(tt.error, cause) {
				t.Fatal("error does not preserve cause")
			}
		})
	}
}

type errorFixture struct {
	Input struct {
		Query string `json:"query"`
	} `json:"input"`
	Result struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"result"`
}

func loadSearchInvocationFixture(t *testing.T, path string) searchInvocationFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture searchInvocationFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func loadErrorFixture(t *testing.T, path string) errorFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture errorFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func searchInvocationFixtureOptions(t *testing.T, arguments map[string]json.RawMessage) []SearchOption {
	t.Helper()

	var options []SearchOption
	if raw, ok := arguments["region"]; ok {
		options = append(options, WithRegion(decodeFixtureString(t, "region", raw)))
	}
	if raw, ok := arguments["safesearch"]; ok {
		options = append(options, WithSafeSearch(decodeFixtureString(t, "safesearch", raw)))
	}
	if raw, ok := arguments["timelimit"]; ok {
		options = append(options, WithTimeLimit(decodeFixtureString(t, "timelimit", raw)))
	}
	if raw, ok := arguments["max_results"]; ok {
		if string(raw) == "null" {
			options = append(options, WithoutMaxResults())
		} else {
			var value int
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatalf("decode max_results: %v", err)
			}
			options = append(options, WithMaxResults(value))
		}
	}
	if raw, ok := arguments["page"]; ok {
		var value int
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatalf("decode page: %v", err)
		}
		options = append(options, WithPage(value))
	}
	if raw, ok := arguments["backend"]; ok {
		options = append(options, WithBackend(decodeFixtureString(t, "backend", raw)))
	}
	return options
}

func decodeFixtureString(t *testing.T, name string, raw json.RawMessage) string {
	t.Helper()

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return value
}

func assertSearchInvocationFixture(t *testing.T, requests []searchRequest, fixture searchInvocationFixture) {
	t.Helper()

	if len(requests) != 1 {
		t.Fatalf("search requests = %d, want 1", len(requests))
	}
	if len(fixture.Result.Output.Calls) != 1 || len(fixture.Result.Output.Selections) != 1 {
		t.Fatalf("fixture %s has invalid one-call shape", fixture.FixtureID)
	}

	request := requests[0]
	call := fixture.Result.Output.Calls[0]
	if request.category != textCategory {
		t.Fatalf("category = %q, want %q", request.category, textCategory)
	}
	if request.query != call.Query {
		t.Fatalf("query = %q, want %q", request.query, call.Query)
	}
	if got, want := request.config.region, call.Kwargs.Region; got != want {
		t.Fatalf("region = %q, want %q", got, want)
	}
	if got, want := request.config.safeSearch, call.Kwargs.SafeSearch; got != want {
		t.Fatalf("safe search = %q, want %q", got, want)
	}
	if got, want := request.config.timeLimit, call.Kwargs.TimeLimit; !optionalStringEqual(got, want) {
		t.Fatalf("time limit = %#v, want %#v", got, want)
	}
	if got, want := request.config.page, call.Kwargs.Page; got != want {
		t.Fatalf("page = %d, want %d", got, want)
	}
	if got, want := request.config.backend, fixture.Result.Output.Selections[0].Backend; got != want {
		t.Fatalf("backend = %q, want %q", got, want)
	}
	assertMaxResultsFixture(t, request.config.maxResults, fixture.Input.Arguments)
}

func optionalStringEqual(got *string, want *string) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func assertMaxResultsFixture(t *testing.T, got *int, arguments map[string]json.RawMessage) {
	t.Helper()

	raw, explicit := arguments["max_results"]
	if !explicit {
		if got == nil || *got != 10 {
			t.Fatalf("max results = %v, want default 10", got)
		}
		return
	}
	if string(raw) == "null" {
		if got != nil {
			t.Fatalf("max results = %d, want nil", *got)
		}
		return
	}
	var want int
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("decode fixture max_results: %v", err)
	}
	if got == nil || *got != want {
		if got == nil {
			t.Fatalf("max results = nil, want %d", want)
		}
		t.Fatalf("max results = %d, want %d", *got, want)
	}
}
