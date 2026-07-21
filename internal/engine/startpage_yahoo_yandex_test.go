package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/jcastillo/goddgs/internal/transport"
)

type repeatedHTMLTextEngineFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Calls []htmlTextSearchInput `json:"calls"`
	} `json:"input"`
	Result struct {
		Output json.RawMessage `json:"output"`
	} `json:"result"`
	Trace []htmlTextEngineTrace `json:"trace"`
}

type fixtureRandomValue struct {
	Function string          `json:"function"`
	NBytes   int             `json:"nbytes"`
	Lower    int             `json:"lower"`
	Upper    int             `json:"upper"`
	Value    json.RawMessage `json:"value"`
}

func TestStartpage_SearchMatchesFrozenFixtures(t *testing.T) {
	testHTMLTextEngineFixtures(t, "startpage", func(client htmlTextTransport) Searcher {
		return NewStartpage(client)
	})
}

func TestYahoo_SearchMatchesFrozenFixtures(t *testing.T) {
	testHTMLTextEngineFixturesWithFixture(t, "yahoo", func(t *testing.T, client htmlTextTransport, fixture htmlTextEngineFixture) Searcher {
		tokenURLSafe, assertUsed := fixtureYahooTokenURLSafe(t, fixture.Trace)
		t.Cleanup(assertUsed)
		return newYahooWithTokenURLSafe(client, tokenURLSafe)
	})
}

func TestYandex_SearchMatchesFrozenFixtures(t *testing.T) {
	testHTMLTextEngineFixturesWithFixture(t, "yandex", func(t *testing.T, client htmlTextTransport, fixture htmlTextEngineFixture) Searcher {
		randomRange, assertUsed := fixtureYandexRandomRange(t, fixture.Trace)
		t.Cleanup(assertUsed)
		return newYandexWithRandomRange(client, randomRange)
	})
}

func TestStartpage_BootstrapsSCForEverySearch(t *testing.T) {
	fixture := loadRepeatedHTMLTextEngineFixture(t, "../../testdata/contracts/engine/engine.text.startpage-sc-bootstrap-per-payload.json")
	client := scriptedHTMLTextTransportFromTrace(t, fixture.FixtureID, fixture.Trace)
	adapter := NewStartpage(client)

	results := runRepeatedHTMLTextSearches(t, adapter, fixture.Input.Calls)
	assertRepeatedEmptyResults(t, results, fixture.Result.Output)
	assertHTMLTextEngineEvents(t, client.Events(), htmlTextEngineFixture{FixtureID: fixture.FixtureID, Trace: fixture.Trace})
}

func TestYahoo_BuildsRandomPathForEverySearch(t *testing.T) {
	fixture := loadRepeatedHTMLTextEngineFixture(t, "../../testdata/contracts/engine/engine.text.yahoo-random-path-per-search.json")
	client := scriptedHTMLTextTransportFromTrace(t, fixture.FixtureID, fixture.Trace)
	tokenURLSafe, assertUsed := fixtureYahooTokenURLSafe(t, fixture.Trace)
	adapter := newYahooWithTokenURLSafe(client, tokenURLSafe)

	results := runRepeatedHTMLTextSearches(t, adapter, fixture.Input.Calls)
	assertUsed()
	assertRepeatedEmptyResults(t, results, fixture.Result.Output)
	assertHTMLTextEngineEvents(t, client.Events(), htmlTextEngineFixture{FixtureID: fixture.FixtureID, Trace: fixture.Trace})
}

func TestYandex_BuildsSearchIDForEverySearch(t *testing.T) {
	fixture := loadRepeatedHTMLTextEngineFixture(t, "../../testdata/contracts/engine/engine.text.yandex-searchid-per-search.json")
	client := scriptedHTMLTextTransportFromTrace(t, fixture.FixtureID, fixture.Trace)
	randomRange, assertUsed := fixtureYandexRandomRange(t, fixture.Trace)
	adapter := newYandexWithRandomRange(client, randomRange)

	results := runRepeatedHTMLTextSearches(t, adapter, fixture.Input.Calls)
	assertUsed()
	assertRepeatedEmptyResults(t, results, fixture.Result.Output)
	assertHTMLTextEngineEvents(t, client.Events(), htmlTextEngineFixture{FixtureID: fixture.FixtureID, Trace: fixture.Trace})
}

