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

	"github.com/jcastilloa/goddgs/internal/normalize"
	"github.com/jcastilloa/goddgs/internal/parser"
	"github.com/jcastilloa/goddgs/internal/transport"
)

type imageEngineFixture struct {
	FixtureID string                 `json:"fixture_id"`
	Input     imageEngineSearchInput `json:"input"`
	Result    imageEngineResult      `json:"result"`
	Trace     []htmlTextEngineTrace  `json:"trace"`
}

type imageEngineSearchInput struct {
	Query        string  `json:"query"`
	Region       string  `json:"region"`
	SafeSearch   string  `json:"safesearch"`
	TimeLimit    *string `json:"timelimit"`
	Page         int     `json:"page"`
	MaxResults   *string `json:"max_results"`
	Size         *string `json:"size"`
	Color        *string `json:"color"`
	TypeImage    *string `json:"type_image"`
	Layout       *string `json:"layout"`
	LicenseImage *string `json:"license_image"`
}

type imageEngineResult struct {
	Status     string          `json:"status"`
	Output     json.RawMessage `json:"output"`
	FieldOrder [][]string      `json:"field_order"`
	Error      struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestBingImages_SearchMatchesFrozenFixtures(t *testing.T) {
	testImageEngineFixtures(t, "bing", func(client htmlTextTransport) Searcher {
		return NewBingImages(client)
	})
}

func TestDuckDuckGoImages_SearchMatchesFrozenFixtures(t *testing.T) {
	testImageEngineFixtures(t, "duckduckgo", func(client htmlTextTransport) Searcher {
		return NewDuckDuckGoImages(client)
	})
}

func TestImageAdapters_SearchHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for name, adapter := range map[string]Searcher{
		"bing":       NewBingImages(&scriptedHTMLTextTransport{}),
		"duckduckgo": NewDuckDuckGoImages(&scriptedHTMLTextTransport{}),
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

func TestImageAdapters_SearchPropagatesTransportError(t *testing.T) {
	sourceErr := errors.New("fixture transport failure")
	for name, adapter := range map[string]Searcher{
		"bing":       NewBingImages(&scriptedHTMLTextTransport{err: sourceErr}),
		"duckduckgo": NewDuckDuckGoImages(&scriptedHTMLTextTransport{err: sourceErr}),
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

func TestImageAdapters_SearchIsConcurrentSafe(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		adapter func(htmlTextTransport) Searcher
		calls   int
	}{
		{
			name:    "bing",
			path:    "../../testdata/contracts/engine/engine.images.bing-happy-html-metadata.json",
			adapter: func(client htmlTextTransport) Searcher { return NewBingImages(client) },
			calls:   32,
		},
		{
			name:    "duckduckgo",
			path:    "../../testdata/contracts/engine/engine.images.duckduckgo-happy-json.json",
			adapter: func(client htmlTextTransport) Searcher { return NewDuckDuckGoImages(client) },
			calls:   24,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := loadImageEngineFixture(t, test.path)
			client := routingImageTransportFromFixture(t, fixture)
			adapter := test.adapter(client)

			errorsByCall := make(chan error, test.calls)
			var group sync.WaitGroup
			for range test.calls {
				group.Add(1)
				go func() {
					defer group.Done()
					results, err := adapter.Search(context.Background(), imageFixtureRequest(fixture))
					if err != nil {
						errorsByCall <- err
						return
					}
					if len(results) != 1 {
						errorsByCall <- errors.New("unexpected image result count")
					}
				}()
			}
			group.Wait()
			close(errorsByCall)
			for err := range errorsByCall {
				t.Errorf("concurrent Search: %v", err)
			}

			wantRequests := test.calls * imageFixtureRequestCount(fixture)
			if got := client.RequestCount(); got != wantRequests {
				t.Fatalf("request count = %d, want %d", got, wantRequests)
			}
		})
	}
}

func testImageEngineFixtures(t *testing.T, name string, factory func(htmlTextTransport) Searcher) {
	t.Helper()

	paths, err := filepath.Glob("../../testdata/contracts/engine/engine.images." + name + "-*.json")
	if err != nil {
		t.Fatalf("find %s image fixtures: %v", name, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no %s image fixtures", name)
	}

	for _, path := range paths {
		fixture := loadImageEngineFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			client := &scriptedHTMLTextTransport{responses: imageFixtureResponses(t, fixture)}
			results, searchErr := factory(client).Search(context.Background(), imageFixtureRequest(fixture))

			assertImageEngineOutcome(t, results, searchErr, fixture)
			assertHTMLTextEngineEvents(t, client.Events(), htmlTextEngineFixture{
				FixtureID: fixture.FixtureID,
				Trace:     fixture.Trace,
			})
		})
	}
}

func loadImageEngineFixture(t testing.TB, path string) imageEngineFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()

	var fixture imageEngineFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func imageFixtureResponses(t testing.TB, fixture imageEngineFixture) []transport.Response {
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

func imageFixtureRequest(fixture imageEngineFixture) SearchRequest {
	input := fixture.Input
	parameters := make([]SearchParameter, 0, 6)
	for _, parameter := range []struct {
		name  string
		value *string
	}{
		{name: "max_results", value: input.MaxResults},
		{name: "size", value: input.Size},
		{name: "color", value: input.Color},
		{name: "type_image", value: input.TypeImage},
		{name: "layout", value: input.Layout},
		{name: "license_image", value: input.LicenseImage},
	} {
		if parameter.value != nil {
			parameters = append(parameters, SearchParameter{Name: parameter.name, Value: *parameter.value})
		}
	}
	return SearchRequest{
		Query:      input.Query,
		Region:     input.Region,
		SafeSearch: input.SafeSearch,
		TimeLimit:  input.TimeLimit,
		Page:       input.Page,
		Parameters: parameters,
	}
}

func assertImageEngineOutcome(t testing.TB, results []Result, err error, fixture imageEngineFixture) {
	t.Helper()

	if fixture.Result.Status == "error" {
		if err == nil {
			t.Fatalf("Search error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
		}
		if err.Error() != fixture.Result.Error.Message {
			t.Fatalf("Search error = %q, want %q", err, fixture.Result.Error.Message)
		}
		assertImageEngineErrorType(t, err, fixture)
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

func assertImageEngineErrorType(t testing.TB, err error, fixture imageEngineFixture) {
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

type routingImageTransport struct {
	mu       sync.Mutex
	routes   map[string]transport.Response
	requests int
}

func (*routingImageTransport) UpdateHeaders([]transport.Field) {}

func (*routingImageTransport) SetCookies(string, []transport.Field) error {
	return nil
}

func (client *routingImageTransport) Do(_ context.Context, request transport.Request) (transport.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.requests++
	response, ok := client.routes[request.URL]
	if !ok {
		return transport.Response{}, errors.New("unexpected image fixture request: " + request.URL)
	}
	return response, nil
}

func (client *routingImageTransport) RequestCount() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.requests
}

func routingImageTransportFromFixture(t testing.TB, fixture imageEngineFixture) *routingImageTransport {
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
	return &routingImageTransport{routes: routes}
}

func imageFixtureRequestCount(fixture imageEngineFixture) int {
	count := 0
	for _, event := range fixture.Trace {
		if event.Kind == "request" {
			count++
		}
	}
	return count
}
