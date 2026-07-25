package transport

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	utls "github.com/sardanioss/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// browserProfile is an immutable browser wire profile. It is deliberately
// internal: the public library promises source behavior, not a generic
// fingerprinting API.
type browserProfile struct {
	sourceVariant         string
	sourceOperatingSystem string
	clientHelloRaw        []byte
	defaultHeaders        []Field
	headerOrder           []string
	settings              []http2.Setting
	connectionWindow      uint32
	streamWindow          *uint32
	initialStreamID       uint32
	priority              *http2.PriorityParam
	pseudoOrder           []string
}

// browserRoundTripperConfig exists only to make the wire contract testable
// with a local TLS endpoint. Production callers use newBrowserRoundTripper.
type browserRoundTripperConfig struct {
	dialContext         func(context.Context, string, string) (net.Conn, error)
	profile             browserProfile
	http2ProfileFactory browserHTTP2ProfileFactory
	tlsConfig           *utls.Config
	fallback            http.RoundTripper
}

type browserRoundTripper struct {
	config browserRoundTripperConfig

	originsMu sync.Mutex
	origins   map[string]*browserOrigin
}

type browserOrigin struct {
	mu        sync.Mutex
	creating  chan struct{}
	transport http.RoundTripper
}

type browserProfileChooser func(int) (int, error)

func newBrowserRoundTripperForProfile(settings clientSettings, profile browserProfile) (*browserRoundTripper, error) {
	return newBrowserRoundTripperForProfileWithHTTP2Factory(settings, profile, nil)
}

func newBrowserRoundTripperForProfileWithHTTP2Factory(settings clientSettings, profile browserProfile, factory browserHTTP2ProfileFactory) (*browserRoundTripper, error) {
	fallback, err := newBaseRoundTripper(settings)
	if err != nil {
		return nil, err
	}
	dialContext, err := newBrowserTargetDialContext(settings)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := browserTLSConfig(settings)
	if err != nil {
		return nil, err
	}
	return newBrowserRoundTripperWithConfig(browserRoundTripperConfig{
		dialContext:         dialContext,
		profile:             profile,
		http2ProfileFactory: factory,
		fallback:            fallback,
		tlsConfig:           tlsConfig,
	}), nil
}

func chooseBrowserProfileIndex(limit int) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("browser profile selection limit %d is invalid", limit)
	}
	choice, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0, fmt.Errorf("read browser profile randomness: %w", err)
	}
	return int(choice.Int64()), nil
}

func newBrowserRoundTripperForTest(config browserRoundTripperConfig) *browserRoundTripper {
	return newBrowserRoundTripperWithConfig(config)
}

func newBrowserRoundTripperWithConfig(config browserRoundTripperConfig) *browserRoundTripper {
	if config.dialContext == nil {
		config.dialContext = (&net.Dialer{}).DialContext
	}
	if config.fallback == nil {
		config.fallback = http.DefaultTransport.(*http.Transport).Clone()
	}
	return &browserRoundTripper{config: config, origins: make(map[string]*browserOrigin)}
}

