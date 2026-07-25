package transport

import (
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
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

	"golang.org/x/net/http2"
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
		if _, ok := native.Transport.(*browserRoundTripper); !ok {
			t.Fatalf("DuckDuckGo native transport = %T, want isolated source-shaped HTTP/2 transport", native.Transport)
		}
	}
	if alpha.client.nativeHTTPClient().Transport == beta.client.nativeHTTPClient().Transport {
		t.Fatal("DuckDuckGo clients share a native transport")
	}
	if got := defaultTransport.ForceAttemptHTTP2; got != defaultForceHTTP2 {
		t.Fatalf("http.DefaultTransport ForceAttemptHTTP2 = %t, want unchanged %t", got, defaultForceHTTP2)
	}
}

func TestDuckDuckGoTextClient_EmitsSourceShapedHTTP2Initialization(t *testing.T) {
	server := newBrowserHTTP2CaptureServer(t, "ddg source fixture")
	defer server.Close()

	pemPath := writeCertificateRootPEM(t, server.certificate)
	client, err := NewDuckDuckGoTextClient(
		Config{Verification: VerifyWithPEMFile(pemPath)},
		[]Field{{Name: "User-Agent", Value: "ddg session fixture"}},
	)
	if err != nil {
		t.Fatalf("NewDuckDuckGoTextClient: %v", err)
	}
	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: server.URL + "/source-shaped"})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got, want := response.Text, "ddg source fixture"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}

	capture := server.Capture(t)
	assertDuckDuckGoHTTP2Initialization(t, capture)
	if got, want := capture.pseudoHeaders, []string{":method", ":authority", ":scheme", ":path"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pseudo-header order = %#v, want %#v", got, want)
	}
	if got, want := capture.headerNames, []string{"accept", "accept-encoding", "user-agent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("header order = %#v, want %#v", got, want)
	}
	if got, want := capture.headers.Get("Accept"), "*/*"; got != want {
		t.Fatalf("accept = %q, want %q", got, want)
	}
	if got, want := capture.headers.Get("Accept-Encoding"), "gzip, deflate, br"; got != want {
		t.Fatalf("accept-encoding = %q, want %q", got, want)
	}
	if got, want := capture.headers.Get("User-Agent"), "ddg session fixture"; got != want {
		t.Fatalf("user-agent = %q, want %q", got, want)
	}
}

func TestDuckDuckGoTextClient_SelectsOneTLSProfilePerClient(t *testing.T) {
	choices := []int{2, 0}
	chooserCalls := 0
	chooser := func(limit int) (int, error) {
		if got, want := limit, 4; got != want {
			t.Fatalf("TLS policy limit = %d, want %d", got, want)
		}
		choice := choices[chooserCalls]
		chooserCalls++
		return choice, nil
	}
	first, err := newDuckDuckGoTextClientWithSessionProfileChooser(Config{}, nil, nil, chooser)
	if err != nil {
		t.Fatalf("first DuckDuckGo client: %v", err)
	}
	second, err := newDuckDuckGoTextClientWithSessionProfileChooser(Config{}, nil, nil, chooser)
	if err != nil {
		t.Fatalf("second DuckDuckGo client: %v", err)
	}
	if got, want := chooserCalls, 2; got != want {
		t.Fatalf("TLS policy chooser calls = %d, want %d", got, want)
	}
	if first.client.duckDuckGoSessionProfile == nil || second.client.duckDuckGoSessionProfile == nil {
		t.Fatal("DuckDuckGo client has no selected TLS session profile")
	}
	if got, want := first.client.duckDuckGoSessionProfile.sourceVariant, "verify_min_tls13"; got != want {
		t.Fatalf("first TLS policy = %q, want %q", got, want)
	}
	if got, want := second.client.duckDuckGoSessionProfile.sourceVariant, "verify_default"; got != want {
		t.Fatalf("second TLS policy = %q, want %q", got, want)
	}
	firstRaw := append([]byte(nil), first.client.duckDuckGoSessionProfile.clientHelloRaw...)
	first.client.duckDuckGoSessionProfile.clientHelloRaw[0] ^= 0xFF
	if got := second.client.duckDuckGoSessionProfile.clientHelloRaw[0]; got == first.client.duckDuckGoSessionProfile.clientHelloRaw[0] {
		t.Fatal("DuckDuckGo clients share mutable TLS profile bytes")
	}
	first.client.duckDuckGoSessionProfile.clientHelloRaw = firstRaw

	offChooserCalls := 0
	off, err := newDuckDuckGoTextClientWithSessionProfileChooser(
		Config{Verification: SkipCertificateVerification()},
		nil,
		nil,
		func(int) (int, error) {
			offChooserCalls++
			return 0, errors.New("verify=false must not choose a TLS policy")
		},
	)
	if err != nil {
		t.Fatalf("verification-off DuckDuckGo client: %v", err)
	}
	if got, want := offChooserCalls, 0; got != want {
		t.Fatalf("verification-off chooser calls = %d, want %d", got, want)
	}
	if got, want := off.client.duckDuckGoSessionProfile.sourceVariant, "verify_off"; got != want {
		t.Fatalf("verification-off TLS profile = %q, want %q", got, want)
	}
}

