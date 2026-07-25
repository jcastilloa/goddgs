package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	utls "github.com/sardanioss/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

func TestBrowserRoundTripper_EmitsFrozenChromeHTTP2Shape(t *testing.T) {
	profile := frozenBrowserProfile(t, "chrome_146", "windows")
	server := newBrowserHTTP2CaptureServer(t, "browser fixture")
	defer server.Close()

	client := &http.Client{Transport: newBrowserRoundTripperForTest(browserRoundTripperConfig{
		profile:   profile,
		tlsConfig: &utls.Config{InsecureSkipVerify: true}, // test-only loopback certificate
	})}
	response, err := client.Get(server.URL + "/shape?needle=1")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if got, want := string(body), "browser fixture"; got != want {
		t.Fatalf("response body = %q, want %q", got, want)
	}
	if got, want := response.ProtoMajor, 2; got != want {
		t.Fatalf("response protocol major = %d, want %d", got, want)
	}

	capture := server.Capture(t)
	if got, want := capture.preface, http2.ClientPreface; got != want {
		t.Fatalf("HTTP/2 preface = %q, want %q", got, want)
	}
	if got, want := capture.settings, []http2.Setting{
		{ID: http2.SettingHeaderTableSize, Val: 65536},
		{ID: http2.SettingEnablePush, Val: 0},
		{ID: http2.SettingInitialWindowSize, Val: 6291456},
		{ID: http2.SettingMaxHeaderListSize, Val: 262144},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("HTTP/2 settings = %#v, want %#v", got, want)
	}
	if got, want := capture.connectionWindow, uint32(15663105); got != want {
		t.Fatalf("HTTP/2 connection window increment = %d, want %d", got, want)
	}
	if got, want := capture.pseudoHeaders, []string{":method", ":authority", ":scheme", ":path"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pseudo-header order = %#v, want %#v", got, want)
	}
	if got, want := capture.headerNames, profile.headerOrder; !reflect.DeepEqual(got, want) {
		t.Fatalf("header order = %#v, want %#v", got, want)
	}
	if got := capture.headers.Get("Sec-Ch-Ua"); !strings.Contains(got, `"146"`) {
		t.Fatalf("sec-ch-ua = %q, want Chrome 146 profile", got)
	}
	if got := capture.headers.Get("User-Agent"); !strings.Contains(got, "Chrome/146.0.0.0") {
		t.Fatalf("User-Agent = %q, want Chrome 146 profile", got)
	}
}

func TestBrowserRoundTripper_EmitsFrozenFirefoxBundle(t *testing.T) {
	profile := frozenBrowserProfile(t, "firefox_148", "windows")
	server := newBrowserHTTP2CaptureServer(t, "firefox fixture")
	defer server.Close()

	client := &http.Client{Transport: newBrowserRoundTripperForTest(browserRoundTripperConfig{
		profile:   profile,
		tlsConfig: &utls.Config{InsecureSkipVerify: true}, // test-only loopback certificate
	})}
	response, err := client.Get(server.URL + "/firefox?needle=1")
	if err != nil {
		select {
		case serverErr := <-server.errs:
			t.Fatalf("GET: %v (loopback server: %v)", err, serverErr)
		default:
		}
		t.Fatalf("GET: %v", err)
	}
	defer response.Body.Close()
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatalf("read response: %v", err)
	}

	capture := server.Capture(t)
	if got, want := capture.settings, profile.settings; !reflect.DeepEqual(got, want) {
		t.Fatalf("HTTP/2 settings = %#v, want %#v", got, want)
	}
	if got, want := capture.connectionWindow, profile.connectionWindow; got != want {
		t.Fatalf("connection window increment = %d, want %d", got, want)
	}
	if got, want := capture.streamWindow, profile.streamWindow; !reflect.DeepEqual(got, want) {
		t.Fatalf("stream window increment = %#v, want %#v", got, want)
	}
	if got, want := capture.streamID, profile.initialStreamID; got != want {
		t.Fatalf("initial stream ID = %d, want %d", got, want)
	}
	if got, want := capture.priority, profile.priority; !reflect.DeepEqual(got, want) {
		t.Fatalf("headers priority = %#v, want %#v", got, want)
	}
	if got, want := capture.pseudoHeaders, prefixedPseudoHeaders(profile.pseudoOrder); !reflect.DeepEqual(got, want) {
		t.Fatalf("pseudo-header order = %#v, want %#v", got, want)
	}
	if got, want := capture.headerNames, profile.headerOrder; !reflect.DeepEqual(got, want) {
		t.Fatalf("header order = %#v, want %#v", got, want)
	}
	if got := capture.headers.Get("Sec-Ch-Ua"); got != "" {
		t.Fatalf("Firefox sec-ch-ua = %q, want absent", got)
	}
	if got := capture.headers.Get("User-Agent"); !strings.Contains(got, "Firefox/148.0") {
		t.Fatalf("User-Agent = %q, want Firefox 148 profile", got)
	}
}

