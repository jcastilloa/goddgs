package transport

import (
	"bufio"
	"context"
	"encoding/binary"
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
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClient_HTTPConnectProxyCarriesSelectedBundleToHTTPSTarget(t *testing.T) {
	profile := frozenBrowserProfile(t, "firefox_148", "ios")
	target := newBrowserHTTP2CaptureServer(t, "tunnel fixture")
	defer target.Close()
	proxy := newHTTPConnectTunnel(t)

	client, err := newClientWithBehaviorAndBrowserProfileChooser(Config{
		Proxy:        stringPointer(proxy.URL()),
		Verification: SkipCertificateVerification(), // loopback target certificate
	}, nil, clientBehavior{followRedirects: true}, fixedBrowserProfileChooser(1, 22))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.browserProfile == nil {
		t.Fatal("proxy client did not select one source browser bundle")
	}
	if got, want := client.browserProfile.sourceVariant, profile.sourceVariant; got != want {
		t.Fatalf("selected variant = %q, want %q", got, want)
	}
	if got, want := client.browserProfile.sourceOperatingSystem, profile.sourceOperatingSystem; got != want {
		t.Fatalf("selected operating system = %q, want %q", got, want)
	}

	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: target.URL + "/through-http-connect"})
	if err != nil {
		t.Fatalf("Do through CONNECT proxy: %v", err)
	}
	if got, want := response.Text, "tunnel fixture"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
	client.CloseIdleConnections()

	if got, want := proxy.Target(t), strings.TrimPrefix(target.URL, "https://"); got != want {
		t.Fatalf("CONNECT target = %q, want %q", got, want)
	}
	assertBrowserHTTP2CaptureMatchesProfile(t, target.Capture(t), profile)
}

func TestClient_HTTPConnectProxyCarriesEveryFrozenSourceBundle(t *testing.T) {
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load browser catalog: %v", err)
	}

	for variantIndex, variant := range catalog.variants {
		for operatingSystemIndex, operatingSystem := range catalog.operatingSystems {
			profile, err := catalog.profile(variantIndex, operatingSystemIndex)
			if err != nil {
				t.Fatalf("catalog profile %s/%s: %v", variant, operatingSystem, err)
			}
			t.Run(variant+"/"+operatingSystem, func(t *testing.T) {
				target := newBrowserHTTP2CaptureServer(t, "every proxy profile fixture")
				defer target.Close()
				proxy := newHTTPConnectTunnel(t)
				client, err := newClientWithBehaviorAndBrowserProfileChooser(Config{
					Proxy:        stringPointer(proxy.URL()),
					Verification: SkipCertificateVerification(),
				}, nil, clientBehavior{followRedirects: true}, fixedBrowserProfileChooser(operatingSystemIndex, variantIndex))
				if err != nil {
					t.Fatalf("new client: %v", err)
				}
				response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: target.URL + "/every-profile"})
				if err != nil {
					t.Fatalf("Do through CONNECT proxy: %v", err)
				}
				if got, want := response.Text, "every proxy profile fixture"; got != want {
					t.Fatalf("response text = %q, want %q", got, want)
				}
				client.CloseIdleConnections()
				if got, want := proxy.Target(t), strings.TrimPrefix(target.URL, "https://"); got != want {
					t.Fatalf("CONNECT target = %q, want %q", got, want)
				}
				assertBrowserHTTP2CaptureMatchesProfile(t, target.Capture(t), profile)
			})
		}
	}
}

