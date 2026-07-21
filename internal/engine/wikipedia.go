package engine

import (
	"context"
	"strings"

	"github.com/jcastillo/goddgs/internal/normalize"
	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
)

// Search runs the frozen Wikipedia open-search and extract sequence.
func (adapter *Wikipedia) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	var client jsonTextTransport
	if adapter != nil {
		client = adapter.transport
	}
	if err := validateJSONTextSearch(ctx, client, "Wikipedia"); err != nil {
		return nil, err
	}

	language, err := wikipediaLanguage(request.Region)
	if err != nil {
		return nil, err
	}
	openSearchURL := wikipediaOpenSearchURL(language, request.Query)
	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    openSearchURL,
		Query:  []transport.Field{},
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 || response.Text == "" {
		return nil, nil
	}

	decoded, err := parser.DecodeJSON([]byte(response.Text))
	if err != nil {
		return nil, err
	}
	title, href, found, err := wikipediaOpenSearchResult(decoded)
	if err != nil {
		return nil, err
	}
	if !found {
		return []Result{}, nil
	}

	body := any("")
	response, err = client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    wikipediaExtractURL(language, normalize.Text(title)),
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode == 200 && response.Text != "" {
		body, err = wikipediaExtractBody([]byte(response.Text))
		if err != nil {
			return nil, err
		}
	}

	result, err := NewCategoryResult("text", []Field{
		{Name: "title", Value: title},
		{Name: "href", Value: href},
		{Name: "body", Value: body},
	})
	if err != nil {
		return nil, err
	}
	normalizedBody, _ := result.Value("body")
	isDisambiguation, err := wikipediaDisambiguation(normalizedBody)
	if err != nil {
		return nil, err
	}
	if isDisambiguation {
		return []Result{}, nil
	}
	return []Result{result}, nil
}

func wikipediaLanguage(region string) (string, error) {
	parts := strings.Split(strings.ToLower(region), "-")
	switch len(parts) {
	case 2:
		return parts[1], nil
	case 0, 1:
		return "", newSourceEngineError("ValueError", "not enough values to unpack (expected 2, got "+sourceInteger(len(parts))+")", nil)
	default:
		return "", newSourceEngineError("ValueError", "too many values to unpack (expected 2)", nil)
	}
}

func wikipediaOpenSearchURL(language, query string) string {
	return wikipediaAPIURL(language) + "?action=opensearch&profile=fuzzy&limit=1&search=" + wikipediaQuote(query)
}

func wikipediaExtractURL(language, title string) string {
	return wikipediaAPIURL(language) + "?action=query&format=json&prop=extracts&titles=" + wikipediaQuote(title) + "&explaintext=0&exintro=0&redirects=1"
}

func wikipediaAPIURL(language string) string {
	return "https://" + language + ".wikipedia.org/w/api.php"
}

func wikipediaQuote(value string) string {
	const hexadecimal = "0123456789ABCDEF"

	var quoted strings.Builder
	quoted.Grow(len(value))
	for _, byteValue := range []byte(value) {
		if isWikipediaQuoteSafe(byteValue) {
			quoted.WriteByte(byteValue)
			continue
		}
		quoted.WriteByte('%')
		quoted.WriteByte(hexadecimal[byteValue>>4])
		quoted.WriteByte(hexadecimal[byteValue&0x0f])
	}
	return quoted.String()
}

func isWikipediaQuoteSafe(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		strings.ContainsRune("_.-/~", rune(value))
}

func wikipediaOpenSearchResult(value any) (title, href string, found bool, err error) {
	values, ok := value.([]any)
	if !ok {
		return "", "", false, newSourceEngineError("TypeError", "'"+sourceTypeName(value)+"' object is not subscriptable", nil)
	}
	if len(values) <= 1 {
		return "", "", false, newSourceEngineError("IndexError", "list index out of range", nil)
	}
	if !sourceTruthy(values[1]) {
		return "", "", false, nil
	}
	titles, ok := values[1].([]any)
	if !ok {
		return "", "", false, newSourceEngineError("TypeError", "'"+sourceTypeName(values[1])+"' object is not subscriptable", nil)
	}
	if len(titles) == 0 {
		return "", "", false, nil
	}
	if len(values) <= 3 {
		return "", "", false, newSourceEngineError("IndexError", "list index out of range", nil)
	}
	hrefs, ok := values[3].([]any)
	if !ok {
		return "", "", false, newSourceEngineError("TypeError", "'"+sourceTypeName(values[3])+"' object is not subscriptable", nil)
	}
	if len(hrefs) == 0 {
		return "", "", false, newSourceEngineError("IndexError", "list index out of range", nil)
	}
	title, err = sourceJSONText(titles[0])
	if err != nil {
		return "", "", false, err
	}
	href, err = sourceJSONText(hrefs[0])
	if err != nil {
		return "", "", false, err
	}
	return title, href, true, nil
}

func wikipediaExtractBody(source []byte) (any, error) {
	decoded, err := parser.DecodeOrderedJSON(source)
	if err != nil {
		return "", err
	}
	root, ok := decoded.(*parser.OrderedObject)
	if !ok {
		return nil, newSourceEngineError("TypeError", "'"+sourceTypeName(decoded)+"' object is not subscriptable", nil)
	}
	query, ok := root.Value("query")
	if !ok {
		return nil, newSourceEngineError("KeyError", "'query'", nil)
	}
	queryObject, ok := query.(*parser.OrderedObject)
	if !ok {
		return nil, newSourceEngineError("TypeError", "'"+sourceTypeName(query)+"' object is not subscriptable", nil)
	}
	pages, ok := queryObject.Value("pages")
	if !ok {
		return nil, newSourceEngineError("KeyError", "'pages'", nil)
	}
	pagesObject, ok := pages.(*parser.OrderedObject)
	if !ok {
		return nil, newSourceEngineError("TypeError", "'"+sourceTypeName(pages)+"' object is not subscriptable", nil)
	}
	fields := pagesObject.Fields()
	if len(fields) == 0 {
		return nil, newSourceEngineError("StopIteration", "", nil)
	}
	page, ok := fields[0].Value.(*parser.OrderedObject)
	if !ok {
		return nil, newSourceEngineError("AttributeError", "'"+sourceTypeName(fields[0].Value)+"' object has no attribute 'get'", nil)
	}
	extract, exists := page.Value("extract")
	if !exists {
		return "", nil
	}
	return extract, nil
}

func wikipediaDisambiguation(value any) (bool, error) {
	text, ok := value.(string)
	if !ok {
		return false, newSourceEngineError("TypeError", "argument of type '"+sourceTypeName(value)+"' is not iterable", nil)
	}
	return strings.Contains(text, "may refer to:"), nil
}

func sourceJSONText(value any) (string, error) {
	text, ok := value.(string)
	if !ok {
		return "", newSourceEngineError("TypeError", "source JSON text is not a string", nil)
	}
	return text, nil
}
