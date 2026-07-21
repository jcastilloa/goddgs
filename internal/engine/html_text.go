package engine

import (
	"context"
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"

	"github.com/jcastillo/goddgs/internal/parser"
	"github.com/jcastillo/goddgs/internal/transport"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type htmlTextTransport interface {
	Do(context.Context, transport.Request) (transport.Response, error)
	SetCookies(string, []transport.Field) error
	UpdateHeaders([]transport.Field)
}

// Brave adapts frozen Brave HTML text behavior.
type Brave struct {
	transport htmlTextTransport
}

// Google adapts frozen Google HTML text behavior.
type Google struct {
	transport htmlTextTransport
	userAgent string
}

// Mojeek adapts frozen Mojeek HTML text behavior.
type Mojeek struct {
	transport htmlTextTransport
}

type googleUserAgentDevice struct {
	androidVersion string
	device         string
	minimumChrome  int
	maximumChrome  int
}

var googleUserAgentOnce = sync.OnceValues(newGoogleUserAgent)

const (
	braveSearchURL  = "https://search.brave.com/search"
	googleSearchURL = "https://www.google.com/search"
	mojeekSearchURL = "https://www.mojeek.com/search"
)

var _ Searcher = (*Brave)(nil)
var _ Searcher = (*Google)(nil)
var _ Searcher = (*Mojeek)(nil)
var _ htmlTextTransport = (*transport.Client)(nil)

// NewBrave constructs a Brave adapter.
func NewBrave(client htmlTextTransport) *Brave {
	return &Brave{transport: client}
}

// NewGoogle constructs a Google adapter with one process-lifetime source
// User-Agent value.
func NewGoogle(client htmlTextTransport) (*Google, error) {
	userAgent, err := googleUserAgentOnce()
	if err != nil {
		return nil, err
	}
	return newGoogleWithUserAgent(client, userAgent), nil
}

func newGoogleWithUserAgent(client htmlTextTransport, userAgent string) *Google {
	adapter := &Google{transport: client, userAgent: userAgent}
	if client != nil {
		client.UpdateHeaders([]transport.Field{{Name: "User-Agent", Value: userAgent}})
	}
	return adapter
}

// NewMojeek constructs a Mojeek adapter.
func NewMojeek(client htmlTextTransport) *Mojeek {
	return &Mojeek{transport: client}
}

// Search runs one Brave search.
func (adapter *Brave) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Brave", braveTransport(adapter))
	if err != nil {
		return nil, err
	}

	country, _, err := sourceRegionPair(request.Region, true)
	if err != nil {
		return nil, err
	}
	cookies := sourceFields(
		transport.Field{Name: country, Value: country},
		transport.Field{Name: "useLocation", Value: "0"},
	)
	if request.SafeSearch != "moderate" {
		safeSearch := "off"
		if request.SafeSearch == "on" {
			safeSearch = "strict"
		}
		cookies = sourceFields(append(cookies, transport.Field{Name: "safesearch", Value: safeSearch})...)
	}
	if err := client.SetCookies("https://search.brave.com", cookies); err != nil {
		return nil, err
	}

	payload, err := bravePayload(request)
	if err != nil {
		return nil, err
	}
	return searchHTMLText(ctx, client, transport.Request{
		Method: "GET",
		URL:    braveSearchURL,
		Query:  payload,
	}, braveItemsXPath, braveFieldQueries())
}

// Search runs one Google search.
func (adapter *Google) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Google", googleTransport(adapter))
	if err != nil {
		return nil, err
	}
	if err := client.SetCookies("google.com", []transport.Field{{Name: "CONSENT", Value: "YES+"}}); err != nil {
		return nil, err
	}

	payload, err := googlePayload(request)
	if err != nil {
		return nil, err
	}
	results, err := searchHTMLText(ctx, client, transport.Request{
		Method: "GET",
		URL:    googleSearchURL,
		Query:  payload,
	}, googleItemsXPath, googleFieldQueries())
	if err != nil || results == nil {
		return results, err
	}
	return googlePostExtractResults(results)
}