func TestDuckDuckGoSessionProfile_ShufflesOnlySourceVariableCipherSuffix(t *testing.T) {
	catalog, err := loadDuckDuckGoSessionProfileCatalog()
	if err != nil {
		t.Fatalf("load DuckDuckGo TLS profiles: %v", err)
	}
	original := catalog.verified[0]
	profile, err := selectDuckDuckGoSessionProfileWithCipherShuffler(
		normalizeSettings(Config{}),
		func(int) (int, error) { return 0, nil },
		func(suites []uint16) error {
			for left, right := 0, len(suites)-1; left < right; left, right = left+1, right-1 {
				suites[left], suites[right] = suites[right], suites[left]
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("select shuffled TLS profile: %v", err)
	}
	start, end, err := duckDuckGoVariableCipherSuiteRange(original)
	if err != nil {
		t.Fatalf("source cipher range: %v", err)
	}
	if got, want := profile.clientHelloRaw[:start], original.clientHelloRaw[:start]; !reflect.DeepEqual(got, want) {
		t.Fatal("TLS cipher shuffle changed the fixed policy prefix")
	}
	if got, want := profile.clientHelloRaw[end:], original.clientHelloRaw[end:]; !reflect.DeepEqual(got, want) {
		t.Fatal("TLS cipher shuffle changed SCSV or later ClientHello fields")
	}
	for index := start; index < end; index += 2 {
		got := profile.clientHelloRaw[index : index+2]
		want := original.clientHelloRaw[end-(index-start)-2 : end-(index-start)]
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("shuffled cipher at byte %d = %x, want reverse %x", index, got, want)
		}
	}

	offCalls := 0
	off, err := selectDuckDuckGoSessionProfileWithCipherShuffler(
		normalizeSettings(Config{Verification: SkipCertificateVerification()}),
		func(int) (int, error) { return 0, errors.New("verify=false must not choose policy") },
		func([]uint16) error {
			offCalls++
			return errors.New("verify=false must not shuffle source ciphers")
		},
	)
	if err != nil {
		t.Fatalf("select verification-off profile: %v", err)
	}
	if got, want := offCalls, 0; got != want {
		t.Fatalf("verification-off cipher shuffler calls = %d, want %d", got, want)
	}
	if got, want := off.sourceVariant, "verify_off"; got != want {
		t.Fatalf("verification-off TLS profile = %q, want %q", got, want)
	}
}

func TestDuckDuckGoSessionProfile_RejectsInvalidPrivateInputs(t *testing.T) {
	catalog, err := loadDuckDuckGoSessionProfileCatalog()
	if err != nil {
		t.Fatalf("load DuckDuckGo TLS profiles: %v", err)
	}
	if _, err := selectDuckDuckGoSessionProfileWithCipherShuffler(
		normalizeSettings(Config{}),
		func(int) (int, error) { return -1, nil },
		func([]uint16) error { return nil },
	); err == nil {
		t.Fatal("negative TLS policy selection error = nil")
	}
	if _, err := shuffleDuckDuckGoSessionCipherSuites(catalog.verified[0], func([]uint16) error {
		return errors.New("cipher source unavailable")
	}); err == nil || !strings.Contains(err.Error(), "cipher source unavailable") {
		t.Fatalf("cipher shuffle error = %v, want wrapped source error", err)
	}
	if _, _, err := duckDuckGoVariableCipherSuiteRange(browserProfile{sourceVariant: "verify_default"}); err == nil {
		t.Fatal("invalid ClientHello cipher range error = nil")
	}
	if _, err := decodeBrowserProfileHTTP2(browserProfileCaptureH2{}); err == nil {
		t.Fatal("empty HTTP/2 capture error = nil")
	}
	if _, err := chooseDuckDuckGoSessionProfileIndex(0); err == nil {
		t.Fatal("zero TLS policy limit error = nil")
	}
	if _, err := randomDuckDuckGoHTTP2Value(2, 1); err == nil {
		t.Fatal("reversed HTTP/2 range error = nil")
	}
	if sameStrings([]string{"source"}, []string{"different"}) {
		t.Fatal("different string slices compare equal")
	}
	profile, err := selectDuckDuckGoSessionProfileWithCipherShuffler(
		normalizeSettings(Config{}),
		func(int) (int, error) { return 0, nil },
		func([]uint16) error { return nil },
	)
	if err != nil {
		t.Fatalf("select TLS profile: %v", err)
	}
	if _, err := newDuckDuckGoHTTP2Profile(profile, func(uint32, uint32) (uint32, error) {
		return 0, errors.New("H2 source unavailable")
	}); err == nil || !strings.Contains(err.Error(), "H2 source unavailable") {
		t.Fatalf("HTTP/2 profile error = %v, want wrapped source error", err)
	}
}

func TestDuckDuckGoHTTP2Profile_FollowsSourceDrawChronologyAndWireOrder(t *testing.T) {
	session, err := selectDuckDuckGoSessionProfile(normalizeSettings(Config{}), func(int) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("select TLS profile: %v", err)
	}
	values := []uint32{111, 4444, 22222, 133, 66000, 1, 0}
	wantRanges := [][2]uint32{{100, 200}, {4000, 5000}, {16384, 65535}, {100, 200}, {65500, 66500}, {0, 1}, {0, 1}}
	index := 0
	profile, err := newDuckDuckGoHTTP2Profile(session, func(minimum, maximum uint32) (uint32, error) {
		if got, want := [2]uint32{minimum, maximum}, wantRanges[index]; got != want {
			t.Fatalf("draw %d range = %v, want %v", index, got, want)
		}
		value := values[index]
		index++
		return value, nil
	})
	if err != nil {
		t.Fatalf("new DuckDuckGo HTTP/2 profile: %v", err)
	}
	if got, want := index, len(values); got != want {
		t.Fatalf("HTTP/2 random draws = %d, want %d", got, want)
	}
	if got, want := profile.settings, []http2.Setting{
		{ID: http2.SettingHeaderTableSize, Val: 4444},
		{ID: http2.SettingEnablePush, Val: 0},
		{ID: http2.SettingInitialWindowSize, Val: 111},
		{ID: http2.SettingMaxFrameSize, Val: 22222},
		{ID: http2.SettingEnableConnectProtocol, Val: 1},
		{ID: http2.SettingMaxConcurrentStreams, Val: 133},
		{ID: http2.SettingMaxHeaderListSize, Val: 66000},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("HTTP/2 settings = %#v, want %#v", got, want)
	}
	if got, want := profile.connectionWindow, uint32(1<<24); got != want {
		t.Fatalf("connection window = %d, want %d", got, want)
	}
	if profile.streamWindow == nil || *profile.streamWindow != 1<<24 {
		t.Fatalf("stream window = %#v, want %d", profile.streamWindow, 1<<24)
	}
	if got, want := profile.clientHelloRaw, session.clientHelloRaw; !reflect.DeepEqual(got, want) {
		t.Fatal("HTTP/2 profile changed the selected TLS session template")
	}
}

func TestDuckDuckGoHTTP2ProfileFactory_SamplesOnConnectionCreationOnly(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, request.URL.Path)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	pemPath := writeServerRootPEM(t, server)
	settings := normalizeSettings(Config{Verification: VerifyWithPEMFile(pemPath)})
	session, err := selectDuckDuckGoSessionProfile(settings, func(int) (int, error) { return 0, nil })
	if err != nil {
		t.Fatalf("select TLS profile: %v", err)
	}
	factoryCalls := 0
	roundTripper, err := newBrowserRoundTripperForProfileWithHTTP2Factory(settings, session, func() (browserProfile, error) {
		factoryCalls++
		return newDuckDuckGoHTTP2Profile(session, func(minimum, _ uint32) (uint32, error) { return minimum, nil })
	})
	if err != nil {
		t.Fatalf("new DuckDuckGo round tripper: %v", err)
	}
	client := &http.Client{Transport: roundTripper}
	request := func(path string) {
		t.Helper()
		response, requestErr := client.Get(server.URL + path)
		if requestErr != nil {
			t.Fatalf("GET %s: %v", path, requestErr)
		}
		defer response.Body.Close()
		if _, requestErr = io.ReadAll(response.Body); requestErr != nil {
			t.Fatalf("read %s: %v", path, requestErr)
		}
	}
	request("/first")
	request("/reused")
	if got, want := factoryCalls, 1; got != want {
		t.Fatalf("profile factory calls after reuse = %d, want %d", got, want)
	}
	roundTripper.CloseIdleConnections()
	request("/reconnected")
	if got, want := factoryCalls, 2; got != want {
		t.Fatalf("profile factory calls after reconnect = %d, want %d", got, want)
	}
}