func TestClient_SOCKSTunnelsCarrySelectedBundleToHTTPSTarget(t *testing.T) {
	tests := []struct {
		name              string
		scheme            string
		profile           browserProfile
		wantAddressType   byte
		wantTargetHost    string
		useLocalhostInURL bool
	}{
		{
			name:            "socks5 local DNS",
			scheme:          "socks5",
			profile:         frozenBrowserProfile(t, "opera_131", "android"),
			wantAddressType: socksAddressIPv4,
			wantTargetHost:  "127.0.0.1",
		},
		{
			name:              "socks5h remote DNS",
			scheme:            "socks5h",
			profile:           frozenBrowserProfile(t, "edge_148", "linux"),
			wantAddressType:   socksAddressDomain,
			wantTargetHost:    "localhost",
			useLocalhostInURL: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := newBrowserHTTP2CaptureServer(t, "SOCKS tunnel fixture")
			defer target.Close()
			proxy := newBrowserSOCKSTunnel(t)

			targetURL := target.URL
			if test.useLocalhostInURL {
				_, port, err := net.SplitHostPort(strings.TrimPrefix(target.URL, "https://"))
				if err != nil {
					t.Fatalf("split target URL: %v", err)
				}
				targetURL = "https://localhost:" + port
			}
			client, err := newClientWithBehaviorAndBrowserProfileChooser(Config{
				Proxy:        stringPointer(test.scheme + "://" + proxy.Address()),
				Verification: SkipCertificateVerification(),
			}, nil, clientBehavior{followRedirects: true}, chooserForProfile(t, test.profile))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: targetURL + "/through-socks"})
			if err != nil {
				t.Fatalf("Do through %s: %v", test.scheme, err)
			}
			if got, want := response.Text, "SOCKS tunnel fixture"; got != want {
				t.Fatalf("response text = %q, want %q", got, want)
			}
			client.CloseIdleConnections()

			capture := proxy.Capture(t)
			if got, want := capture.addressType, test.wantAddressType; got != want {
				t.Fatalf("SOCKS address type = %d, want %d", got, want)
			}
			if got, want := capture.host, test.wantTargetHost; got != want {
				t.Fatalf("SOCKS target host = %q, want %q", got, want)
			}
			assertBrowserHTTP2CaptureMatchesProfile(t, target.Capture(t), test.profile)
		})
	}
}

func TestClient_HTTPSConnectProxyCarriesSelectedBundleToHTTPSTarget(t *testing.T) {
	profile := frozenBrowserProfile(t, "safari_26.3", "macos")
	target := newBrowserHTTP2CaptureServer(t, "HTTPS CONNECT fixture")
	defer target.Close()
	proxy := newHTTPSConnectTunnel(t)
	defer proxy.Close()

	client, err := newClientWithBehaviorAndBrowserProfileChooser(Config{
		Proxy:        stringPointer(proxy.URL()),
		Verification: SkipCertificateVerification(), // loopback proxy and target certificates
	}, nil, clientBehavior{followRedirects: true}, chooserForProfile(t, profile))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: target.URL + "/through-https-connect"})
	if err != nil {
		t.Fatalf("Do through HTTPS CONNECT proxy: %v", err)
	}
	if got, want := response.Text, "HTTPS CONNECT fixture"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
	client.CloseIdleConnections()

	if got, want := proxy.Target(t), strings.TrimPrefix(target.URL, "https://"); got != want {
		t.Fatalf("HTTPS CONNECT target = %q, want %q", got, want)
	}
	assertBrowserHTTP2CaptureMatchesProfile(t, target.Capture(t), profile)
}

