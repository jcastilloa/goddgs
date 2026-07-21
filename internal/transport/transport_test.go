package transport

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const sourceHTTPClientDefaultTimeout = 10 * time.Second

type transportFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Constructor map[string]json.RawMessage `json:"constructor"`
		Case        string                     `json:"case"`
	} `json:"input"`
	Result struct {
		Output json.RawMessage `json:"output"`
		Status string          `json:"status"`
	} `json:"result"`
}

type expectedClientSettings struct {
	proxy       *string
	timeout     *time.Duration
	verify      bool
	pemFilePath string
}

func TestNewClient_MatchesFrozenConstructorFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/transport/transport.constructor-*.json")
	if err != nil {
		t.Fatalf("find transport fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no transport constructor fixtures")
	}

	for _, path := range paths {
		fixture := loadTransportFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			config, want := configFromFixture(t, fixture)
			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			if client == nil {
				t.Fatal("NewClient() = nil, want isolated transport client")
			}
			assertClientConfiguration(t, client, want)
		})
	}
}

func TestClient_DoMaterializesAndClosesNativeBody(t *testing.T) {
	var closes atomic.Int32
	client, err := newClient(Config{}, roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       closeCountingBody{Reader: bytes.NewReader([]byte("transport fixture bytes")), closes: &closes},
			Header:     make(http.Header),
		}, nil
	}))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}

	response, err := client.Do(t.Context(), Request{
		Method:  http.MethodPost,
		URL:     "https://transport.fixture/request",
		Query:   []Field{{Name: "q", Value: "needle"}},
		Cookies: []Field{{Name: "fixture_cookie", Value: "value"}},
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if closes.Load() != 1 {
		t.Fatalf("native response body closes = %d, want 1", closes.Load())
	}
	if got, want := response.StatusCode, http.StatusCreated; got != want {
		t.Fatalf("response status = %d, want %d", got, want)
	}
	if got, want := string(response.Content), "transport fixture bytes"; got != want {
		t.Fatalf("response content = %q, want %q", got, want)
	}
	if got, want := response.Text, "transport fixture bytes"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
}

func TestClient_DoPreservesReplacementDecodingForInvalidUTF8(t *testing.T) {
	client, err := newClient(Config{}, roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte{'a', 0xff, 'b'})),
			Header:     make(http.Header),
		}, nil
	}))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}

	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: "https://transport.fixture/invalid-utf8"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got, want := string(response.Content), "a\xffb"; got != want {
		t.Fatalf("response content = %q, want %q", got, want)
	}
	if got, want := response.Text, "a\ufffdb"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
}

func TestClient_DoClassifiesContextCancellation(t *testing.T) {
	client, err := newClient(Config{}, roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	}))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = client.Do(ctx, Request{Method: http.MethodGet, URL: "https://transport.fixture/cancel"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Do cancellation error = %v, want context.Canceled", err)
	}
}

func TestClient_DoClassifiesTimeout(t *testing.T) {
	client, err := newClient(Config{}, roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	}))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}

	_, err = client.Do(t.Context(), Request{Method: http.MethodGet, URL: "https://transport.fixture/timeout"})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("Do timeout error = %v, want errors.Is(err, ErrTimeout)", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Do timeout cause = %v, want context.DeadlineExceeded", err)
	}
}

func TestClient_DoPreservesGenericRoundTripCause(t *testing.T) {
	cause := errors.New("fixture transport failure")
	client, err := newClient(Config{}, roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, cause
	}))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}

	_, err = client.Do(t.Context(), Request{Method: http.MethodGet, URL: "https://transport.fixture/generic-error"})
	if !errors.Is(err, cause) {
		t.Fatalf("Do generic error = %v, want errors.Is(err, %v)", err, cause)
	}
}

