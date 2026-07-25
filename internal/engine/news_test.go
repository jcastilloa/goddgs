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
	"time"

	"github.com/jcastillo/goddgs/internal/normalize"
	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
)

var fixtureNewsNow = func() time.Time {
	return time.Date(2024, time.April, 5, 6, 7, 8, 987654321, time.UTC)
}

type newsEngineFixture struct {
	FixtureID string                `json:"fixture_id"`
	Input     htmlTextSearchInput   `json:"input"`
	Result    newsEngineResult      `json:"result"`
	Trace     []htmlTextEngineTrace `json:"trace"`
}

type newsEngineResult struct {
	Status     string          `json:"status"`
	Output     json.RawMessage `json:"output"`
	FieldOrder [][]string      `json:"field_order"`
	Error      struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestBingNews_SearchMatchesFrozenFixtures(t *testing.T) {
	testNewsEngineFixtures(t, "bing", func(client htmlTextTransport) Searcher {
		return newBingNewsWithClock(client, fixtureNewsNow)
	})
}

func TestDuckDuckGoNews_SearchMatchesFrozenFixtures(t *testing.T) {
	testNewsEngineFixtures(t, "duckduckgo", func(client htmlTextTransport) Searcher {
		return NewDuckDuckGoNews(client)
	})
}

func TestYahooNews_SearchMatchesFrozenFixtures(t *testing.T) {
	testNewsEngineFixtures(t, "yahoo", func(client htmlTextTransport) Searcher {
		return newYahooNewsWithClock(client, fixtureNewsNow)
	})
}

func TestNewsAdapters_SearchHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for name, adapter := range map[string]Searcher{
		"bing":       NewBingNews(&scriptedHTMLTextTransport{}),
		"duckduckgo": NewDuckDuckGoNews(&scriptedHTMLTextTransport{}),
		"yahoo":      NewYahooNews(&scriptedHTMLTextTransport{}),
	} {
		t.Run(name, func(t *testing.T) {
			results, err := adapter.Search(ctx, SearchRequest{Query: "fixture", Region: "us-en"})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Search error = %v, want context.Canceled", err)
			}
			if results != nil {
				t.Fatalf("Search results = %#v, want nil", results)
			}
		})
	}
}

func TestNewsAdapters_SearchPropagatesTransportError(t *testing.T) {
	sourceErr := errors.New("fixture transport failure")
	for name, adapter := range map[string]Searcher{
		"bing":       NewBingNews(&scriptedHTMLTextTransport{err: sourceErr}),
		"duckduckgo": NewDuckDuckGoNews(&scriptedHTMLTextTransport{err: sourceErr}),
		"yahoo":      NewYahooNews(&scriptedHTMLTextTransport{err: sourceErr}),
	} {
		t.Run(name, func(t *testing.T) {
			results, err := adapter.Search(context.Background(), SearchRequest{Query: "fixture", Region: "us-en"})
			if !errors.Is(err, sourceErr) {
				t.Fatalf("Search error = %v, want wrapped source error", err)
			}
			if results != nil {
				t.Fatalf("Search results = %#v, want nil", results)
			}
		})
	}
}

func TestNewsAdapters_SearchIsConcurrentSafe(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		adapter func(htmlTextTransport) Searcher
		calls   int
	}{
		{
			name: "bing",
			path: "../../testdata/contracts/engine/engine.news.bing-happy-html.json",
			adapter: func(client htmlTextTransport) Searcher {
				return newBingNewsWithClock(client, fixtureNewsNow)
			},
			calls: 32,
		},
		{
			name: "duckduckgo",
			path: "../../testdata/contracts/engine/engine.news.duckduckgo-happy-json.json",
			adapter: func(client htmlTextTransport) Searcher {
				return NewDuckDuckGoNews(client)
			},
			calls: 24,
		},
		{
			name: "yahoo",
			path: "../../testdata/contracts/engine/engine.news.yahoo-happy-html-postprocess.json",
			adapter: func(client htmlTextTransport) Searcher {
				return newYahooNewsWithClock(client, fixtureNewsNow)
			},
			calls: 32,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := loadNewsEngineFixture(t, test.path)
			client := routingNewsTransportFromFixture(t, fixture)
			adapter := test.adapter(client)

			errorsByCall := make(chan error, test.calls)
			var group sync.WaitGroup
			for range test.calls {
				group.Add(1)
				go func() {
					defer group.Done()
					results, err := adapter.Search(context.Background(), htmlTextSearchRequestFromInput(fixture.Input))
					if err != nil {
						errorsByCall <- err
						return
					}
					if len(results) != 1 {
						errorsByCall <- errors.New("unexpected news result count")
					}
				}()
			}
			group.Wait()
			close(errorsByCall)
			for err := range errorsByCall {
				t.Errorf("concurrent Search: %v", err)
			}

			wantRequests := test.calls * newsFixtureRequestCount(fixture)
			if got := client.RequestCount(); got != wantRequests {
				t.Fatalf("request count = %d, want %d", got, wantRequests)
			}
		})
	}
}

