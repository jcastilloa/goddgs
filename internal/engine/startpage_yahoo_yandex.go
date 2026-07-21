package engine

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/jcastillo/goddgs/internal/normalize"
	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
)

// Startpage adapts frozen Startpage HTML text behavior.
type Startpage struct {
	transport htmlTextTransport
}

// Yahoo adapts frozen Yahoo HTML text behavior.
type Yahoo struct {
	transport    htmlTextTransport
	tokenURLSafe func(int) (string, error)
}

// Yandex adapts frozen Yandex HTML text behavior.
type Yandex struct {
	transport   htmlTextTransport
	randomRange func(int, int) (int, error)
}

const (
	startpageHomeURL   = "https://www.startpage.com/"
	startpageSearchURL = "https://www.startpage.com/sp/search"
	yahooSearchURL     = "https://search.yahoo.com/search"
	yandexSearchURL    = "https://yandex.com/search/site/"
)

var (
	_ Searcher = (*Startpage)(nil)
	_ Searcher = (*Yahoo)(nil)
	_ Searcher = (*Yandex)(nil)
)

// NewStartpage constructs a Startpage adapter.
func NewStartpage(client htmlTextTransport) *Startpage {
	adapter := &Startpage{transport: client}
	if client != nil {
		client.UpdateHeaders([]transport.Field{{Name: "Referer", Value: startpageHomeURL}})
	}
	return adapter
}

// NewYahoo constructs a Yahoo adapter.
func NewYahoo(client htmlTextTransport) *Yahoo {
	return newYahooWithTokenURLSafe(client, sourceTokenURLSafe)
}

func newYahooWithTokenURLSafe(client htmlTextTransport, tokenURLSafe func(int) (string, error)) *Yahoo {
	return &Yahoo{transport: client, tokenURLSafe: tokenURLSafe}
}

// NewYandex constructs a Yandex adapter.
func NewYandex(client htmlTextTransport) *Yandex {
	return newYandexWithRandomRange(client, sourceRandomRange)
}

func newYandexWithRandomRange(client htmlTextTransport, randomRange func(int, int) (int, error)) *Yandex {
	return &Yandex{transport: client, randomRange: randomRange}
}

// Search runs one Startpage search.
func (adapter *Startpage) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Startpage", startpageTransport(adapter))
	if err != nil {
		return nil, err
	}

	payload, err := startpagePayload(ctx, client, request)
	if err != nil {
		return nil, err
	}
	return searchHTMLText(ctx, client, transport.Request{
		Method: "POST",
		URL:    startpageSearchURL,
		Form:   payload,
	}, startpageItemsXPath, startpageFieldQueries())
}

// Search runs one Yahoo search.
func (adapter *Yahoo) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Yahoo", yahooTransport(adapter))
	if err != nil {
		return nil, err
	}
	if adapter.tokenURLSafe == nil {
		return nil, errors.New("Yahoo token source is unavailable")
	}

	searchURL, err := yahooURL(adapter.tokenURLSafe)
	if err != nil {
		return nil, err
	}
	results, err := searchHTMLText(ctx, client, transport.Request{
		Method: "GET",
		URL:    searchURL,
		Query:  yahooPayload(request),
	}, yahooItemsXPath, yahooFieldQueries())
	if err != nil || results == nil {
		return results, err
	}
	return yahooPostExtractResults(results)
}

// Search runs one Yandex search.
func (adapter *Yandex) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Yandex", yandexTransport(adapter))
	if err != nil {
		return nil, err
	}
	if adapter.randomRange == nil {
		return nil, errors.New("Yandex random source is unavailable")
	}

	searchID, err := adapter.randomRange(1_000_000, 9_999_999)
	if err != nil {
		return nil, err
	}
	return searchHTMLText(ctx, client, transport.Request{
		Method: "GET",
		URL:    yandexSearchURL,
		Query:  yandexPayload(request, searchID),
	}, yandexItemsXPath, yandexFieldQueries())
}