func TestClient_DoCopiesRequestFieldsBeforeRoundTrip(t *testing.T) {
	query := []Field{{Name: "q", Value: "before"}}
	form := []Field{{Name: "data", Value: "before"}}
	headers := []Field{{Name: "X-Fixture", Value: "before"}}
	cookies := []Field{{Name: "fixture_cookie", Value: "before"}}
	entered := make(chan struct{})
	release := make(chan struct{})
	seen := make(chan struct {
		query  string
		form   string
		header string
		cookie string
	}, 1)
	client, err := newClient(Config{}, roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		close(entered)
		<-release
		if err := request.ParseForm(); err != nil {
			return nil, err
		}
		seen <- struct {
			query  string
			form   string
			header string
			cookie string
		}{
			query:  request.URL.Query().Get("q"),
			form:   request.PostForm.Get("data"),
			header: request.Header.Get("X-Fixture"),
			cookie: request.Header.Get("Cookie"),
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	}))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := client.Do(t.Context(), Request{
			Method:  http.MethodPost,
			URL:     "https://transport.fixture/copy",
			Query:   query,
			Form:    form,
			Headers: headers,
			Cookies: cookies,
		})
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("Do returned before RoundTrip received the request: %v", err)
	}
	query[0].Value = "after"
	form[0].Value = "after"
	headers[0].Value = "after"
	cookies[0].Value = "after"
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Do: %v", err)
	}
	got := <-seen
	want := struct {
		query  string
		form   string
		header string
		cookie string
	}{query: "before", form: "before", header: "before", cookie: "fixture_cookie=before"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip request = %#v, want original copied fields %#v", got, want)
	}
}

func TestClient_UpdateHeadersAndSetCookiesRemainClientLocal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, _ := request.Cookie("fixture_cookie")
		cookieValue := ""
		if cookie != nil {
			cookieValue = cookie.Value
		}
		_, _ = io.WriteString(writer, request.Header.Get("X-Source-Header")+"|"+cookieValue)
	}))
	defer server.Close()

	first, err := NewClient(Config{})
	if err != nil {
		t.Fatalf("NewClient first: %v", err)
	}
	second, err := NewClient(Config{})
	if err != nil {
		t.Fatalf("NewClient second: %v", err)
	}
	first.UpdateHeaders([]Field{{Name: "X-Source-Header", Value: "first"}})
	if err := first.SetCookies(server.URL, []Field{{Name: "fixture_cookie", Value: "first-cookie"}}); err != nil {
		t.Fatalf("SetCookies: %v", err)
	}

	if got, want := mustDo(t, first, server.URL).Text, "first|first-cookie"; got != want {
		t.Fatalf("first client state = %q, want %q", got, want)
	}
	if got, want := mustDo(t, second, server.URL).Text, "|"; got != want {
		t.Fatalf("second client state = %q, want %q", got, want)
	}
}

func TestClient_SetCookiesAcceptsSourceDomainWithoutScheme(t *testing.T) {
	client, err := NewClient(Config{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if err := client.SetCookies("google.com", []Field{{Name: "CONSENT", Value: "YES+"}}); err != nil {
		t.Fatalf("SetCookies domain: %v", err)
	}

	targetURL, err := url.Parse("https://google.com/search")
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}
	cookies := client.jar.Cookies(targetURL)
	if len(cookies) != 1 || cookies[0].Name != "CONSENT" || cookies[0].Value != "YES+" {
		t.Fatalf("domain cookies = %#v, want CONSENT=YES+", cookies)
	}
}

func TestClient_ConcurrentFirstRequestsAreRaceSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "concurrent fixture")
	}))
	defer server.Close()

	client, err := NewClient(Config{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	const callers = 32
	start := make(chan struct{})
	errorsByCaller := make(chan error, callers)
	for range callers {
		go func() {
			<-start
			response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: server.URL})
			if err == nil && response.Text != "concurrent fixture" {
				err = fmt.Errorf("response text = %q", response.Text)
			}
			errorsByCaller <- err
		}()
	}
	close(start)
	for range callers {
		if err := <-errorsByCaller; err != nil {
			t.Fatalf("concurrent Do: %v", err)
		}
	}
}

