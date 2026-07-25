package extract

import (
	"context"
	"errors"

	"github.com/jcastillo/goddgs/internal/transport"
)

var errUnavailable = errors.New("extract is not implemented")

// Request carries immutable source extraction inputs.
type Request struct {
	Method string
	URL    string
	Format string
	Config transport.Config
}

// Result preserves the source URL and raw-or-rendered content.
type Result struct {
	URL     string
	Content any
}

// Fetcher is the consumer-side source response boundary.
type Fetcher interface {
	Fetch(context.Context, Request) (transport.Response, error)
}

// Rendered contains source-compatible HTML renderings.
type Rendered struct {
	Markdown string
	Plain    string
	Rich     string
}

// Renderer produces rendered source representations from decoded response text.
type Renderer interface {
	Render(context.Context, string) (Rendered, error)
}

// Extractor coordinates fetching and one source-selected representation.
type Extractor struct {
	fetcher  Fetcher
	renderer Renderer
}

// New constructs an extractor over explicit fetch and rendering ports.
func New(fetcher Fetcher, renderer Renderer) *Extractor {
	return &Extractor{fetcher: fetcher, renderer: renderer}
}

// Extract retrieves one source representation.
func (e *Extractor) Extract(context.Context, Request) (Result, error) {
	return Result{}, errUnavailable
}
