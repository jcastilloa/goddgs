package search

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type resultFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Categories  []string         `json:"categories"`
		CacheFields []string         `json:"cache_fields"`
		FieldOrder  [][]string       `json:"field_order"`
		Items       []map[string]any `json:"items"`
		Response    struct {
			Results []map[string]any `json:"results"`
		} `json:"response"`
	} `json:"input"`
	Result struct {
		Status     string             `json:"status"`
		FieldOrder [][]string         `json:"field_order"`
		Output     json.RawMessage    `json:"output"`
		Error      resultFixtureError `json:"error"`
	} `json:"result"`
}

type resultFixtureError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type dynamicResultFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Category string `json:"category"`
		Updates  []struct {
			Name  string `json:"name"`
			Value any    `json:"value"`
		} `json:"updates"`
	} `json:"input"`
	Result struct {
		FieldOrder [][]string     `json:"field_order"`
		Output     map[string]any `json:"output"`
	} `json:"result"`
}

func TestResultsAggregator_MatchesFrozenFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/pure/pure.aggregate-*.json")
	if err != nil {
		t.Fatalf("find aggregation fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no aggregation fixtures")
	}

	for _, path := range paths {
		fixture := loadResultFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			aggregator, err := NewResultsAggregator(fixture.Input.CacheFields)
			if err != nil {
				t.Fatalf("NewResultsAggregator(%v): %v", fixture.Input.CacheFields, err)
			}

			for index, item := range fixture.Input.Items {
				result, err := NewResult(fieldsFromFixture(t, fixture.FixtureID, item, fixture.Input.FieldOrder[index]))
				if err != nil {
					t.Fatalf("NewResult input %d: %v", index, err)
				}
				err = aggregator.Append(result)
				if fixture.Result.Status == "error" {
					if index != len(fixture.Input.Items)-1 {
						if err != nil {
							t.Fatalf("Append input %d: %v", index, err)
						}
						continue
					}
					if err == nil {
						t.Fatalf("Append input %d error = nil, want %s: %q", index, fixture.Result.Error.Type, fixture.Result.Error.Message)
					}
					if got := err.Error(); got != fixture.Result.Error.Message {
						t.Fatalf("Append input %d error = %q, want %q", index, got, fixture.Result.Error.Message)
					}
					return
				}
				if err != nil {
					t.Fatalf("Append input %d: %v", index, err)
				}
			}
			if fixture.Result.Status == "error" {
				t.Fatalf("fixture %s error was not observed", fixture.FixtureID)
			}

			want, wantLength := aggregationOutput(t, fixture)
			if wantLength != nil && aggregator.Len() != *wantLength {
				t.Fatalf("Len() = %d, want %d", aggregator.Len(), *wantLength)
			}
			assertResultSequence(t, aggregator.Extract(), want, fixture.Result.FieldOrder)
		})
	}
}

func TestNewResult_PreservesFrozenHeterogeneousVideoValues(t *testing.T) {
	fixture := loadResultFixture(t, "../../testdata/contracts/pure/pure.video-heterogeneous-values.json")
	want := fixtureOutput(t, fixture)
	if len(fixture.Input.Response.Results) != 1 || len(want) != 1 || len(fixture.Result.FieldOrder) != 1 {
		t.Fatalf("fixture %s does not contain one video result", fixture.FixtureID)
	}

	result, err := NewResult(fieldsFromFixture(
		t,
		fixture.FixtureID,
		fixture.Input.Response.Results[0],
		fixture.Result.FieldOrder[0],
	))
	if err != nil {
		t.Fatalf("NewResult(video): %v", err)
	}

	assertResultSequence(t, []Result{result}, want, fixture.Result.FieldOrder)
	statistics, ok := result.Map()["statistics"].(map[string]any)
	if !ok {
		t.Fatalf("statistics type = %T, want map[string]any", result.Map()["statistics"])
	}
	if got, want := statistics["viewCount"], json.Number("29059"); got != want {
		t.Fatalf("statistics.viewCount = %#v (%T), want %s (%T)", got, got, want, want)
	}
}

func TestNewCategoryResult_MatchesFrozenCategoryFieldShapes(t *testing.T) {
	paths := []string{
		"../../testdata/contracts/pure/pure.result-category-field-shapes.json",
		"../../testdata/contracts/pure/pure.result-falsy-named-fields.json",
	}
	for _, path := range paths {
		fixture := loadResultFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			want := fixtureOutput(t, fixture)
			if len(fixture.Input.Categories) != len(fixture.Input.Items) || len(want) != len(fixture.Result.FieldOrder) {
				t.Fatalf("fixture %s has inconsistent category result shapes", fixture.FixtureID)
			}

			got := make([]Result, len(fixture.Input.Items))
			for index, item := range fixture.Input.Items {
				result, err := NewCategoryResult(
					fixture.Input.Categories[index],
					fieldsFromFixture(t, fixture.FixtureID, item, fixture.Input.FieldOrder[index]),
				)
				if err != nil {
					t.Fatalf("NewCategoryResult(%s): %v", fixture.Input.Categories[index], err)
				}
				got[index] = result
			}
			assertResultSequence(t, got, want, fixture.Result.FieldOrder)
		})
	}
}

