// Package transport owns HTTP request construction and response lifecycle for
// source-compatible engine adapters.
package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

const sourceDefaultTimeout = 10 * time.Second

var (
	// ErrTimeout classifies a transport operation that exceeded its deadline.
	ErrTimeout = errors.New("transport timeout")
)

// Field is an ordered request field. Ordering is retained because source
// engine payloads may depend on it.
type Field struct {
	Name  string
	Value string
}

// Request is an immutable-at-call-boundary transport request value.
type Request struct {
	Method  string
	URL     string
	Query   []Field
	Form    []Field
	Headers []Field
	Cookies []Field
}

// Response is the materialized source-visible HTTP response. Content retains
// original bytes; Text is its source-compatible decoded representation.
type Response struct {
	StatusCode int
	Content    []byte
	Text       string
}

// Config configures an isolated transport client.
type Config struct {
	Proxy        *string
	Timeout      Timeout
	Verification Verification
}

// Timeout distinguishes the source default from an explicit disabled timeout.
type Timeout struct {
	set      bool
	duration *time.Duration
}

// WithTimeout returns an explicit client timeout.
func WithTimeout(duration time.Duration) Timeout {
	return Timeout{set: true, duration: durationReference(duration)}
}

// WithoutTimeout returns an explicit source-compatible disabled timeout.
func WithoutTimeout() Timeout {
	return Timeout{set: true}
}

type verificationMode uint8

const (
	verificationDefault verificationMode = iota
	verificationBoolean
	verificationPEMFile
)

// Verification selects certificate verification behavior.
type Verification struct {
	mode        verificationMode
	verify      bool
	pemFilePath string
}

// VerifyCertificates enables normal certificate verification.
func VerifyCertificates() Verification {
	return Verification{mode: verificationBoolean, verify: true}
}

// SkipCertificateVerification disables certificate verification only when the
// caller explicitly requests the source-compatible behavior.
func SkipCertificateVerification() Verification {
	return Verification{mode: verificationBoolean, verify: false}
}

// VerifyWithPEMFile enables certificate verification using the supplied PEM
// root file.
func VerifyWithPEMFile(path string) Verification {
	return Verification{mode: verificationPEMFile, verify: true, pemFilePath: path}
}

type clientSettings struct {
	proxy       *string
	timeout     *time.Duration
	verify      bool
	pemFilePath string
}

// Client owns isolated transport configuration and native HTTP state.
type Client struct {
	settings clientSettings
	jar      *cookiejar.Jar

	headersMu sync.RWMutex
	headers   http.Header

	initializeOnce sync.Once
	initializeErr  error
	httpClientMu   sync.RWMutex
	httpClient     *http.Client
}

// NewClient creates an isolated transport client from a configuration value.
func NewClient(config Config) (*Client, error) {
	return newClient(config, nil)
}

func newClient(config Config, roundTripper http.RoundTripper) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}
	client := &Client{
		settings: normalizeSettings(config),
		jar:      jar,
		headers:  make(http.Header),
	}
	if roundTripper != nil {
		client.initializeOnce.Do(func() {
			client.setHTTPClient(client.newHTTPClient(roundTripper))
		})
	}
	return client, nil
}

