package extract

import (
	"context"

	"github.com/jcastilloa/goddgs/internal/transport"
)

type transportFetcher struct {
	client *transport.Client
}

// NewTransportFetcher creates one isolated source transport for an extraction.
func NewTransportFetcher(config transport.Config) (Fetcher, error) {
	client, err := transport.NewClient(config)
	if err != nil {
		return nil, err
	}
	return transportFetcher{client: client}, nil
}

func (fetcher transportFetcher) Fetch(ctx context.Context, request Request) (transport.Response, error) {
	return fetcher.client.Do(ctx, transport.Request{
		Method: request.Method,
		URL:    request.URL,
	})
}

func (fetcher transportFetcher) CloseIdleConnections() {
	fetcher.client.CloseIdleConnections()
}
