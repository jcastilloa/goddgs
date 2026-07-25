package search

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"

	"github.com/jcastilloa/goddgs/internal/engine"
)

type categoryDifferentialFixture struct {
	Input struct {
		Query string                     `json:"query"`
		Cases []categoryDifferentialCase `json:"cases"`
	} `json:"input"`
	Result struct {
		Output map[string]json.RawMessage `json:"output"`
	} `json:"result"`
}

type categoryDifferentialCase struct {
	Label      string            `json:"label"`
	Category   string            `json:"category"`
	Backend    string            `json:"backend"`
	MaxResults int               `json:"max_results"`
	Outcomes   map[string]string `json:"outcomes"`
	Region     string            `json:"region"`
	SafeSearch string            `json:"safesearch"`
	TimeLimit  *string           `json:"timelimit"`
	Page       int               `json:"page"`
	Parameters [][]string        `json:"parameters"`
}

type categoryDifferentialOutput struct {
	Selection              []selectedEngine           `json:"selection"`
	SelectionShuffleInputs [][]string                 `json:"selection_shuffle_inputs"`
	Started                []string                   `json:"started"`
	Calls                  []categoryDifferentialCall `json:"calls"`
	Identifiers            []string                   `json:"identifiers"`
	Error                  *struct {
		Type      string  `json:"type"`
		Message   string  `json:"message"`
		CauseType *string `json:"cause_type"`
	} `json:"error"`
}

func TestCategorySelectorAndScheduler_MatchFrozenDifferential(t *testing.T) {
	fixture := loadCategoryDifferentialFixture(t, "../../testdata/contracts/pure/pure.category-selector-scheduler-differential.json")

	for _, testCase := range fixture.Input.Cases {
		t.Run(testCase.Label, func(t *testing.T) {
			var shuffleInputs [][]string
			selector := NewBackendSelector(engine.FrozenRegistry().Categories(), func(keys []string) {
				shuffleInputs = append(shuffleInputs, append([]string(nil), keys...))
				reverseStrings(keys)
			})
			selection, err := selector.Select(testCase.Category, testCase.Backend)
			if err != nil {
				t.Fatalf("Select(%q, %q): %v", testCase.Category, testCase.Backend, err)
			}

			want := decodeCategoryDifferentialOutput(t, fixture.Result.Output, testCase.Label)
			if actual := projectSelection(selection); !reflect.DeepEqual(actual, want.Selection) {
				t.Fatalf("selection = %#v, want %#v", actual, want.Selection)
			}
			if !reflect.DeepEqual(shuffleInputs, want.SelectionShuffleInputs) {
				t.Fatalf("selection shuffle inputs = %#v, want %#v", shuffleInputs, want.SelectionShuffleInputs)
			}

			region, safeSearch, page := categoryDifferentialDefaults(testCase)
			started, calls, scheduled := categoryDifferentialEngines(t, testCase, selection, fixture.Input.Query)
			maxResults := testCase.MaxResults
			result, err := NewScheduler().Search(context.Background(), ScheduleRequest{
				Query:      fixture.Input.Query,
				Region:     region,
				SafeSearch: safeSearch,
				TimeLimit:  testCase.TimeLimit,
				Page:       page,
				Parameters: categoryDifferentialParameters(t, testCase.Parameters),
				MaxResults: &maxResults,
			}, scheduled)
			if want.Error != nil {
				assertCategoryDifferentialError(t, err, want.Error.Type, want.Error.Message, want.Error.CauseType)
			} else {
				if err != nil {
					t.Fatalf("Search: %v", err)
				}
				if actual := categoryDifferentialIdentifiers(t, result, testCase.Category); !reflect.DeepEqual(actual, want.Identifiers) {
					t.Fatalf("identifiers = %#v, want %#v", actual, want.Identifiers)
				}
			}
			if actual := started.values(); !reflect.DeepEqual(actual, want.Started) {
				t.Fatalf("started = %#v, want %#v", actual, want.Started)
			}
			if actual := calls.values(); !reflect.DeepEqual(actual, canonicalCategoryDifferentialCalls(want.Calls)) {
				t.Fatalf("calls = %#v, want %#v", actual, want.Calls)
			}
		})
	}
}

func loadCategoryDifferentialFixture(t *testing.T, path string) categoryDifferentialFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	var fixture categoryDifferentialFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func decodeCategoryDifferentialOutput(t *testing.T, outputs map[string]json.RawMessage, label string) categoryDifferentialOutput {
	t.Helper()

	raw, exists := outputs[label]
	if !exists {
		t.Fatalf("fixture misses %q output", label)
	}
	var output categoryDifferentialOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode %q output: %v", label, err)
	}
	return output
}

type categoryDifferentialStarted struct {
	mu      sync.Mutex
	entries []string
}

type categoryDifferentialCall struct {
	Name         string         `json:"name"`
	Query        string         `json:"query"`
	KeywordOrder []string       `json:"keyword_order"`
	Kwargs       map[string]any `json:"kwargs"`
}

type categoryDifferentialCalls struct {
	mu      sync.Mutex
	entries []categoryDifferentialCall
}

func (c *categoryDifferentialCalls) add(name string, request EngineRequest) {
	c.mu.Lock()
	defer c.mu.Unlock()

	kwargs := map[string]any{
		"region":     request.Region,
		"safesearch": request.SafeSearch,
		"page":       strconv.Itoa(request.Page),
	}
	if request.TimeLimit == nil {
		kwargs["timelimit"] = nil
	} else {
		kwargs["timelimit"] = *request.TimeLimit
	}
	order := []string{"region", "safesearch", "timelimit", "page"}
	for _, parameter := range request.Parameters {
		kwargs[parameter.Name] = parameter.Value
		order = append(order, parameter.Name)
	}
	c.entries = append(c.entries, categoryDifferentialCall{
		Name: name, Query: request.Query, KeywordOrder: order, Kwargs: kwargs,
	})
}

