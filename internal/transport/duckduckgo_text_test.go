package transport

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDuckDuckGoTextClient_MatchesFrozenConstructorAndRequestFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/transport/transport.ddg-http2-constructor-*.json")
	if err != nil {
		t.Fatalf("find DuckDuckGo transport fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no DuckDuckGo transport constructor fixtures")
	}

	for _, path := range paths {
		fixture := loadTransportFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			config, want := configFromFixture(t, fixture)
			headers := headersFromFixture(t, fixture)
			var seen struct {
				form    string
				headers http.Header
				method  string
				url     string
			}
			client, err := newDuckDuckGoTextClient(config, headers, roundTripperFunc(func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					return nil, err
				}
				seen.form = string(body)
				seen.headers = request.Header.Clone()
				seen.method = request.Method
				seen.url = request.URL.String()
				return &http.Response{
					StatusCode: http.StatusMultiStatus,
					Body:       io.NopCloser(strings.NewReader("ddg transport fixture bytes")),
					Header:     make(http.Header),
				}, nil
			}))
			if err != nil {
				t.Fatalf("newDuckDuckGoTextClient: %v", err)
			}
			assertClientConfiguration(t, client.client, want)
			if native := client.client.nativeHTTPClient(); native == nil || native.CheckRedirect == nil {
				t.Fatal("DuckDuckGo client does not disable redirect following")
			}

			response, err := client.Do(t.Context(), Request{
				Method: fixture.Input.Method,
				URL:    "https://transport.fixture/ddg",
				Form:   []Field{{Name: "q", Value: "needle"}},
			})
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			var wantResponse fixtureResponse
			decodeFixtureOutput(t, fixture, &wantResponse)
			assertTransportResponse(t, response, wantResponse)
			if got, want := seen.method, fixture.Input.Method; got != want {
				t.Fatalf("request method = %q, want %q", got, want)
			}
			if got, want := seen.url, "https://transport.fixture/ddg"; got != want {
				t.Fatalf("request URL = %q, want %q", got, want)
			}
			if got, want := seen.form, "q=needle"; got != want {
				t.Fatalf("request form = %q, want %q", got, want)
			}
			if got, want := seen.headers.Get("Content-Type"), "application/x-www-form-urlencoded"; got != want {
				t.Fatalf("content type = %q, want %q", got, want)
			}
			for _, header := range headers {
				if got := seen.headers.Get(header.Name); got != header.Value {
					t.Fatalf("header %q = %q, want %q", header.Name, got, header.Value)
				}
			}
		})
	}
}

func TestDuckDuckGoTextClient_MatchesFrozenErrorClassificationFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/transport/transport.ddg-http2-*-error-classification.json")
	if err != nil {
		t.Fatalf("find DuckDuckGo error fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no DuckDuckGo error fixtures")
	}

	for _, path := range paths {
		fixture := loadTransportFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			cause := errors.New("fixture " + fixture.Input.Failure)
			if fixture.Input.Failure == "timeout" {
				cause = errors.New("fixture timed out")
			}
			client, err := newDuckDuckGoTextClient(Config{}, nil, roundTripperFunc(func(*http.Request) (*http.Response, error) {
				return nil, cause
			}))
			if err != nil {
				t.Fatalf("newDuckDuckGoTextClient: %v", err)
			}

			_, err = client.Do(t.Context(), Request{Method: fixture.Input.Method, URL: "https://transport.fixture/ddg"})
			if err == nil {
				t.Fatalf("Do error = nil, want %s", fixture.Result.Error.Type)
			}
			if !errors.Is(err, cause) {
				t.Fatalf("Do error = %v, want wrapped cause %v", err, cause)
			}
			if fixture.Result.Error.Type == "TimeoutException" && !errors.Is(err, ErrTimeout) {
				t.Fatalf("Do timeout error = %v, want errors.Is(err, ErrTimeout)", err)
			}
			if fixture.Result.Error.Type == "DDGSException" && errors.Is(err, ErrTimeout) {
				t.Fatalf("Do generic error = %v, must not classify as ErrTimeout", err)
			}
		})
	}
}

func TestDuckDuckGoTextClient_DoesNotFollowFrozenRedirect(t *testing.T) {
	fixture := loadTransportFixture(t, "../../testdata/contracts/transport/transport.ddg-http2-loopback-redirect-not-followed.json")
	var targetCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/redirect":
			writer.Header().Set("Location", "/target")
			writer.WriteHeader(http.StatusFound)
			_, _ = io.WriteString(writer, "redirect")
		case "/target":
			targetCalls++
			_, _ = io.WriteString(writer, "target")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client, err := NewDuckDuckGoTextClient(Config{}, nil)
	if err != nil {
		t.Fatalf("NewDuckDuckGoTextClient: %v", err)
	}
	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: server.URL + "/redirect"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	var want fixtureResponse
	decodeFixtureOutput(t, fixture, &want)
	assertTransportResponse(t, response, want)
	if targetCalls != 0 {
		t.Fatalf("redirect target calls = %d, want 0", targetCalls)
	}
}

