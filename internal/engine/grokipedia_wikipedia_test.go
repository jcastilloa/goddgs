package engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
)

type jsonTextEngineFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Query      string  `json:"query"`
		Region     string  `json:"region"`
		SafeSearch string  `json:"safesearch"`
		TimeLimit  *string `json:"timelimit"`
		Page       int     `json:"page"`
	} `json:"input"`
	Result struct {
		Status     string          `json:"status"`
		Output     json.RawMessage `json:"output"`
		FieldOrder [][]string      `json:"field_order"`
		Error      struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"result"`
	Trace []jsonTextEngineTrace `json:"trace"`
}

type jsonTextEngineTrace struct {
	Kind       string            `json:"kind"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Query      map[string]string `json:"query"`
	QueryOrder []string          `json:"query_order"`
	Status     int               `json:"status"`
	Text       string            `json:"text"`
	Content    string            `json:"content_hex"`
}

type scriptedJSONTextTransport struct {
	mu        sync.Mutex
	responses []transport.Response
	requests  []transport.Request
}

func (client *scriptedJSONTextTransport) Do(_ context.Context, request transport.Request) (transport.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	client.requests = append(client.requests, cloneTransportRequest(request))
	if len(client.responses) == 0 {
		return transport.Response{}, errors.New("unexpected fixture transport request")
	}
	response := client.responses[0]
	client.responses = client.responses[1:]
	return response, nil
}

func (client *scriptedJSONTextTransport) Requests() []transport.Request {
	client.mu.Lock()
	defer client.mu.Unlock()

	requests := make([]transport.Request, len(client.requests))
	for index, request := range client.requests {
		requests[index] = cloneTransportRequest(request)
	}
	return requests
}

type routingJSONTextTransport struct {
	mu        sync.Mutex
	responses map[string]transport.Response
	requests  int
}

func (client *routingJSONTextTransport) Do(_ context.Context, request transport.Request) (transport.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()

	response, ok := client.responses[request.URL]
	if !ok {
		return transport.Response{}, errors.New("unexpected fixture transport request")
	}
	client.requests++
	response.Content = append([]byte(nil), response.Content...)
	return response, nil
}

func (client *routingJSONTextTransport) RequestCount() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.requests
}

func TestGrokipedia_SearchMatchesFrozenFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/engine/engine.text.grokipedia-*.json")
	if err != nil {
		t.Fatalf("find Grokipedia fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no Grokipedia fixtures")
	}

	for _, path := range paths {
		fixture := loadJSONTextEngineFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			client := scriptedTransportFromFixture(t, fixture)
			results, err := NewGrokipedia(client).Search(t.Context(), fixtureSearchRequest(fixture))
			assertJSONTextEngineOutcome(t, results, err, fixture)
			assertJSONTextEngineRequests(t, client.Requests(), fixture)
		})
	}
}

func TestWikipedia_SearchMatchesFrozenFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/engine/engine.text.wikipedia-*.json")
	if err != nil {
		t.Fatalf("find Wikipedia fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no Wikipedia fixtures")
	}

	for _, path := range paths {
		fixture := loadJSONTextEngineFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			client := scriptedTransportFromFixture(t, fixture)
			results, err := NewWikipedia(client).Search(t.Context(), fixtureSearchRequest(fixture))
			assertJSONTextEngineOutcome(t, results, err, fixture)
			assertJSONTextEngineRequests(t, client.Requests(), fixture)
		})
	}
}

func TestJSONTextAdapters_SearchHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, test := range []struct {
		name    string
		client  *scriptedJSONTextTransport
		adapter Searcher
	}{
		{
			name:   "grokipedia",
			client: &scriptedJSONTextTransport{},
		},
		{
			name:   "wikipedia",
			client: &scriptedJSONTextTransport{},
		},
	} {
		test.adapter = NewGrokipedia(test.client)
		if test.name == "wikipedia" {
			test.adapter = NewWikipedia(test.client)
		}
		t.Run(test.name, func(t *testing.T) {
			results, err := test.adapter.Search(ctx, SearchRequest{Query: "fixture", Region: "us-en"})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Search error = %v, want context.Canceled", err)
			}
			if results != nil {
				t.Fatalf("Search results = %#v, want nil", results)
			}
			if got := len(test.client.Requests()); got != 0 {
				t.Fatalf("request count = %d, want 0", got)
			}
		})
	}
}