func TestClient_DoHonorsConfiguredTimeoutAgainstLoopback(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	serverDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(writer, "late response")
		close(serverDone)
	}))
	defer server.Close()

	client, err := NewClient(Config{Timeout: WithTimeout(20 * time.Millisecond)})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: server.URL})
		result <- err
	}()
	select {
	case <-started:
	case err := <-result:
		t.Fatalf("Do returned before loopback received the request: %v", err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("Do timeout error = %v, want ErrTimeout", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Do did not honor configured timeout")
	}
	close(release)
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("loopback handler did not finish after release")
	}
}

func TestConfig_ClonesCallerOwnedTimeout(t *testing.T) {
	timeout := time.Second
	client, err := NewClient(Config{Timeout: WithTimeout(timeout)})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	timeout = 0
	if got, want := client.settings.timeout, durationPointer(time.Second); !sameDurationPointer(got, want) {
		t.Fatalf("client timeout after caller mutation = %v, want %v", got, want)
	}
}

func TestClient_DoMatchesFrozenLoopbackCookieRedirectGzipAndStatusFixtures(t *testing.T) {
	server := newTransportLoopbackServer(t)
	client, err := NewClient(Config{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	t.Run("cookies and redirects", func(t *testing.T) {
		fixture := loadTransportFixture(t, "../../testdata/contracts/transport/transport.loopback-cookies-and-redirects.json")
		var want struct {
			Set      fixtureResponse `json:"set"`
			Cookie   fixtureResponse `json:"cookie"`
			Redirect fixtureResponse `json:"redirect"`
		}
		decodeFixtureOutput(t, fixture, &want)

		assertTransportResponse(t, mustDo(t, client, server.URL+"/set-cookie"), want.Set)
		assertTransportResponse(t, mustDo(t, client, server.URL+"/cookie-check"), want.Cookie)
		assertTransportResponse(t, mustDo(t, client, server.URL+"/redirect"), want.Redirect)
	})

	t.Run("gzip", func(t *testing.T) {
		fixture := loadTransportFixture(t, "../../testdata/contracts/transport/transport.loopback-gzip-decompression.json")
		var want fixtureResponse
		decodeFixtureOutput(t, fixture, &want)
		assertTransportResponse(t, mustDo(t, client, server.URL+"/gzip"), want)
	})

	t.Run("non-200 remains response", func(t *testing.T) {
		fixture := loadTransportFixture(t, "../../testdata/contracts/transport/transport.loopback-non-200-preserved.json")
		var want fixtureResponse
		decodeFixtureOutput(t, fixture, &want)
		assertTransportResponse(t, mustDo(t, client, server.URL+"/status"), want)
	})
}

func TestClient_DoHonorsTLSVerificationAndPEM(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, "tls fixture")
	}))
	defer server.Close()

	t.Run("default verification rejects unknown root", func(t *testing.T) {
		client, err := NewClient(Config{})
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		_, err = client.Do(t.Context(), Request{Method: http.MethodGet, URL: server.URL})
		if err == nil {
			t.Fatal("Do() error = nil, want certificate-verification failure")
		}
		if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "x509") {
			t.Fatalf("Do() error = %v, want certificate-verification failure", err)
		}
	})

	t.Run("verify false accepts fixture certificate", func(t *testing.T) {
		client, err := NewClient(Config{Verification: SkipCertificateVerification()})
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		response := mustDo(t, client, server.URL)
		if got, want := response.Text, "tls fixture"; got != want {
			t.Fatalf("response text = %q, want %q", got, want)
		}
	})

	t.Run("custom PEM accepts fixture certificate", func(t *testing.T) {
		certificate := server.Certificate()
		if certificate == nil {
			t.Fatal("TLS server certificate = nil")
		}
		pemPath := filepath.Join(t.TempDir(), "fixture-root.pem")
		pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
		if err := os.WriteFile(pemPath, pemBytes, 0o600); err != nil {
			t.Fatalf("write PEM: %v", err)
		}

		client, err := NewClient(Config{Verification: VerifyWithPEMFile(pemPath)})
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		response := mustDo(t, client, server.URL)
		if got, want := response.Text, "tls fixture"; got != want {
			t.Fatalf("response text = %q, want %q", got, want)
		}
	})
}