func TestClient_DirectTLSOverridesCarrySelectedBundleToHTTPSTarget(t *testing.T) {
	profile := frozenBrowserProfile(t, "firefox_140", "windows")

	tests := []struct {
		name      string
		configure func(testing.TB, *browserHTTP2CaptureServer) Config
	}{
		{
			name: "disabled verification",
			configure: func(testing.TB, *browserHTTP2CaptureServer) Config {
				return Config{Verification: SkipCertificateVerification()}
			},
		},
		{
			name: "custom PEM root",
			configure: func(t testing.TB, target *browserHTTP2CaptureServer) Config {
				t.Helper()
				if len(target.certificate.Certificate) == 0 {
					t.Fatal("browser capture target has no certificate")
				}
				path := filepath.Join(t.TempDir(), "target-root.pem")
				pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: target.certificate.Certificate[0]})
				if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
					t.Fatalf("write target root: %v", err)
				}
				return Config{Verification: VerifyWithPEMFile(path)}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := newBrowserHTTP2CaptureServer(t, "TLS override fixture")
			defer target.Close()
			client, err := newClientWithBehaviorAndBrowserProfileChooser(test.configure(t, target), nil, clientBehavior{followRedirects: true}, chooserForProfile(t, profile))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: target.URL + "/tls-override"})
			if err != nil {
				t.Fatalf("Do with %s: %v", test.name, err)
			}
			if got, want := response.Text, "TLS override fixture"; got != want {
				t.Fatalf("response text = %q, want %q", got, want)
			}
			client.CloseIdleConnections()
			assertBrowserHTTP2CaptureMatchesProfile(t, target.Capture(t), profile)
		})
	}
}

func TestClient_HTTPProxyKeepsPlainHTTPFreeOfBrowserHeaders(t *testing.T) {
	seen := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen <- request.Header.Get("User-Agent")
		_, _ = io.WriteString(writer, "plain proxy fixture")
	}))
	defer proxy.Close()
	client, err := newClientWithBehaviorAndBrowserProfileChooser(Config{Proxy: stringPointer(proxy.URL)}, nil, clientBehavior{followRedirects: true}, fixedBrowserProfileChooser(4, 2))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if client.browserProfile == nil {
		t.Fatal("HTTP proxy client did not select its future HTTPS browser bundle")
	}
	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: "http://plain.proxy.fixture/through-proxy"})
	if err != nil {
		t.Fatalf("Do plain HTTP through proxy: %v", err)
	}
	if got, want := response.Text, "plain proxy fixture"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
	select {
	case got := <-seen:
		if got != "Go-http-client/1.1" {
			t.Fatalf("plain HTTP proxy User-Agent = %q, want standard HTTP transport", got)
		}
	case <-time.After(time.Second):
		t.Fatal("plain HTTP proxy received no request")
	}
}

func TestClient_HTTPConnectProxyForwardsCredentialsWithoutChangingTargetBundle(t *testing.T) {
	profile := frozenBrowserProfile(t, "chrome_144", "windows")
	target := newBrowserHTTP2CaptureServer(t, "authenticated tunnel")
	defer target.Close()
	proxy := newHTTPConnectTunnel(t)

	proxyURL, err := url.Parse(proxy.URL())
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}
	proxyURL.User = url.UserPassword("source-user", "source-pass")
	client, err := newClientWithBehaviorAndBrowserProfileChooser(Config{
		Proxy:        stringPointer(proxyURL.String()),
		Verification: SkipCertificateVerification(),
	}, nil, clientBehavior{followRedirects: true}, chooserForProfile(t, profile))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: target.URL + "/authenticated"})
	if err != nil {
		t.Fatalf("Do through authenticated CONNECT proxy: %v", err)
	}
	if got, want := response.Text, "authenticated tunnel"; got != want {
		t.Fatalf("response text = %q, want %q", got, want)
	}
	client.CloseIdleConnections()

	if got, want := proxy.ProxyAuthorization(t), "Basic c291cmNlLXVzZXI6c291cmNlLXBhc3M="; got != want {
		t.Fatalf("Proxy-Authorization = %q, want %q", got, want)
	}
	assertBrowserHTTP2CaptureMatchesProfile(t, target.Capture(t), profile)
}