func TestDuckDuckGoTextClient_CarriesSourceProfileAcrossTargetRoutes(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{
			name: "disabled verification",
			run: func(t *testing.T) {
				target := newBrowserHTTP2CaptureServer(t, "disabled verification")
				defer target.Close()
				client, err := NewDuckDuckGoTextClient(Config{Verification: SkipCertificateVerification()}, nil)
				if err != nil {
					t.Fatalf("NewDuckDuckGoTextClient: %v", err)
				}
				assertDuckDuckGoTargetRoute(t, client, target, "/verify-off", "disabled verification")
			},
		},
		{
			name: "HTTP CONNECT",
			run: func(t *testing.T) {
				target := newBrowserHTTP2CaptureServer(t, "HTTP CONNECT")
				defer target.Close()
				proxy := newHTTPConnectTunnel(t)
				client, err := NewDuckDuckGoTextClient(Config{
					Proxy:        stringPointer(proxy.URL()),
					Verification: SkipCertificateVerification(),
				}, nil)
				if err != nil {
					t.Fatalf("NewDuckDuckGoTextClient: %v", err)
				}
				assertDuckDuckGoTargetRoute(t, client, target, "/http-connect", "HTTP CONNECT")
				if got, want := proxy.Target(t), strings.TrimPrefix(target.URL, "https://"); got != want {
					t.Fatalf("CONNECT target = %q, want %q", got, want)
				}
			},
		},
		{
			name: "HTTPS CONNECT",
			run: func(t *testing.T) {
				target := newBrowserHTTP2CaptureServer(t, "HTTPS CONNECT")
				defer target.Close()
				proxy := newHTTPSConnectTunnel(t)
				defer proxy.Close()
				client, err := NewDuckDuckGoTextClient(Config{
					Proxy:        stringPointer(proxy.URL()),
					Verification: SkipCertificateVerification(),
				}, nil)
				if err != nil {
					t.Fatalf("NewDuckDuckGoTextClient: %v", err)
				}
				assertDuckDuckGoTargetRoute(t, client, target, "/https-connect", "HTTPS CONNECT")
				if got, want := proxy.Target(t), strings.TrimPrefix(target.URL, "https://"); got != want {
					t.Fatalf("HTTPS CONNECT target = %q, want %q", got, want)
				}
			},
		},
		{
			name: "SOCKS5",
			run: func(t *testing.T) {
				target := newBrowserHTTP2CaptureServer(t, "SOCKS5")
				defer target.Close()
				proxy := newBrowserSOCKSTunnel(t)
				client, err := NewDuckDuckGoTextClient(Config{
					Proxy:        stringPointer("socks5://" + proxy.Address()),
					Verification: SkipCertificateVerification(),
				}, nil)
				if err != nil {
					t.Fatalf("NewDuckDuckGoTextClient: %v", err)
				}
				assertDuckDuckGoTargetRoute(t, client, target, "/socks5", "SOCKS5")
				if got, want := proxy.Capture(t).addressType, byte(socksAddressIPv4); got != want {
					t.Fatalf("SOCKS address type = %d, want %d", got, want)
				}
			},
		},
		{
			name: "SOCKS5H",
			run: func(t *testing.T) {
				target := newBrowserHTTP2CaptureServer(t, "SOCKS5H")
				defer target.Close()
				proxy := newBrowserSOCKSTunnel(t)
				client, err := NewDuckDuckGoTextClient(Config{
					Proxy:        stringPointer("socks5h://" + proxy.Address()),
					Verification: SkipCertificateVerification(),
				}, nil)
				if err != nil {
					t.Fatalf("NewDuckDuckGoTextClient: %v", err)
				}
				_, port, err := net.SplitHostPort(strings.TrimPrefix(target.URL, "https://"))
				if err != nil {
					t.Fatalf("split target URL: %v", err)
				}
				response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: "https://localhost:" + port + "/socks5h"})
				if err != nil {
					t.Fatalf("Do: %v", err)
				}
				if got, want := response.Text, "SOCKS5H"; got != want {
					t.Fatalf("response text = %q, want %q", got, want)
				}
				client.CloseIdleConnections()
				if got, want := proxy.Capture(t).addressType, byte(socksAddressDomain); got != want {
					t.Fatalf("SOCKS address type = %d, want %d", got, want)
				}
				assertDuckDuckGoHTTP2Initialization(t, target.Capture(t))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, test.run)
	}
}