func TestClient_DoUsesHTTPAndHTTPSProxy(t *testing.T) {
	tests := []struct {
		name       string
		newServer  func(http.Handler) *httptest.Server
		config     func(*httptest.Server) Config
		wantScheme string
	}{
		{
			name:      "HTTP proxy",
			newServer: httptest.NewServer,
			config: func(server *httptest.Server) Config {
				return Config{Proxy: stringPointer(server.URL)}
			},
			wantScheme: "http",
		},
		{
			name:      "HTTPS proxy with explicit verification off",
			newServer: httptest.NewTLSServer,
			config: func(server *httptest.Server) Config {
				return Config{
					Proxy:        stringPointer(server.URL),
					Verification: SkipCertificateVerification(),
				}
			},
			wantScheme: "https",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seen := make(chan struct {
				method string
				url    string
			}, 1)
			server := test.newServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				seen <- struct {
					method string
					url    string
				}{method: request.Method, url: request.URL.String()}
				_, _ = io.WriteString(writer, "proxy fixture")
			}))
			defer server.Close()

			client, err := NewClient(test.config(server))
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			response := mustDo(t, client, "http://transport.fixture/probe?q=needle")
			if got, want := response.Text, "proxy fixture"; got != want {
				t.Fatalf("response text = %q, want %q", got, want)
			}
			select {
			case got := <-seen:
				if got.method != http.MethodGet {
					t.Fatalf("proxy request method = %q, want GET", got.method)
				}
				if got.url != "http://transport.fixture/probe?q=needle" {
					t.Fatalf("proxy request URL = %q, want absolute target URL", got.url)
				}
			case <-time.After(time.Second):
				t.Fatalf("%s proxy received no request", test.wantScheme)
			}
		})
	}
}

func TestClient_DoPreservesFrozenSOCKSResolutionMode(t *testing.T) {
	tests := []struct {
		fixturePath string
		scheme      string
		wantType    byte
		wantHost    string
	}{
		{
			fixturePath: "../../testdata/contracts/transport/transport.loopback-socks5-resolution.json",
			scheme:      "socks5",
			wantType:    socksAddressIPv4,
			wantHost:    "127.0.0.1",
		},
		{
			fixturePath: "../../testdata/contracts/transport/transport.loopback-socks5h-resolution.json",
			scheme:      "socks5h",
			wantType:    socksAddressDomain,
			wantHost:    "localhost",
		},
	}

	for _, test := range tests {
		t.Run(test.scheme, func(t *testing.T) {
			fixture := loadTransportFixture(t, test.fixturePath)
			proxy := newSOCKSFixtureServer(t)
			client, err := NewClient(Config{Proxy: stringPointer(test.scheme + "://" + proxy.Address())})
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			response := mustDo(t, client, "http://localhost:43210/probe")
			if got, want := response.Text, "socks fixture"; got != want {
				t.Fatalf("response text = %q, want %q", got, want)
			}

			got := proxy.Observed(t)
			if got.addressType != test.wantType || got.host != test.wantHost || got.port != 43210 {
				t.Fatalf("SOCKS CONNECT = %#v, want type=%d host=%q port=43210", got, test.wantType, test.wantHost)
			}
			if fixture.Result.Status != "ok" {
				t.Fatalf("fixture %s status = %q, want ok", fixture.FixtureID, fixture.Result.Status)
			}
		})
	}
}

type fixtureResponse struct {
	ContentHex string `json:"content_hex"`
	Status     int    `json:"status"`
	Text       string `json:"text"`
}

type closeCountingBody struct {
	io.Reader
	closes *atomic.Int32
}