func TestBrowserRoundTripper_EmitsEveryFrozenSourceBundle(t *testing.T) {
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
				server := newBrowserHTTP2CaptureServer(t, "profile fixture")
				defer server.Close()

				client := &http.Client{Transport: newBrowserRoundTripperForTest(browserRoundTripperConfig{
					profile:   profile,
					tlsConfig: &utls.Config{InsecureSkipVerify: true}, // test-only loopback certificate
				})}
				response, err := client.Get(server.URL + "/profile")
				if err != nil {
					t.Fatalf("GET: %v", err)
				}
				defer response.Body.Close()
				if _, err := io.ReadAll(response.Body); err != nil {
					t.Fatalf("read response: %v", err)
				}

				assertBrowserHTTP2CaptureMatchesProfile(t, server.Capture(t), profile)
			})
		}
	}
}

func assertBrowserHTTP2CaptureMatchesProfile(t testing.TB, capture browserHTTP2Capture, profile browserProfile) {
	t.Helper()
	if got, want := capture.preface, http2.ClientPreface; got != want {
		t.Fatalf("HTTP/2 preface = %q, want %q", got, want)
	}
	if got, want := capture.settings, profile.settings; !reflect.DeepEqual(got, want) {
		t.Fatalf("HTTP/2 settings = %#v, want %#v", got, want)
	}
	if got, want := capture.connectionWindow, profile.connectionWindow; got != want {
		t.Fatalf("connection window increment = %d, want %d", got, want)
	}
	if got, want := capture.streamWindow, profile.streamWindow; !reflect.DeepEqual(got, want) {
		t.Fatalf("stream window increment = %#v, want %#v", got, want)
	}
	if got, want := capture.streamID, profile.initialStreamID; got != want {
		t.Fatalf("initial stream ID = %d, want %d", got, want)
	}
	if got, want := capture.priority, profile.priority; !reflect.DeepEqual(got, want) {
		t.Fatalf("headers priority = %#v, want %#v", got, want)
	}
	if got, want := capture.pseudoHeaders, prefixedPseudoHeaders(profile.pseudoOrder); !reflect.DeepEqual(got, want) {
		t.Fatalf("pseudo-header order = %#v, want %#v", got, want)
	}
	if got, want := capture.headerNames, profile.headerOrder; !reflect.DeepEqual(got, want) {
		t.Fatalf("header order = %#v, want %#v", got, want)
	}
	for _, header := range profile.defaultHeaders {
		if got, want := capture.headers.Get(header.Name), header.Value; got != want {
			t.Fatalf("header %s = %q, want %q", header.Name, got, want)
		}
	}
}

func frozenBrowserProfile(t testing.TB, variant, operatingSystem string) browserProfile {
	t.Helper()
	catalog, err := loadBrowserProfileCatalog()
	if err != nil {
		t.Fatalf("load browser catalog: %v", err)
	}
	for variantIndex, candidateVariant := range catalog.variants {
		if candidateVariant != variant {
			continue
		}
		for operatingSystemIndex, candidateOperatingSystem := range catalog.operatingSystems {
			if candidateOperatingSystem == operatingSystem {
				profile, err := catalog.profile(variantIndex, operatingSystemIndex)
				if err != nil {
					t.Fatalf("catalog profile %s/%s: %v", variant, operatingSystem, err)
				}
				return profile
			}
		}
	}
	t.Fatalf("browser catalog has no %s/%s", variant, operatingSystem)
	return browserProfile{}
}