func TestJSONTextAdapters_SearchPropagatesTransportError(t *testing.T) {
	sourceError := errors.New("fixture transport failure")
	for name, adapter := range map[string]Searcher{
		"grokipedia": NewGrokipedia(errorJSONTextTransport{err: sourceError}),
		"wikipedia":  NewWikipedia(errorJSONTextTransport{err: sourceError}),
	} {
		t.Run(name, func(t *testing.T) {
			results, err := adapter.Search(context.Background(), SearchRequest{Query: "fixture", Region: "us-en"})
			if !errors.Is(err, sourceError) {
				t.Fatalf("Search error = %v, want wrapped source error", err)
			}
			if results != nil {
				t.Fatalf("Search results = %#v, want nil", results)
			}
		})
	}
}

func TestJSONTextAdapters_SearchIsConcurrentSafe(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		adapter func(jsonTextTransport) Searcher
		calls   int
	}{
		{
			name:    "grokipedia",
			path:    "../../testdata/contracts/engine/engine.text.grokipedia-first-result-and-snippet-tail.json",
			adapter: func(client jsonTextTransport) Searcher { return NewGrokipedia(client) },
			calls:   32,
		},
		{
			name:    "wikipedia",
			path:    "../../testdata/contracts/engine/engine.text.wikipedia-opensearch-and-extract.json",
			adapter: func(client jsonTextTransport) Searcher { return NewWikipedia(client) },
			calls:   24,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := loadJSONTextEngineFixture(t, test.path)
			client := &routingJSONTextTransport{responses: fixtureResponseRoutes(t, fixture)}
			adapter := test.adapter(client)

			errCh := make(chan error, test.calls)
			var group sync.WaitGroup
			for range test.calls {
				group.Add(1)
				go func() {
					defer group.Done()
					results, err := adapter.Search(context.Background(), fixtureSearchRequest(fixture))
					if err != nil {
						errCh <- err
						return
					}
					if len(results) != 1 {
						errCh <- errors.New("unexpected JSON text result count")
					}
				}()
			}
			group.Wait()
			close(errCh)
			for err := range errCh {
				t.Errorf("concurrent Search: %v", err)
			}

			requestCount := 0
			for _, event := range fixture.Trace {
				if event.Kind == "request" {
					requestCount++
				}
			}
			if got, want := client.RequestCount(), test.calls*requestCount; got != want {
				t.Fatalf("request count = %d, want %d", got, want)
			}
		})
	}
}

type errorJSONTextTransport struct {
	err error
}

func (transportValue errorJSONTextTransport) Do(context.Context, transport.Request) (transport.Response, error) {
	return transport.Response{}, transportValue.err
}

func loadJSONTextEngineFixture(t testing.TB, path string) jsonTextEngineFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var fixture jsonTextEngineFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func scriptedTransportFromFixture(t testing.TB, fixture jsonTextEngineFixture) *scriptedJSONTextTransport {
	t.Helper()

	responses := make([]transport.Response, 0)
	for _, event := range fixture.Trace {
		if event.Kind != "response" {
			continue
		}
		content, err := hex.DecodeString(event.Content)
		if err != nil {
			t.Fatalf("decode %s response content: %v", fixture.FixtureID, err)
		}
		responses = append(responses, transport.Response{
			StatusCode: event.Status,
			Content:    content,
			Text:       event.Text,
		})
	}
	return &scriptedJSONTextTransport{responses: responses}
}

func fixtureResponseRoutes(t testing.TB, fixture jsonTextEngineFixture) map[string]transport.Response {
	t.Helper()

	routes := make(map[string]transport.Response)
	for index, event := range fixture.Trace {
		if event.Kind != "request" {
			continue
		}
		if index+1 >= len(fixture.Trace) || fixture.Trace[index+1].Kind != "response" {
			t.Fatalf("fixture %s request %d lacks following response", fixture.FixtureID, index)
		}
		responseEvent := fixture.Trace[index+1]
		content, err := hex.DecodeString(responseEvent.Content)
		if err != nil {
			t.Fatalf("decode %s response content: %v", fixture.FixtureID, err)
		}
		routes[event.URL] = transport.Response{
			StatusCode: responseEvent.Status,
			Content:    content,
			Text:       responseEvent.Text,
		}
	}
	return routes
}

func fixtureSearchRequest(fixture jsonTextEngineFixture) SearchRequest {
	return SearchRequest{
		Query:      fixture.Input.Query,
		Region:     fixture.Input.Region,
		SafeSearch: fixture.Input.SafeSearch,
		TimeLimit:  fixture.Input.TimeLimit,
		Page:       fixture.Input.Page,
	}
}

