package engine

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jcastillo/goddgs/internal/normalize"
	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
)

const (
	bingNewsSearchURL       = "https://www.bing.com/news/infinitescrollajax"
	duckDuckGoNewsHomeURL   = "https://duckduckgo.com"
	duckDuckGoNewsSearchURL = "https://duckduckgo.com/news.js"
	yahooNewsSearchURL      = "https://news.search.yahoo.com/search"
)

var (
	bingNewsRelativeDate  = regexp.MustCompile(`(?i)\b(\d+)\s*(days|tagen|jours|giorni|dias|días|дн\.|день)?\b`)
	yahooNewsRelativeDate = regexp.MustCompile(`(?i)\b(\d+)\s*(year|month|week|day|hour|minute)s?\b`)
)

// BingNews adapts frozen Bing News behavior.
type BingNews struct {
	transport htmlTextTransport
	now       func() time.Time
}

// DuckDuckGoNews adapts frozen DuckDuckGo News behavior.
type DuckDuckGoNews struct {
	transport htmlTextTransport
}

// YahooNews adapts frozen Yahoo News behavior.
type YahooNews struct {
	transport htmlTextTransport
	now       func() time.Time
}

var (
	_ Searcher = (*BingNews)(nil)
	_ Searcher = (*DuckDuckGoNews)(nil)
	_ Searcher = (*YahooNews)(nil)
)

// NewBingNews constructs a Bing News adapter.
func NewBingNews(client htmlTextTransport) *BingNews {
	return newBingNewsWithClock(client, func() time.Time { return time.Now().UTC() })
}

func newBingNewsWithClock(client htmlTextTransport, now func() time.Time) *BingNews {
	return &BingNews{transport: client, now: now}
}

// NewDuckDuckGoNews constructs a DuckDuckGo News adapter.
func NewDuckDuckGoNews(client htmlTextTransport) *DuckDuckGoNews {
	return &DuckDuckGoNews{transport: client}
}

// NewYahooNews constructs a Yahoo News adapter.
func NewYahooNews(client htmlTextTransport) *YahooNews {
	return newYahooNewsWithClock(client, func() time.Time { return time.Now().UTC() })
}

func newYahooNewsWithClock(client htmlTextTransport, now func() time.Time) *YahooNews {
	return &YahooNews{transport: client, now: now}
}

// Search runs one Bing News search.
func (adapter *BingNews) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Bing News", bingNewsTransport(adapter))
	if err != nil {
		return nil, err
	}
	payload, err := bingNewsPayload(request)
	if err != nil {
		return nil, err
	}
	results, err := searchHTMLNews(ctx, client, transport.Request{
		Method: "GET",
		URL:    bingNewsSearchURL,
		Query:  payload,
	}, bingNewsItemsXPath, bingNewsFieldQueries())
	if err != nil || results == nil {
		return results, err
	}
	return bingNewsPostExtractResults(results, newsNow(adapter.now))
}

// Search runs one DuckDuckGo News search.
func (adapter *DuckDuckGoNews) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "DuckDuckGo News", duckDuckGoNewsTransport(adapter))
	if err != nil {
		return nil, err
	}
	payload, err := duckDuckGoNewsPayload(ctx, client, request)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    duckDuckGoNewsSearchURL,
		Query:  payload,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 || response.Text == "" {
		return nil, nil
	}
	return duckDuckGoNewsResults(response.Text)
}

// Search runs one Yahoo News search.
func (adapter *YahooNews) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Yahoo News", yahooNewsTransport(adapter))
	if err != nil {
		return nil, err
	}
	results, err := searchHTMLNews(ctx, client, transport.Request{
		Method: "GET",
		URL:    yahooNewsSearchURL,
		Query:  yahooNewsPayload(request),
	}, yahooNewsItemsXPath, yahooNewsFieldQueries())
	if err != nil || results == nil {
		return results, err
	}
	return yahooNewsPostExtractResults(results, newsNow(adapter.now))
}

func bingNewsTransport(adapter *BingNews) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func duckDuckGoNewsTransport(adapter *DuckDuckGoNews) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func yahooNewsTransport(adapter *YahooNews) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func bingNewsPayload(request SearchRequest) ([]transport.Field, error) {
	country, languageCode, err := sourceRegionPair(request.Region, true)
	if err != nil {
		return nil, err
	}
	payload := []transport.Field{
		{Name: "q", Value: request.Query},
		{Name: "InfiniteScroll", Value: "1"},
		{Name: "first", Value: sourceInteger(request.Page*10 + 1)},
		{Name: "SFX", Value: sourceInteger(request.Page)},
		{Name: "cc", Value: country},
		{Name: "setlang", Value: languageCode},
	}
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		interval, ok := map[string]string{
			"d": `interval="4"`,
			"w": `interval="7"`,
			"m": `interval="9"`,
			"y": `interval="9"`,
		}[*request.TimeLimit]
		if !ok {
			return nil, sourceKeyError(*request.TimeLimit)
		}
		payload = append(payload, transport.Field{Name: "qft", Value: interval})
	}
	return payload, nil
}

