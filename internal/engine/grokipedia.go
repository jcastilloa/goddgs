package engine

import (
	"context"
	"errors"
	"strings"

	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
)

const grokipediaSearchURL = "https://grokipedia.com/api/typeahead"

// Search runs one Grokipedia request.
func (adapter *Grokipedia) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	var client jsonTextTransport
	if adapter != nil {
		client = adapter.transport
	}
	if err := validateJSONTextSearch(ctx, client, "Grokipedia"); err != nil {
		return nil, err
	}

	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    grokipediaSearchURL,
		Query: []transport.Field{
			{Name: "query", Value: request.Query},
			{Name: "limit", Value: "1"},
		},
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 || response.Text == "" {
		return nil, nil
	}

	decoded, err := parser.DecodeOrderedJSON([]byte(response.Text))
	if err != nil {
		return nil, err
	}
	root, ok := decoded.(*parser.OrderedObject)
	if !ok {
		return nil, newSourceEngineError("AttributeError", "'"+sourceTypeName(decoded)+"' object has no attribute 'get'", nil)
	}

	items, exists := root.Value("results")
	if !exists || !sourceTruthy(items) {
		return []Result{}, nil
	}
	values, ok := items.([]any)
	if !ok || len(values) == 0 {
		return nil, newSourceEngineError("TypeError", "'"+sourceTypeName(items)+"' object is not subscriptable", nil)
	}
	item, ok := values[0].(*parser.OrderedObject)
	if !ok {
		return nil, newSourceEngineError("AttributeError", "'"+sourceTypeName(values[0])+"' object has no attribute 'get'", nil)
	}

	title, err := grokipediaTitle(item)
	if err != nil {
		return nil, err
	}
	snippet, err := grokipediaSnippet(item)
	if err != nil {
		return nil, err
	}
	slug, exists := item.Value("slug")
	if !exists {
		return nil, newSourceEngineError("KeyError", "'slug'", nil)
	}

	body := snippet
	if _, tail, found := strings.Cut(snippet, "\n\n"); found {
		body = tail
	}
	return grokipediaTextResult(title, sourcePythonString(slug), body)
}

func grokipediaTextResult(title, slug, body string) ([]Result, error) {
	result, err := NewCategoryResult("text", []Field{
		{Name: "title", Value: title},
		{Name: "href", Value: "https://grokipedia.com/page/" + slug},
		{Name: "body", Value: body},
	})
	if err != nil {
		return nil, err
	}
	return []Result{result}, nil
}

func validateJSONTextSearch(ctx context.Context, client jsonTextTransport, name string) error {
	if ctx == nil {
		return errors.New(name + " search context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if client == nil {
		return errors.New(name + " transport is unavailable")
	}
	return nil
}

func grokipediaTitle(item *parser.OrderedObject) (string, error) {
	value, exists := item.Value("title")
	if !exists {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", newSourceEngineError("AttributeError", "'"+sourceTypeName(value)+"' object has no attribute 'strip'", nil)
	}
	return strings.Trim(text, "_"), nil
}

func grokipediaSnippet(item *parser.OrderedObject) (string, error) {
	value, exists := item.Value("snippet")
	if !exists {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", newSourceEngineError("TypeError", "argument of type '"+sourceTypeName(value)+"' is not iterable", nil)
	}
	return text, nil
}
