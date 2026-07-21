package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type resultConstructionFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Categories []string         `json:"categories"`
		FieldOrder [][]string       `json:"field_order"`
		Items      []map[string]any `json:"items"`
		Response   struct {
			Results []map[string]any `json:"results"`
		} `json:"response"`
	} `json:"input"`
	Result struct {
		FieldOrder [][]string      `json:"field_order"`
		Output     json.RawMessage `json:"output"`
	} `json:"result"`
}

func TestNewCategoryResult_MatchesFrozenDynamicFieldOrder(t *testing.T) {
	var fixture struct {
		Input struct {
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
	decodeFixture(t, "../../testdata/contracts/pure/pure.result-dynamic-field-order.json", &fixture)

	updates := make([]Field, len(fixture.Input.Updates))
	for index, update := range fixture.Input.Updates {
		updates[index] = Field{Name: update.Name, Value: update.Value}
	}

	result, err := NewCategoryResult(fixture.Input.Category, updates)
	if err != nil {
		t.Fatalf("NewCategoryResult(%q): %v", fixture.Input.Category, err)
	}
	if !reflect.DeepEqual(result.Map(), fixture.Result.Output) {
		t.Fatalf("result map = %#v, want %#v", result.Map(), fixture.Result.Output)
	}

	if len(fixture.Result.FieldOrder) != 1 {
		t.Fatalf("fixture field-order count = %d, want 1", len(fixture.Result.FieldOrder))
	}
	fields := result.Fields()
	actualOrder := make([]string, len(fields))
	for index, field := range fields {
		actualOrder[index] = field.Name
	}
	if !reflect.DeepEqual(actualOrder, fixture.Result.FieldOrder[0]) {
		t.Fatalf("field order = %v, want %v", actualOrder, fixture.Result.FieldOrder[0])
	}
}

func TestNewCategoryResult_MatchesFrozenCategoryFixtures(t *testing.T) {
	paths := []string{
		"../../testdata/contracts/pure/pure.result-category-default-shapes.json",
		"../../testdata/contracts/pure/pure.result-category-field-shapes.json",
		"../../testdata/contracts/pure/pure.result-falsy-named-fields.json",
	}
	for _, path := range paths {
		fixture := loadResultConstructionFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			want := decodeResultMaps(t, fixture.FixtureID, fixture.Result.Output)
			if len(fixture.Input.Categories) != len(want) || len(want) != len(fixture.Result.FieldOrder) {
				t.Fatalf("fixture categories/results/order counts = %d/%d/%d", len(fixture.Input.Categories), len(want), len(fixture.Result.FieldOrder))
			}

			for index, category := range fixture.Input.Categories {
				updates := resultUpdates(t, fixture, index)
				result, err := NewCategoryResult(category, updates)
				if err != nil {
					t.Fatalf("NewCategoryResult(%q): %v", category, err)
				}
				assertResult(t, result, want[index], fixture.Result.FieldOrder[index])
			}
		})
	}
}

func TestNewResult_MatchesFrozenHeterogeneousVideoFixture(t *testing.T) {
	fixture := loadResultConstructionFixture(t, "../../testdata/contracts/pure/pure.video-heterogeneous-values.json")
	if len(fixture.Input.Response.Results) != 1 || len(fixture.Result.FieldOrder) != 1 {
		t.Fatalf("fixture %s does not contain one ordered video result", fixture.FixtureID)
	}
	want := decodeResultMaps(t, fixture.FixtureID, fixture.Result.Output)
	if len(want) != 1 {
		t.Fatalf("fixture %s output count = %d, want 1", fixture.FixtureID, len(want))
	}

	result, err := NewResult(fieldsFromMap(t, fixture.FixtureID, fixture.Input.Response.Results[0], fixture.Result.FieldOrder[0]))
	if err != nil {
		t.Fatalf("NewResult(video): %v", err)
	}
	assertResult(t, result, want[0], fixture.Result.FieldOrder[0])
}

func TestResult_ValueAndUnknownCategory(t *testing.T) {
	result, err := NewCategoryResult("text", []Field{{Name: "title", Value: "fixture"}})
	if err != nil {
		t.Fatalf("NewCategoryResult(text): %v", err)
	}
	if got, ok := result.Value("title"); !ok || got != "fixture" {
		t.Fatalf("Value(title) = %#v, %t; want fixture, true", got, ok)
	}
	if got, ok := result.Value("missing"); ok || got != nil {
		t.Fatalf("Value(missing) = %#v, %t; want nil, false", got, ok)
	}
	if _, err := NewCategoryResult("unknown", nil); !errors.Is(err, ErrUnknownResultCategory) {
		t.Fatalf("NewCategoryResult(unknown) error = %v, want ErrUnknownResultCategory", err)
	}
}

func loadResultConstructionFixture(t *testing.T, path string) resultConstructionFixture {
	t.Helper()

	var fixture resultConstructionFixture
	decodeFixture(t, path, &fixture)
	return fixture
}

func decodeFixture(t *testing.T, path string, target any) {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func resultUpdates(t *testing.T, fixture resultConstructionFixture, index int) []Field {
	t.Helper()
	if len(fixture.Input.Items) == 0 || len(fixture.Input.FieldOrder) == 0 {
		return nil
	}
	if len(fixture.Input.Items) != len(fixture.Input.Categories) || len(fixture.Input.FieldOrder) != len(fixture.Input.Items) {
		t.Fatalf("fixture %s input shapes are inconsistent", fixture.FixtureID)
	}
	return fieldsFromMap(t, fixture.FixtureID, fixture.Input.Items[index], fixture.Input.FieldOrder[index])
}

func fieldsFromMap(t *testing.T, fixtureID string, values map[string]any, order []string) []Field {
	t.Helper()

	fields := make([]Field, 0, len(order))
	for _, name := range order {
		value, ok := values[name]
		if !ok {
			t.Fatalf("fixture %s missing field %q", fixtureID, name)
		}
		fields = append(fields, Field{Name: name, Value: value})
	}
	if len(fields) != len(values) {
		t.Fatalf("fixture %s field order has %d fields, values has %d", fixtureID, len(fields), len(values))
	}
	return fields
}

func decodeResultMaps(t *testing.T, fixtureID string, raw json.RawMessage) []map[string]any {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var output []map[string]any
	if err := decoder.Decode(&output); err != nil {
		t.Fatalf("decode %s output: %v", fixtureID, err)
	}
	return output
}

func assertResult(t *testing.T, result Result, want map[string]any, wantOrder []string) {
	t.Helper()
	if !reflect.DeepEqual(result.Map(), want) {
		t.Fatalf("result map = %#v, want %#v", result.Map(), want)
	}

	fields := result.Fields()
	actualOrder := make([]string, len(fields))
	for index, field := range fields {
		actualOrder[index] = field.Name
	}
	if !reflect.DeepEqual(actualOrder, wantOrder) {
		t.Fatalf("field order = %v, want %v", actualOrder, wantOrder)
	}
}

func TestResultFixtures_AreAvailable(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/pure/pure.result-*.json")
	if err != nil {
		t.Fatalf("glob result fixtures: %v", err)
	}
	if len(paths) < 4 {
		t.Fatalf("result fixture count = %d, want at least 4", len(paths))
	}
}
