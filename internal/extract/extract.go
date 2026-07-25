package extract

import (
	"context"
	"errors"
	"fmt"

	"github.com/jcastilloa/goddgs/internal/transport"
)

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

type idleConnectionCloser interface {
	CloseIdleConnections()
}

// Rendered contains practical HTML renderings selected by the requested format.
type Rendered struct {
	Markdown string
	Plain    string
	Rich     string
}

// Renderer produces practical HTML representations from decoded response text.
type Renderer interface {
	Render(context.Context, string) (Rendered, error)
}

type practicalRenderer struct{}

// NewPracticalRenderer creates the approved practical, non-source-identical HTML renderer.
func NewPracticalRenderer() Renderer {
	return practicalRenderer{}
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

// Extract retrieves one source-selected representation.
func (e *Extractor) Extract(ctx context.Context, request Request) (Result, error) {
	if ctx == nil {
		return Result{}, errors.New("extract context is nil")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if e == nil || e.fetcher == nil {
		return Result{}, errors.New("extract fetcher is unavailable")
	}
	if closer, ok := e.fetcher.(idleConnectionCloser); ok {
		defer closer.CloseIdleConnections()
	}

	response, err := e.fetcher.Fetch(ctx, request)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if response.StatusCode != 200 {
		return Result{}, fmt.Errorf("Failed to fetch %s: HTTP %d", request.URL, response.StatusCode)
	}

	result := Result{URL: request.URL}
	switch request.Format {
	case "content":
		result.Content = append([]byte(nil), response.Content...)
		return result, nil
	case "text":
		result.Content = response.Text
		return result, nil
	}
	if e.renderer == nil {
		return Result{}, errors.New("extract renderer is unavailable")
	}
	rendered, err := e.renderer.Render(ctx, response.Text)
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	switch request.Format {
	case "text_plain":
		result.Content = rendered.Plain
	case "text_rich":
		result.Content = rendered.Rich
	default:
		result.Content = rendered.Markdown
	}
	return result, nil
}