func (b closeCountingBody) Close() error {
	b.closes.Add(1)
	return nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func loadTransportFixture(t testing.TB, path string) transportFixture {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture transportFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func configFromFixture(t testing.TB, fixture transportFixture) (Config, expectedClientSettings) {
	t.Helper()

	config := Config{}
	want := expectedClientSettings{timeout: durationPointer(sourceHTTPClientDefaultTimeout), verify: true}
	constructor := fixture.Input.Constructor
	if raw, ok := constructor["proxy"]; ok && !isJSONNull(raw) {
		proxy := decodeJSONValue[string](t, raw)
		config.Proxy = stringPointer(proxy)
		want.proxy = stringPointer(proxy)
	}
	if raw, ok := constructor["timeout"]; ok {
		if isJSONNull(raw) {
			config.Timeout = WithoutTimeout()
			want.timeout = nil
		} else {
			seconds := decodeJSONValue[float64](t, raw)
			timeout := time.Duration(seconds * float64(time.Second))
			config.Timeout = WithTimeout(timeout)
			want.timeout = durationPointer(timeout)
		}
	}
	if raw, ok := constructor["verify"]; ok {
		if isJSONNull(raw) {
			t.Fatal("source constructor verify must not be null")
		}
		var boolValue bool
		if err := json.Unmarshal(raw, &boolValue); err == nil {
			if boolValue {
				config.Verification = VerifyCertificates()
			} else {
				config.Verification = SkipCertificateVerification()
			}
			want.verify = boolValue
		} else {
			pemFile := decodeJSONValue[string](t, raw)
			config.Verification = VerifyWithPEMFile(pemFile)
			want.verify = true
			want.pemFilePath = pemFile
		}
	}

	return config, want
}

func assertClientConfiguration(t testing.TB, client *Client, want expectedClientSettings) {
	t.Helper()
	if !sameStringPointer(client.settings.proxy, want.proxy) {
		t.Fatalf("client proxy = %v, want %v", client.settings.proxy, want.proxy)
	}
	if !sameDurationPointer(client.settings.timeout, want.timeout) {
		t.Fatalf("client timeout = %v, want %v", client.settings.timeout, want.timeout)
	}
	if got := client.settings.verify; got != want.verify {
		t.Fatalf("client verify = %t, want %t", got, want.verify)
	}
	if got := client.settings.pemFilePath; got != want.pemFilePath {
		t.Fatalf("client PEM file = %q, want %q", got, want.pemFilePath)
	}
}

func decodeFixtureOutput(t testing.TB, fixture transportFixture, target any) {
	t.Helper()
	if err := json.Unmarshal(fixture.Result.Output, target); err != nil {
		t.Fatalf("decode fixture %s output: %v", fixture.FixtureID, err)
	}
}

func decodeJSONValue[T any](t testing.TB, raw json.RawMessage) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode fixture value %s: %v", raw, err)
	}
	return value
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func mustDo(t testing.TB, client *Client, targetURL string) Response {
	t.Helper()
	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: targetURL})
	if err != nil {
		t.Fatalf("Do(%s): %v", targetURL, err)
	}
	return response
}

func assertTransportResponse(t testing.TB, got Response, want fixtureResponse) {
	t.Helper()
	if got.StatusCode != want.Status {
		t.Fatalf("response status = %d, want %d", got.StatusCode, want.Status)
	}
	if got.Text != want.Text {
		t.Fatalf("response text = %q, want %q", got.Text, want.Text)
	}
	if want.ContentHex == "" {
		return
	}
	if gotHex := hexEncode(got.Content); gotHex != want.ContentHex {
		t.Fatalf("response content hex = %s, want %s", gotHex, want.ContentHex)
	}
}

func newTransportLoopbackServer(t testing.TB) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/set-cookie":
			http.SetCookie(writer, &http.Cookie{Name: "fixture_cookie", Value: "source", Path: "/"})
			_, _ = io.WriteString(writer, "cookie set")
		case "/cookie-check":
			cookie, err := request.Cookie("fixture_cookie")
			if err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(writer, cookie.Name+"="+cookie.Value)
		case "/redirect":
			http.Redirect(writer, request, "/target", http.StatusFound)
		case "/target":
			_, _ = io.WriteString(writer, "redirect target")
		case "/gzip":
			writer.Header().Set("Content-Encoding", "gzip")
			gzipWriter := gzip.NewWriter(writer)
			_, _ = io.WriteString(gzipWriter, "compressed fixture")
			_ = gzipWriter.Close()
		case "/status":
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(writer, "unavailable")
		default:
			http.NotFound(writer, request)
		}
	}))
}

