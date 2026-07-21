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
	"regexp"
	"sync"
	"testing"

	"github.com/jcastillo/goddgs/internal/transport"
)

type htmlTextEngineFixture struct {
	FixtureID string `json:"fixture_id"`
	Contract  struct {
		Operation string `json:"operation"`
	} `json:"contract"`
	Input  htmlTextSearchInput `json:"input"`
	Result struct {
		Status     string          `json:"status"`
		Output     json.RawMessage `json:"output"`
		FieldOrder [][]string      `json:"field_order"`
		Error      struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"result"`
	Trace []htmlTextEngineTrace `json:"trace"`
}

type htmlTextSearchInput struct {
	Query      string  `json:"query"`
	Region     string  `json:"region"`
	SafeSearch string  `json:"safesearch"`
	TimeLimit  *string `json:"timelimit"`
	Page       int     `json:"page"`
}

type htmlTextEngineTrace struct {
	Kind        string            `json:"kind"`
	Note        string            `json:"note"`
	Headers     map[string]string `json:"headers"`
	Cookies     map[string]string `json:"cookies"`
	CookieOrder []string          `json:"cookie_order"`
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Query       map[string]string `json:"query"`
	QueryOrder  []string          `json:"query_order"`
	Form        map[string]string `json:"form"`
	FormOrder   []string          `json:"form_order"`
	Value       json.RawMessage   `json:"value"`
	StatusCode  int               `json:"status"`
	Text        string            `json:"text"`
	ContentHex  string            `json:"content_hex"`
}

type htmlTextTransportEvent struct {
	kind    string
	headers []transport.Field
	url     string
	cookies []transport.Field
	request transport.Request
}

type scriptedHTMLTextTransport struct {
	mu        sync.Mutex
	responses []transport.Response
	events    []htmlTextTransportEvent
	err       error
}

func (client *scriptedHTMLTextTransport) UpdateHeaders(fields []transport.Field) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.events = append(client.events, htmlTextTransportEvent{
		kind:    "headers_update",
		headers: cloneTransportFields(fields),
	})
}

func (client *scriptedHTMLTextTransport) SetCookies(rawURL string, fields []transport.Field) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.events = append(client.events, htmlTextTransportEvent{
		kind:    "cookie",
		url:     rawURL,
		cookies: cloneTransportFields(fields),
	})
	return client.err
}

func (client *scriptedHTMLTextTransport) Do(_ context.Context, request transport.Request) (transport.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.events = append(client.events, htmlTextTransportEvent{
		kind:    "request",
		request: cloneTransportRequest(request),
	})
	if client.err != nil {
		return transport.Response{}, client.err
	}
	if len(client.responses) == 0 {
		return transport.Response{}, errors.New("unexpected fixture transport request")
	}
	response := client.responses[0]
	client.responses = client.responses[1:]
	return response, nil
}

func (client *scriptedHTMLTextTransport) Events() []htmlTextTransportEvent {
	client.mu.Lock()
	defer client.mu.Unlock()
	events := make([]htmlTextTransportEvent, len(client.events))
	for index, event := range client.events {
		events[index] = htmlTextTransportEvent{
			kind:    event.kind,
			headers: cloneTransportFields(event.headers),
			url:     event.url,
			cookies: cloneTransportFields(event.cookies),
			request: cloneTransportRequest(event.request),
		}
	}
	return events
}

func TestBrave_SearchMatchesFrozenFixtures(t *testing.T) {
	testHTMLTextEngineFixtures(t, "brave", func(client htmlTextTransport) Searcher {
		return NewBrave(client)
	})
}

func TestGoogle_SearchMatchesFrozenFixtures(t *testing.T) {
	userAgent := frozenGoogleUserAgent(t)
	testHTMLTextEngineFixtures(t, "google", func(client htmlTextTransport) Searcher {
		return newGoogleWithUserAgent(client, userAgent)
	})
}

func TestMojeek_SearchMatchesFrozenFixtures(t *testing.T) {
	testHTMLTextEngineFixtures(t, "mojeek", func(client htmlTextTransport) Searcher {
		return NewMojeek(client)
	})
}

func TestGoogleUserAgent_MatchesFrozenFixture(t *testing.T) {
	var fixture struct {
		Input struct {
			DeviceIndex  int   `json:"device_index"`
			RandomValues []int `json:"random_values"`
		} `json:"input"`
		Result struct {
			Output string `json:"output"`
		} `json:"result"`
	}
	loadHTMLTextFixture(t, "../../testdata/contracts/pure/pure.google-module-lifetime-user-agent-shape.json", &fixture)
	if len(fixture.Input.RandomValues) != 3 {
		t.Fatalf("random value count = %d, want 3", len(fixture.Input.RandomValues))
	}
	if got := googleUserAgentFromValues(fixture.Input.DeviceIndex, fixture.Input.RandomValues[0], fixture.Input.RandomValues[1], fixture.Input.RandomValues[2]); got != fixture.Result.Output {
		t.Fatalf("Google user-agent = %q, want %q", got, fixture.Result.Output)
	}
}

func TestSourceFields_PreservesFrozenPythonDictionaryReplacementOrder(t *testing.T) {
	got := sourceFields(
		transport.Field{Name: "safesearch", Value: "safesearch"},
		transport.Field{Name: "useLocation", Value: "0"},
		transport.Field{Name: "safesearch", Value: "strict"},
	)
	want := []transport.Field{
		{Name: "safesearch", Value: "strict"},
		{Name: "useLocation", Value: "0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sourceFields = %#v, want %#v", got, want)
	}
}

func TestNewGoogle_ReusesOneProcessLifetimeUserAgent(t *testing.T) {
	first := &scriptedHTMLTextTransport{}
	second := &scriptedHTMLTextTransport{}
	firstAdapter, err := NewGoogle(first)
	if err != nil {
		t.Fatalf("NewGoogle(first): %v", err)
	}
	secondAdapter, err := NewGoogle(second)
	if err != nil {
		t.Fatalf("NewGoogle(second): %v", err)
	}
	if firstAdapter.userAgent != secondAdapter.userAgent {
		t.Fatalf("Google user-agents differ: %q != %q", firstAdapter.userAgent, secondAdapter.userAgent)
	}
	if !googleUserAgentPattern.MatchString(firstAdapter.userAgent) {
		t.Fatalf("Google user-agent = %q, want frozen source shape", firstAdapter.userAgent)
	}
	for _, client := range []*scriptedHTMLTextTransport{first, second} {
		events := client.Events()
		if len(events) != 1 || events[0].kind != "headers_update" {
			t.Fatalf("constructor events = %#v, want one header update", events)
		}
		if !matchesStringFields(events[0].headers, map[string]string{"User-Agent": firstAdapter.userAgent}) {
			t.Fatalf("constructor headers = %#v, want User-Agent %q", events[0].headers, firstAdapter.userAgent)
		}
	}
}

func TestNewGoogle_ConcurrentConstructionReusesProcessLifetimeUserAgent(t *testing.T) {
	const constructors = 32

	userAgents := make(chan string, constructors)
	errorsByConstructor := make(chan error, constructors)
	var group sync.WaitGroup
	for range constructors {
		group.Add(1)
		go func() {
			defer group.Done()
			adapter, err := NewGoogle(&scriptedHTMLTextTransport{})
			if err != nil {
				errorsByConstructor <- err
				return
			}
			userAgents <- adapter.userAgent
		}()
	}
	group.Wait()
	close(errorsByConstructor)
	close(userAgents)

	for err := range errorsByConstructor {
		t.Errorf("NewGoogle: %v", err)
	}
	var first string
	for userAgent := range userAgents {
		if first == "" {
			first = userAgent
			continue
		}
		if userAgent != first {
			t.Fatalf("Google user-agents differ: %q != %q", userAgent, first)
		}
	}
	if first == "" {
		t.Fatal("NewGoogle returned no user-agent")
	}
}

func TestHTMLTextAdapters_SearchHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	userAgent := frozenGoogleUserAgent(t)

	for name, adapter := range map[string]Searcher{
		"brave":  NewBrave(&scriptedHTMLTextTransport{}),
		"google": newGoogleWithUserAgent(&scriptedHTMLTextTransport{}, userAgent),
		"mojeek": NewMojeek(&scriptedHTMLTextTransport{}),
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

func TestHTMLTextAdapters_SearchPropagatesTransportError(t *testing.T) {
	sourceError := errors.New("fixture transport failure")
	userAgent := frozenGoogleUserAgent(t)
	for name, adapter := range map[string]Searcher{
		"brave":  NewBrave(&scriptedHTMLTextTransport{err: sourceError}),
		"google": newGoogleWithUserAgent(&scriptedHTMLTextTransport{err: sourceError}, userAgent),
		"mojeek": NewMojeek(&scriptedHTMLTextTransport{err: sourceError}),
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

func TestHTMLTextAdapters_SearchIsConcurrentSafe(t *testing.T) {
	userAgent := frozenGoogleUserAgent(t)
	tests := []struct {
		name       string
		fixture    string
		newAdapter func(htmlTextTransport) Searcher
	}{
		{
			name:    "brave",
			fixture: "../../testdata/contracts/engine/engine.text.brave-happy-html.json",
			newAdapter: func(client htmlTextTransport) Searcher {
				return NewBrave(client)
			},
		},
		{
			name:    "google",
			fixture: "../../testdata/contracts/engine/engine.text.google-consent-and-redirect-filter.json",
			newAdapter: func(client htmlTextTransport) Searcher {
				return newGoogleWithUserAgent(client, userAgent)
			},
		},
		{
			name:    "mojeek",
			fixture: "../../testdata/contracts/engine/engine.text.mojeek-happy-html.json",
			newAdapter: func(client htmlTextTransport) Searcher {
				return NewMojeek(client)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := loadHTMLTextEngineFixture(t, test.fixture)
			client := routingHTMLTextTransportFromFixture(t, fixture)
			adapter := test.newAdapter(client)

			const calls = 32
			errorsByCall := make(chan error, calls)
			var group sync.WaitGroup
			for range calls {
				group.Add(1)
				go func() {
					defer group.Done()
					results, err := adapter.Search(context.Background(), htmlTextSearchRequest(fixture))
					if err != nil {
						errorsByCall <- err
						return
					}
					if len(results) != 1 {
						errorsByCall <- errors.New("unexpected HTML text result count")
					}
				}()
			}
			group.Wait()
			close(errorsByCall)
			for err := range errorsByCall {
				t.Errorf("concurrent Search: %v", err)
			}
			if got, want := client.RequestCount(), calls; got != want {
				t.Fatalf("request count = %d, want %d", got, want)
			}
		})
	}
}

func testHTMLTextEngineFixtures(t *testing.T, engineName string, newAdapter func(htmlTextTransport) Searcher) {
	testHTMLTextEngineFixturesWithFixture(t, engineName, func(_ *testing.T, client htmlTextTransport, _ htmlTextEngineFixture) Searcher {
		return newAdapter(client)
	})
}

func testHTMLTextEngineFixturesWithFixture(
	t *testing.T,
	engineName string,
	newAdapter func(*testing.T, htmlTextTransport, htmlTextEngineFixture) Searcher,
) {
	t.Helper()
	paths, err := filepath.Glob("../../testdata/contracts/engine/engine.text." + engineName + "-*.json")
	if err != nil {
		t.Fatalf("find %s fixtures: %v", engineName, err)
	}
	if len(paths) == 0 {
		t.Fatalf("no %s fixtures", engineName)
	}

	for _, path := range paths {
		fixture := loadHTMLTextEngineFixture(t, path)
		if fixture.Contract.Operation != "search" {
			continue
		}
		t.Run(fixture.FixtureID, func(t *testing.T) {
			client := scriptedHTMLTextTransportFromFixture(t, fixture)
			results, err := newAdapter(t, client, fixture).Search(t.Context(), htmlTextSearchRequest(fixture))
			assertHTMLTextEngineOutcome(t, results, err, fixture)
			assertHTMLTextEngineEvents(t, client.Events(), fixture)
		})
	}
}

func loadHTMLTextEngineFixture(t testing.TB, path string) htmlTextEngineFixture {
	t.Helper()
	var fixture htmlTextEngineFixture
	loadHTMLTextFixture(t, path, &fixture)
	return fixture
}

func loadHTMLTextFixture(t testing.TB, path string, target any) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func frozenGoogleUserAgent(t testing.TB) string {
	t.Helper()
	var fixture struct {
		Result struct {
			Output string `json:"output"`
		} `json:"result"`
	}
	loadHTMLTextFixture(t, "../../testdata/contracts/pure/pure.google-module-lifetime-user-agent-shape.json", &fixture)
	return fixture.Result.Output
}

func scriptedHTMLTextTransportFromFixture(t testing.TB, fixture htmlTextEngineFixture) *scriptedHTMLTextTransport {
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
	return &scriptedHTMLTextTransport{responses: responses}
}

type routingHTMLTextTransport struct {
	mu       sync.Mutex
	response transport.Response
	requests int
}

func routingHTMLTextTransportFromFixture(t testing.TB, fixture htmlTextEngineFixture) *routingHTMLTextTransport {
	t.Helper()
	client := scriptedHTMLTextTransportFromFixture(t, fixture)
	if len(client.responses) != 1 {
		t.Fatalf("fixture %s response count = %d, want 1", fixture.FixtureID, len(client.responses))
	}
	return &routingHTMLTextTransport{response: client.responses[0]}
}

func (client *routingHTMLTextTransport) UpdateHeaders([]transport.Field) {}

func (client *routingHTMLTextTransport) SetCookies(string, []transport.Field) error {
	return nil
}

func (client *routingHTMLTextTransport) Do(_ context.Context, request transport.Request) (transport.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if request.Method != "GET" {
		return transport.Response{}, errors.New("unexpected fixture transport method")
	}
	client.requests++
	response := client.response
	response.Content = append([]byte(nil), response.Content...)
	return response, nil
}

func (client *routingHTMLTextTransport) RequestCount() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.requests
}

func htmlTextSearchRequest(fixture htmlTextEngineFixture) SearchRequest {
	return htmlTextSearchRequestFromInput(fixture.Input)
}

func htmlTextSearchRequestFromInput(input htmlTextSearchInput) SearchRequest {
	return SearchRequest{
		Query:      input.Query,
		Region:     input.Region,
		SafeSearch: input.SafeSearch,
		TimeLimit:  input.TimeLimit,
		Page:       input.Page,
	}
}

func assertHTMLTextEngineOutcome(t testing.TB, results []Result, err error, fixture htmlTextEngineFixture) {
	t.Helper()
	if fixture.Result.Status == "error" {
		if err == nil {
			t.Fatalf("Search error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
		}
		if err.Error() != fixture.Result.Error.Message {
			t.Fatalf("Search error = %q, want %q", err, fixture.Result.Error.Message)
		}
		var sourceError *sourceEngineError
		if !errors.As(err, &sourceError) || sourceError.sourceType != fixture.Result.Error.Type {
			t.Fatalf("Search error = %#v, want source type %q", sourceError, fixture.Result.Error.Type)
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

func decodeHTMLTextResults(t testing.TB, fixtureID string, output json.RawMessage) []map[string]any {
	t.Helper()
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var container struct {
			Results json.RawMessage `json:"results"`
		}
		if err := json.Unmarshal(trimmed, &container); err != nil {
			t.Fatalf("decode %s output container: %v", fixtureID, err)
		}
		trimmed = container.Results
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var results []map[string]any
	if err := decoder.Decode(&results); err != nil {
		t.Fatalf("decode %s output: %v", fixtureID, err)
	}
	return results
}

func assertHTMLTextEngineEvents(t testing.TB, actual []htmlTextTransportEvent, fixture htmlTextEngineFixture) {
	t.Helper()
	expected := make([]htmlTextEngineTrace, 0)
	for _, event := range fixture.Trace {
		if event.Kind == "note" || event.Kind == "cookie" || event.Kind == "request" {
			expected = append(expected, event)
		}
	}
	if len(actual) != len(expected) {
		t.Fatalf("event count = %d, want %d", len(actual), len(expected))
	}
	for index, expectedEvent := range expected {
		actualEvent := actual[index]
		switch expectedEvent.Kind {
		case "note":
			if actualEvent.kind != "headers_update" || expectedEvent.Note != "headers_update" {
				t.Fatalf("event %d = %#v, want headers_update", index, actualEvent)
			}
			wantHeaders := expectedEvent.Headers
			if marker, ok := wantHeaders["User-Agent"]; ok && marker == "<source-module-lifetime-random-google>" {
				wantHeaders = map[string]string{"User-Agent": frozenGoogleUserAgent(t)}
			}
			if !matchesStringFields(actualEvent.headers, wantHeaders) {
				t.Fatalf("event %d headers = %#v, want %#v", index, actualEvent.headers, wantHeaders)
			}
		case "cookie":
			if actualEvent.kind != "cookie" || actualEvent.url != expectedEvent.URL {
				t.Fatalf("event %d cookie = %#v, want url %q", index, actualEvent, expectedEvent.URL)
			}
			wantCookies := orderedFixtureFields(t, fixture.FixtureID, expectedEvent.Cookies, expectedEvent.CookieOrder)
			if !reflect.DeepEqual(actualEvent.cookies, wantCookies) {
				t.Fatalf("event %d cookies = %#v, want %#v", index, actualEvent.cookies, wantCookies)
			}
		case "request":
			if actualEvent.kind != "request" || actualEvent.request.Method != expectedEvent.Method || actualEvent.request.URL != expectedEvent.URL {
				t.Fatalf("event %d request = %#v, want %s %s", index, actualEvent, expectedEvent.Method, expectedEvent.URL)
			}
			wantQuery := orderedFixtureFields(t, fixture.FixtureID, expectedEvent.Query, expectedEvent.QueryOrder)
			if !reflect.DeepEqual(actualEvent.request.Query, wantQuery) {
				t.Fatalf("event %d query = %#v, want %#v", index, actualEvent.request.Query, wantQuery)
			}
			wantForm := orderedFixtureFields(t, fixture.FixtureID, expectedEvent.Form, expectedEvent.FormOrder)
			if !reflect.DeepEqual(actualEvent.request.Form, wantForm) {
				t.Fatalf("event %d form = %#v, want %#v", index, actualEvent.request.Form, wantForm)
			}
			if len(actualEvent.request.Headers) != 0 || len(actualEvent.request.Cookies) != 0 {
				t.Fatalf("event %d direct request headers/cookies = %#v, want none", index, actualEvent.request)
			}
		default:
			t.Fatalf("fixture %s unexpected expected event %q", fixture.FixtureID, expectedEvent.Kind)
		}
	}
}

func matchesStringFields(fields []transport.Field, want map[string]string) bool {
	if len(fields) != len(want) {
		return false
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if value, ok := want[field.Name]; !ok || value != field.Value {
			return false
		}
		if _, exists := seen[field.Name]; exists {
			return false
		}
		seen[field.Name] = struct{}{}
	}
	return true
}

var googleUserAgentPattern = regexp.MustCompile(`^Mozilla/5\.0 \(Linux; Android (5\.0|6\.0|8\.0); .+\) AppleWebKit/537\.36 \(KHTML, like Gecko\) Chrome/(?:39|[4-5][0-9]|60)\.0\.[0-9]{4}\.[0-9]{4} Mobile Safari/537\.36NSTNWV$`)