func prefixedPseudoHeaders(order []string) []string {
	result := make([]string, 0, len(order))
	for _, pseudo := range order {
		switch pseudo {
		case "m":
			result = append(result, ":method")
		case "a":
			result = append(result, ":authority")
		case "s":
			result = append(result, ":scheme")
		case "p":
			result = append(result, ":path")
		default:
			result = append(result, ":"+pseudo)
		}
	}
	return result
}

func TestBrowserRoundTripper_HonorsCancellationAndClosesIdleConnection(t *testing.T) {
	profile := frozenBrowserProfile(t, "chrome_146", "windows")
	server := newBrowserHTTP2CaptureServer(t, "")
	defer server.Close()
	server.HoldResponse()

	roundTripper := newBrowserRoundTripperForTest(browserRoundTripperConfig{
		profile:   profile,
		tlsConfig: &utls.Config{InsecureSkipVerify: true}, // test-only loopback certificate
	})
	client := &http.Client{Transport: roundTripper}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/cancel", nil)
		if err == nil {
			_, err = client.Do(request)
		}
		result <- err
	}()
	server.WaitForRequest(t)
	cancel()
	select {
	case err := <-result:
		if !errorsIsContextCanceled(err) {
			t.Fatalf("cancellation error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RoundTrip did not return after cancellation")
	}
	roundTripper.CloseIdleConnections()
}

func TestBrowserRoundTripper_RetriesOriginCreationAfterCanceledHandshake(t *testing.T) {
	profile := frozenBrowserProfile(t, "chrome_146", "windows")
	server := newBrowserHTTP2CaptureServer(t, "retry fixture")
	defer server.Close()

	var calls sync.Mutex
	attempts := 0
	firstDialStarted := make(chan struct{})
	roundTripper := newBrowserRoundTripperForTest(browserRoundTripperConfig{
		profile:   profile,
		tlsConfig: &utls.Config{InsecureSkipVerify: true}, // test-only loopback certificate
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			calls.Lock()
			attempts++
			attempt := attempts
			calls.Unlock()
			if attempt == 1 {
				close(firstDialStarted)
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	})
	client := &http.Client{Transport: roundTripper}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/first", nil)
	if err != nil {
		t.Fatalf("build first request: %v", err)
	}
	firstResult := make(chan error, 1)
	go func() {
		response, doErr := client.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		firstResult <- doErr
	}()
	select {
	case <-firstDialStarted:
	case <-time.After(time.Second):
		t.Fatal("first TLS dial was not started")
	}
	cancel()
	select {
	case doErr := <-firstResult:
		if !errorsIsContextCanceled(doErr) {
			t.Fatalf("canceled first request error = %v, want context.Canceled", doErr)
		}
	case <-time.After(time.Second):
		t.Fatal("first request did not return after canceled handshake")
	}

	response, err := client.Get(server.URL + "/second")
	if err != nil {
		t.Fatalf("second GET: %v", err)
	}
	defer response.Body.Close()
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatalf("read second response: %v", err)
	}
	calls.Lock()
	gotAttempts := attempts
	calls.Unlock()
	if gotAttempts != 2 {
		t.Fatalf("dial attempts = %d, want retry after canceled initialization", gotAttempts)
	}
}

func TestBrowserRoundTripper_ReusesHTTP2ConnectionForOrigin(t *testing.T) {
	profile := frozenBrowserProfile(t, "chrome_146", "windows")
	var connections atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, request.URL.Path)
	}))
	server.EnableHTTP2 = true
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	}
	server.StartTLS()
	defer server.Close()

	roundTripper := newBrowserRoundTripperForTest(browserRoundTripperConfig{
		profile:   profile,
		tlsConfig: &utls.Config{InsecureSkipVerify: true}, // test-only loopback certificate
	})
	client := &http.Client{Transport: roundTripper}
	for _, path := range []string{"/first", "/second"} {
		response, err := client.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if closeErr != nil {
			t.Fatalf("close %s: %v", path, closeErr)
		}
		if got, want := string(body), path; got != want {
			t.Fatalf("response %s = %q, want %q", path, got, want)
		}
	}
	if got, want := connections.Load(), int32(1); got != want {
		t.Fatalf("TLS connections = %d, want %d for one origin", got, want)
	}
	roundTripper.CloseIdleConnections()
}