func duckDuckGoNewsPayload(ctx context.Context, client htmlTextTransport, request SearchRequest) ([]transport.Field, error) {
	vqd, err := duckDuckGoNewsVQD(ctx, client, request.Query)
	if err != nil {
		return nil, err
	}
	safeSearchKey := sourceLowerText(request.SafeSearch)
	safeSearch, ok := map[string]string{"on": "1", "moderate": "-1", "off": "-2"}[safeSearchKey]
	if !ok {
		return nil, sourceKeyError(safeSearchKey)
	}
	payload := []transport.Field{
		{Name: "l", Value: request.Region},
		{Name: "o", Value: "json"},
		{Name: "noamp", Value: "1"},
		{Name: "q", Value: request.Query},
		{Name: "vqd", Value: vqd},
		{Name: "p", Value: safeSearch},
	}
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		payload = append(payload, transport.Field{Name: "df", Value: *request.TimeLimit})
	}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "s", Value: sourceInteger((request.Page - 1) * 30)})
	}
	return payload, nil
}

func duckDuckGoNewsVQD(ctx context.Context, client htmlTextTransport, query string) (string, error) {
	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    duckDuckGoNewsHomeURL,
		Query:  []transport.Field{{Name: "q", Value: query}},
	})
	if err != nil {
		return "", err
	}
	return normalize.VQD(response.Content, query)
}

func yahooNewsPayload(request SearchRequest) []transport.Field {
	payload := []transport.Field{{Name: "p", Value: request.Query}}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "b", Value: sourceInteger((request.Page-1)*10 + 1)})
	}
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		payload = append(payload, transport.Field{Name: "btf", Value: *request.TimeLimit})
	}
	return payload
}

