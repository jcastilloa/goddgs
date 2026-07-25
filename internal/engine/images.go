package engine

import (
	"context"
	"math/big"
	"strings"
	"unicode"

	"github.com/jcastillo/goddgs/internal/normalize"
	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
)

const (
	bingImagesSearchURL       = "https://www.bing.com/images/async"
	bingImagesItemsXPath      = "//div[./div[@class='imgpt']/a[@m] and ./div[@class='infopt']]"
	duckDuckGoImagesHomeURL   = "https://duckduckgo.com"
	duckDuckGoImagesSearchURL = "https://duckduckgo.com/i.js"
)

// BingImages adapts frozen Bing image-search behavior.
type BingImages struct {
	transport htmlTextTransport
}

// DuckDuckGoImages adapts frozen DuckDuckGo image-search behavior.
type DuckDuckGoImages struct {
	transport htmlTextTransport
}

var _ Searcher = (*BingImages)(nil)
var _ Searcher = (*DuckDuckGoImages)(nil)

// NewBingImages constructs a Bing Images adapter.
func NewBingImages(client htmlTextTransport) *BingImages {
	return &BingImages{transport: client}
}

// NewDuckDuckGoImages constructs a DuckDuckGo Images adapter.
func NewDuckDuckGoImages(client htmlTextTransport) *DuckDuckGoImages {
	if client != nil {
		client.UpdateHeaders(duckDuckGoImagesHeaders())
	}
	return &DuckDuckGoImages{transport: client}
}

// Search runs one Bing Images search.
func (adapter *BingImages) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Bing Images", bingImagesTransport(adapter))
	if err != nil {
		return nil, err
	}

	payload, err := bingImagesPayload(request)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    bingImagesSearchURL,
		Query:  payload,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 || response.Text == "" {
		return nil, nil
	}
	return bingImagesResults(ctx, response.Text)
}

// Search runs one DuckDuckGo Images search.
func (adapter *DuckDuckGoImages) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "DuckDuckGo Images", duckDuckGoImagesTransport(adapter))
	if err != nil {
		return nil, err
	}

	payload, err := duckDuckGoImagesPayload(ctx, client, request)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    duckDuckGoImagesSearchURL,
		Query:  payload,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 || response.Text == "" {
		return nil, nil
	}
	return duckDuckGoImagesResults(response.Text)
}

func bingImagesTransport(adapter *BingImages) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func duckDuckGoImagesTransport(adapter *DuckDuckGoImages) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func duckDuckGoImagesHeaders() []transport.Field {
	return []transport.Field{
		{Name: "Accept", Value: "*/*"},
		{Name: "Accept-Language", Value: "en-US,en;q=0.5"},
		{Name: "Referer", Value: "https://duckduckgo.com/"},
		{Name: "Sec-GPC", Value: "1"},
		{Name: "Connection", Value: "keep-alive"},
		{Name: "Sec-Fetch-Dest", Value: "empty"},
		{Name: "Sec-Fetch-Mode", Value: "cors"},
		{Name: "Sec-Fetch-Site", Value: "same-origin"},
		{Name: "Priority", Value: "u=4"},
	}
}

func bingImagesPayload(request SearchRequest) ([]transport.Field, error) {
	maxResults, hasMaxResults := imageParameter(request, "max_results")
	count, err := bingImagesCount(maxResults, hasMaxResults)
	if err != nil {
		return nil, err
	}

	pageOffset := big.NewInt(int64(request.Page - 1))
	first := new(big.Int).Mul(pageOffset, count)
	first.Add(first, big.NewInt(1))
	payload := []transport.Field{
		{Name: "q", Value: request.Query},
		{Name: "async", Value: "1"},
		{Name: "first", Value: first.String()},
		{Name: "count", Value: count.String()},
	}
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		minutes, ok := map[string]string{
			"day":   "1440",
			"week":  "10080",
			"month": "44640",
			"year":  "525600",
		}[*request.TimeLimit]
		if !ok {
			return nil, sourceKeyError(*request.TimeLimit)
		}
		payload = append(payload, transport.Field{Name: "qft", Value: "filterui:age-lt" + minutes})
	}
	return payload, nil
}

func bingImagesCount(value string, present bool) (*big.Int, error) {
	if !present {
		value = "10"
	}
	count, ok := sourcePythonInteger(value)
	if !ok {
		return nil, newSourceEngineError("ValueError", "invalid literal for int() with base 10: "+sourcePythonStringRepr(value), nil)
	}
	if count.Cmp(big.NewInt(35)) < 0 {
		return big.NewInt(35), nil
	}
	return count, nil
}