func TestClient_HTTPConnectProxyCancellationClosesTunnelAndAllowsProfileRetry(t *testing.T) {
	profile := frozenBrowserProfile(t, "chrome_148", "ios")
	target := newBrowserHTTP2CaptureServer(t, "retry through proxy")
	defer target.Close()
	proxy := newBlockingThenForwardHTTPConnectTunnel(t)

	client, err := newClientWithBehaviorAndBrowserProfileChooser(Config{
		Proxy:        stringPointer(proxy.URL()),
		Verification: SkipCertificateVerification(),
	}, nil, clientBehavior{followRedirects: true}, chooserForProfile(t, profile))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	firstResult := make(chan error, 1)
	go func() {
		_, err := client.Do(ctx, Request{Method: http.MethodGet, URL: target.URL + "/cancel-connect"})
		firstResult <- err
	}()
	proxy.WaitForFirstConnect(t)
	cancel()
	select {
	case err := <-firstResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled CONNECT error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled CONNECT request did not return")
	}

	response, err := client.Do(t.Context(), Request{Method: http.MethodGet, URL: target.URL + "/retry-connect"})
	if err != nil {
		t.Fatalf("retry after canceled CONNECT: %v", err)
	}
	if got, want := response.Text, "retry through proxy"; got != want {
		t.Fatalf("retry response text = %q, want %q", got, want)
	}
	client.CloseIdleConnections()
	assertBrowserHTTP2CaptureMatchesProfile(t, target.Capture(t), profile)
}

type httpConnectTunnel struct {
	listener net.Listener
	targets  chan string
	auth     chan string
	errs     chan error
	done     chan struct{}
	close    sync.Once
}

func newHTTPConnectTunnel(t testing.TB) *httpConnectTunnel {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen HTTP CONNECT proxy: %v", err)
	}
	proxy := &httpConnectTunnel{
		listener: listener,
		targets:  make(chan string, 1),
		auth:     make(chan string, 1),
		errs:     make(chan error, 1),
		done:     make(chan struct{}),
	}
	go proxy.serve()
	t.Cleanup(func() { proxy.Close(t) })
	return proxy
}

func (proxy *httpConnectTunnel) URL() string {
	return "http://" + proxy.listener.Addr().String()
}

func (proxy *httpConnectTunnel) Target(t testing.TB) string {
	t.Helper()
	select {
	case target := <-proxy.targets:
		return target
	case err := <-proxy.errs:
		t.Fatalf("HTTP CONNECT proxy: %v", err)
	case <-time.After(time.Second):
		t.Fatal("HTTP CONNECT proxy did not receive a CONNECT request")
	}
	return ""
}

func (proxy *httpConnectTunnel) ProxyAuthorization(t testing.TB) string {
	t.Helper()
	select {
	case authorization := <-proxy.auth:
		return authorization
	case err := <-proxy.errs:
		t.Fatalf("HTTP CONNECT proxy: %v", err)
	case <-time.After(time.Second):
		t.Fatal("HTTP CONNECT proxy did not receive Proxy-Authorization")
	}
	return ""
}

func (proxy *httpConnectTunnel) Close(t testing.TB) {
	t.Helper()
	proxy.close.Do(func() { _ = proxy.listener.Close() })
	select {
	case <-proxy.done:
	case <-time.After(time.Second):
		t.Error("HTTP CONNECT proxy did not stop")
	}
}

func (proxy *httpConnectTunnel) serve() {
	defer close(proxy.done)
	connection, err := proxy.listener.Accept()
	if err != nil {
		if !errors.Is(err, net.ErrClosed) {
			proxy.recordError(fmt.Errorf("accept HTTP CONNECT proxy: %w", err))
		}
		return
	}
	defer connection.Close()

	reader := bufio.NewReader(connection)
	request, err := http.ReadRequest(reader)
	if err != nil {
		proxy.recordError(fmt.Errorf("read CONNECT request: %w", err))
		return
	}
	defer request.Body.Close()
	if request.Method != http.MethodConnect {
		proxy.recordError(fmt.Errorf("proxy method = %s, want CONNECT", request.Method))
		return
	}
	proxy.targets <- request.Host
	proxy.auth <- request.Header.Get("Proxy-Authorization")

	target, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", request.Host)
	if err != nil {
		proxy.recordError(fmt.Errorf("dial CONNECT target %q: %w", request.Host, err))
		return
	}
	defer target.Close()
	if _, err := io.WriteString(connection, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		proxy.recordError(fmt.Errorf("write CONNECT response: %w", err))
		return
	}

	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(target, reader)
		_ = target.Close()
		close(copyDone)
	}()
	_, _ = io.Copy(connection, target)
	_ = connection.Close()
	<-copyDone
}