func TestDuckDuckGoTextClient_SourceShapedConnectionHonorsCancellation(t *testing.T) {
	server := newBrowserHTTP2CaptureServer(t, "")
	server.HoldResponse()
	defer server.Close()

	client, err := NewDuckDuckGoTextClient(Config{Verification: SkipCertificateVerification()}, nil)
	if err != nil {
		t.Fatalf("NewDuckDuckGoTextClient: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, requestErr := client.Do(ctx, Request{Method: http.MethodGet, URL: server.URL + "/cancel-source-shaped"})
		result <- requestErr
	}()
	server.WaitForRequest(t)
	cancel()
	select {
	case requestErr := <-result:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("Do cancellation error = %v, want context.Canceled", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("source-shaped DuckDuckGo request did not return after cancellation")
	}
	client.CloseIdleConnections()
}

func assertDuckDuckGoTargetRoute(t testing.TB, client *DuckDuckGoTextClient, target *browserHTTP2CaptureServer, path, wantBody string) {
	t.Helper()
	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: target.URL + path})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := response.Text; got != wantBody {
		t.Fatalf("response text = %q, want %q", got, wantBody)
	}
	client.CloseIdleConnections()
	assertDuckDuckGoHTTP2Initialization(t, target.Capture(t))
}

func assertDuckDuckGoHTTP2Initialization(t testing.TB, capture browserHTTP2Capture) {
	t.Helper()
	if got, want := capture.preface, http2.ClientPreface; got != want {
		t.Fatalf("HTTP/2 preface = %q, want %q", got, want)
	}
	if got, want := capture.connectionWindow, uint32(1<<24); got != want {
		t.Fatalf("connection window increment = %d, want %d", got, want)
	}
	if capture.streamWindow == nil || *capture.streamWindow != 1<<24 {
		t.Fatalf("stream window increment = %#v, want %d", capture.streamWindow, 1<<24)
	}
	settings := capture.settings
	if len(settings) != 7 {
		t.Fatalf("HTTP/2 setting count = %d, want 7 (%#v)", len(settings), settings)
	}
	wantIDs := []http2.SettingID{
		http2.SettingHeaderTableSize,
		http2.SettingEnablePush,
		http2.SettingInitialWindowSize,
		http2.SettingMaxFrameSize,
		http2.SettingEnableConnectProtocol,
		http2.SettingMaxConcurrentStreams,
		http2.SettingMaxHeaderListSize,
	}
	for index, setting := range settings {
		if got, want := setting.ID, wantIDs[index]; got != want {
			t.Fatalf("setting[%d] ID = %d, want %d", index, got, want)
		}
	}
	assertDuckDuckGoSettingRange(t, settings[0].Val, 4000, 5000, "HEADER_TABLE_SIZE")
	assertDuckDuckGoSettingRange(t, settings[1].Val, 0, 1, "ENABLE_PUSH")
	assertDuckDuckGoSettingRange(t, settings[2].Val, 100, 200, "INITIAL_WINDOW_SIZE")
	assertDuckDuckGoSettingRange(t, settings[3].Val, 16384, 65535, "MAX_FRAME_SIZE")
	assertDuckDuckGoSettingRange(t, settings[4].Val, 0, 1, "ENABLE_CONNECT_PROTOCOL")
	assertDuckDuckGoSettingRange(t, settings[5].Val, 100, 200, "MAX_CONCURRENT_STREAMS")
	assertDuckDuckGoSettingRange(t, settings[6].Val, 65500, 66500, "MAX_HEADER_LIST_SIZE")
}

func assertDuckDuckGoSettingRange(t testing.TB, value, minimum, maximum uint32, name string) {
	t.Helper()
	if value < minimum || value > maximum {
		t.Fatalf("%s = %d, want [%d, %d]", name, value, minimum, maximum)
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

func writeCertificateRootPEM(t testing.TB, certificate tls.Certificate) string {
	t.Helper()
	pemPath := filepath.Join(t.TempDir(), "fixture-root.pem")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})
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
