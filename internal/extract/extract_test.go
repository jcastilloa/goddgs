package extract

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jcastillo/goddgs/internal/transport"
)

type extractFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Format             string `json:"fmt"`
		ResponseContentHex string `json:"response_content_hex"`
		Status             int    `json:"status"`
		URL                string `json:"url"`
	} `json:"input"`
	Result struct {
		Status string `json:"status"`
		Output struct {
			Content     string `json:"content"`
			ContentHex  string `json:"content_hex"`
			ContentKind string `json:"content_kind"`
			URL         string `json:"url"`
		} `json:"output"`
		Error struct {
			CauseType *string `json:"cause_type"`
			Message   string  `json:"message"`
			Type      string  `json:"type"`
		} `json:"error"`
	} `json:"result"`
}

func TestExtractor_MatchesFrozenFixtures(t *testing.T) {
	paths, err := fixturePaths()
	if err != nil {
		t.Fatalf("find extract fixtures: %v", err)
	}
	for _, path := range paths {
		fixture := loadExtractFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			response := extractFixtureResponse(t, fixture)
			fetcher := &recordingFetcher{response: response}
			calls := &rendererCalls{}
			renderer := fixedRenderer{
				Rendered: Rendered{
					Markdown: "# Fixture heading\n\nHello [link][1].\n* One\n* Two\n\n[1]: https://target.example/path\n",
					Plain:    "Fixture heading\n\nHello link.\nOne\nTwo\n",
					Rich:     "# Fixture heading\n\nHello link.\n* One\n* Two\n",
				},
				calls: calls,
			}
			request := Request{Method: http.MethodGet, URL: fixture.Input.URL, Format: fixture.Input.Format}
			result, err := New(fetcher, renderer).Extract(t.Context(), request)
			if actual := fetcher.requests(); !reflect.DeepEqual(actual, []Request{request}) {
				t.Fatalf("fetch requests = %#v, want %#v", actual, []Request{request})
			}
			if fixture.Result.Status == "error" {
				assertExtractError(t, err, fixture.Result.Error.Type, fixture.Result.Error.Message, fixture.Result.Error.CauseType)
				if got := calls.len(); got != 0 {
					t.Fatalf("renderer calls = %d, want 0", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			assertExtractResult(t, result, fixture)
			wantRender := fixture.Input.Format != "content" && fixture.Input.Format != "text"
			if got := calls.len(); got != boolToInt(wantRender) {
				t.Fatalf("renderer calls = %d, want %d", got, boolToInt(wantRender))
			}
		})
	}
}

func TestExtractor_HonorsCancellationBeforeFetch(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	fetcher := &recordingFetcher{}
	_, err := New(fetcher, fixedRenderer{}).Extract(ctx, Request{Method: http.MethodGet, URL: "https://extract.fixture/canceled", Format: "content"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Extract error = %v, want context.Canceled", err)
	}
	if got := len(fetcher.requests()); got != 0 {
		t.Fatalf("fetch calls = %d, want 0", got)
	}
}

func TestExtractor_ForwardsFrozenTransportConfiguration(t *testing.T) {
	fixture := loadExtractConstructorFixture(t, "../../testdata/contracts/pure/pure.extract-constructor-forwarding.json")
	for _, testCase := range fixture.Input.Cases {
		t.Run(testCase.Name, func(t *testing.T) {
			fetcher := &recordingFetcher{response: transport.Response{StatusCode: 200, Content: []byte("raw"), Text: "raw"}}
			config := extractTransportConfig(testCase)
			_, err := New(fetcher, fixedRenderer{}).Extract(t.Context(), Request{
				Method: http.MethodGet, URL: "https://extract.fixture/page", Format: "text", Config: config,
			})
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			requests := fetcher.requests()
			if len(requests) != 1 {
				t.Fatalf("fetch requests = %d, want 1", len(requests))
			}
			if !reflect.DeepEqual(requests[0].Config, config) {
				t.Fatalf("config = %#v, want %#v", requests[0].Config, config)
			}
		})
	}
}

func TestExtractor_PropagatesFetcherError(t *testing.T) {
	fetchErr := errors.New("fixture fetch failed")
	fetcher := &recordingFetcher{err: fetchErr}
	request := Request{Method: http.MethodGet, URL: "https://extract.fixture/error", Format: "content"}
	_, err := New(fetcher, fixedRenderer{}).Extract(t.Context(), request)
	if !errors.Is(err, fetchErr) {
		t.Fatalf("Extract error = %v, want fetch error", err)
	}
	if actual := fetcher.requests(); !reflect.DeepEqual(actual, []Request{request}) {
		t.Fatalf("fetch requests = %#v, want %#v", actual, []Request{request})
	}
}

type recordingFetcher struct {
	mu       sync.Mutex
	response transport.Response
	err      error
	recorded []Request
}

func (f *recordingFetcher) Fetch(ctx context.Context, request Request) (transport.Response, error) {
	if err := ctx.Err(); err != nil {
		return transport.Response{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorded = append(f.recorded, request)
	return f.response, f.err
}

func (f *recordingFetcher) requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.recorded...)
}

type fixedRenderer struct {
	Rendered
	err   error
	calls *rendererCalls
}

func (r fixedRenderer) Render(ctx context.Context, text string) (Rendered, error) {
	if err := ctx.Err(); err != nil {
		return Rendered{}, err
	}
	if r.calls != nil {
		r.calls.add(text)
	}
	return r.Rendered, r.err
}

type rendererCalls struct {
	mu    sync.Mutex
	texts []string
}

func (calls *rendererCalls) add(text string) {
	calls.mu.Lock()
	defer calls.mu.Unlock()
	calls.texts = append(calls.texts, text)
}

func (calls *rendererCalls) len() int {
	calls.mu.Lock()
	defer calls.mu.Unlock()
	return len(calls.texts)
}

func fixturePaths() ([]string, error) {
	return filepath.Glob("../../testdata/contracts/extract/*.json")
}

func loadExtractFixture(t *testing.T, path string) extractFixture {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture extractFixture
	if err := json.NewDecoder(bytes.NewReader(contents)).Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func extractFixtureResponse(t *testing.T, fixture extractFixture) transport.Response {
	t.Helper()
	contentHex := fixture.Input.ResponseContentHex
	if contentHex == "" && fixture.Result.Status == "ok" && fixture.Result.Output.ContentHex != "" {
		contentHex = fixture.Result.Output.ContentHex
	}
	content, err := hex.DecodeString(contentHex)
	if err != nil {
		t.Fatalf("decode content hex: %v", err)
	}
	return transport.Response{
		StatusCode: extractFixtureStatus(fixture.Input.Status),
		Content:    content,
		Text:       strings.ToValidUTF8(string(content), "\ufffd"),
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func extractFixtureStatus(status int) int {
	if status == 0 {
		return 200
	}
	return status
}

func assertExtractResult(t *testing.T, result Result, fixture extractFixture) {
	t.Helper()
	if result.URL != fixture.Result.Output.URL {
		t.Fatalf("URL = %q, want %q", result.URL, fixture.Result.Output.URL)
	}
	switch fixture.Result.Output.ContentKind {
	case "bytes":
		content, ok := result.Content.([]byte)
		if !ok {
			t.Fatalf("Content = %T, want []byte", result.Content)
		}
		want, err := hex.DecodeString(fixture.Result.Output.ContentHex)
		if err != nil {
			t.Fatalf("decode expected bytes: %v", err)
		}
		if !bytes.Equal(content, want) {
			t.Fatalf("Content = %x, want %x", content, want)
		}
	case "string":
		if content, ok := result.Content.(string); !ok || content != fixture.Result.Output.Content {
			t.Fatalf("Content = %#v, want %q", result.Content, fixture.Result.Output.Content)
		}
	default:
		t.Fatalf("unsupported fixture content kind %q", fixture.Result.Output.ContentKind)
	}
}

func assertExtractError(t *testing.T, err error, wantType, wantMessage string, wantCauseType *string) {
	t.Helper()
	if err == nil || err.Error() != wantMessage {
		t.Fatalf("Extract error = %v, want %q", err, wantMessage)
	}
	if wantType != "DDGSException" || wantCauseType != nil {
		t.Fatalf("unexpected frozen error shape: %s cause=%v", wantType, wantCauseType)
	}
}

type extractConstructorFixture struct {
	Input struct {
		Cases []struct {
			Name    string  `json:"name"`
			Proxy   *string `json:"proxy"`
			Timeout *int    `json:"timeout"`
			Verify  any     `json:"verify"`
		} `json:"cases"`
	} `json:"input"`
}

func loadExtractConstructorFixture(t *testing.T, path string) extractConstructorFixture {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture extractConstructorFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func extractTransportConfig(testCase struct {
	Name    string  `json:"name"`
	Proxy   *string `json:"proxy"`
	Timeout *int    `json:"timeout"`
	Verify  any     `json:"verify"`
}) transport.Config {
	config := transport.Config{Proxy: testCase.Proxy}
	if testCase.Timeout == nil {
		config.Timeout = transport.WithoutTimeout()
	} else {
		config.Timeout = transport.WithTimeout(time.Duration(*testCase.Timeout) * time.Second)
	}
	switch verify := testCase.Verify.(type) {
	case bool:
		if verify {
			config.Verification = transport.VerifyCertificates()
		} else {
			config.Verification = transport.SkipCertificateVerification()
		}
	case string:
		config.Verification = transport.VerifyWithPEMFile(verify)
	}
	return config
}