func startpageTransport(adapter *Startpage) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func yahooTransport(adapter *Yahoo) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func yandexTransport(adapter *Yandex) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func startpagePayload(ctx context.Context, client htmlTextTransport, request SearchRequest) ([]transport.Field, error) {
	country, languageCode, err := sourceRegionPair(request.Region, true)
	if err != nil {
		return nil, err
	}
	sc, err := startpageSC(ctx, client)
	if err != nil {
		return nil, err
	}
	safeSearchKey := sourceLowerText(request.SafeSearch)
	safeSearch, ok := map[string]string{
		"on":       "heavy",
		"moderate": "moderate",
		"off":      "none",
	}[safeSearchKey]
	if !ok {
		return nil, sourceKeyError(safeSearchKey)
	}

	payload := []transport.Field{
		{Name: "query", Value: request.Query},
		{Name: "cat", Value: "web"},
		{Name: "t", Value: "device"},
		{Name: "sc", Value: sc},
		{Name: "lui", Value: "english"},
		{Name: "language", Value: "english"},
		{Name: "abp", Value: "1"},
		{Name: "abd", Value: "0"},
		{Name: "abe", Value: "0"},
		{Name: "qsr", Value: languageCode + "_" + sourceUpperText(country)},
		{Name: "qadf", Value: safeSearch},
		{Name: "segment", Value: "organic"},
	}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "page", Value: sourceInteger(request.Page)})
	}
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		payload = append(payload, transport.Field{Name: "with_date", Value: *request.TimeLimit})
	}
	return payload, nil
}

func startpageSC(ctx context.Context, client htmlTextTransport) (string, error) {
	response, err := client.Do(ctx, transport.Request{Method: "GET", URL: startpageHomeURL})
	if err != nil {
		return "", err
	}
	if response.Text == "" {
		return "", newSourceEngineError("ParserError", "Document is empty", nil)
	}
	document, err := parser.ParseHTML(ctx, response.Text)
	if err != nil {
		return "", err
	}
	values, err := document.Values(ctx, startpageSCXPath)
	if err != nil {
		return "", err
	}
	if len(values) == 0 {
		return "", nil
	}
	return values[0], nil
}

func yahooURL(tokenURLSafe func(int) (string, error)) (string, error) {
	ylt, err := tokenURLSafe(18)
	if err != nil {
		return "", err
	}
	ylu, err := tokenURLSafe(35)
	if err != nil {
		return "", err
	}
	return yahooSearchURL + ";_ylt=" + ylt + ";_ylu=" + ylu, nil
}

func yahooPayload(request SearchRequest) []transport.Field {
	payload := []transport.Field{{Name: "p", Value: request.Query}}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "b", Value: sourceInteger((request.Page-1)*7 + 1)})
	}
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		payload = append(payload, transport.Field{Name: "btf", Value: *request.TimeLimit})
	}
	return payload
}

func yandexPayload(request SearchRequest, searchID int) []transport.Field {
	payload := []transport.Field{
		{Name: "text", Value: request.Query},
		{Name: "web", Value: "1"},
		{Name: "searchid", Value: sourceInteger(searchID)},
	}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "p", Value: sourceInteger(request.Page - 1)})
	}
	return payload
}

func yahooPostExtractResults(results []Result) ([]Result, error) {
	postResults := make([]Result, 0, len(results))
	for index := range results {
		result := &results[index]
		hrefValue, _ := result.Value("href")
		href, _ := hrefValue.(string)
		if strings.HasPrefix(href, "https://www.bing.com/aclick?") {
			continue
		}
		if strings.Contains(href, "/RU=") {
			if err := result.set(Field{Name: "href", Value: yahooExtractURL(href)}); err != nil {
				return nil, err
			}
		}
		postResults = append(postResults, *result)
	}
	return postResults, nil
}

func yahooExtractURL(value string) string {
	value = strings.SplitN(value, "/RU=", 2)[1]
	value = strings.SplitN(value, "/RK=", 2)[0]
	value = strings.SplitN(value, "/RS=", 2)[0]
	return normalize.URL(strings.ReplaceAll(value, "+", " "))
}

func sourceTokenURLSafe(nbytes int) (string, error) {
	raw := make([]byte, nbytes)
	if _, err := cryptorand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func startpageFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "title", XPath: ".//h2//text()"},
		{Name: "href", XPath: "./a/@href"},
		{Name: "body", XPath: ".//p//text()"},
	}
}

func yahooFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "title", XPath: ".//div[contains(@class, 'Title')]//h3//text()"},
		{Name: "href", XPath: ".//div[contains(@class, 'Title')]//a/@href"},
		{Name: "body", XPath: ".//div[contains(@class, 'Text')]//text()"},
	}
}

func yandexFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "title", XPath: ".//h3//text()"},
		{Name: "href", XPath: ".//h3//a/@href"},
		{Name: "body", XPath: ".//div[contains(@class, 'text')]//text()"},
	}
}

const (
	startpageItemsXPath = "//div[contains(@class, 'result')][./a]"
	startpageSCXPath    = "//form[@id=\"search\"]//input[@name=\"sc\"]/@value"
	yahooItemsXPath     = "//div[contains(@class, 'relsrch')]"
	yandexItemsXPath    = "//li[contains(@class, 'serp-item')]"
)