func assertJSONTextEngineOutcome(t testing.TB, results []Result, err error, fixture jsonTextEngineFixture) {
	t.Helper()

	if fixture.Result.Status == "error" {
		assertJSONTextEngineError(t, err, fixture)
		if results != nil {
			t.Fatalf("Search results = %#v, want nil with source error", results)
		}
		return
	}
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	var want []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(fixture.Result.Output))
	decoder.UseNumber()
	if err := decoder.Decode(&want); err != nil {
		t.Fatalf("decode %s result: %v", fixture.FixtureID, err)
	}
	if want == nil {
		if results != nil {
			t.Fatalf("results = %#v, want nil", results)
		}
		return
	}
	if results == nil {
		t.Fatal("results = nil, want non-nil source list")
	}
	if len(results) != len(want) {
		t.Fatalf("result count = %d, want %d", len(results), len(want))
	}
	if len(fixture.Result.FieldOrder) != len(want) {
		t.Fatalf("field-order count = %d, want %d", len(fixture.Result.FieldOrder), len(want))
	}
	for index, result := range results {
		if !reflect.DeepEqual(result.Map(), want[index]) {
			t.Fatalf("result %d map = %#v, want %#v", index, result.Map(), want[index])
		}
		fields := result.Fields()
		actualOrder := make([]string, len(fields))
		for fieldIndex, field := range fields {
			actualOrder[fieldIndex] = field.Name
		}
		if !reflect.DeepEqual(actualOrder, fixture.Result.FieldOrder[index]) {
			t.Fatalf("result %d field order = %v, want %v", index, actualOrder, fixture.Result.FieldOrder[index])
		}
	}
}

func assertJSONTextEngineError(t testing.TB, err error, fixture jsonTextEngineFixture) {
	t.Helper()

	if err == nil {
		t.Fatalf("Search error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
	}
	if err.Error() != fixture.Result.Error.Message {
		t.Fatalf("Search error = %q, want %q", err, fixture.Result.Error.Message)
	}
	switch fixture.Result.Error.Type {
	case "JSONDecodeError":
		var sourceError *parser.JSONDecodeError
		if !errors.As(err, &sourceError) {
			t.Fatalf("Search error = %T, want *parser.JSONDecodeError", err)
		}
	default:
		var sourceError *sourceEngineError
		if !errors.As(err, &sourceError) || sourceError.sourceType != fixture.Result.Error.Type {
			t.Fatalf("Search error = %#v, want source type %q", sourceError, fixture.Result.Error.Type)
		}
	}
}

func assertJSONTextEngineRequests(t testing.TB, requests []transport.Request, fixture jsonTextEngineFixture) {
	t.Helper()

	expected := make([]jsonTextEngineTrace, 0)
	for _, event := range fixture.Trace {
		if event.Kind == "request" {
			expected = append(expected, event)
		}
	}
	if len(requests) != len(expected) {
		t.Fatalf("request count = %d, want %d", len(requests), len(expected))
	}
	for index, request := range requests {
		event := expected[index]
		if request.Method != event.Method || request.URL != event.URL {
			t.Fatalf("request %d = %s %s, want %s %s", index, request.Method, request.URL, event.Method, event.URL)
		}
		wantQuery := orderedFixtureFields(t, fixture.FixtureID, event.Query, event.QueryOrder)
		if !reflect.DeepEqual(request.Query, wantQuery) {
			t.Fatalf("request %d query = %#v, want %#v", index, request.Query, wantQuery)
		}
		if len(request.Form) != 0 || len(request.Headers) != 0 || len(request.Cookies) != 0 {
			t.Fatalf("request %d unexpected fields = form=%v headers=%v cookies=%v", index, request.Form, request.Headers, request.Cookies)
		}
	}
}

func orderedFixtureFields(t testing.TB, fixtureID string, values map[string]string, order []string) []transport.Field {
	t.Helper()

	if values == nil && order == nil {
		return nil
	}
	if len(values) != len(order) {
		t.Fatalf("fixture %s values/order lengths = %d/%d", fixtureID, len(values), len(order))
	}
	fields := make([]transport.Field, 0, len(order))
	for _, name := range order {
		value, ok := values[name]
		if !ok {
			t.Fatalf("fixture %s order references missing field %q", fixtureID, name)
		}
		fields = append(fields, transport.Field{Name: name, Value: value})
	}
	return fields
}

func TestJSONTextFixtureErrorsAreSourceClassified(t *testing.T) {
	for _, path := range []string{
		"../../testdata/contracts/engine/engine.text.grokipedia-missing-slug-error.json",
		"../../testdata/contracts/engine/engine.text.wikipedia-invalid-region-error.json",
	} {
		fixture := loadJSONTextEngineFixture(t, path)
		if fixture.Result.Status != "error" {
			t.Fatalf("fixture %s status = %q, want error", fixture.FixtureID, fixture.Result.Status)
		}
	}
}