const (
	socksVersion       = 5
	socksConnect       = 1
	socksAddressIPv4   = 1
	socksAddressDomain = 3
)

type socksObservation struct {
	addressType byte
	host        string
	port        uint16
	requestLine string
}

type socksFixtureServer struct {
	listener net.Listener
	observed chan socksObservation
	done     chan struct{}
}

func newSOCKSFixtureServer(t testing.TB) *socksFixtureServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen SOCKS fixture: %v", err)
	}
	server := &socksFixtureServer{
		listener: listener,
		observed: make(chan socksObservation, 1),
		done:     make(chan struct{}),
	}
	go func() {
		defer close(server.done)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		if err := serveSOCKSFixture(connection, server.observed); err != nil {
			return
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		select {
		case <-server.done:
		case <-time.After(time.Second):
			t.Error("SOCKS fixture server did not stop")
		}
	})
	return server
}

func (server *socksFixtureServer) Address() string {
	return server.listener.Addr().String()
}

func (server *socksFixtureServer) Observed(t testing.TB) socksObservation {
	t.Helper()
	select {
	case observed := <-server.observed:
		return observed
	case <-time.After(time.Second):
		t.Fatal("SOCKS fixture server did not observe CONNECT")
		return socksObservation{}
	}
}

func serveSOCKSFixture(connection net.Conn, observed chan<- socksObservation) error {
	reader := bufio.NewReader(connection)
	version, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if version != socksVersion {
		return fmt.Errorf("SOCKS greeting version = %d", version)
	}
	methodCount, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if _, err := io.ReadFull(reader, make([]byte, methodCount)); err != nil {
		return err
	}
	if _, err := connection.Write([]byte{socksVersion, 0}); err != nil {
		return err
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}
	if header[0] != socksVersion || header[1] != socksConnect || header[2] != 0 {
		return fmt.Errorf("invalid SOCKS CONNECT header %v", header)
	}
	observation := socksObservation{addressType: header[3]}
	switch observation.addressType {
	case socksAddressIPv4:
		address := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(reader, address); err != nil {
			return err
		}
		observation.host = net.IP(address).String()
	case socksAddressDomain:
		length, err := reader.ReadByte()
		if err != nil {
			return err
		}
		domain := make([]byte, length)
		if _, err := io.ReadFull(reader, domain); err != nil {
			return err
		}
		observation.host = string(domain)
	default:
		return fmt.Errorf("unexpected SOCKS address type %d", observation.addressType)
	}
	if err := binary.Read(reader, binary.BigEndian, &observation.port); err != nil {
		return err
	}
	if _, err := connection.Write([]byte{socksVersion, 0, 0, socksAddressIPv4, 0, 0, 0, 0, 0, 0}); err != nil {
		return err
	}

	request, err := http.ReadRequest(reader)
	if err != nil {
		return err
	}
	defer request.Body.Close()
	observation.requestLine = request.Method + " " + request.URL.RequestURI() + " " + request.Proto
	observed <- observation
	_, err = io.WriteString(connection, "HTTP/1.1 200 OK\r\nContent-Length: 13\r\nConnection: close\r\n\r\nsocks fixture")
	return err
}

func stringPointer(value string) *string {
	return &value
}

func durationPointer(value time.Duration) *time.Duration {
	return &value
}

func sameStringPointer(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func sameDurationPointer(left, right *time.Duration) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}

func hexEncode(value []byte) string {
	const hex = "0123456789abcdef"
	encoded := make([]byte, len(value)*2)
	for index, byteValue := range value {
		encoded[index*2] = hex[byteValue>>4]
		encoded[index*2+1] = hex[byteValue&0x0f]
	}
	return string(encoded)
}
