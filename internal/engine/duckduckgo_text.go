package engine

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
)

const (
	duckDuckGoTextCategory     = "text"
	duckDuckGoTextSearchURL    = "https://html.duckduckgo.com/html/"
	duckDuckGoTextItemsXPath   = "//div[contains(@class, 'body')]"
	duckDuckGoTextYJSURLPrefix = "https://duckduckgo.com/y.js?"
)

type duckDuckGoTextTransport interface {
	Do(context.Context, transport.Request) (transport.Response, error)
}

// DuckDuckGoText adapts frozen DuckDuckGo HTML text behavior.
type DuckDuckGoText struct {
	transport duckDuckGoTextTransport
}

var _ Searcher = (*DuckDuckGoText)(nil)
var _ duckDuckGoTextTransport = (*transport.DuckDuckGoTextClient)(nil)

// NewDuckDuckGoText constructs a DuckDuckGo text adapter.
func NewDuckDuckGoText(client duckDuckGoTextTransport) *DuckDuckGoText {
	return &DuckDuckGoText{transport: client}
}

// Search runs one DuckDuckGo text request.
func (adapter *DuckDuckGoText) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	if ctx == nil {
		return nil, errors.New("DuckDuckGo text search context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if adapter == nil || adapter.transport == nil {
		return nil, errors.New("DuckDuckGo text transport is unavailable")
	}

	response, err := adapter.transport.Do(ctx, transport.Request{
		Method: "POST",
		URL:    duckDuckGoTextSearchURL,
		Form:   duckDuckGoTextPayload(request),
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 || response.Text == "" {
		return nil, nil
	}

	document, err := parser.ParseHTML(ctx, response.Text)
	if err != nil {
		return nil, err
	}
	items, err := document.Extract(ctx, duckDuckGoTextItemsXPath, duckDuckGoTextFieldQueries())
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(items))
	for _, item := range items {
		fields := make([]Field, len(item.Fields))
		for index, field := range item.Fields {
			fields[index] = Field{Name: field.Name, Value: field.Joined}
		}
		result, err := NewCategoryResult(duckDuckGoTextCategory, fields)
		if err != nil {
			return nil, err
		}
		if value, ok := result.Value("href"); ok {
			if href, ok := value.(string); ok && strings.HasPrefix(href, duckDuckGoTextYJSURLPrefix) {
				continue
			}
		}
		results = append(results, result)
	}
	return results, nil
}

func duckDuckGoTextFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "title", XPath: ".//h2//text()"},
		{Name: "href", XPath: "./a/@href"},
		{Name: "body", XPath: "./a//text()"},
	}
}

func duckDuckGoTextPayload(request SearchRequest) []transport.Field {
	payload := []transport.Field{
		{Name: "q", Value: request.Query},
		{Name: "b", Value: ""},
		{Name: "l", Value: request.Region},
	}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "s", Value: strconv.Itoa(10 + (request.Page-2)*15)})
	}
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		payload = append(payload, transport.Field{Name: "df", Value: *request.TimeLimit})
	}
	return payload
}