func TestBrowserRoundTripper_UsesChromeConnectionWindowForLargeRequestBody(t *testing.T) {
	const bodySize = 96 * 1024
	profile := frozenBrowserProfile(t, "chrome_146", "windows")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(writer, strconv.Itoa(len(body)))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	roundTripper := newBrowserRoundTripperForTest(browserRoundTripperConfig{
		profile:   profile,
		tlsConfig: &utls.Config{InsecureSkipVerify: true}, // test-only loopback certificate
	})
	client := &http.Client{Transport: roundTripper, Timeout: time.Second}
	response, err := client.Post(server.URL+"/large", "application/octet-stream", strings.NewReader(strings.Repeat("x", bodySize)))
	if err != nil {
		t.Fatalf("POST large body: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if got, want := string(body), strconv.Itoa(bodySize); got != want {
		t.Fatalf("received body size = %q, want %q", got, want)
	}
}

func errorsIsContextCanceled(err error) bool {
	return err != nil && (err == context.Canceled || strings.Contains(err.Error(), context.Canceled.Error()))
}

type browserHTTP2Capture struct {
	preface          string
	settings         []http2.Setting
	connectionWindow uint32
	streamWindow     *uint32
	streamID         uint32
	priority         *http2.PriorityParam
	pseudoHeaders    []string
	headerNames      []string
	headers          http.Header
}

type browserHTTP2CaptureServer struct {
	URL string

	listener    net.Listener
	certificate tls.Certificate
	capture     chan browserHTTP2Capture
	errs        chan error
	request     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func newBrowserHTTP2CaptureServer(t testing.TB, response string) *browserHTTP2CaptureServer {
	t.Helper()
	seed := httptest.NewUnstartedServer(http.NotFoundHandler())
	seed.StartTLS()
	certificate := seed.TLS.Certificates[0]
	seed.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &browserHTTP2CaptureServer{
		URL:         "https://" + listener.Addr().String(),
		listener:    listener,
		certificate: certificate,
		capture:     make(chan browserHTTP2Capture, 1),
		errs:        make(chan error, 1),
		request:     make(chan struct{}),
		release:     make(chan struct{}),
	}
	go server.serve(t, certificate, response)
	return server
}

func (server *browserHTTP2CaptureServer) serve(t testing.TB, certificate tls.Certificate, response string) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			server.recordError(fmt.Errorf("panic: %v", recovered))
		}
	}()
	connection, err := server.listener.Accept()
	if err != nil {
		server.recordError(err)
		return
	}
	defer connection.Close()
	tlsConnection := tls.Server(connection, &tls.Config{Certificates: []tls.Certificate{certificate}, NextProtos: []string{http2.NextProtoTLS}})
	if err := tlsConnection.Handshake(); err != nil {
		server.recordError(fmt.Errorf("TLS handshake: %w", err))
		return
	}
	preface := make([]byte, len(http2.ClientPreface))
	if _, err := io.ReadFull(tlsConnection, preface); err != nil {
		server.recordError(fmt.Errorf("read preface: %w", err))
		return
	}
	framer := http2.NewFramer(tlsConnection, tlsConnection)
	framer.ReadMetaHeaders = hpack.NewDecoder(65536, nil)
	settingsFrame, err := framer.ReadFrame()
	if err != nil {
		server.recordError(fmt.Errorf("read settings: %w", err))
		return
	}
	settings, ok := settingsFrame.(*http2.SettingsFrame)
	if !ok {
		server.recordError(fmt.Errorf("first frame = %T, want SETTINGS", settingsFrame))
		return
	}
	var observedSettings []http2.Setting
	_ = settings.ForeachSetting(func(setting http2.Setting) error {
		observedSettings = append(observedSettings, setting)
		return nil
	})
	windowFrame, err := framer.ReadFrame()
	if err != nil {
		server.recordError(fmt.Errorf("read connection window: %w", err))
		return
	}
	window, ok := windowFrame.(*http2.WindowUpdateFrame)
	if !ok || window.StreamID != 0 {
		server.recordError(fmt.Errorf("second frame = %T stream=%d, want connection WINDOW_UPDATE", windowFrame, window.StreamID))
		return
	}
	var requestFrame *http2.MetaHeadersFrame
	var streamWindow *uint32
	for requestFrame == nil {
		frame, readErr := framer.ReadFrame()
		if readErr != nil {
			server.recordError(fmt.Errorf("read request headers: %w", readErr))
			return
		}
		if value, ok := frame.(*http2.MetaHeadersFrame); ok {
			requestFrame = value
			break
		}
		if value, ok := frame.(*http2.WindowUpdateFrame); ok && value.StreamID != 0 {
			increment := value.Increment
			streamWindow = &increment
		}
	}
	if requestFrame == nil {
		server.recordError(fmt.Errorf("request headers are unavailable"))
		return
	}
	// Firefox sends its stream window update immediately after HEADERS. Drain
	// only frames available before the server writes its response.
	_ = tlsConnection.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	for streamWindow == nil {
		frame, readErr := framer.ReadFrame()
		if readErr != nil {
			break
		}
		if value, ok := frame.(*http2.WindowUpdateFrame); ok && value.StreamID == requestFrame.StreamID {
			increment := value.Increment
			streamWindow = &increment
		}
	}
	_ = tlsConnection.SetReadDeadline(time.Time{})
	headers := make(http.Header)
	pseudoHeaders := make([]string, 0, 4)
	headerNames := make([]string, 0, len(requestFrame.Fields)-4)
	for _, field := range requestFrame.Fields {
		if strings.HasPrefix(field.Name, ":") {
			pseudoHeaders = append(pseudoHeaders, field.Name)
			continue
		}
		headerNames = append(headerNames, field.Name)
		headers.Add(field.Name, field.Value)
	}
	server.capture <- browserHTTP2Capture{
		preface:          string(preface),
		settings:         observedSettings,
		connectionWindow: window.Increment,
		streamWindow:     streamWindow,
		streamID:         requestFrame.StreamID,
		priority:         headerPriority(requestFrame.HeadersFrame),
		pseudoHeaders:    pseudoHeaders,
		headerNames:      headerNames,
		headers:          headers,
	}
	close(server.request)

	if response == "" {
		<-server.release
		return
	}
	if err := framer.WriteSettings(); err != nil {
		server.recordError(fmt.Errorf("write settings: %w", err))
		return
	}
	if err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      requestFrame.StreamID,
		BlockFragment: encodeBrowserResponseHeaders(t, http.StatusOK),
		EndHeaders:    true,
		EndStream:     response == "",
	}); err != nil {
		server.recordError(fmt.Errorf("write headers: %w", err))
		return
	}
	if response != "" {
		if err := framer.WriteData(requestFrame.StreamID, true, []byte(response)); err != nil {
			server.recordError(fmt.Errorf("write data: %w", err))
		}
	}
	<-server.release
}