func TestDuckDuckGoTextClient_UsesHTTP2AndKeepsHeadersRequestLocal(t *testing.T) {
	const callers = 24
	type observation struct {
		path  string
		proto int
		ua    string
	}
	observed := make(chan observation, callers)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observed <- observation{path: request.URL.Path, proto: request.ProtoMajor, ua: request.Header.Get("User-Agent")}
		_, _ = io.WriteString(writer, request.URL.Path)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	pemPath := writeServerRootPEM(t, server)
	defaultTransport := http.DefaultTransport.(*http.Transport)
	defaultForceHTTP2 := defaultTransport.ForceAttemptHTTP2
	alphaHeaders := []Field{{Name: "User-Agent", Value: "fixture alpha"}}
	alpha, err := NewDuckDuckGoTextClient(Config{Verification: VerifyWithPEMFile(pemPath)}, alphaHeaders)
	if err != nil {
		t.Fatalf("NewDuckDuckGoTextClient alpha: %v", err)
	}
	alphaHeaders[0].Value = "caller mutation"
	beta, err := NewDuckDuckGoTextClient(Config{Verification: VerifyWithPEMFile(pemPath)}, []Field{{Name: "User-Agent", Value: "fixture beta"}})
	if err != nil {
		t.Fatalf("NewDuckDuckGoTextClient beta: %v", err)
	}
	if alpha.client.jar == beta.client.jar {
		t.Fatal("DuckDuckGo clients share a cookie jar")
	}
	start := make(chan struct{})
	errorsByCaller := make(chan error, callers)
	var callersWG sync.WaitGroup
	for index := range callers {
		callersWG.Add(1)
		go func() {
			defer callersWG.Done()
			<-start
			client := alpha
			path := "/alpha"
			if index%2 != 0 {
				client = beta
				path = "/beta"
			}
			response, err := client.Do(t.Context(), Request{Method: http.MethodPost, URL: server.URL + path, Form: []Field{{Name: "q", Value: "needle"}}})
			if err == nil && response.Text != path {
				err = fmt.Errorf("response text = %q, want %q", response.Text, path)
			}
			errorsByCaller <- err
		}()
	}
	close(start)
	callersWG.Wait()
	close(errorsByCaller)
	for err := range errorsByCaller {
		if err != nil {
			t.Fatalf("concurrent Do: %v", err)
		}
	}
	close(observed)
	for got := range observed {
		if got.proto != 2 {
			t.Fatalf("request %s protocol major = %d, want HTTP/2", got.path, got.proto)
		}
		wantUA := "fixture alpha"
		if got.path == "/beta" {
			wantUA = "fixture beta"
		}
		if got.ua != wantUA {
			t.Fatalf("request %s User-Agent = %q, want %q", got.path, got.ua, wantUA)
		}
	}
	for _, client := range []*DuckDuckGoTextClient{alpha, beta} {
		native := client.client.nativeHTTPClient()
		if native == nil {
			t.Fatal("DuckDuckGo native client is unavailable after request")
		}
		transport, ok := native.Transport.(*http.Transport)
		if !ok || !transport.ForceAttemptHTTP2 {
			t.Fatalf("DuckDuckGo native transport = %T ForceAttemptHTTP2=%t, want enabled HTTP/2 transport", native.Transport, ok && transport.ForceAttemptHTTP2)
		}
	}
	if alpha.client.nativeHTTPClient().Transport == beta.client.nativeHTTPClient().Transport {
		t.Fatal("DuckDuckGo clients share a native transport")
	}
	if got := defaultTransport.ForceAttemptHTTP2; got != defaultForceHTTP2 {
		t.Fatalf("http.DefaultTransport ForceAttemptHTTP2 = %t, want unchanged %t", got, defaultForceHTTP2)
	}
}

func TestDuckDuckGoTextClient_HonorsCallerCancellationAndReleasesIdleConnections(t *testing.T) {
	started := make(chan struct{})
	client, err := newDuckDuckGoTextClient(Config{}, nil, roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	}))
	if err != nil {
		t.Fatalf("newDuckDuckGoTextClient: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := client.Do(ctx, Request{Method: http.MethodGet, URL: "https://transport.fixture/cancel"})
		result <- err
	}()
	select {
	case <-started:
	case err := <-result:
		t.Fatalf("Do returned before RoundTrip received the request: %v", err)
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Do cancellation error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Do did not return after caller cancellation")
	}
	client.CloseIdleConnections()
}

func headersFromFixture(t testing.TB, fixture transportFixture) []Field {
	t.Helper()
	raw, ok := fixture.Input.Constructor["headers"]
	if !ok || isJSONNull(raw) {
		return nil
	}
	headers := decodeJSONValue[map[string]string](t, raw)
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]Field, 0, len(names))
	for _, name := range names {
		fields = append(fields, Field{Name: name, Value: headers[name]})
	}
	return fields
}

func writeServerRootPEM(t testing.TB, server *httptest.Server) string {
	t.Helper()
	certificate := server.Certificate()
	if certificate == nil {
		t.Fatal("TLS server certificate = nil")
	}
	pemPath := filepath.Join(t.TempDir(), "fixture-root.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(pemPath, pemBytes, 0o600); err != nil {
		t.Fatalf("write PEM: %v", err)
	}
	return pemPath
}

func TestHeadersFromFixture_IsStableAndLossless(t *testing.T) {
	fixture := loadTransportFixture(t, "../../testdata/contracts/transport/transport.ddg-http2-constructor-post.json")
	got := headersFromFixture(t, fixture)
	want := []Field{{Name: "User-Agent", Value: "fixture DDG UA"}, {Name: "X-Fixture", Value: "value"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("headersFromFixture = %#v, want %#v", got, want)
	}
}
