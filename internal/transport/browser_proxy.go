package transport

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// newBrowserTargetDialContext returns a dialer that reaches the HTTPS target,
// not merely the proxy. browserRoundTripper applies the selected ClientHello
// after this function returns, so CONNECT and SOCKS routes preserve the same
// target-facing profile as a direct request.
func newBrowserTargetDialContext(settings clientSettings) (func(context.Context, string, string) (net.Conn, error), error) {
	if settings.proxy == nil {
		return (&net.Dialer{}).DialContext, nil
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
		return httpConnectDialContext(proxyURL, settings), nil
	case "socks5", "socks5h":
		return socksDialContext(proxyURL, proxyURL.Scheme == "socks5")
	default:
		return nil, fmt.Errorf("unsupported proxy scheme %q", proxyURL.Scheme)
	}
}

func httpConnectDialContext(proxyURL *url.URL, settings clientSettings) func(context.Context, string, string) (net.Conn, error) {
	proxyAddress := proxyDialAddress(proxyURL)
	return func(ctx context.Context, network, targetAddress string) (net.Conn, error) {
		connection, err := (&net.Dialer{}).DialContext(ctx, network, proxyAddress)
		if err != nil {
			return nil, err
		}
		stopCancel := context.AfterFunc(ctx, func() { _ = connection.Close() })
		defer stopCancel()

		if proxyURL.Scheme == "https" {
			connection, err = tlsProxyConnection(ctx, connection, proxyURL, settings)
			if err != nil {
				_ = connection.Close()
				return nil, err
			}
		}
		if err := writeHTTPConnect(connection, targetAddress, proxyURL); err != nil {
			_ = connection.Close()
			return nil, err
		}

		reader := bufio.NewReader(connection)
		response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodConnect})
		if err != nil {
			_ = connection.Close()
			return nil, fmt.Errorf("read CONNECT response: %w", err)
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			content, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
			_ = connection.Close()
			return nil, fmt.Errorf("CONNECT proxy %q returned %s: %s", proxyURL.Redacted(), response.Status, strings.TrimSpace(string(content)))
		}
		return &bufferedTunnelConn{Conn: connection, reader: reader}, nil
	}
}

func proxyDialAddress(proxyURL *url.URL) string {
	if _, _, err := net.SplitHostPort(proxyURL.Host); err == nil {
		return proxyURL.Host
	}
	defaultPort := "80"
	if proxyURL.Scheme == "https" {
		defaultPort = "443"
	}
	return net.JoinHostPort(proxyURL.Hostname(), defaultPort)
}

func tlsProxyConnection(ctx context.Context, connection net.Conn, proxyURL *url.URL, settings clientSettings) (net.Conn, error) {
	configuration, err := tlsConfigFor(settings)
	if err != nil {
		return nil, err
	}
	configuration = configuration.Clone()
	configuration.ServerName = proxyURL.Hostname()
	tlsConnection := tls.Client(connection, configuration)
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		return nil, fmt.Errorf("TLS handshake with proxy %q: %w", proxyURL.Redacted(), err)
	}
	return tlsConnection, nil
}

func writeHTTPConnect(writer io.Writer, targetAddress string, proxyURL *url.URL) error {
	var request strings.Builder
	fmt.Fprintf(&request, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddress, targetAddress)
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		credential := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + password))
		fmt.Fprintf(&request, "Proxy-Authorization: Basic %s\r\n", credential)
	}
	request.WriteString("\r\n")
	_, err := io.WriteString(writer, request.String())
	return err
}

type bufferedTunnelConn struct {
	net.Conn
	reader *bufio.Reader
}

func (connection *bufferedTunnelConn) Read(buffer []byte) (int, error) {
	return connection.reader.Read(buffer)
}