func headerPriority(frame *http2.HeadersFrame) *http2.PriorityParam {
	if !frame.HasPriority() {
		return nil
	}
	priority := frame.Priority
	return &priority
}

func (server *browserHTTP2CaptureServer) recordError(err error) {
	select {
	case server.errs <- err:
	default:
	}
}

func encodeBrowserResponseHeaders(t testing.TB, status int) []byte {
	t.Helper()
	var encoded bytes.Buffer
	encoder := hpack.NewEncoder(&encoded)
	if err := encoder.WriteField(hpack.HeaderField{Name: ":status", Value: strconv.Itoa(status)}); err != nil {
		t.Fatalf("encode response status: %v", err)
	}
	return encoded.Bytes()
}

func (server *browserHTTP2CaptureServer) Capture(t testing.TB) browserHTTP2Capture {
	t.Helper()
	select {
	case capture := <-server.capture:
		return capture
	case err := <-server.errs:
		t.Fatalf("browser capture server: %v", err)
		return browserHTTP2Capture{}
	case <-time.After(time.Second):
		t.Fatal("browser request capture timed out")
		return browserHTTP2Capture{}
	}
}

func (server *browserHTTP2CaptureServer) WaitForRequest(t testing.TB) {
	t.Helper()
	select {
	case <-server.request:
	case <-time.After(time.Second):
		t.Fatal("browser request was not received")
	}
}

func (server *browserHTTP2CaptureServer) HoldResponse() {}

func (server *browserHTTP2CaptureServer) Close() {
	server.releaseOnce.Do(func() { close(server.release) })
	_ = server.listener.Close()
}