type httpsConnectTunnel struct {
	server  *httptest.Server
	targets chan string
	errs    chan error
}

func newHTTPSConnectTunnel(t testing.TB) *httpsConnectTunnel {
	t.Helper()
	proxy := &httpsConnectTunnel{
		targets: make(chan string, 1),
		errs:    make(chan error, 1),
	}
	proxy.server = httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodConnect {
			proxy.recordError(fmt.Errorf("HTTPS proxy method = %s, want CONNECT", request.Method))
			http.Error(writer, "CONNECT required", http.StatusMethodNotAllowed)
			return
		}
		proxy.targets <- request.Host
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			proxy.recordError(errors.New("HTTPS proxy does not support hijacking"))
			http.Error(writer, "hijacking unavailable", http.StatusInternalServerError)
			return
		}
		connection, buffered, err := hijacker.Hijack()
		if err != nil {
			proxy.recordError(fmt.Errorf("hijack HTTPS CONNECT: %w", err))
			return
		}
		defer connection.Close()
		target, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", request.Host)
		if err != nil {
			proxy.recordError(fmt.Errorf("dial HTTPS CONNECT target %q: %w", request.Host, err))
			return
		}
		defer target.Close()
		if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			proxy.recordError(fmt.Errorf("write HTTPS CONNECT response: %w", err))
			return
		}
		if err := buffered.Flush(); err != nil {
			proxy.recordError(fmt.Errorf("flush HTTPS CONNECT response: %w", err))
			return
		}
		bridgeBrowserTunnel(connection, buffered.Reader, target)
	}))
	proxy.server.EnableHTTP2 = false
	proxy.server.StartTLS()
	return proxy
}

func (proxy *httpsConnectTunnel) URL() string { return proxy.server.URL }

func (proxy *httpsConnectTunnel) Target(t testing.TB) string {
	t.Helper()
	select {
	case target := <-proxy.targets:
		return target
	case err := <-proxy.errs:
		t.Fatalf("HTTPS CONNECT proxy: %v", err)
	case <-time.After(time.Second):
		t.Fatal("HTTPS CONNECT proxy did not receive a CONNECT request")
	}
	return ""
}

func (proxy *httpsConnectTunnel) Close() { proxy.server.Close() }

func (proxy *httpsConnectTunnel) recordError(err error) {
	select {
	case proxy.errs <- err:
	default:
	}
}

func (proxy *httpConnectTunnel) recordError(err error) {
	select {
	case proxy.errs <- err:
	default:
	}
}

type blockingThenForwardHTTPConnectTunnel struct {
	listener     net.Listener
	firstConnect chan struct{}
	errs         chan error
	done         chan struct{}
	close        sync.Once
}

func newBlockingThenForwardHTTPConnectTunnel(t testing.TB) *blockingThenForwardHTTPConnectTunnel {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen blocking HTTP CONNECT proxy: %v", err)
	}
	proxy := &blockingThenForwardHTTPConnectTunnel{
		listener:     listener,
		firstConnect: make(chan struct{}),
		errs:         make(chan error, 1),
		done:         make(chan struct{}),
	}
	go proxy.serve()
	t.Cleanup(func() { proxy.Close(t) })
	return proxy
}

func (proxy *blockingThenForwardHTTPConnectTunnel) URL() string {
	return "http://" + proxy.listener.Addr().String()
}

