package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/jcastilloa/goddgs/internal/parser"
	"github.com/jcastilloa/goddgs/internal/transport"
)

const (
	annasArchiveSearchURLPrefix = "https://annas-archive."
	annasArchiveTLDCount        = 3
)

var annasArchiveSearchURLOnce = sync.OnceValues(newAnnasArchiveSearchURL)

// AnnasArchive adapts frozen Anna's Archive book-search behavior.
type AnnasArchive struct {
	transport htmlTextTransport
	searchURL string
}

var _ Searcher = (*AnnasArchive)(nil)

// NewAnnasArchive constructs an Anna's Archive adapter.
func NewAnnasArchive(client htmlTextTransport) (*AnnasArchive, error) {
	searchURL, err := annasArchiveSearchURLOnce()
	if err != nil {
		return nil, err
	}
	return newAnnasArchiveWithSearchURL(client, searchURL), nil
}

func newAnnasArchiveWithSearchURL(client htmlTextTransport, searchURL string) *AnnasArchive {
	return &AnnasArchive{transport: client, searchURL: searchURL}
}

// Search runs one Anna's Archive book search.
func (adapter *AnnasArchive) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Anna's Archive", annasArchiveTransport(adapter))
	if err != nil {
		return nil, err
	}
	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    adapter.searchURL,
		Query:  annasArchivePayload(request),
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 || response.Text == "" {
		return nil, nil
	}
	return annasArchiveResults(ctx, adapter.searchURL, response.Text)
}

func annasArchiveTransport(adapter *AnnasArchive) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func newAnnasArchiveSearchURL() (string, error) {
	index, err := sourceRandomInt(annasArchiveTLDCount)
	if err != nil {
		return "", fmt.Errorf("select Anna's Archive TLD: %w", err)
	}
	return annasArchiveSearchURLPrefix + annasArchiveTLD(index) + "/search", nil
}

func annasArchiveTLD(index int) string {
	switch index {
	case 0:
		return "gd"
	case 1:
		return "gl"
	default:
		return "pk"
	}
}

func annasArchivePayload(request SearchRequest) []transport.Field {
	return []transport.Field{
		{Name: "q", Value: request.Query},
		{Name: "page", Value: sourceInteger(request.Page)},
	}
}

func annasArchiveResults(ctx context.Context, searchURL, source string) ([]Result, error) {
	document, err := parser.ParseHTML(ctx, parser.RemoveCommentDelimiters(source))
	if err != nil {
		return nil, err
	}
	items, err := document.Extract(ctx, annasArchiveItemsXPath, annasArchiveFieldQueries())
	if err != nil {
		return nil, err
	}

	baseURL, _, _ := strings.Cut(searchURL, "/search")
	results := make([]Result, 0, len(items))
	for _, item := range items {
		fields := make([]Field, len(item.Fields))
		for index, field := range item.Fields {
			fields[index] = Field{Name: field.Name, Value: field.Joined}
		}
		result, err := NewCategoryResult("books", fields)
		if err != nil {
			return nil, err
		}
		url, _ := result.Value("url")
		if err := result.set(Field{Name: "url", Value: baseURL + url.(string)}); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func annasArchiveFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "title", XPath: ".//a[contains(@class, 'text-lg')]//text()"},
		{Name: "author", XPath: ".//a[span[contains(@class, 'user')]]//text()"},
		{Name: "publisher", XPath: ".//a[span[contains(@class, 'company')]]//text()"},
		{Name: "info", XPath: ".//div[contains(@class, 'text-gray-800')]/text()"},
		{Name: "url", XPath: "./a/@href"},
		{Name: "thumbnail", XPath: ".//img/@src"},
	}
}

const annasArchiveItemsXPath = "//div[contains(@class, 'record-list-outer')]/div"