func searchHTMLNews(
	ctx context.Context,
	client htmlTextTransport,
	request transport.Request,
	itemsXPath string,
	queries []parser.FieldQuery,
) ([]Result, error) {
	response, err := client.Do(ctx, request)
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
	items, err := document.Extract(ctx, itemsXPath, queries)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(items))
	for _, item := range items {
		fields := make([]Field, len(item.Fields))
		for index, field := range item.Fields {
			fields[index] = Field{Name: field.Name, Value: field.Joined}
		}
		result, err := NewCategoryResult("news", fields)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func duckDuckGoNewsResults(source string) ([]Result, error) {
	decoded, err := parser.DecodeJSON([]byte(source))
	if err != nil {
		return nil, err
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return nil, sourceAttributeGetError(decoded)
	}
	itemsValue, exists := root["results"]
	if !exists {
		itemsValue = []any{}
	}
	if itemsValue == nil {
		return nil, newSourceEngineError("TypeError", "'NoneType' object is not iterable", nil)
	}
	items, err := duckDuckGoNewsItems(itemsValue)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(items))
	for _, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return nil, sourceAttributeGetError(item)
		}
		result, err := NewCategoryResult("news", []Field{
			{Name: "date", Value: fields["date"]},
			{Name: "title", Value: fields["title"]},
			{Name: "body", Value: fields["excerpt"]},
			{Name: "url", Value: fields["url"]},
			{Name: "image", Value: fields["image"]},
			{Name: "source", Value: fields["source"]},
		})
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func duckDuckGoNewsItems(value any) ([]any, error) {
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

func bingNewsPostExtractResults(results []Result, now time.Time) ([]Result, error) {
	for index := range results {
		dateValue, _ := results[index].Value("date")
		date, _ := dateValue.(string)
		if err := results[index].set(Field{Name: "date", Value: bingNewsDate(date, now)}); err != nil {
			return nil, err
		}
		imageValue, _ := results[index].Value("image")
		image, _ := imageValue.(string)
		if image != "" {
			image = "https://www.bing.com" + strings.SplitN(image, "&", 2)[0]
		}
		if err := results[index].set(Field{Name: "image", Value: image}); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func yahooNewsPostExtractResults(results []Result, now time.Time) ([]Result, error) {
	for index := range results {
		if err := yahooNewsPostExtractResult(&results[index], now); err != nil {
			return results, nil
		}
	}
	return results, nil
}

func yahooNewsPostExtractResult(result *Result, now time.Time) error {
	date, err := resultTextValue(result, "date")
	if err != nil {
		return err
	}
	if err := result.set(Field{Name: "date", Value: yahooNewsDate(date, now)}); err != nil {
		return err
	}
	url, err := resultTextValue(result, "url")
	if err != nil {
		return err
	}
	url, err = yahooNewsURL(url)
	if err != nil {
		return err
	}
	if err := result.set(Field{Name: "url", Value: url}); err != nil {
		return err
	}
	image, err := resultTextValue(result, "image")
	if err != nil {
		return err
	}
	if marker := strings.Index(image, "-/"); marker >= 0 {
		image = image[marker+2:]
	}
	if err := result.set(Field{Name: "image", Value: image}); err != nil {
		return err
	}
	source, err := resultTextValue(result, "source")
	if err != nil {
		return err
	}
	source = strings.SplitN(source, " ·  via Yahoo", 2)[0]
	return result.set(Field{Name: "source", Value: source})
}

func resultTextValue(result *Result, name string) (string, error) {
	if result == nil {
		return "", errors.New("news result is unavailable")
	}
	value, _ := result.Value(name)
	text, ok := value.(string)
	if !ok {
		return "", errors.New("news result field is not text")
	}
	return text, nil
}

func yahooNewsURL(value string) (string, error) {
	_, value, ok := strings.Cut(value, "/RU=")
	if !ok {
		return "", errors.New("Yahoo redirect URL does not contain /RU=")
	}
	value, _, ok = strings.Cut(value, "/RK=")
	if !ok {
		return "", errors.New("Yahoo redirect URL does not contain /RK=")
	}
	value, _, _ = strings.Cut(value, "?")
	return strings.ReplaceAll(value, "+", " "), nil
}

func bingNewsDate(value string, now time.Time) string {
	for _, layout := range []string{"02.01.2006", "01/02/2006", "02/01/2006"} {
		parsed, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return parsed.UTC().Format("2006-01-02T15:04:05-07:00")
		}
	}
	match := bingNewsRelativeDate.FindStringSubmatch(value)
	if len(match) == 0 {
		return value
	}
	count, err := strconv.Atoi(match[1])
	if err != nil {
		return value
	}
	return now.UTC().AddDate(0, 0, -count).Truncate(time.Second).Format("2006-01-02T15:04:05-07:00")
}

func yahooNewsDate(value string, now time.Time) string {
	match := yahooNewsRelativeDate.FindStringSubmatch(value)
	if len(match) == 0 {
		return value
	}
	count, err := strconv.Atoi(match[1])
	if err != nil {
		return value
	}
	return now.UTC().Add(-yahooNewsDuration(count, match[2])).Truncate(time.Second).Format("2006-01-02T15:04:05-07:00")
}

func yahooNewsDuration(count int, unit string) time.Duration {
	switch strings.ToLower(unit) {
	case "minute":
		return time.Duration(count) * time.Minute
	case "hour":
		return time.Duration(count) * time.Hour
	case "day":
		return time.Duration(count) * 24 * time.Hour
	case "week":
		return time.Duration(count) * 7 * 24 * time.Hour
	case "month":
		return time.Duration(count) * 30 * 24 * time.Hour
	case "year":
		return time.Duration(count) * 365 * 24 * time.Hour
	default:
		return 0
	}
}

func newsNow(clock func() time.Time) time.Time {
	if clock == nil {
		return time.Now().UTC()
	}
	return clock().UTC()
}

func bingNewsFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "date", XPath: ".//span[@aria-label]//@aria-label"},
		{Name: "title", XPath: "@data-title"},
		{Name: "body", XPath: ".//div[@class='snippet']//text()"},
		{Name: "url", XPath: "@url"},
		{Name: "image", XPath: ".//a[contains(@class, 'image')]//@src"},
		{Name: "source", XPath: "@data-author"},
	}
}

func yahooNewsFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "date", XPath: ".//span[contains(@class, 'time')]//text()"},
		{Name: "title", XPath: ".//h4//text()"},
		{Name: "body", XPath: ".//p//text()"},
		{Name: "url", XPath: ".//h4/a/@href"},
		{Name: "image", XPath: "(.//img/@data-src | .//img/@src)[1]"},
		{Name: "source", XPath: ".//span[contains(@class, 'source')]//text()"},
	}
}

const (
	bingNewsItemsXPath  = "//div[contains(@class, 'newsitem')]"
	yahooNewsItemsXPath = "//div[@id='web']//li[a]"
)