func (c *categoryDifferentialCalls) values() []categoryDifferentialCall {
	c.mu.Lock()
	defer c.mu.Unlock()

	values := append([]categoryDifferentialCall(nil), c.entries...)
	sort.Slice(values, func(left, right int) bool { return values[left].Name < values[right].Name })
	return values
}

func canonicalCategoryDifferentialCalls(calls []categoryDifferentialCall) []categoryDifferentialCall {
	canonical := make([]categoryDifferentialCall, len(calls))
	for index, call := range calls {
		kwargs := make(map[string]any, len(call.Kwargs))
		for name, value := range call.Kwargs {
			kwargs[name] = value
		}
		if page, ok := kwargs["page"].(float64); ok {
			kwargs["page"] = strconv.Itoa(int(page))
		}
		call.Kwargs = kwargs
		canonical[index] = call
	}
	sort.Slice(canonical, func(left, right int) bool { return canonical[left].Name < canonical[right].Name })
	return canonical
}

func (s *categoryDifferentialStarted) add(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, name)
}

func (s *categoryDifferentialStarted) values() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	values := append([]string(nil), s.entries...)
	sort.Strings(values)
	return values
}

func categoryDifferentialEngines(
	t *testing.T,
	testCase categoryDifferentialCase,
	selection []engine.Metadata,
	query string,
) (*categoryDifferentialStarted, *categoryDifferentialCalls, []ScheduledEngine) {
	t.Helper()

	started := &categoryDifferentialStarted{}
	calls := &categoryDifferentialCalls{}
	scheduled := make([]ScheduledEngine, len(selection))
	for index, metadata := range selection {
		metadata := metadata
		scheduled[index] = ScheduledEngine{
			Name:     metadata.Name,
			Provider: metadata.Provider,
			Search: func(_ context.Context, request EngineRequest) ([]Result, error) {
				started.add(metadata.Name)
				calls.add(metadata.Name, request)
				switch testCase.Outcomes[metadata.Name] {
				case "timeout":
					return nil, errors.New("operation timed out exactly")
				case "empty":
					return nil, nil
				case "error":
					return nil, errors.New("source failure exactly")
				}
				result, err := categoryDifferentialResult(testCase.Category, query, metadata.Name)
				if err != nil {
					return nil, err
				}
				return []Result{result}, nil
			},
		}
	}
	return started, calls, scheduled
}

func categoryDifferentialParameters(t *testing.T, parameters [][]string) []SourceParameter {
	t.Helper()
	converted := make([]SourceParameter, len(parameters))
	for index, parameter := range parameters {
		if len(parameter) != 2 {
			t.Fatalf("parameter %d = %#v, want name/value", index, parameter)
		}
		converted[index] = SourceParameter{Name: parameter[0], Value: parameter[1]}
	}
	return converted
}

func categoryDifferentialDefaults(testCase categoryDifferentialCase) (region, safeSearch string, page int) {
	region, safeSearch, page = "us-en", "moderate", 1
	if testCase.Region != "" {
		region = testCase.Region
	}
	if testCase.SafeSearch != "" {
		safeSearch = testCase.SafeSearch
	}
	if testCase.Page != 0 {
		page = testCase.Page
	}
	return region, safeSearch, page
}

func categoryDifferentialResult(category, query, name string) (Result, error) {
	url := "https://" + name + ".fixture.example"
	title := query + " " + name
	switch category {
	case "text":
		return NewCategoryResult(category, []Field{{Name: "title", Value: title}, {Name: "href", Value: url}, {Name: "body", Value: title}})
	case "images":
		return NewCategoryResult(category, []Field{{Name: "title", Value: title}, {Name: "image", Value: url + "/image"}, {Name: "url", Value: url}})
	case "news":
		return NewCategoryResult(category, []Field{{Name: "title", Value: title}, {Name: "body", Value: title}, {Name: "url", Value: url}})
	case "videos":
		return NewCategoryResult(category, []Field{{Name: "title", Value: title}, {Name: "description", Value: title}, {Name: "embed_url", Value: url}})
	case "books":
		return NewCategoryResult(category, []Field{{Name: "title", Value: title}, {Name: "url", Value: url}})
	default:
		return Result{}, errors.New("unexpected category " + category)
	}
}

func categoryDifferentialIdentifiers(t *testing.T, results []map[string]any, category string) []string {
	t.Helper()

	field := map[string]string{
		"text":   "href",
		"images": "image",
		"news":   "url",
		"videos": "embed_url",
		"books":  "url",
	}[category]
	identifiers := make([]string, len(results))
	for index, result := range results {
		value, ok := result[field].(string)
		if !ok {
			t.Fatalf("result %d %s = %#v, want string", index, field, result[field])
		}
		identifiers[index] = value
	}
	return identifiers
}

func assertCategoryDifferentialError(t *testing.T, err error, wantType, wantMessage string, wantCauseType *string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Search error = nil, want %s: %q", wantType, wantMessage)
	}
	if err.Error() != wantMessage {
		t.Fatalf("Search error = %q, want %q", err.Error(), wantMessage)
	}
	var sourceError *sourceSchedulerError
	if !errors.As(err, &sourceError) {
		t.Fatalf("Search error = %T, want source scheduler error", err)
	}
	if sourceError.sourceType != wantType {
		t.Fatalf("Search error type = %q, want %q", sourceError.sourceType, wantType)
	}
	if wantCauseType == nil && sourceError.cause != nil {
		t.Fatalf("Search cause = %T, want nil", sourceError.cause)
	}
}