func (proxy *blockingThenForwardHTTPConnectTunnel) WaitForFirstConnect(t testing.TB) {
	t.Helper()
	select {
	case <-proxy.firstConnect:
	case err := <-proxy.errs:
		t.Fatalf("blocking HTTP CONNECT proxy: %v", err)
	case <-time.After(time.Second):
		t.Fatal("blocking HTTP CONNECT proxy did not receive first CONNECT")
	}
}

func (proxy *blockingThenForwardHTTPConnectTunnel) Close(t testing.TB) {
	t.Helper()
	proxy.close.Do(func() { _ = proxy.listener.Close() })
	select {
	case <-proxy.done:
	case <-time.After(time.Second):
		t.Error("blocking HTTP CONNECT proxy did not stop")
	}
}

func (proxy *blockingThenForwardHTTPConnectTunnel) serve() {
	defer close(proxy.done)
	for attempt := 0; attempt < 2; attempt++ {
		connection, err := proxy.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				proxy.recordError(fmt.Errorf("accept blocking HTTP CONNECT proxy: %w", err))
			}
			return
		}
		reader := bufio.NewReader(connection)
		request, err := http.ReadRequest(reader)
		if err != nil {
			_ = connection.Close()
			proxy.recordError(fmt.Errorf("read blocking CONNECT request: %w", err))
			return
		}
		_ = request.Body.Close()
		if request.Method != http.MethodConnect {
			_ = connection.Close()
			proxy.recordError(fmt.Errorf("blocking proxy method = %s, want CONNECT", request.Method))
			return
		}
		if attempt == 0 {
			close(proxy.firstConnect)
			_, _ = io.Copy(io.Discard, reader)
			_ = connection.Close()
			continue
		}

		target, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", request.Host)
		if err != nil {
			_ = connection.Close()
			proxy.recordError(fmt.Errorf("dial retry CONNECT target %q: %w", request.Host, err))
			return
		}
		if _, err := io.WriteString(connection, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			_ = target.Close()
			_ = connection.Close()
			proxy.recordError(fmt.Errorf("write retry CONNECT response: %w", err))
			return
		}
		bridgeBrowserTunnel(connection, reader, target)
		return
	}
}

func (proxy *blockingThenForwardHTTPConnectTunnel) recordError(err error) {
	select {
	case proxy.errs <- err:
	default:
	}
}

type browserSOCKSCapture struct {
	addressType byte
	host        string
	port        uint16
}

type browserSOCKSTunnel struct {
	listener net.Listener
	captures chan browserSOCKSCapture
	errs     chan error
	done     chan struct{}
	close    sync.Once
}

func newBrowserSOCKSTunnel(t testing.TB) *browserSOCKSTunnel {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen SOCKS browser tunnel: %v", err)
	}
	proxy := &browserSOCKSTunnel{
		listener: listener,
		captures: make(chan browserSOCKSCapture, 1),
		errs:     make(chan error, 1),
		done:     make(chan struct{}),
	}
	go proxy.serve()
	t.Cleanup(func() { proxy.Close(t) })
	return proxy
}

func (proxy *browserSOCKSTunnel) Address() string { return proxy.listener.Addr().String() }

func (proxy *browserSOCKSTunnel) Capture(t testing.TB) browserSOCKSCapture {
	t.Helper()
	select {
	case capture := <-proxy.captures:
		return capture
	case err := <-proxy.errs:
		t.Fatalf("SOCKS browser tunnel: %v", err)
	case <-time.After(time.Second):
		t.Fatal("SOCKS browser tunnel did not receive CONNECT")
	}
	return browserSOCKSCapture{}
}

func (proxy *browserSOCKSTunnel) Close(t testing.TB) {
	t.Helper()
	proxy.close.Do(func() { _ = proxy.listener.Close() })
	select {
	case <-proxy.done:
	case <-time.After(time.Second):
		t.Error("SOCKS browser tunnel did not stop")
	}
}