func sourcePythonInteger(value string) (*big.Int, bool) {
	value = sourcePythonTrimSpace(value)
	if value == "" {
		return nil, false
	}

	sign := ""
	if value[0] == '+' || value[0] == '-' {
		sign = value[:1]
		value = value[1:]
	}
	if value == "" {
		return nil, false
	}

	var digits strings.Builder
	digits.Grow(len(value))
	previousUnderscore := false
	for _, character := range value {
		if character == '_' {
			if digits.Len() == 0 || previousUnderscore {
				return nil, false
			}
			previousUnderscore = true
			continue
		}
		digit, ok := sourceUnicodeDecimalDigit(character)
		if !ok {
			return nil, false
		}
		digits.WriteByte('0' + digit)
		previousUnderscore = false
	}
	if digits.Len() == 0 || previousUnderscore {
		return nil, false
	}
	return new(big.Int).SetString(sign+digits.String(), 10)
}

func sourcePythonSpace(value rune) bool {
	return unicode.IsSpace(value) || 0x1c <= value && value <= 0x1f
}

func sourcePythonTrimSpace(value string) string {
	return strings.TrimFunc(value, sourcePythonSpace)
}

func sourcePythonFields(value string) []string {
	return strings.FieldsFunc(value, sourcePythonSpace)
}

func sourceUnicodeDecimalDigit(value rune) (byte, bool) {
	for _, sourceRange := range unicode.Digit.R16 {
		if value < rune(sourceRange.Lo) || value > rune(sourceRange.Hi) {
			continue
		}
		delta := value - rune(sourceRange.Lo)
		if delta%rune(sourceRange.Stride) != 0 {
			continue
		}
		return byte((delta / rune(sourceRange.Stride)) % 10), true
	}
	for _, sourceRange := range unicode.Digit.R32 {
		if value < rune(sourceRange.Lo) || value > rune(sourceRange.Hi) {
			continue
		}
		delta := value - rune(sourceRange.Lo)
		if delta%rune(sourceRange.Stride) != 0 {
			continue
		}
		return byte((delta / rune(sourceRange.Stride)) % 10), true
	}
	return 0, false
}

func bingImagesResults(ctx context.Context, source string) ([]Result, error) {
	document, err := parser.ParseHTML(ctx, source)
	if err != nil {
		return nil, err
	}
	items, err := document.Extract(ctx, bingImagesItemsXPath, []parser.FieldQuery{
		{Name: "metadata", XPath: ".//a[@class='iusc']/@m"},
		{Name: "dimension", XPath: ".//div[contains(@class, 'img_info')][./span]/span[@class='nowrap']/text()"},
		{Name: "source", XPath: ".//div[@class='lnkw']//a/text()"},
	})
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(items))
	for _, item := range items {
		metadata := item.Fields[0].Raw
		if len(metadata) == 0 {
			continue
		}
		decoded, err := parser.DecodeJSON([]byte(metadata[0]))
		if err != nil {
			return nil, err
		}
		if !imageMapping(decoded) {
			return nil, sourceAttributeGetError(decoded)
		}
		updates := []Field{
			{Name: "title", Value: imageMappingValueOrNil(decoded, "t")},
			{Name: "image", Value: imageMappingValueOrNil(decoded, "murl")},
			{Name: "thumbnail", Value: imageMappingValueOrNil(decoded, "turl")},
			{Name: "url", Value: imageMappingValueOrNil(decoded, "purl")},
		}
		if len(item.Fields[1].Raw) != 0 {
			width, height, err := bingImageDimensions(item.Fields[1].Raw[0])
			if err != nil {
				return nil, err
			}
			updates = append(updates, Field{Name: "width", Value: width}, Field{Name: "height", Value: height})
		}
		if len(item.Fields[2].Raw) != 0 {
			updates = append(updates, Field{Name: "source", Value: item.Fields[2].Raw[0]})
		}
		result, err := NewCategoryResult("images", updates)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func bingImageDimensions(value string) (string, string, error) {
	parts := strings.Split(strings.ReplaceAll(value, "×", "x"), "x")
	switch len(parts) {
	case 2:
		// Continue below.
	case 0, 1:
		return "", "", newSourceEngineError("ValueError", "not enough values to unpack (expected 2, got "+sourceInteger(len(parts))+")", nil)
	default:
		return "", "", newSourceEngineError("ValueError", "too many values to unpack (expected 2)", nil)
	}
	heightParts := sourcePythonFields(parts[1])
	if len(heightParts) == 0 {
		return "", "", newSourceEngineError("IndexError", "list index out of range", nil)
	}
	return sourcePythonTrimSpace(parts[0]), sourcePythonTrimSpace(heightParts[0]), nil
}

func duckDuckGoImagesPayload(ctx context.Context, client htmlTextTransport, request SearchRequest) ([]transport.Field, error) {
	timeLimit := ""
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		value, ok := map[string]string{"d": "Day", "w": "Week", "m": "Month", "y": "Year"}[*request.TimeLimit]
		if !ok {
			return nil, sourceKeyError(*request.TimeLimit)
		}
		timeLimit = "time:" + value
	}
	size := imageFilter("size", imageParameterValue(request, "size"))
	color := imageFilter("color", imageParameterValue(request, "color"))
	typeImage := imageFilter("type", imageParameterValue(request, "type_image"))
	layout := imageFilter("layout", imageParameterValue(request, "layout"))
	licenseImage := imageFilter("license", imageParameterValue(request, "license_image"))

	vqd, err := duckDuckGoImagesVQD(ctx, client, request.Query)
	if err != nil {
		return nil, err
	}
	safeSearchKey := sourceLowerText(request.SafeSearch)
	safeSearch, ok := map[string]string{"on": "1", "moderate": "1", "off": "-1"}[safeSearchKey]
	if !ok {
		return nil, sourceKeyError(safeSearchKey)
	}

	payload := []transport.Field{
		{Name: "o", Value: "json"},
		{Name: "q", Value: request.Query},
		{Name: "l", Value: request.Region},
		{Name: "vqd", Value: vqd},
		{Name: "p", Value: safeSearch},
		{Name: "ct", Value: "AT"},
	}
	if timeLimit != "" || size != "" || color != "" || typeImage != "" || layout != "" || licenseImage != "" {
		payload = append(payload, transport.Field{Name: "f", Value: strings.Join([]string{timeLimit, size, color, typeImage, layout, licenseImage}, ",")})
	}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "s", Value: sourceInteger((request.Page - 1) * 100)})
	}
	return payload, nil
}