func TestNewCategoryResult_MatchesFrozenDefaultShapes(t *testing.T) {
	fixture := loadResultFixture(t, "../../testdata/contracts/pure/pure.result-category-default-shapes.json")
	want := fixtureOutput(t, fixture)
	if len(fixture.Input.Categories) != len(want) || len(want) != len(fixture.Result.FieldOrder) {
		t.Fatalf("fixture %s has inconsistent default category shapes", fixture.FixtureID)
	}

	got := make([]Result, len(fixture.Input.Categories))
	for index, category := range fixture.Input.Categories {
		result, err := NewCategoryResult(category, nil)
		if err != nil {
			t.Fatalf("NewCategoryResult(%q): %v", category, err)
		}
		got[index] = result
	}
	assertResultSequence(t, got, want, fixture.Result.FieldOrder)
}

func TestResultSet_MatchesFrozenDynamicFieldOrder(t *testing.T) {
	fixture := loadDynamicResultFixture(t, "../../testdata/contracts/pure/pure.result-dynamic-field-order.json")
	if len(fixture.Result.FieldOrder) != 1 {
		t.Fatalf("fixture %s field-order count = %d, want 1", fixture.FixtureID, len(fixture.Result.FieldOrder))
	}

	updates := make([]Field, len(fixture.Input.Updates))
	for index, update := range fixture.Input.Updates {
		updates[index] = Field{Name: update.Name, Value: update.Value}
	}
	result, err := NewCategoryResult(fixture.Input.Category, updates)
	if err != nil {
		t.Fatalf("NewCategoryResult(%q): %v", fixture.Input.Category, err)
	}
	assertResultSequence(t, []Result{result}, []map[string]any{fixture.Result.Output}, fixture.Result.FieldOrder)
}

func TestResultsAggregator_RejectsEmptyCacheFields(t *testing.T) {
	aggregator, err := NewResultsAggregator(nil)
	if aggregator != nil {
		t.Fatalf("NewResultsAggregator(nil) = %#v, want nil", aggregator)
	}
	if err == nil || err.Error() != "At least one cache_field must be provided" {
		t.Fatalf("NewResultsAggregator(nil) error = %v, want source ValueError message", err)
	}
}

func TestResultsAggregator_ReportsMissingCacheField(t *testing.T) {
	aggregator, err := NewResultsAggregator([]string{"href"})
	if err != nil {
		t.Fatalf("NewResultsAggregator: %v", err)
	}
	result, err := NewResult([]Field{{Name: "title", Value: "only title"}})
	if err != nil {
		t.Fatalf("NewResult: %v", err)
	}
	if err := aggregator.Append(result); err == nil {
		t.Fatal("Append error = nil, want source missing-cache-field failure")
	}
}

func TestNewCategoryResult_RejectsUnknownCategory(t *testing.T) {
	_, err := NewCategoryResult("unknown", nil)
	if !errors.Is(err, errUnknownResultCategory) {
		t.Fatalf("unknown category error = %v, want errUnknownResultCategory", err)
	}
}

func loadResultFixture(t *testing.T, path string) resultFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()

	var fixture resultFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func loadDynamicResultFixture(t *testing.T, path string) dynamicResultFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()

	var fixture dynamicResultFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func fieldsFromFixture(t *testing.T, fixtureID string, values map[string]any, order []string) []Field {
	t.Helper()

	fields := make([]Field, 0, len(order))
	for _, name := range order {
		value, ok := values[name]
		if !ok {
			t.Fatalf("fixture %s field %q is missing from values", fixtureID, name)
		}
		fields = append(fields, Field{Name: name, Value: value})
	}
	if len(fields) != len(values) {
		t.Fatalf("fixture %s field order has %d fields, values has %d", fixtureID, len(fields), len(values))
	}
	return fields
}

func aggregationOutput(t *testing.T, fixture resultFixture) ([]map[string]any, *int) {
	t.Helper()

	var wrapped struct {
		Items  []map[string]any `json:"items"`
		Length int              `json:"length"`
	}
	decoder := json.NewDecoder(bytes.NewReader(fixture.Result.Output))
	decoder.UseNumber()
	if err := decoder.Decode(&wrapped); err == nil && wrapped.Items != nil {
		return wrapped.Items, &wrapped.Length
	}
	return fixtureOutput(t, fixture), nil
}

func fixtureOutput(t *testing.T, fixture resultFixture) []map[string]any {
	t.Helper()

	var output []map[string]any
	decoder := json.NewDecoder(bytes.NewReader(fixture.Result.Output))
	decoder.UseNumber()
	if err := decoder.Decode(&output); err != nil {
		t.Fatalf("decode %s output: %v", fixture.FixtureID, err)
	}
	return output
}

func assertResultSequence(t *testing.T, got []Result, want []map[string]any, wantOrder [][]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("result count = %d, want %d", len(got), len(want))
	}
	if len(wantOrder) != len(want) {
		t.Fatalf("fixture field-order count = %d, want %d", len(wantOrder), len(want))
	}
	for index, wantResult := range want {
		if actual := got[index].Map(); !reflect.DeepEqual(actual, wantResult) {
			t.Fatalf("result %d map = %#v, want %#v", index, actual, wantResult)
		}
		fields := got[index].Fields()
		actualOrder := make([]string, len(fields))
		for fieldIndex, field := range fields {
			actualOrder[fieldIndex] = field.Name
		}
		if !reflect.DeepEqual(actualOrder, wantOrder[index]) {
			t.Fatalf("result %d field order = %v, want %v", index, actualOrder, wantOrder[index])
		}
	}
}
