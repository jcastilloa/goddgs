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

type videoEngineFixture struct {
	FixtureID string                 `json:"fixture_id"`
	Input     videoEngineSearchInput `json:"input"`
	Result    videoEngineResult      `json:"result"`
	Trace     []htmlTextEngineTrace  `json:"trace"`
}

type videoEngineSearchInput struct {
	Query         string  `json:"query"`
	Region        string  `json:"region"`
	SafeSearch    string  `json:"safesearch"`
	TimeLimit     *string `json:"timelimit"`
	Page          int     `json:"page"`
	Resolution    *string `json:"resolution"`
	Duration      *string `json:"duration"`
	LicenseVideos *string `json:"license_videos"`
}

type videoEngineResult struct {
	Status     string          `json:"status"`
	Output     json.RawMessage `json:"output"`
	FieldOrder [][]string      `json:"field_order"`
	Error      struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestDuckDuckGoVideos_SearchMatchesFrozenFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/engine/engine.videos.duckduckgo-*.json")
	if err != nil {
		t.Fatalf("find video fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no DuckDuckGo Videos fixtures")
	}

	for _, path := range paths {
		fixture := loadVideoEngineFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			client := &scriptedHTMLTextTransport{responses: videoFixtureResponses(t, fixture)}
			results, searchErr := NewDuckDuckGoVideos(client).Search(t.Context(), videoFixtureRequest(fixture))

			assertVideoEngineOutcome(t, results, searchErr, fixture)
			assertHTMLTextEngineEvents(t, client.Events(), htmlTextEngineFixture{
				FixtureID: fixture.FixtureID,
				Trace:     fixture.Trace,
			})
		})
	}
}

func TestDuckDuckGoVideos_SearchHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := NewDuckDuckGoVideos(&scriptedHTMLTextTransport{}).Search(ctx, SearchRequest{Query: "fixture"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search error = %v, want context.Canceled", err)
	}
	if results != nil {
		t.Fatalf("Search results = %#v, want nil", results)
	}
}

func TestDuckDuckGoVideos_SearchPropagatesTransportError(t *testing.T) {
	sourceErr := errors.New("fixture transport failure")
	results, err := NewDuckDuckGoVideos(&scriptedHTMLTextTransport{err: sourceErr}).Search(
		context.Background(),
		SearchRequest{Query: "fixture"},
	)
	if !errors.Is(err, sourceErr) {
		t.Fatalf("Search error = %v, want wrapped source error", err)
	}
	if results != nil {
		t.Fatalf("Search results = %#v, want nil", results)
	}
}

func TestDuckDuckGoVideos_SearchIsConcurrentSafe(t *testing.T) {
	fixture := loadVideoEngineFixture(t, "../../testdata/contracts/engine/engine.videos.duckduckgo-happy-heterogeneous-json.json")
	client := routingVideoTransportFromFixture(t, fixture)
	adapter := NewDuckDuckGoVideos(client)

	const calls = 32
	errorsByCall := make(chan error, calls)
	var group sync.WaitGroup
	for range calls {
		group.Add(1)
		go func() {
			defer group.Done()
			results, err := adapter.Search(context.Background(), videoFixtureRequest(fixture))
			if err != nil {
				errorsByCall <- err
				return
			}
			if len(results) != 1 {
				errorsByCall <- errors.New("unexpected video result count")
			}
		}()
	}
	group.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		t.Errorf("concurrent Search: %v", err)
	}

	wantRequests := calls * videoFixtureRequestCount(fixture)
	if got := client.RequestCount(); got != wantRequests {
		t.Fatalf("request count = %d, want %d", got, wantRequests)
	}
}

func loadVideoEngineFixture(t testing.TB, path string) videoEngineFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()

	var fixture videoEngineFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func videoFixtureResponses(t testing.TB, fixture videoEngineFixture) []transport.Response {
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

func videoFixtureRequest(fixture videoEngineFixture) SearchRequest {
	input := fixture.Input
	parameters := make([]SearchParameter, 0, 3)
	for _, parameter := range []struct {
		name  string
		value *string
	}{
		{name: "resolution", value: input.Resolution},
		{name: "duration", value: input.Duration},
		{name: "license_videos", value: input.LicenseVideos},
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

func assertVideoEngineOutcome(t testing.TB, results []Result, err error, fixture videoEngineFixture) {
	t.Helper()

	if fixture.Result.Status == "error" {
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

type routingVideoTransport struct {
	mu       sync.Mutex
	routes   map[string]transport.Response
	requests int
}

func (*routingVideoTransport) UpdateHeaders([]transport.Field) {}

func (*routingVideoTransport) SetCookies(string, []transport.Field) error {
	return nil
}

func (client *routingVideoTransport) Do(_ context.Context, request transport.Request) (transport.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.requests++
	response, ok := client.routes[request.URL]
	if !ok {
		return transport.Response{}, errors.New("unexpected video fixture request: " + request.URL)
	}
	response.Content = append([]byte(nil), response.Content...)
	return response, nil
}

func (client *routingVideoTransport) RequestCount() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.requests
}

func routingVideoTransportFromFixture(t testing.TB, fixture videoEngineFixture) *routingVideoTransport {
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
	return &routingVideoTransport{routes: routes}
}

func videoFixtureRequestCount(fixture videoEngineFixture) int {
	count := 0
	for _, event := range fixture.Trace {
		if event.Kind == "request" {
			count++
		}
	}
	return count
}
