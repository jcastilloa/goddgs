package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// DuckDuckGoTextClient owns the isolated temporary HTTP/2 capability required
// by the frozen DuckDuckGo text engine. It deliberately does not claim the
// Python client's randomized TLS or HTTP/2 fingerprint behavior.
type DuckDuckGoTextClient struct {
	client *Client
}

// NewDuckDuckGoTextClient creates an isolated DuckDuckGo text transport.
func NewDuckDuckGoTextClient(config Config, headers []Field) (*DuckDuckGoTextClient, error) {
	return newDuckDuckGoTextClient(config, headers, nil)
}

func newDuckDuckGoTextClient(config Config, headers []Field, roundTripper http.RoundTripper) (*DuckDuckGoTextClient, error) {
	client, err := newClientWithBehavior(config, roundTripper, clientBehavior{
		followRedirects: false,
		forceHTTP2:      true,
	})
	if err != nil {
		return nil, err
	}
	client.UpdateHeaders(headers)
	return &DuckDuckGoTextClient{client: client}, nil
}

// Do executes one DuckDuckGo text request with caller cancellation.
func (client *DuckDuckGoTextClient) Do(ctx context.Context, request Request) (Response, error) {
	if client == nil || client.client == nil {
		return Response{}, errors.New("DuckDuckGo text transport is unavailable")
	}
	response, err := client.client.Do(ctx, request)
	if err != nil {
		return Response{}, classifyDuckDuckGoTextError(err)
	}
	return response, nil
}

func classifyDuckDuckGoTextError(err error) error {
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.Canceled) {
		return err
	}
	if strings.Contains(err.Error(), "timed out") {
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	}
	return err
}

// CloseIdleConnections releases this client's idle native connections.
func (client *DuckDuckGoTextClient) CloseIdleConnections() {
	if client != nil && client.client != nil {
		client.client.CloseIdleConnections()
	}
}