func testNewsEngineFixtures(t *testing.T, name string, factory func(htmlTextTransport) Searcher) {
	t.Helper()

	paths, err := filepath.Glob("../../testdata/contracts/engine/engine.news." + name + "-*.json")
	if err != nil {
		t.Fatalf("find %s news fixtures: %v", name, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no %s news fixtures", name)
	}

	for _, path := range paths {
		fixture := loadNewsEngineFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			client := &scriptedHTMLTextTransport{responses: newsFixtureResponses(t, fixture)}
			results, searchErr := factory(client).Search(t.Context(), htmlTextSearchRequestFromInput(fixture.Input))

			assertNewsEngineOutcome(t, results, searchErr, fixture)
			assertHTMLTextEngineEvents(t, client.Events(), htmlTextEngineFixture{
				FixtureID: fixture.FixtureID,
				Trace:     fixture.Trace,
			})
		})
	}
}

func loadNewsEngineFixture(t testing.TB, path string) newsEngineFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()

	var fixture newsEngineFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func newsFixtureResponses(t testing.TB, fixture newsEngineFixture) []transport.Response {
	t.Helper()

	responses := make([]transport.Response, 0)
	for _, event := range fixture.Trace {
		if event.Kind != "response" {
			continue
		}
		content, err := hex.DecodeString(event.ContentHex)
		if err != nil {
			t.Fatalf("decode %s response content: %v", fixture.FixtureID, err)
		}
		responses = append(responses, transport.Response{
			StatusCode: event.StatusCode,
			Content:    content,
			Text:       event.Text,
		})
	}
	return responses
}

func assertNewsEngineOutcome(t testing.TB, results []Result, err error, fixture newsEngineFixture) {
	t.Helper()

	if fixture.Result.Status == "error" {
		if err == nil {
			t.Fatalf("Search error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
		}
		if err.Error() != fixture.Result.Error.Message {
			t.Fatalf("Search error = %q, want %q", err, fixture.Result.Error.Message)
		}
		assertNewsEngineErrorType(t, err, fixture)
		if results != nil {
			t.Fatalf("Search results = %#v, want nil with source error", results)
		}
		return
	}
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	want := decodeHTMLTextResults(t, fixture.FixtureID, fixture.Result.Output)
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
		order := make([]string, len(fields))
		for fieldIndex, field := range fields {
			order[fieldIndex] = field.Name
		}
		if !reflect.DeepEqual(order, fixture.Result.FieldOrder[index]) {
			t.Fatalf("result %d field order = %v, want %v", index, order, fixture.Result.FieldOrder[index])
		}
	}
}

func assertNewsEngineErrorType(t testing.TB, err error, fixture newsEngineFixture) {
	t.Helper()

	switch fixture.Result.Error.Type {
	case "JSONDecodeError":
		var sourceError *parser.JSONDecodeError
		if !errors.As(err, &sourceError) {
			t.Fatalf("Search error = %T, want JSONDecodeError", err)
		}
	case "DDGSException":
		if !errors.Is(err, normalize.ErrVQD) {
			t.Fatalf("Search error = %v, want VQD source error", err)
		}
	default:
		var sourceError *sourceEngineError
		if !errors.As(err, &sourceError) || sourceError.sourceType != fixture.Result.Error.Type {
			t.Fatalf("Search error = %#v, want source type %q", sourceError, fixture.Result.Error.Type)
		}
	}
}

type routingNewsTransport struct {
	mu       sync.Mutex
	routes   map[string]transport.Response
	requests int
}

func (*routingNewsTransport) UpdateHeaders([]transport.Field) {}

func (*routingNewsTransport) SetCookies(string, []transport.Field) error {
	return nil
}

func (client *routingNewsTransport) Do(_ context.Context, request transport.Request) (transport.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.requests++
	response, ok := client.routes[request.URL]
	if !ok {
		return transport.Response{}, errors.New("unexpected news fixture request: " + request.URL)
	}
	response.Content = append([]byte(nil), response.Content...)
	return response, nil
}

func (client *routingNewsTransport) RequestCount() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.requests
}

func routingNewsTransportFromFixture(t testing.TB, fixture newsEngineFixture) *routingNewsTransport {
	t.Helper()

	routes := make(map[string]transport.Response)
	for index, event := range fixture.Trace {
		if event.Kind != "request" {
			continue
		}
		if index+1 >= len(fixture.Trace) || fixture.Trace[index+1].Kind != "response" {
			t.Fatalf("fixture %s request %d lacks following response", fixture.FixtureID, index)
		}
		response := fixture.Trace[index+1]
		content, err := hex.DecodeString(response.ContentHex)
		if err != nil {
			t.Fatalf("decode %s response content: %v", fixture.FixtureID, err)
		}
		routes[event.URL] = transport.Response{
			StatusCode: response.StatusCode,
			Content:    content,
			Text:       response.Text,
		}
	}
	return &routingNewsTransport{routes: routes}
}

func newsFixtureRequestCount(fixture newsEngineFixture) int {
	count := 0
	for _, event := range fixture.Trace {
		if event.Kind == "request" {
			count++
		}
	}
	return count
}
