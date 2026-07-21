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

	"github.com/jcastillo/goddgs/internal/transport"
)

type duckDuckGoTextFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Query      string  `json:"query"`
		Region     string  `json:"region"`
		SafeSearch string  `json:"safesearch"`
		TimeLimit  *string `json:"timelimit"`
		Page       int     `json:"page"`
	} `json:"input"`
	Result struct {
		Output     json.RawMessage `json:"output"`
		FieldOrder [][]string      `json:"field_order"`
	} `json:"result"`
	Trace []duckDuckGoTextTrace `json:"trace"`
}

type duckDuckGoTextTrace struct {
	Kind      string            `json:"kind"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Form      map[string]string `json:"form"`
	FormOrder []string          `json:"form_order"`
	Status    int               `json:"status"`
	Text      string            `json:"text"`
	Content   string            `json:"content_hex"`
}

type recordingDuckDuckGoTransport struct {
	mu       sync.Mutex
	response transport.Response
	err      error
	requests []transport.Request
}

func (client *recordingDuckDuckGoTransport) Do(_ context.Context, request transport.Request) (transport.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.requests = append(client.requests, cloneTransportRequest(request))
	return client.response, client.err
}

func (client *recordingDuckDuckGoTransport) Requests() []transport.Request {
	client.mu.Lock()
	defer client.mu.Unlock()
	requests := make([]transport.Request, len(client.requests))
	for index, request := range client.requests {
		requests[index] = cloneTransportRequest(request)
	}
	return requests
}

func TestDuckDuckGoText_SearchMatchesFrozenFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/engine/engine.text.duckduckgo-*.json")
	if err != nil {
		t.Fatalf("find DuckDuckGo fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no DuckDuckGo text fixtures")
	}

	for _, path := range paths {
		fixture := loadDuckDuckGoTextFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			response := duckDuckGoTextResponse(t, fixture)
			requester := &recordingDuckDuckGoTransport{response: response}
			adapter := NewDuckDuckGoText(requester)

			results, err := adapter.Search(context.Background(), SearchRequest{
				Query:      fixture.Input.Query,
				Region:     fixture.Input.Region,
				SafeSearch: fixture.Input.SafeSearch,
				TimeLimit:  fixture.Input.TimeLimit,
				Page:       fixture.Input.Page,
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}

			requests := requester.Requests()
			if len(requests) != 1 {
				t.Fatalf("request count = %d, want 1", len(requests))
			}
			assertDuckDuckGoRequest(t, requests[0], fixture)
			assertDuckDuckGoResults(t, results, fixture)
		})
	}
}

func TestDuckDuckGoText_SearchHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	requester := &recordingDuckDuckGoTransport{}
	adapter := NewDuckDuckGoText(requester)

	results, err := adapter.Search(ctx, SearchRequest{Query: "fixture", Region: "us-en", Page: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search error = %v, want context.Canceled", err)
	}
	if results != nil {
		t.Fatalf("Search results = %#v, want nil", results)
	}
	if got := len(requester.Requests()); got != 0 {
		t.Fatalf("request count = %d, want 0", got)
	}
}

func TestDuckDuckGoText_SearchPropagatesTransportError(t *testing.T) {
	sourceError := errors.New("fixture transport failure")
	requester := &recordingDuckDuckGoTransport{err: sourceError}
	adapter := NewDuckDuckGoText(requester)

	results, err := adapter.Search(context.Background(), SearchRequest{Query: "fixture", Region: "us-en", Page: 1})
	if !errors.Is(err, sourceError) {
		t.Fatalf("Search error = %v, want wrapped source error", err)
	}
	if results != nil {
		t.Fatalf("Search results = %#v, want nil", results)
	}
}

func TestDuckDuckGoText_SearchIsConcurrentSafe(t *testing.T) {
	fixture := loadDuckDuckGoTextFixture(t, "../../testdata/contracts/engine/engine.text.duckduckgo-happy-html-and-yjs-filter.json")
	requester := &recordingDuckDuckGoTransport{response: duckDuckGoTextResponse(t, fixture)}
	adapter := NewDuckDuckGoText(requester)

	const calls = 32
	errCh := make(chan error, calls)
	var group sync.WaitGroup
	for range calls {
		group.Add(1)
		go func() {
			defer group.Done()
			results, err := adapter.Search(context.Background(), SearchRequest{
				Query:      fixture.Input.Query,
				Region:     fixture.Input.Region,
				SafeSearch: fixture.Input.SafeSearch,
				TimeLimit:  fixture.Input.TimeLimit,
				Page:       fixture.Input.Page,
			})
			if err != nil {
				errCh <- err
				return
			}
			if len(results) != 1 {
				errCh <- errors.New("unexpected DuckDuckGo result count")
			}
		}()
	}
	group.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent Search: %v", err)
	}
	if got := len(requester.Requests()); got != calls {
		t.Fatalf("request count = %d, want %d", got, calls)
	}
}

func loadDuckDuckGoTextFixture(t *testing.T, path string) duckDuckGoTextFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()

	var fixture duckDuckGoTextFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func duckDuckGoTextResponse(t *testing.T, fixture duckDuckGoTextFixture) transport.Response {
	t.Helper()

	for _, event := range fixture.Trace {
		if event.Kind != "response" {
			continue
		}
		content, err := hex.DecodeString(event.Content)
		if err != nil {
			t.Fatalf("decode %s response content: %v", fixture.FixtureID, err)
		}
		return transport.Response{StatusCode: event.Status, Content: content, Text: event.Text}
	}
	t.Fatalf("fixture %s has no response trace", fixture.FixtureID)
	return transport.Response{}
}

func assertDuckDuckGoRequest(t *testing.T, request transport.Request, fixture duckDuckGoTextFixture) {
	t.Helper()

	var expected duckDuckGoTextTrace
	for _, event := range fixture.Trace {
		if event.Kind == "request" {
			expected = event
			break
		}
	}
	if expected.Method == "" {
		t.Fatalf("fixture %s has no request trace", fixture.FixtureID)
	}
	if request.Method != expected.Method || request.URL != expected.URL {
		t.Fatalf("request method/url = %s %s, want %s %s", request.Method, request.URL, expected.Method, expected.URL)
	}
	if len(request.Query) != 0 || len(request.Headers) != 0 || len(request.Cookies) != 0 {
		t.Fatalf("request extra fields = query=%v headers=%v cookies=%v, want none", request.Query, request.Headers, request.Cookies)
	}

	wantForm := make([]transport.Field, 0, len(expected.FormOrder))
	for _, name := range expected.FormOrder {
		value, ok := expected.Form[name]
		if !ok {
			t.Fatalf("fixture %s form order references missing %q", fixture.FixtureID, name)
		}
		wantForm = append(wantForm, transport.Field{Name: name, Value: value})
	}
	if !reflect.DeepEqual(request.Form, wantForm) {
		t.Fatalf("request form = %#v, want %#v", request.Form, wantForm)
	}
}

func assertDuckDuckGoResults(t *testing.T, results []Result, fixture duckDuckGoTextFixture) {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(fixture.Result.Output))
	decoder.UseNumber()
	var want []map[string]any
	if err := decoder.Decode(&want); err != nil {
		t.Fatalf("decode %s output: %v", fixture.FixtureID, err)
	}
	if want == nil {
		if results != nil {
			t.Fatalf("results = %#v, want nil", results)
		}
		return
	}
	if results == nil {
		t.Fatal("results = nil, want non-nil slice")
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
		order := make([]string, len(fields))
		for fieldIndex, field := range fields {
			order[fieldIndex] = field.Name
		}
		if !reflect.DeepEqual(order, fixture.Result.FieldOrder[index]) {
			t.Fatalf("result %d field order = %v, want %v", index, order, fixture.Result.FieldOrder[index])
		}
	}
}

func cloneTransportRequest(request transport.Request) transport.Request {
	return transport.Request{
		Method:  request.Method,
		URL:     request.URL,
		Query:   append([]transport.Field(nil), request.Query...),
		Form:    append([]transport.Field(nil), request.Form...),
		Headers: append([]transport.Field(nil), request.Headers...),
		Cookies: append([]transport.Field(nil), request.Cookies...),
	}
}