// Do builds a request from copied values, executes it with the caller context,
// then materializes and closes the native response body before returning.
func (client *Client) Do(ctx context.Context, sourceRequest Request) (Response, error) {
	if ctx == nil {
		return Response{}, errors.New("transport request context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if err := client.ensureHTTPClient(); err != nil {
		return Response{}, err
	}

	request, err := client.buildRequest(ctx, sourceRequest)
	if err != nil {
		return Response{}, err
	}
	httpClient := client.nativeHTTPClient()
	if httpClient == nil {
		return Response{}, errors.New("transport HTTP client is unavailable")
	}
	nativeResponse, err := httpClient.Do(request)
	if err != nil {
		return Response{}, classifyError(err)
	}
	defer nativeResponse.Body.Close()

	content, err := io.ReadAll(nativeResponse.Body)
	if err != nil {
		return Response{}, classifyError(err)
	}
	return Response{
		StatusCode: nativeResponse.StatusCode,
		Content:    content,
		Text:       strings.ToValidUTF8(string(content), "\ufffd"),
	}, nil
}

// UpdateHeaders applies source engine default headers to this client only.
func (client *Client) UpdateHeaders(fields []Field) {
	client.headersMu.Lock()
	defer client.headersMu.Unlock()
	for _, field := range fields {
		client.headers.Set(field.Name, field.Value)
	}
}

// SetCookies applies source engine cookies to this client only.
func (client *Client) SetCookies(rawURL string, fields []Field) error {
	targetURL, err := cookieTargetURL(rawURL)
	if err != nil {
		return fmt.Errorf("parse cookie URL %q: %w", rawURL, err)
	}
	cookies := make([]*http.Cookie, 0, len(fields))
	for _, field := range fields {
		cookies = append(cookies, &http.Cookie{Name: field.Name, Value: field.Value})
	}
	client.jar.SetCookies(targetURL, cookies)
	return nil
}

// CloseIdleConnections releases this client's idle base transport connections.
func (client *Client) CloseIdleConnections() {
	if httpClient := client.nativeHTTPClient(); httpClient != nil {
		httpClient.CloseIdleConnections()
	}
}

func normalizeSettings(config Config) clientSettings {
	settings := clientSettings{
		proxy:   cloneStringPointer(config.Proxy),
		timeout: durationReference(sourceDefaultTimeout),
		verify:  true,
	}
	if config.Timeout.set {
		settings.timeout = cloneDurationPointer(config.Timeout.duration)
	}
	switch config.Verification.mode {
	case verificationBoolean:
		settings.verify = config.Verification.verify
	case verificationPEMFile:
		settings.pemFilePath = config.Verification.pemFilePath
	}
	return settings
}

func (client *Client) ensureHTTPClient() error {
	client.initializeOnce.Do(func() {
		var roundTripper http.RoundTripper
		roundTripper, client.initializeErr = newBaseRoundTripper(client.settings)
		if client.initializeErr != nil {
			return
		}
		client.setHTTPClient(client.newHTTPClient(roundTripper))
	})
	return client.initializeErr
}

func (client *Client) setHTTPClient(httpClient *http.Client) {
	client.httpClientMu.Lock()
	defer client.httpClientMu.Unlock()
	client.httpClient = httpClient
}

func (client *Client) nativeHTTPClient() *http.Client {
	client.httpClientMu.RLock()
	defer client.httpClientMu.RUnlock()
	return client.httpClient
}

func (client *Client) newHTTPClient(roundTripper http.RoundTripper) *http.Client {
	httpClient := &http.Client{
		Transport: roundTripper,
		Jar:       client.jar,
	}
	if client.settings.timeout != nil {
		httpClient.Timeout = *client.settings.timeout
	}
	return httpClient
}

func (client *Client) buildRequest(ctx context.Context, sourceRequest Request) (*http.Request, error) {
	targetURL, err := url.Parse(sourceRequest.URL)
	if err != nil {
		return nil, fmt.Errorf("parse request URL %q: %w", sourceRequest.URL, err)
	}
	targetURL.RawQuery = appendEncodedFields(targetURL.RawQuery, sourceRequest.Query)

	formBody := encodeFields(sourceRequest.Form)
	request, err := http.NewRequestWithContext(ctx, sourceRequest.Method, targetURL.String(), strings.NewReader(formBody))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if len(sourceRequest.Form) > 0 {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	client.headersMu.RLock()
	for name, values := range client.headers {
		request.Header[name] = append([]string(nil), values...)
	}
	client.headersMu.RUnlock()
	for _, field := range sourceRequest.Headers {
		request.Header.Set(field.Name, field.Value)
	}
	for _, field := range sourceRequest.Cookies {
		request.AddCookie(&http.Cookie{Name: field.Name, Value: field.Value})
	}
	return request, nil
}

func appendEncodedFields(existing string, fields []Field) string {
	encoded := encodeFields(fields)
	if encoded == "" {
		return existing
	}
	if existing == "" {
		return encoded
	}
	return existing + "&" + encoded
}

func encodeFields(fields []Field) string {
	if len(fields) == 0 {
		return ""
	}
	var builder strings.Builder
	for index, field := range fields {
		if index > 0 {
			builder.WriteByte('&')
		}
		builder.WriteString(url.QueryEscape(field.Name))
		builder.WriteByte('=')
		builder.WriteString(url.QueryEscape(field.Value))
	}
	return builder.String()
}

func cookieTargetURL(rawURL string) (*url.URL, error) {
	targetURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if targetURL.Scheme == "" && targetURL.Host == "" && targetURL.Path != "" {
		targetURL, err = url.Parse("https://" + rawURL)
		if err != nil {
			return nil, err
		}
	}
	if targetURL.Scheme == "" || targetURL.Host == "" {
		return nil, errors.New("cookie URL requires scheme and host")
	}
	return targetURL, nil
}

func newBaseRoundTripper(settings clientSettings) (*http.Transport, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil

	tlsConfig, err := tlsConfigFor(settings)
	if err != nil {
		return nil, err
	}
	transport.TLSClientConfig = tlsConfig

	if settings.proxy == nil {
		return transport, nil
	}
	proxyURL, err := url.Parse(*settings.proxy)
	if err != nil {
		return nil, fmt.Errorf("parse proxy URL %q: %w", *settings.proxy, err)
	}
	if proxyURL.Host == "" {
		return nil, fmt.Errorf("proxy URL %q has no host", *settings.proxy)
	}
	switch proxyURL.Scheme {
	case "http", "https":
		transport.Proxy = http.ProxyURL(proxyURL)
	case "socks5", "socks5h":
		dialContext, err := socksDialContext(proxyURL, proxyURL.Scheme == "socks5")
		if err != nil {
			return nil, err
		}
		transport.DialContext = dialContext
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
	}
	return transport, nil
}

func tlsConfigFor(settings clientSettings) (*tls.Config, error) {
	config := &tls.Config{InsecureSkipVerify: !settings.verify} //nolint:gosec // explicit caller compatibility option
	if settings.pemFilePath == "" {
		return config, nil
	}
	pemBytes, err := os.ReadFile(settings.pemFilePath)
	if err != nil {
		return nil, fmt.Errorf("read certificate PEM %q: %w", settings.pemFilePath, err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(pemBytes) {
		return nil, fmt.Errorf("parse certificate PEM %q", settings.pemFilePath)
	}
	config.RootCAs = roots
	return config, nil
}

func socksDialContext(proxyURL *url.URL, resolveLocally bool) (func(context.Context, string, string) (net.Conn, error), error) {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		auth = &proxy.Auth{User: proxyURL.User.Username(), Password: password}
	}
	dialer, err := proxy.SOCKS5("tcp", proxyURL.Host, auth, &net.Dialer{})
	if err != nil {
		return nil, fmt.Errorf("create SOCKS dialer: %w", err)
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, errors.New("SOCKS dialer does not support context cancellation")
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		target := address
		if resolveLocally {
			resolved, err := resolveIPv4(ctx, address)
			if err != nil {
				return nil, err
			}
			target = resolved
		}
		return contextDialer.DialContext(ctx, network, target)
	}, nil
}

func resolveIPv4(ctx context.Context, address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("split SOCKS target %q: %w", address, err)
	}
	if ip := net.ParseIP(host); ip != nil {
		return address, nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", fmt.Errorf("resolve SOCKS target %q: %w", host, err)
	}
	for _, address := range addresses {
		if ipv4 := address.IP.To4(); ipv4 != nil {
			return net.JoinHostPort(ipv4.String(), port), nil
		}
	}
	return "", fmt.Errorf("resolve SOCKS target %q: no IPv4 address", host)
}

func classifyError(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	}
	return err
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneDurationPointer(value *time.Duration) *time.Duration {
	if value == nil {
		return nil
	}
	return durationReference(*value)
}

func durationReference(value time.Duration) *time.Duration {
	return &value
}
