package engine

import (
	"context"
	"strings"

	"github.com/jcastilloa/goddgs/internal/normalize"
	"github.com/jcastilloa/goddgs/internal/parser"
	"github.com/jcastilloa/goddgs/internal/transport"
)

const (
	duckDuckGoVideosHomeURL   = "https://duckduckgo.com"
	duckDuckGoVideosSearchURL = "https://duckduckgo.com/v.js"
)

// DuckDuckGoVideos adapts frozen DuckDuckGo video-search behavior.
type DuckDuckGoVideos struct {
	transport htmlTextTransport
}

var _ Searcher = (*DuckDuckGoVideos)(nil)

// NewDuckDuckGoVideos constructs a DuckDuckGo Videos adapter.
func NewDuckDuckGoVideos(client htmlTextTransport) *DuckDuckGoVideos {
	return &DuckDuckGoVideos{transport: client}
}

// Search runs one DuckDuckGo Videos search.
func (adapter *DuckDuckGoVideos) Search(ctx context.Context, request SearchRequest) ([]Result, error) {
	client, err := htmlTextClientFor(ctx, "DuckDuckGo Videos", duckDuckGoVideosTransport(adapter))
	if err != nil {
		return nil, err
	}

	payload, err := duckDuckGoVideosPayload(ctx, client, request)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    duckDuckGoVideosSearchURL,
		Query:  payload,
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 || response.Text == "" {
		return nil, nil
	}
	return duckDuckGoVideosResults(response.Text)
}

func duckDuckGoVideosTransport(adapter *DuckDuckGoVideos) htmlTextTransport {
	if adapter == nil {
		return nil
	}
	return adapter.transport
}

func duckDuckGoVideosPayload(ctx context.Context, client htmlTextTransport, request SearchRequest) ([]transport.Field, error) {
	vqd, err := duckDuckGoVideosVQD(ctx, client, request.Query)
	if err != nil {
		return nil, err
	}

	safeSearchKey := sourceLowerText(request.SafeSearch)
	safeSearch, ok := map[string]string{"on": "1", "moderate": "-1", "off": "-2"}[safeSearchKey]
	if !ok {
		return nil, sourceKeyError(safeSearchKey)
	}

	timeLimit := ""
	if request.TimeLimit != nil && *request.TimeLimit != "" {
		timeLimit = "publishedAfter:" + *request.TimeLimit
	}
	resolution := imageFilter("videoDefinition", sourceParameterValue(request, "resolution"))
	duration := imageFilter("videoDuration", sourceParameterValue(request, "duration"))
	license := imageFilter("videoLicense", sourceParameterValue(request, "license_videos"))
	payload := []transport.Field{
		{Name: "l", Value: request.Region},
		{Name: "o", Value: "json"},
		{Name: "q", Value: request.Query},
		{Name: "vqd", Value: vqd},
		{Name: "f", Value: strings.Join([]string{timeLimit, resolution, duration, license}, ",")},
		{Name: "p", Value: safeSearch},
	}
	if request.Page > 1 {
		payload = append(payload, transport.Field{Name: "s", Value: sourceInteger((request.Page - 1) * 60)})
	}
	return payload, nil
}

func duckDuckGoVideosVQD(ctx context.Context, client htmlTextTransport, query string) (string, error) {
	response, err := client.Do(ctx, transport.Request{
		Method: "GET",
		URL:    duckDuckGoVideosHomeURL,
		Query:  []transport.Field{{Name: "q", Value: query}},
	})
	if err != nil {
		return "", err
	}
	return normalize.VQD(response.Content, query)
}

func duckDuckGoVideosResults(source string) ([]Result, error) {
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
	items, err := duckDuckGoJSONItems(itemsValue)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(items))
	for _, item := range items {
		if _, ok := item.(map[string]any); !ok {
			return nil, sourceAttributeGetError(item)
		}
		updates := make([]Field, 0, len(duckDuckGoVideoResultFields))
		for _, name := range duckDuckGoVideoResultFields {
			updates = append(updates, Field{Name: name, Value: imageMappingValueOrNil(item, name)})
		}
		result, err := NewCategoryResult("videos", updates)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

var duckDuckGoVideoResultFields = [...]string{
	"content",
	"description",
	"duration",
	"embed_html",
	"embed_url",
	"image_token",
	"images",
	"provider",
	"published",
	"publisher",
	"statistics",
	"title",
	"uploader",
}