// Search runs one Mojeek search.
func (adapter *Mojeek) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "Mojeek", mojeekTransport(adapter))
	if err != nil {
		return nil, err
	}

	country, languageCode, err := sourceRegionPair(request.Region, true)
	if err != nil {
		return nil, err
	}
	if err := client.SetCookies("https://www.mojeek.com", []transport.Field{
		{Name: "arc", Value: country},
		{Name: "lb", Value: languageCode},
	}); err != nil {
		return nil, err
	}

	return searchHTMLText(ctx, client, transport.Request{
		Method: "GET",
		URL:    mojeekSearchURL,
		Query:  mojeekPayload(request),
	}, mojeekItemsXPath, mojeekFieldQueries())
}

func braveTransport(adapter *Brave) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func googleTransport(adapter *Google) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func mojeekTransport(adapter *Mojeek) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func htmlTextClientFor(ctx context.Context, name string, client htmlTextTransport) (htmlTextTransport, error) {
	if ctx == nil {
		return nil, errors.New(name + " search context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New(name + " transport is unavailable")
	}
	return client, nil
}

func bravePayload(request SearchRequest) ([]transport.Field, error) {
	payload := []transport.Field{
		{Name: "q", Value: request.Query},
		{Name: "source", Value: "web"},
	}
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		timeLimit, ok := map[string]string{"d": "pd", "w": "pw", "m": "pm", "y": "py"}[*request.TimeLimit]
		if !ok {
			return nil, sourceKeyError(*request.TimeLimit)
		}
		payload = append(payload, transport.Field{Name: "tf", Value: timeLimit})
	}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "offset", Value: sourceInteger(request.Page - 1)})
	}
	return payload, nil
}

func googlePayload(request SearchRequest) ([]transport.Field, error) {
	safeSearch, ok := map[string]string{
		"on":       "2",
		"moderate": "1",
		"off":      "0",
	}[sourceLowerText(request.SafeSearch)]
	if !ok {
		return nil, sourceKeyError(sourceLowerText(request.SafeSearch))
	}
	country, languageCode, err := sourceRegionPair(request.Region, false)
	if err != nil {
		return nil, err
	}
	payload := []transport.Field{
		{Name: "q", Value: request.Query},
		{Name: "filter", Value: safeSearch},
		{Name: "start", Value: sourceInteger((request.Page - 1) * 10)},
		{Name: "hl", Value: languageCode + "-" + sourceUpperText(country)},
		{Name: "lr", Value: "lang_" + languageCode},
		{Name: "cr", Value: "country" + sourceUpperText(country)},
	}
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		payload = append(payload, transport.Field{Name: "tbs", Value: "qdr:" + *request.TimeLimit})
	}
	return payload, nil
}

func mojeekPayload(request SearchRequest) []transport.Field {
	payload := []transport.Field{{Name: "q", Value: request.Query}}
	if request.SafeSearch == "on" {
		payload = append(payload, transport.Field{Name: "safe", Value: "1"})
	}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "s", Value: sourceInteger((request.Page-1)*10 + 1)})
	}
	return payload
}

func searchHTMLText(
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
		result, err := NewCategoryResult("text", fields)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func googlePostExtractResults(results []Result) ([]Result, error) {
	postResults := make([]Result, 0, len(results))
	for _, result := range results {
		hrefValue, _ := result.Value("href")
		href, _ := hrefValue.(string)
		if strings.HasPrefix(href, "/url?q=") {
			href = strings.Split(strings.Split(href, "?q=")[1], "&")[0]
			if err := result.set(Field{Name: "href", Value: href}); err != nil {
				return nil, err
			}
			hrefValue, _ = result.Value("href")
			href, _ = hrefValue.(string)
		}
		titleValue, _ := result.Value("title")
		title, _ := titleValue.(string)
		if title != "" && strings.HasPrefix(href, "http") {
			postResults = append(postResults, result)
		}
	}
	return postResults, nil
}

func braveFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "title", XPath: ".//div[(contains(@class,'title') or contains(@class,'sitename-container')) and position()=last()]//text()"},
		{Name: "href", XPath: ".//a[div[contains(@class, 'title')]]/@href"},
		{Name: "body", XPath: ".//div[contains(@class, 'snippet')]//div[contains(@class, 'content')]//text()"},
	}
}

func googleFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "title", XPath: ".//h3//text()"},
		{Name: "href", XPath: ".//a[.//h3]/@href"},
		{Name: "body", XPath: "./div/div[last()]//text()"},
	}
}

func mojeekFieldQueries() []parser.FieldQuery {
	return []parser.FieldQuery{
		{Name: "title", XPath: ".//h2//text()"},
		{Name: "href", XPath: ".//h2/a/@href"},
		{Name: "body", XPath: ".//p[@class='s']//text()"},
	}
}

const (
	braveItemsXPath  = "//div[@data-type='web']"
	googleItemsXPath = "//div[@data-hveid][.//h3]"
	mojeekItemsXPath = "//ul[contains(@class, 'results')]/li"
)

func sourceRegionPair(region string, lowercase bool) (string, string, error) {
	if lowercase {
		region = sourceLowerText(region)
	}
	parts := strings.Split(region, "-")
	switch len(parts) {
	case 2:
		return parts[0], parts[1], nil
	case 0, 1:
		return "", "", newSourceEngineError("ValueError", "not enough values to unpack (expected 2, got "+sourceInteger(len(parts))+")", nil)
	default:
		return "", "", newSourceEngineError("ValueError", "too many values to unpack (expected 2)", nil)
	}
}

func sourceKeyError(key string) error {
	return newSourceEngineError("KeyError", "'"+key+"'", nil)
}

func sourceLowerText(value string) string {
	return cases.Lower(language.Und).String(value)
}

func sourceUpperText(value string) string {
	return cases.Upper(language.Und).String(value)
}

func sourceFields(fields ...transport.Field) []transport.Field {
	result := make([]transport.Field, 0, len(fields))
	indexes := make(map[string]int, len(fields))
	for _, field := range fields {
		if index, exists := indexes[field.Name]; exists {
			result[index].Value = field.Value
			continue
		}
		indexes[field.Name] = len(result)
		result = append(result, field)
	}
	return result
}

func newGoogleUserAgent() (string, error) {
	deviceIndex, err := sourceRandomInt(len(googleUserAgentDevices))
	if err != nil {
		return "", fmt.Errorf("select Google Android device: %w", err)
	}
	device := googleUserAgentDevices[deviceIndex]
	chromeMajor, err := sourceRandomRange(device.minimumChrome, device.maximumChrome)
	if err != nil {
		return "", fmt.Errorf("select Google Chrome major version: %w", err)
	}
	chromeBuild, err := sourceRandomRange(1000, 9999)
	if err != nil {
		return "", fmt.Errorf("select Google Chrome build: %w", err)
	}
	chromePatch, err := sourceRandomRange(1000, 1999)
	if err != nil {
		return "", fmt.Errorf("select Google Chrome patch: %w", err)
	}
	return googleUserAgentFromValues(deviceIndex, chromeMajor, chromeBuild, chromePatch), nil
}

func sourceRandomRange(minimum, maximum int) (int, error) {
	if maximum < minimum {
		return 0, errors.New("source random range is invalid")
	}
	value, err := sourceRandomInt(maximum - minimum + 1)
	if err != nil {
		return 0, err
	}
	return minimum + value, nil
}

func sourceRandomInt(limit int) (int, error) {
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func googleUserAgentFromValues(deviceIndex, chromeMajor, chromeBuild, chromePatch int) string {
	device := googleUserAgentDevices[deviceIndex]
	return "Mozilla/5.0 (Linux; Android " + device.androidVersion + "; " + device.device + ") " +
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + sourceInteger(chromeMajor) + ".0." +
		sourceInteger(chromeBuild) + "." + sourceInteger(chromePatch) + " Mobile Safari/537.36NSTNWV"
}

var googleUserAgentDevices = [...]googleUserAgentDevice{
	{androidVersion: "5.0", device: "SM-G900P Build/LRX21T", minimumChrome: 39, maximumChrome: 60},
	{androidVersion: "6.0", device: "Nexus 5 Build/MRA58N", minimumChrome: 39, maximumChrome: 60},
	{androidVersion: "8.0", device: "Pixel 2 Build/OPD3.170816.012", minimumChrome: 39, maximumChrome: 60},
}