func duckDuckGoImagesVQD(ctx context.Context, client htmlTextTransport, query string) (string, error) {
	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    duckDuckGoImagesHomeURL,
		Query:  []transport.Field{{Name: "q", Value: query}},
	})
	if err != nil {
		return "", err
	}
	return normalize.VQD(response.Content, query)
}

func imageFilter(prefix, value string) string {
	if value == "" {
		return ""
	}
	return prefix + ":" + value
}

func duckDuckGoImagesResults(source string) ([]Result, error) {
	decoded, err := parser.DecodeJSON([]byte(source))
	if err != nil {
		return nil, err
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return nil, sourceAttributeGetError(decoded)
	}
	resultsValue, hasResults := root["results"]
	if !hasResults {
		resultsValue = []any{}
	}
	if resultsValue == nil {
		return nil, newSourceEngineError("TypeError", "'NoneType' object is not iterable", nil)
	}
	items, err := duckDuckGoImageItems(resultsValue)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(items))
	for _, item := range items {
		if _, ok := item.(map[string]any); !ok {
			return nil, sourceAttributeGetError(item)
		}
		updates := make([]Field, 0, len(duckDuckGoImageResultFields))
		for _, name := range duckDuckGoImageResultFields {
			updates = append(updates, Field{Name: name, Value: imageMappingValueOrNil(item, name)})
		}
		result, err := NewCategoryResult("images", updates)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func duckDuckGoImageItems(value any) ([]any, error) {
	switch typed := value.(type) {
	case []any:
		return typed, nil
	case map[string]any:
		if len(typed) == 0 {
			return []any{}, nil
		}
		return []any{""}, nil
	case string:
		if typed == "" {
			return []any{}, nil
		}
		return []any{""}, nil
	default:
		return nil, newSourceEngineError("TypeError", "'"+sourceTypeName(value)+"' object is not iterable", nil)
	}
}

func sourceAttributeGetError(value any) error {
	return newSourceEngineError("AttributeError", "'"+sourceTypeName(value)+"' object has no attribute 'get'", nil)
}

var duckDuckGoImageResultFields = [...]string{
	"title",
	"image",
	"thumbnail",
	"url",
	"height",
	"width",
	"source",
}

func imageParameter(request SearchRequest, name string) (string, bool) {
	for index := len(request.Parameters) - 1; index >= 0; index-- {
		if request.Parameters[index].Name == name {
			return request.Parameters[index].Value, true
		}
	}
	return "", false
}

func imageParameterValue(request SearchRequest, name string) string {
	value, _ := imageParameter(request, name)
	return value
}

func imageMappingValue(value any, name string) (any, bool) {
	switch object := value.(type) {
	case map[string]any:
		result, ok := object[name]
		return result, ok
	case *parser.OrderedObject:
		return object.Value(name)
	default:
		return nil, false
	}
}

func imageMapping(value any) bool {
	switch value.(type) {
	case map[string]any, *parser.OrderedObject:
		return true
	default:
		return false
	}
}

func imageMappingValueOrNil(value any, name string) any {
	result, _ := imageMappingValue(value, name)
	return result
}