func browserTLSConfig(settings clientSettings) (*utls.Config, error) {
	config := &utls.Config{
		InsecureSkipVerify: !settings.verify, //nolint:gosec // caller-controlled source compatibility option
		NextProtos:         []string{http2.NextProtoTLS, "http/1.1"},
	}
	if settings.pemFilePath == "" {
		return config, nil
	}
	pemBytes, err := os.ReadFile(settings.pemFilePath)
	if err != nil {
		return nil, fmt.Errorf("read TLS PEM file: %w", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("TLS PEM file contains no certificates")
	}
	config.RootCAs = roots
	return config, nil
}

func (roundTripper *browserRoundTripper) origin(host string) *browserOrigin {
	roundTripper.originsMu.Lock()
	defer roundTripper.originsMu.Unlock()
	if origin := roundTripper.origins[host]; origin != nil {
		return origin
	}
	origin := &browserOrigin{}
	roundTripper.origins[host] = origin
	return origin
}

func (origin *browserOrigin) initialize(ctx context.Context, create func(context.Context) (http.RoundTripper, error)) error {
	for {
		origin.mu.Lock()
		if origin.transport != nil {
			origin.mu.Unlock()
			return nil
		}
		if wait := origin.creating; wait != nil {
			origin.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		wait := make(chan struct{})
		origin.creating = wait
		origin.mu.Unlock()

		transport, err := create(ctx)
		origin.mu.Lock()
		if err == nil {
			origin.transport = transport
		}
		origin.creating = nil
		close(wait)
		origin.mu.Unlock()
		return err
	}
}

func (origin *browserOrigin) roundTripper() http.RoundTripper {
	origin.mu.Lock()
	defer origin.mu.Unlock()
	return origin.transport
}

func (origin *browserOrigin) closeIdleConnections() {
	origin.mu.Lock()
	transport := origin.transport
	origin.mu.Unlock()
	if closer, ok := transport.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func (roundTripper *browserRoundTripper) originRoundTripper(ctx context.Context, host string) (http.RoundTripper, error) {
	origin := roundTripper.origin(host)
	if err := origin.initialize(ctx, func(ctx context.Context) (http.RoundTripper, error) {
		return roundTripper.newOriginTransport(ctx, host)
	}); err != nil {
		return nil, contextErrorOr(ctx, err)
	}
	transport := origin.roundTripper()
	if transport == nil {
		return nil, errors.New("browser origin transport is unavailable")
	}
	return transport, nil
}

func (roundTripper *browserRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errors.New("browser transport request is nil")
	}
	if err := request.Context().Err(); err != nil {
		return nil, err
	}
	if request.URL.Scheme != "https" {
		return roundTripper.config.fallback.RoundTrip(request)
	}
	request = cloneBrowserRequest(request, roundTripper.config.profile.defaultHeaders)
	transport, err := roundTripper.originRoundTripper(request.Context(), request.URL.Host)
	if err != nil {
		return nil, err
	}
	return transport.RoundTrip(request)
}

func (roundTripper *browserRoundTripper) newOriginTransport(ctx context.Context, host string) (http.RoundTripper, error) {
	connection, err := roundTripper.dialTLS(ctx, host)
	if err != nil {
		return nil, err
	}
	alpn := connection.ConnectionState().NegotiatedProtocol
	if alpn == http2.NextProtoTLS {
		factory := roundTripper.config.http2ProfileFactory
		if factory == nil {
			profile := cloneBrowserProfile(roundTripper.config.profile)
			factory = func() (browserProfile, error) { return cloneBrowserProfile(profile), nil }
		}
		return newBrowserHTTP2TransportWithProfileFactory(factory, connection, func(ctx context.Context) (*utls.UConn, error) {
			return roundTripper.dialTLS(ctx, host)
		}), nil
	}
	_ = connection.Close()
	return &http.Transport{
		DialTLSContext: func(ctx context.Context, _, address string) (net.Conn, error) {
			return roundTripper.dialTLS(ctx, address)
		},
		ForceAttemptHTTP2: false,
	}, nil
}

func (roundTripper *browserRoundTripper) dialTLS(ctx context.Context, address string) (*utls.UConn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
		address = net.JoinHostPort(address, "443")
	}
	configuration := roundTripper.config.tlsConfig.Clone()
	configuration.ServerName = host
	connection, err := roundTripper.config.dialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	uconnection, err := newBrowserTLSConnection(connection, configuration, roundTripper.config.profile)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	if err := uconnection.HandshakeContext(ctx); err != nil {
		_ = connection.Close()
		return nil, err
	}
	return uconnection, nil
}

func newBrowserTLSConnection(connection net.Conn, configuration *utls.Config, profile browserProfile) (*utls.UConn, error) {
	if len(profile.clientHelloRaw) == 0 {
		return nil, fmt.Errorf("browser profile %s/%s has no source ClientHello", profile.sourceVariant, profile.sourceOperatingSystem)
	}

	// ApplyPreset shares extension slices with its input. Parse the frozen raw
	// ClientHello for every connection so GREASE, key shares and SNI remain
	// connection-local instead of leaking state across origins or goroutines.
	specification, err := (&utls.Fingerprinter{AllowBluntMimicry: true}).RawClientHello(profile.clientHelloRaw)
	if err != nil {
		return nil, fmt.Errorf("decode source ClientHello %s/%s: %w", profile.sourceVariant, profile.sourceOperatingSystem, err)
	}
	if err := preserveSourcePaddingExtension(specification, profile.clientHelloRaw); err != nil {
		return nil, fmt.Errorf("preserve source ClientHello padding %s/%s: %w", profile.sourceVariant, profile.sourceOperatingSystem, err)
	}
	for _, extension := range specification.Extensions {
		if serverName, ok := extension.(*utls.SNIExtension); ok {
			serverName.ServerName = ""
		}
	}

	uconnection := utls.UClient(connection, configuration, utls.HelloCustom)
	if err := uconnection.ApplyPreset(specification); err != nil {
		return nil, fmt.Errorf("apply source ClientHello %s/%s: %w", profile.sourceVariant, profile.sourceOperatingSystem, err)
	}
	return uconnection, nil
}

// preserveSourcePaddingExtension keeps a captured padding extension even when
// its body is empty. uTLS otherwise converts it to BoringSSL's conditional
// padding policy, which may omit TLS extension 21 and change Safari's JA3.
func preserveSourcePaddingExtension(specification *utls.ClientHelloSpec, raw []byte) error {
	paddingLength, present, err := sourceClientHelloPaddingLength(raw)
	if err != nil || !present {
		return err
	}
	for index, extension := range specification.Extensions {
		if _, ok := extension.(*utls.UtlsPaddingExtension); ok {
			specification.Extensions[index] = &utls.UtlsPaddingExtension{PaddingLen: paddingLength, WillPad: true}
			return nil
		}
	}
	return errors.New("source contains padding extension but uTLS did not decode it")
}

func sourceClientHelloPaddingLength(raw []byte) (int, bool, error) {
	if len(raw) < 5 || raw[0] != 22 {
		return 0, false, errors.New("source ClientHello TLS record is invalid")
	}
	recordLength := int(binary.BigEndian.Uint16(raw[3:5]))
	if recordLength+5 != len(raw) {
		return 0, false, errors.New("source ClientHello TLS record length is invalid")
	}
	handshake := raw[5:]
	if len(handshake) < 4 || handshake[0] != 1 {
		return 0, false, errors.New("source ClientHello handshake is invalid")
	}
	handshakeLength := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	if handshakeLength+4 != len(handshake) {
		return 0, false, errors.New("source ClientHello handshake length is invalid")
	}

	offset := 4 + 2 + 32
	if offset >= len(handshake) {
		return 0, false, errors.New("source ClientHello session ID is missing")
	}
	offset += 1 + int(handshake[offset])
	if offset+2 > len(handshake) {
		return 0, false, errors.New("source ClientHello cipher suites are missing")
	}
	cipherSuiteLength := int(binary.BigEndian.Uint16(handshake[offset : offset+2]))
	offset += 2 + cipherSuiteLength
	if offset >= len(handshake) {
		return 0, false, errors.New("source ClientHello compression methods are missing")
	}
	offset += 1 + int(handshake[offset])
	if offset+2 > len(handshake) {
		return 0, false, errors.New("source ClientHello extensions are missing")
	}
	extensionLength := int(binary.BigEndian.Uint16(handshake[offset : offset+2]))
	offset += 2
	end := offset + extensionLength
	if end != len(handshake) {
		return 0, false, errors.New("source ClientHello extensions length is invalid")
	}
	for offset < end {
		if offset+4 > end {
			return 0, false, errors.New("source ClientHello extension is truncated")
		}
		extensionType := binary.BigEndian.Uint16(handshake[offset : offset+2])
		contentLength := int(binary.BigEndian.Uint16(handshake[offset+2 : offset+4]))
		offset += 4
		if offset+contentLength > end {
			return 0, false, errors.New("source ClientHello extension content is truncated")
		}
		if extensionType == 21 {
			return contentLength, true, nil
		}
		offset += contentLength
	}
	return 0, false, nil
}

func (roundTripper *browserRoundTripper) CloseIdleConnections() {
	roundTripper.originsMu.Lock()
	origins := make([]*browserOrigin, 0, len(roundTripper.origins))
	for _, origin := range roundTripper.origins {
		origins = append(origins, origin)
	}
	roundTripper.originsMu.Unlock()
	for _, origin := range origins {
		origin.closeIdleConnections()
	}
	if closer, ok := roundTripper.config.fallback.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

func cloneBrowserRequest(request *http.Request, defaults []Field) *http.Request {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	for _, field := range defaults {
		if clone.Header.Get(field.Name) == "" {
			clone.Header.Set(field.Name, field.Value)
		}
	}
	return clone
}

type browserHTTP2Transport struct {
	profileFactory browserHTTP2ProfileFactory
	dial           func(context.Context) (*utls.UConn, error)

	mu         sync.Mutex
	connection *browserHTTP2Connection
	initial    *utls.UConn
	creating   chan struct{}
}

type browserHTTP2ProfileFactory func() (browserProfile, error)

func newBrowserHTTP2Transport(profile browserProfile, initial *utls.UConn, dial func(context.Context) (*utls.UConn, error)) *browserHTTP2Transport {
	return newBrowserHTTP2TransportWithProfileFactory(func() (browserProfile, error) {
		return cloneBrowserProfile(profile), nil
	}, initial, dial)
}

func newBrowserHTTP2TransportWithProfileFactory(factory browserHTTP2ProfileFactory, initial *utls.UConn, dial func(context.Context) (*utls.UConn, error)) *browserHTTP2Transport {
	return &browserHTTP2Transport{profileFactory: factory, initial: initial, dial: dial}
}

func (transport *browserHTTP2Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	connection, err := transport.connectionFor(request.Context())
	if err != nil {
		return nil, err
	}
	response, err := connection.roundTrip(request)
	if err != nil {
		transport.invalidate(connection)
	}
	return response, err
}

func (transport *browserHTTP2Transport) connectionFor(ctx context.Context) (*browserHTTP2Connection, error) {
	for {
		transport.mu.Lock()
		if transport.connection != nil && !transport.connection.closed() {
			connection := transport.connection
			transport.mu.Unlock()
			return connection, nil
		}
		if wait := transport.creating; wait != nil {
			transport.mu.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		wait := make(chan struct{})
		transport.creating = wait
		transport.mu.Unlock()

		connection := transport.initial
		transport.initial = nil
		var err error
		if connection == nil {
			connection, err = transport.dial(ctx)
		}
		var created *browserHTTP2Connection
		if err == nil {
			profile, profileErr := transport.profileFactory()
			if profileErr != nil {
				err = profileErr
			} else {
				created, err = newBrowserHTTP2Connection(profile, connection)
			}
		}
		if err != nil && connection != nil {
			_ = connection.Close()
		}

		transport.mu.Lock()
		if err == nil {
			transport.connection = created
		}
		transport.creating = nil
		close(wait)
		transport.mu.Unlock()
		return created, err
	}
}

func (transport *browserHTTP2Transport) invalidate(connection *browserHTTP2Connection) {
	transport.mu.Lock()
	if transport.connection == connection {
		transport.connection = nil
	}
	transport.mu.Unlock()
	connection.close()
}

func (transport *browserHTTP2Transport) CloseIdleConnections() {
	transport.mu.Lock()
	connection := transport.connection
	transport.connection = nil
	transport.mu.Unlock()
	if connection != nil {
		connection.close()
	}
}

type browserHTTP2Connection struct {
	profile    browserProfile
	connection net.Conn
	framer     *http2.Framer
	encoder    *hpack.Encoder
	headerBuf  *bytes.Buffer

	gate       chan struct{}
	stateMu    sync.RWMutex
	isClosed   bool
	nextStream uint32
	sendWindow int64
	maxFrame   uint32
}

func newBrowserHTTP2Connection(profile browserProfile, connection net.Conn) (*browserHTTP2Connection, error) {
	initialStreamID := profile.initialStreamID
	if initialStreamID == 0 {
		initialStreamID = 1
	}
	client := &browserHTTP2Connection{
		profile:    profile,
		connection: connection,
		gate:       make(chan struct{}, 1),
		nextStream: initialStreamID,
		sendWindow: 65535,
		maxFrame:   16384,
		headerBuf:  new(bytes.Buffer),
	}
	client.gate <- struct{}{}
	client.encoder = hpack.NewEncoder(client.headerBuf)
	client.framer = http2.NewFramer(connection, connection)
	client.framer.ReadMetaHeaders = hpack.NewDecoder(65536, nil)
	client.framer.MaxHeaderListSize = 262144
	if _, err := io.WriteString(connection, http2.ClientPreface); err != nil {
		return nil, err
	}
	if err := client.framer.WriteSettings(profile.settings...); err != nil {
		return nil, err
	}
	if profile.connectionWindow > 0 {
		if err := client.framer.WriteWindowUpdate(0, profile.connectionWindow); err != nil {
			return nil, err
		}
	}
	return client, nil
}

func (connection *browserHTTP2Connection) roundTrip(request *http.Request) (*http.Response, error) {
	select {
	case <-request.Context().Done():
		return nil, request.Context().Err()
	case <-connection.gate:
	}
	defer func() { connection.gate <- struct{}{} }()
	if connection.closed() {
		return nil, errors.New("browser HTTP/2 connection is closed")
	}
	stopCancel := context.AfterFunc(request.Context(), connection.close)
	defer stopCancel()

	streamID := connection.nextStream
	connection.nextStream += 2
	body, err := browserRequestBody(request)
	if err != nil {
		return nil, err
	}
	if err := connection.writeHeaders(streamID, request, len(body) == 0); err != nil {
		connection.close()
		return nil, contextErrorOr(request.Context(), err)
	}
	if len(body) > 0 {
		if err := connection.writeBody(streamID, body); err != nil {
			connection.close()
			return nil, contextErrorOr(request.Context(), err)
		}
	}
	response, err := connection.readResponse(streamID, request)
	if err != nil {
		connection.close()
		return nil, contextErrorOr(request.Context(), err)
	}
	return response, nil
}

func browserRequestBody(request *http.Request) ([]byte, error) {
	if request.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(request.Body)
	closeErr := request.Body.Close()
	if err != nil {
		return nil, err
	}
	return body, closeErr
}

func contextErrorOr(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	return err
}

func (connection *browserHTTP2Connection) writeHeaders(streamID uint32, request *http.Request, endStream bool) error {
	connection.headerBuf.Reset()
	for _, pseudo := range connection.profile.pseudoOrder {
		name, value := browserPseudoHeader(pseudo, request)
		if err := connection.encoder.WriteField(hpack.HeaderField{Name: name, Value: value}); err != nil {
			return err
		}
	}
	written := make(map[string]bool)
	emit := func(name string) error {
		lower := strings.ToLower(name)
		if written[lower] || browserSkippedHTTP2Header(lower) {
			return nil
		}
		values := request.Header.Values(name)
		if len(values) == 0 {
			return nil
		}
		written[lower] = true
		for _, value := range values {
			if err := connection.encoder.WriteField(hpack.HeaderField{Name: lower, Value: value}); err != nil {
				return err
			}
		}
		return nil
	}
	for _, name := range connection.profile.headerOrder {
		if err := emit(name); err != nil {
			return err
		}
	}
	for name := range request.Header {
		if err := emit(name); err != nil {
			return err
		}
	}
	parameters := http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: connection.headerBuf.Bytes(),
		EndHeaders:    true,
		EndStream:     endStream,
	}
	if connection.profile.priority != nil {
		parameters.Priority = *connection.profile.priority
	}
	if err := connection.framer.WriteHeaders(parameters); err != nil {
		return err
	}
	if connection.profile.streamWindow != nil {
		return connection.framer.WriteWindowUpdate(streamID, *connection.profile.streamWindow)
	}
	return nil
}

func browserPseudoHeader(pseudo string, request *http.Request) (string, string) {
	switch pseudo {
	case "m", "method":
		return ":method", request.Method
	case "a", "authority":
		host := request.Host
		if host == "" {
			host = request.URL.Host
		}
		return ":authority", host
	case "s", "scheme":
		return ":scheme", "https"
	case "p", "path":
		path := request.URL.RequestURI()
		if path == "" {
			path = "/"
		}
		return ":path", path
	default:
		return "", ""
	}
}

func browserSkippedHTTP2Header(name string) bool {
	switch name {
	case "host", "connection", "proxy-connection", "keep-alive", "transfer-encoding", "upgrade":
		return true
	}
	return false
}

func (connection *browserHTTP2Connection) writeBody(streamID uint32, body []byte) error {
	for len(body) > 0 {
		size := len(body)
		if int64(size) > connection.sendWindow {
			size = int(connection.sendWindow)
		}
		if size > int(connection.maxFrame) {
			size = int(connection.maxFrame)
		}
		if size <= 0 {
			if err := connection.pump(); err != nil {
				return err
			}
			continue
		}
		endStream := size == len(body)
		if err := connection.framer.WriteData(streamID, endStream, body[:size]); err != nil {
			return err
		}
		connection.sendWindow -= int64(size)
		body = body[size:]
	}
	return nil
}

func (connection *browserHTTP2Connection) readResponse(streamID uint32, request *http.Request) (*http.Response, error) {
	var (
		headers http.Header
		status  int
		body    bytes.Buffer
	)
	for {
		frame, err := connection.framer.ReadFrame()
		if err != nil {
			return nil, err
		}
		switch frame := frame.(type) {
		case *http2.MetaHeadersFrame:
			if frame.StreamID != streamID {
				continue
			}
			headers = make(http.Header)
			for _, field := range frame.Fields {
				if field.Name == ":status" {
					status, _ = strconv.Atoi(field.Value)
				} else if !strings.HasPrefix(field.Name, ":") {
					headers.Add(field.Name, field.Value)
				}
			}
			if frame.StreamEnded() {
				return browserHTTP2Response(status, headers, &body, request), nil
			}
		case *http2.DataFrame:
			if frame.StreamID != streamID {
				continue
			}
			body.Write(frame.Data())
			if length := len(frame.Data()); length > 0 {
				if err := connection.framer.WriteWindowUpdate(0, uint32(length)); err != nil {
					return nil, err
				}
				if err := connection.framer.WriteWindowUpdate(streamID, uint32(length)); err != nil {
					return nil, err
				}
			}
			if frame.StreamEnded() {
				return browserHTTP2Response(status, headers, &body, request), nil
			}
		case *http2.RSTStreamFrame:
			if frame.StreamID == streamID {
				return nil, fmt.Errorf("browser HTTP/2 stream reset: %v", frame.ErrCode)
			}
		case *http2.GoAwayFrame:
			if streamID > frame.LastStreamID {
				return nil, fmt.Errorf("browser HTTP/2 GOAWAY: %v", frame.ErrCode)
			}
		case *http2.SettingsFrame:
			if !frame.IsAck() {
				connection.applySettings(frame)
				if err := connection.framer.WriteSettingsAck(); err != nil {
					return nil, err
				}
			}
		case *http2.PingFrame:
			if !frame.IsAck() {
				if err := connection.framer.WritePing(true, frame.Data); err != nil {
					return nil, err
				}
			}
		case *http2.WindowUpdateFrame:
			connection.sendWindow += int64(frame.Increment)
		}
	}
}

func (connection *browserHTTP2Connection) pump() error {
	frame, err := connection.framer.ReadFrame()
	if err != nil {
		return err
	}
	switch frame := frame.(type) {
	case *http2.SettingsFrame:
		if !frame.IsAck() {
			connection.applySettings(frame)
			return connection.framer.WriteSettingsAck()
		}
	case *http2.WindowUpdateFrame:
		connection.sendWindow += int64(frame.Increment)
	case *http2.PingFrame:
		if !frame.IsAck() {
			return connection.framer.WritePing(true, frame.Data)
		}
	}
	return nil
}

func (connection *browserHTTP2Connection) applySettings(frame *http2.SettingsFrame) {
	_ = frame.ForeachSetting(func(setting http2.Setting) error {
		switch setting.ID {
		case http2.SettingMaxFrameSize:
			connection.maxFrame = setting.Val
		}
		return nil
	})
}

func browserHTTP2Response(status int, headers http.Header, body *bytes.Buffer, request *http.Request) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{
		StatusCode:    status,
		Status:        strconv.Itoa(status) + " " + http.StatusText(status),
		Proto:         "HTTP/2.0",
		ProtoMajor:    2,
		ProtoMinor:    0,
		Header:        headers,
		Body:          io.NopCloser(bytes.NewReader(body.Bytes())),
		ContentLength: int64(body.Len()),
		Request:       request,
	}
}

func (connection *browserHTTP2Connection) closed() bool {
	connection.stateMu.RLock()
	defer connection.stateMu.RUnlock()
	return connection.isClosed
}

func (connection *browserHTTP2Connection) close() {
	connection.stateMu.Lock()
	if connection.isClosed {
		connection.stateMu.Unlock()
		return
	}
	connection.isClosed = true
	connection.stateMu.Unlock()
	_ = connection.connection.Close()
}