func TestStartpageYahooYandex_HonorCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for name, adapter := range map[string]Searcher{
		"startpage": NewStartpage(&scriptedHTMLTextTransport{}),
		"yahoo":     newYahooWithTokenURLSafe(&scriptedHTMLTextTransport{}, func(int) (string, error) { return "token", nil }),
		"yandex":    newYandexWithRandomRange(&scriptedHTMLTextTransport{}, func(int, int) (int, error) { return 1_000_000, nil }),
	} {
		t.Run(name, func(t *testing.T) {
			results, err := adapter.Search(ctx, SearchRequest{Query: "fixture", Region: "us-en", SafeSearch: "moderate", Page: 1})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Search error = %v, want context.Canceled", err)
			}
			if results != nil {
				t.Fatalf("Search results = %#v, want nil", results)
			}
		})
	}
}

func TestStartpageYahooYandex_SearchPropagatesTransportError(t *testing.T) {
	sourceError := errors.New("fixture transport failure")
	for name, adapter := range map[string]Searcher{
		"startpage": NewStartpage(&scriptedHTMLTextTransport{err: sourceError}),
		"yahoo":     newYahooWithTokenURLSafe(&scriptedHTMLTextTransport{err: sourceError}, func(int) (string, error) { return "token", nil }),
		"yandex":    newYandexWithRandomRange(&scriptedHTMLTextTransport{err: sourceError}, func(int, int) (int, error) { return 1_000_000, nil }),
	} {
		t.Run(name, func(t *testing.T) {
			results, err := adapter.Search(t.Context(), SearchRequest{Query: "fixture", Region: "us-en", SafeSearch: "moderate", Page: 1})
			if !errors.Is(err, sourceError) {
				t.Fatalf("Search error = %v, want wrapped source error", err)
			}
			if results != nil {
				t.Fatalf("Search results = %#v, want nil", results)
			}
		})
	}
}

