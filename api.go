package ddgs

import (
	"context"
	"errors"
)

var (
	// ErrDDGS classifies a source-compatible DDGS failure.
	ErrDDGS = errors.New("ddgs error")
	// ErrTimeout classifies a source-compatible DDGS timeout.
	ErrTimeout = errors.New("ddgs timeout")
	// ErrRateLimit classifies a source-compatible DDGS rate-limit failure.
	ErrRateLimit = errors.New("ddgs rate limit")
)

var errFacadeUnavailable = errors.New("ddgs operation is not implemented")

// RawResult preserves source result fields and dynamic value types.
type RawResult map[string]any

// ExtractResult preserves source extraction URL and content. Content is a
// string for rendered or raw text and []byte for source content format.
type ExtractResult struct {
	URL     string
	Content any
}

type ddgsErrorKind uint8

const (
	ddgsErrorGeneric ddgsErrorKind = iota
	ddgsErrorTimeout
	ddgsErrorRateLimit
)

// DDGSError is a classifiable source-compatible DDGS error.
type DDGSError struct {
	kind    ddgsErrorKind
	message string
	cause   error
}

// Error returns a source-compatible error message.
func (e *DDGSError) Error() string {
	if e.message != "" {
		return e.message
	}
	return ErrDDGS.Error()
}

// Unwrap returns the underlying error, if present.
func (e *DDGSError) Unwrap() error {
	return e.cause
}

// Is reports whether target classifies this source error.
func (e *DDGSError) Is(target error) bool {
	if target == ErrDDGS {
		return true
	}
	if target == ErrTimeout {
		return e.kind == ddgsErrorTimeout
	}
	return target == ErrRateLimit && e.kind == ddgsErrorRateLimit
}

// SearchOption configures one search operation.
type SearchOption func(*searchConfig)

// ExtractOption configures one extraction operation.
type ExtractOption func(*extractConfig)

type operationExecutor interface {
	search(context.Context, searchRequest) ([]RawResult, error)
	extract(context.Context, extractRequest) (ExtractResult, error)
}

type searchCategory string

const (
	textCategory   searchCategory = "text"
	imagesCategory searchCategory = "images"
	newsCategory   searchCategory = "news"
	videosCategory searchCategory = "videos"
	booksCategory  searchCategory = "books"
)

type searchRequest struct {
	category searchCategory
	query    string
	config   searchConfig
}

type searchConfig struct {
	region     string
	safeSearch string
	timeLimit  *string
	maxResults *int
	page       int
	backend    string
}

type extractRequest struct {
	url    string
	config extractConfig
}

type extractConfig struct {
	format string
}

// WithRegion configures a source region.
func WithRegion(region string) SearchOption {
	return func(config *searchConfig) {
		config.region = region
	}
}

// WithSafeSearch configures a source safe-search mode.
func WithSafeSearch(safeSearch string) SearchOption {
	return func(config *searchConfig) {
		config.safeSearch = safeSearch
	}
}

// WithTimeLimit configures a source time limit.
func WithTimeLimit(timeLimit string) SearchOption {
	return func(config *searchConfig) {
		config.timeLimit = &timeLimit
	}
}

// WithMaxResults configures a source maximum result count.
func WithMaxResults(maxResults int) SearchOption {
	return func(config *searchConfig) {
		config.maxResults = &maxResults
	}
}

// WithoutMaxResults configures source unlimited-result semantics.
func WithoutMaxResults() SearchOption {
	return func(config *searchConfig) {
		config.maxResults = nil
	}
}

// WithPage configures a source result page.
func WithPage(page int) SearchOption {
	return func(config *searchConfig) {
		config.page = page
	}
}

// WithBackend configures one or more source backends.
func WithBackend(backend string) SearchOption {
	return func(config *searchConfig) {
		config.backend = backend
	}
}

// WithExtractFormat configures a source extraction format.
func WithExtractFormat(format string) ExtractOption {
	return func(config *extractConfig) {
		config.format = format
	}
}

// Text performs a source-compatible text search.
func (d *DDGS) Text(ctx context.Context, query string, options ...SearchOption) ([]RawResult, error) {
	return d.search(ctx, textCategory, query, options)
}

// Images performs a source-compatible image search.
func (d *DDGS) Images(ctx context.Context, query string, options ...SearchOption) ([]RawResult, error) {
	return d.search(ctx, imagesCategory, query, options)
}

// News performs a source-compatible news search.
func (d *DDGS) News(ctx context.Context, query string, options ...SearchOption) ([]RawResult, error) {
	return d.search(ctx, newsCategory, query, options)
}

// Videos performs a source-compatible video search.
func (d *DDGS) Videos(ctx context.Context, query string, options ...SearchOption) ([]RawResult, error) {
	return d.search(ctx, videosCategory, query, options)
}

// Books performs a source-compatible book search.
func (d *DDGS) Books(ctx context.Context, query string, options ...SearchOption) ([]RawResult, error) {
	return d.search(ctx, booksCategory, query, options)
}

// Extract fetches source-compatible extracted content.
func (d *DDGS) Extract(ctx context.Context, url string, options ...ExtractOption) (ExtractResult, error) {
	if err := ctx.Err(); err != nil {
		return ExtractResult{}, err
	}

	config := defaultExtractConfig()
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	if d.executor == nil {
		return ExtractResult{}, errFacadeUnavailable
	}
	return d.executor.extract(ctx, extractRequest{url: url, config: config})
}

func (d *DDGS) search(ctx context.Context, category searchCategory, query string, options []SearchOption) ([]RawResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if query == "" {
		return nil, newDDGSError(ddgsErrorGeneric, "query is mandatory.", nil)
	}

	config := defaultSearchConfig()
	for _, option := range options {
		if option != nil {
			option(&config)
		}
	}
	if d.executor == nil {
		return nil, errFacadeUnavailable
	}
	return d.executor.search(ctx, searchRequest{category: category, query: query, config: config})
}

func defaultSearchConfig() searchConfig {
	maxResults := 10
	return searchConfig{
		region:     "us-en",
		safeSearch: "moderate",
		maxResults: &maxResults,
		page:       1,
		backend:    "auto",
	}
}

func defaultExtractConfig() extractConfig {
	return extractConfig{format: "text_markdown"}
}

func newDDGSError(kind ddgsErrorKind, message string, cause error) *DDGSError {
	return &DDGSError{kind: kind, message: message, cause: cause}
}