func (proxy *browserSOCKSTunnel) serve() {
	defer close(proxy.done)
	connection, err := proxy.listener.Accept()
	if err != nil {
		if !errors.Is(err, net.ErrClosed) {
			proxy.recordError(fmt.Errorf("accept SOCKS browser tunnel: %w", err))
		}
		return
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	capture, err := acceptBrowserSOCKSConnect(connection, reader)
	if err != nil {
		proxy.recordError(err)
		return
	}
	proxy.captures <- capture
	target, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", net.JoinHostPort(capture.host, fmt.Sprint(capture.port)))
	if err != nil {
		proxy.recordError(fmt.Errorf("dial SOCKS target %q: %w", capture.host, err))
		return
	}
	defer target.Close()
	bridgeBrowserTunnel(connection, reader, target)
}

func acceptBrowserSOCKSConnect(connection net.Conn, reader *bufio.Reader) (browserSOCKSCapture, error) {
	version, err := reader.ReadByte()
	if err != nil {
		return browserSOCKSCapture{}, err
	}
	if version != socksVersion {
		return browserSOCKSCapture{}, fmt.Errorf("SOCKS version = %d, want %d", version, socksVersion)
	}
	methodCount, err := reader.ReadByte()
	if err != nil {
		return browserSOCKSCapture{}, err
	}
	if _, err := io.ReadFull(reader, make([]byte, methodCount)); err != nil {
		return browserSOCKSCapture{}, err
	}
	if _, err := connection.Write([]byte{socksVersion, 0}); err != nil {
		return browserSOCKSCapture{}, err
	}
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return browserSOCKSCapture{}, err
	}
	if header[0] != socksVersion || header[1] != socksConnect || header[2] != 0 {
		return browserSOCKSCapture{}, fmt.Errorf("invalid SOCKS CONNECT header %v", header)
	}
	capture := browserSOCKSCapture{addressType: header[3]}
	switch capture.addressType {
	case socksAddressIPv4:
		address := make([]byte, net.IPv4len)
		if _, err := io.ReadFull(reader, address); err != nil {
			return browserSOCKSCapture{}, err
		}
		capture.host = net.IP(address).String()
	case socksAddressDomain:
		length, err := reader.ReadByte()
		if err != nil {
			return browserSOCKSCapture{}, err
		}
		address := make([]byte, length)
		if _, err := io.ReadFull(reader, address); err != nil {
			return browserSOCKSCapture{}, err
		}
		capture.host = string(address)
	default:
		return browserSOCKSCapture{}, fmt.Errorf("unexpected SOCKS address type %d", capture.addressType)
	}
	if err := binary.Read(reader, binary.BigEndian, &capture.port); err != nil {
		return browserSOCKSCapture{}, err
	}
	if _, err := connection.Write([]byte{socksVersion, 0, 0, socksAddressIPv4, 0, 0, 0, 0, 0, 0}); err != nil {
		return browserSOCKSCapture{}, err
	}
	return capture, nil
}

func bridgeBrowserTunnel(connection net.Conn, reader io.Reader, target net.Conn) {
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(target, reader)
		_ = target.Close()
		close(done)
	}()
	_, _ = io.Copy(connection, target)
	_ = connection.Close()
	<-done
}

func (proxy *browserSOCKSTunnel) recordError(err error) {
	select {
	case proxy.errs <- err:
	default:
	}
}

func chooserForProfile(t testing.TB, profile browserProfile) browserProfileChooser {
	t.Helper()
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load browser catalog: %v", err)
	}
	variantIndex := indexOfBrowserProfilePart(t, catalog.variants, profile.sourceVariant)
	operatingSystemIndex := indexOfBrowserProfilePart(t, catalog.operatingSystems, profile.sourceOperatingSystem)
	return fixedBrowserProfileChooser(operatingSystemIndex, variantIndex)
}

func indexOfBrowserProfilePart(t testing.TB, values []string, target string) int {
	t.Helper()
	for index, value := range values {
		if value == target {
			return index
		}
	}
	t.Fatalf("browser profile part %q is not in %#v", target, values)
	return 0
}