func TestNewYahoo_UsesSourceTokenURLSafeShape(t *testing.T) {
	client := &scriptedHTMLTextTransport{responses: []transport.Response{{StatusCode: 200, Text: "<html><body></body></html>"}}}
	adapter := NewYahoo(client)
	if _, err := adapter.Search(t.Context(), SearchRequest{Query: "fixture", Page: 1}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	events := client.Events()
	if len(events) != 1 || events[0].kind != "request" {
		t.Fatalf("events = %#v, want one request", events)
	}
	parts := strings.Split(events[0].request.URL, ";")
	if len(parts) != 3 {
		t.Fatalf("Yahoo URL = %q, want source token path", events[0].request.URL)
	}
	for _, token := range []struct {
		prefix string
		length int
	}{
		{prefix: "_ylt=", length: 18},
		{prefix: "_ylu=", length: 35},
	} {
		encoded := strings.TrimPrefix(parts[len(parts)-2], token.prefix)
		if token.prefix == "_ylu=" {
			encoded = strings.TrimPrefix(parts[len(parts)-1], token.prefix)
		}
		if strings.ContainsRune(encoded, '=') {
			t.Fatalf("token %q contains base64 padding", encoded)
		}
		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil || len(decoded) != token.length {
			t.Fatalf("token %q decoded length = %d, error = %v; want %d", encoded, len(decoded), err, token.length)
		}
	}
}

func TestNewYandex_UsesSourceInclusiveSearchIDRange(t *testing.T) {
	client := &scriptedHTMLTextTransport{responses: []transport.Response{{StatusCode: 200, Text: "<html><body></body></html>"}}}
	adapter := NewYandex(client)
	if _, err := adapter.Search(t.Context(), SearchRequest{Query: "fixture", Page: 1}); err != nil {
		t.Fatalf("Search: %v", err)
	}

	events := client.Events()
	if len(events) != 1 || events[0].kind != "request" {
		t.Fatalf("events = %#v, want one request", events)
	}
	value, err := strconv.Atoi(fieldValue(events[0].request.Query, "searchid"))
	if err != nil || value < 1_000_000 || value > 9_999_999 {
		t.Fatalf("searchid = %q (%d, %v), want inclusive source range", fieldValue(events[0].request.Query, "searchid"), value, err)
	}
}

func TestStartpageYahooYandex_SearchIsConcurrentSafe(t *testing.T) {
	tests := []struct {
		name       string
		newAdapter func() Searcher
	}{
		{
			name: "startpage",
			newAdapter: func() Searcher {
				return NewStartpage(&startpageRoutingHTMLTextTransport{})
			},
		},
		{
			name: "yahoo",
			newAdapter: func() Searcher {
				return NewYahoo(&routingHTMLTextTransport{response: transport.Response{StatusCode: 200, Text: "<html><body></body></html>"}})
			},
		},
		{
			name: "yandex",
			newAdapter: func() Searcher {
				return NewYandex(&routingHTMLTextTransport{response: transport.Response{StatusCode: 200, Text: "<html><body></body></html>"}})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := test.newAdapter()
			const calls = 32
			errorsByCall := make(chan error, calls)
			var group sync.WaitGroup
			for range calls {
				group.Add(1)
				go func() {
					defer group.Done()
					results, err := adapter.Search(context.Background(), SearchRequest{Query: "fixture", Region: "us-en", SafeSearch: "moderate", Page: 1})
					if err != nil {
						errorsByCall <- err
						return
					}
					if results == nil || len(results) != 0 {
						errorsByCall <- fmt.Errorf("unexpected result %#v", results)
					}
				}()
			}
			group.Wait()
			close(errorsByCall)
			for err := range errorsByCall {
				t.Errorf("concurrent Search: %v", err)
			}
		})
	}
}

type startpageRoutingHTMLTextTransport struct {
	mu sync.Mutex
}

func (*startpageRoutingHTMLTextTransport) UpdateHeaders([]transport.Field) {}

func (*startpageRoutingHTMLTextTransport) SetCookies(string, []transport.Field) error {
	return nil
}

func (client *startpageRoutingHTMLTextTransport) Do(_ context.Context, request transport.Request) (transport.Response, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	switch {
	case request.Method == "GET" && request.URL == startpageHomeURL:
		return transport.Response{StatusCode: 200, Text: `<form id="search"><input name="sc" value="fixture-sc"></form>`}, nil
	case request.Method == "POST" && request.URL == startpageSearchURL:
		return transport.Response{StatusCode: 200, Text: "<html><body></body></html>"}, nil
	default:
		return transport.Response{}, fmt.Errorf("unexpected Startpage request %s %s", request.Method, request.URL)
	}
}

func fieldValue(fields []transport.Field, name string) string {
	for _, field := range fields {
		if field.Name == name {
			return field.Value
		}
	}
	return ""
}

func loadRepeatedHTMLTextEngineFixture(t testing.TB, path string) repeatedHTMLTextEngineFixture {
	t.Helper()
	var fixture repeatedHTMLTextEngineFixture
	loadHTMLTextFixture(t, path, &fixture)
	return fixture
}

func scriptedHTMLTextTransportFromTrace(t testing.TB, fixtureID string, trace []htmlTextEngineTrace) *scriptedHTMLTextTransport {
	t.Helper()
	return scriptedHTMLTextTransportFromFixture(t, htmlTextEngineFixture{FixtureID: fixtureID, Trace: trace})
}

func runRepeatedHTMLTextSearches(t testing.TB, adapter Searcher, calls []htmlTextSearchInput) [][]Result {
	t.Helper()
	results := make([][]Result, 0, len(calls))
	for index, input := range calls {
		result, err := adapter.Search(t.Context(), htmlTextSearchRequestFromInput(input))
		if err != nil {
			t.Fatalf("search %d: %v", index, err)
		}
		results = append(results, result)
	}
	return results
}

func assertRepeatedEmptyResults(t testing.TB, results [][]Result, raw json.RawMessage) {
	t.Helper()
	var expected struct {
		Results []json.RawMessage `json:"results"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&expected); err != nil {
		t.Fatalf("decode repeated output: %v", err)
	}
	if len(results) != len(expected.Results) {
		t.Fatalf("search result count = %d, want %d", len(results), len(expected.Results))
	}
	for index, expectedResult := range expected.Results {
		var want []map[string]any
		if err := json.Unmarshal(expectedResult, &want); err != nil {
			t.Fatalf("decode result %d: %v", index, err)
		}
		if want == nil {
			if results[index] != nil {
				t.Fatalf("search %d results = %#v, want nil", index, results[index])
			}
			continue
		}
		if results[index] == nil || len(results[index]) != len(want) {
			t.Fatalf("search %d results = %#v, want %#v", index, results[index], want)
		}
	}
}

func fixtureYahooTokenURLSafe(t testing.TB, trace []htmlTextEngineTrace) (func(int) (string, error), func()) {
	t.Helper()
	values := fixtureRandomValues(t, trace)
	index := 0
	return func(nbytes int) (string, error) {
			if index >= len(values) {
				return "", fmt.Errorf("unexpected token_urlsafe(%d)", nbytes)
			}
			expected := values[index]
			index++
			if expected.Function != "token_urlsafe" || expected.NBytes != nbytes {
				return "", fmt.Errorf("token_urlsafe(%d), want %s(%d)", nbytes, expected.Function, expected.NBytes)
			}
			var value string
			if err := json.Unmarshal(expected.Value, &value); err != nil {
				return "", fmt.Errorf("decode token_urlsafe value: %w", err)
			}
			return value, nil
		}, func() {
			if index != len(values) {
				t.Errorf("token_urlsafe calls = %d, want %d", index, len(values))
			}
		}
}

func fixtureYandexRandomRange(t testing.TB, trace []htmlTextEngineTrace) (func(int, int) (int, error), func()) {
	t.Helper()
	values := fixtureRandomValues(t, trace)
	index := 0
	return func(lower, upper int) (int, error) {
			if index >= len(values) {
				return 0, fmt.Errorf("unexpected randint(%d, %d)", lower, upper)
			}
			expected := values[index]
			index++
			if expected.Function != "randint" || expected.Lower != lower || expected.Upper != upper {
				return 0, fmt.Errorf("randint(%d, %d), want %s(%d, %d)", lower, upper, expected.Function, expected.Lower, expected.Upper)
			}
			var value int
			if err := json.Unmarshal(expected.Value, &value); err != nil {
				return 0, fmt.Errorf("decode randint value: %w", err)
			}
			return value, nil
		}, func() {
			if index != len(values) {
				t.Errorf("randint calls = %d, want %d", index, len(values))
			}
		}
}

func fixtureRandomValues(t testing.TB, trace []htmlTextEngineTrace) []fixtureRandomValue {
	t.Helper()
	values := make([]fixtureRandomValue, 0)
	for _, event := range trace {
		if event.Kind != "random" {
			continue
		}
		var value fixtureRandomValue
		if err := json.Unmarshal(event.Value, &value); err != nil {
			t.Fatalf("decode random fixture value: %v", err)
		}
		values = append(values, value)
	}
	return values
}

func TestFixtureRandomValuesPreserveTraceOrder(t *testing.T) {
	fixture := loadHTMLTextEngineFixture(t, "../../testdata/contracts/engine/engine.text.yahoo-random-path-and-redirect-unwrapping.json")
	values := fixtureRandomValues(t, fixture.Trace)
	want := []fixtureRandomValue{
		{Function: "token_urlsafe", NBytes: 18},
		{Function: "token_urlsafe", NBytes: 35},
	}
	for index, expected := range want {
		if values[index].Function != expected.Function || values[index].NBytes != expected.NBytes {
			t.Fatalf("random %d = %#v, want %#v", index, values[index], expected)
		}
	}
}
