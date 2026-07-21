package parser

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type jsonFixture struct {
	FixtureID string `json:"fixture_id"`
	Contract  struct {
		Operation string `json:"operation"`
	} `json:"contract"`
	Input struct {
		JSON string   `json:"json"`
		Path []string `json:"path"`
	} `json:"input"`
	Result struct {
		Status string          `json:"status"`
		Output json.RawMessage `json:"output"`
		Error  struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"result"`
}

func TestDecodeJSON_MatchesFrozenPythonFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/parser/*-json-*.json")
	if err != nil {
		t.Fatalf("find parser JSON fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no parser JSON fixtures")
	}

	matched := 0
	for _, path := range paths {
		fixture := loadJSONFixture(t, path)
		if fixture.Contract.Operation != "json_loads" {
			continue
		}
		matched++
		t.Run(fixture.FixtureID, func(t *testing.T) {
			got, err := DecodeJSON([]byte(fixture.Input.JSON))
			if fixture.Result.Status == "error" {
				assertJSONFixtureError(t, "DecodeJSON", err, fixture)
				return
			}
			if err != nil {
				t.Fatalf("DecodeJSON: %v", err)
			}

			want := decodeJSONFixtureValue(t, fixture.Result.Output)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("DecodeJSON() = %#v, want %#v", got, want)
			}
		})
	}
	if matched == 0 {
		t.Fatal("no json_loads parser fixtures")
	}
}

func TestDecodeOrderedJSON_MatchesFrozenWikipediaObjectOrderFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/parser/parser.text.wikipedia-json-pages-*.json")
	if err != nil {
		t.Fatalf("find ordered Wikipedia JSON fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no ordered Wikipedia JSON fixtures")
	}

	for _, path := range paths {
		fixture := loadJSONFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			if fixture.Contract.Operation != "json_object_items" {
				t.Fatalf("operation = %q, want json_object_items", fixture.Contract.Operation)
			}

			value, err := DecodeOrderedJSON([]byte(fixture.Input.JSON))
			if err != nil {
				t.Fatalf("DecodeOrderedJSON: %v", err)
			}
			object := orderedObjectAt(t, value, fixture.Input.Path)
			fields := object.Fields()
			var want []struct {
				Name  string          `json:"name"`
				Value json.RawMessage `json:"value"`
			}
			if err := json.Unmarshal(fixture.Result.Output, &want); err != nil {
				t.Fatalf("decode %s output: %v", fixture.FixtureID, err)
			}
			if len(fields) != len(want) {
				t.Fatalf("fields = %#v, want %d entries", fields, len(want))
			}
			for index, expected := range want {
				field := fields[index]
				if field.Name != expected.Name {
					t.Fatalf("field %d name = %q, want %q", index, field.Name, expected.Name)
				}
				wantValue := decodeJSONFixtureValue(t, expected.Value)
				if gotValue := orderedValueAsPlain(field.Value); !reflect.DeepEqual(gotValue, wantValue) {
					t.Fatalf("field %d value = %#v, want %#v", index, gotValue, wantValue)
				}
			}
		})
	}
}

func TestDecodeOrderedJSON_MatchesFrozenPythonJSONFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/parser/*-json-*.json")
	if err != nil {
		t.Fatalf("find parser JSON fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no parser JSON fixtures")
	}

	matched := 0
	for _, path := range paths {
		fixture := loadJSONFixture(t, path)
		if fixture.Contract.Operation != "json_loads" {
			continue
		}
		matched++
		t.Run(fixture.FixtureID, func(t *testing.T) {
			got, err := DecodeOrderedJSON([]byte(fixture.Input.JSON))
			if fixture.Result.Status == "error" {
				assertJSONFixtureError(t, "DecodeOrderedJSON", err, fixture)
				return
			}
			if err != nil {
				t.Fatalf("DecodeOrderedJSON: %v", err)
			}

			want := decodeJSONFixtureValue(t, fixture.Result.Output)
			if plain := orderedValueAsPlain(got); !reflect.DeepEqual(plain, want) {
				t.Fatalf("DecodeOrderedJSON() = %#v, want %#v", plain, want)
			}
		})
	}
	if matched == 0 {
		t.Fatal("no json_loads parser fixtures")
	}
}

func TestDecodeJSON_MatchesFrozenPythonNonFiniteFixture(t *testing.T) {
	fixture := loadJSONFixture(t, "../../testdata/contracts/parser/parser.text.grokipedia-json-nonfinite-literals.json")
	if fixture.Contract.Operation != "json_loads_nonfinite" {
		t.Fatalf("operation = %q, want json_loads_nonfinite", fixture.Contract.Operation)
	}

	for name, decode := range map[string]func([]byte) (any, error){
		"plain":   DecodeJSON,
		"ordered": DecodeOrderedJSON,
	} {
		t.Run(name, func(t *testing.T) {
			value, err := decode([]byte(fixture.Input.JSON))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			plain := orderedValueAsPlain(value)
			actual := pythonNonFiniteFixtureValue(t, plain)
			want := decodeJSONFixtureValue(t, fixture.Result.Output)
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("decoded non-finite value = %#v, want %#v", actual, want)
			}
		})
	}
}

func pythonNonFiniteFixtureValue(t testing.TB, value any) any {
	t.Helper()

	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("decoded value = %T, want map[string]any", value)
	}
	values, ok := object["values"].([]any)
	if !ok {
		t.Fatalf("values = %T, want []any", object["values"])
	}
	encodedValues := make([]any, len(values))
	for index, item := range values {
		encodedValues[index] = pythonNonFiniteValue(t, item)
	}
	nested, ok := object["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested = %T, want map[string]any", object["nested"])
	}
	finite, ok := object["finite"].([]any)
	if !ok {
		t.Fatalf("finite = %T, want []any", object["finite"])
	}
	encodedFinite := make([]any, len(finite))
	for index, item := range finite {
		encodedFinite[index] = pythonFiniteValue(t, item)
	}
	overflow, ok := object["overflow"].([]any)
	if !ok {
		t.Fatalf("overflow = %T, want []any", object["overflow"])
	}
	encodedOverflow := make([]any, len(overflow))
	for index, item := range overflow {
		encodedOverflow[index] = pythonOverflowValue(t, item)
	}
	flags, ok := nested["flags"].([]any)
	if !ok {
		t.Fatalf("nested flags = %T, want []any", nested["flags"])
	}
	return map[string]any{
		"values": encodedValues,
		"nested": map[string]any{
			"type":    pythonNonFiniteValue(t, nested["value"])["type"],
			"repr":    pythonNonFiniteValue(t, nested["value"])["repr"],
			"flags":   flags,
			"escaped": nested["escaped"],
		},
		"finite":   encodedFinite,
		"overflow": encodedOverflow,
		"literal":  object["literal"],
	}
}

func pythonNonFiniteValue(t testing.TB, value any) map[string]any {
	t.Helper()

	number, ok := value.(float64)
	if !ok {
		t.Fatalf("non-finite value = %T, want float64", value)
	}
	representation := ""
	switch {
	case math.IsNaN(number):
		representation = "nan"
	case math.IsInf(number, 1):
		representation = "inf"
	case math.IsInf(number, -1):
		representation = "-inf"
	default:
		t.Fatalf("value = %v, want non-finite", number)
	}
	return map[string]any{"type": "float", "repr": representation}
}

func pythonFiniteValue(t testing.TB, value any) map[string]any {
	t.Helper()

	number, ok := value.(json.Number)
	if !ok {
		t.Fatalf("finite value = %T, want json.Number", value)
	}
	if !strings.ContainsAny(number.String(), ".eE") {
		integer, err := strconv.ParseInt(number.String(), 10, 64)
		if err == nil {
			return map[string]any{"type": "int", "repr": strconv.FormatInt(integer, 10)}
		}
	}
	decimal, err := strconv.ParseFloat(number.String(), 64)
	if err != nil {
		t.Fatalf("parse source float %q: %v", number, err)
	}
	representation := strconv.FormatFloat(decimal, 'g', -1, 64)
	if !strings.ContainsAny(representation, ".eE") {
		representation += ".0"
	}
	return map[string]any{"type": "float", "repr": representation}
}

func pythonOverflowValue(t testing.TB, value any) map[string]any {
	t.Helper()

	number, ok := value.(json.Number)
	if !ok {
		t.Fatalf("overflow value = %T, want json.Number", value)
	}
	parsed, err := strconv.ParseFloat(number.String(), 64)
	if !math.IsInf(parsed, 0) {
		t.Fatalf("overflow number %q = %v / %v, want infinite float", number, parsed, err)
	}
	representation := "inf"
	if math.Signbit(parsed) {
		representation = "-inf"
	}
	return map[string]any{"type": "float", "repr": representation}
}

func assertJSONFixtureError(t testing.TB, operation string, err error, fixture jsonFixture) {
	t.Helper()

	if err == nil {
		t.Fatalf("%s error = nil, want %s: %s", operation, fixture.Result.Error.Type, fixture.Result.Error.Message)
	}
	var sourceError *JSONDecodeError
	if !errors.As(err, &sourceError) {
		t.Fatalf("%s error = %T, want *JSONDecodeError", operation, err)
	}
	if err.Error() != fixture.Result.Error.Message {
		t.Fatalf("%s error = %q, want %q", operation, err, fixture.Result.Error.Message)
	}
}

func decodeJSONFixtureValue(t *testing.T, raw json.RawMessage) any {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("decode fixture output: %v", err)
	}
	return value
}

func orderedObjectAt(t testing.TB, value any, path []string) *OrderedObject {
	t.Helper()

	object, ok := value.(*OrderedObject)
	if !ok {
		t.Fatalf("root value = %T, want *OrderedObject", value)
	}
	for _, name := range path {
		value, ok := object.Value(name)
		if !ok {
			t.Fatalf("object misses path component %q", name)
		}
		object, ok = value.(*OrderedObject)
		if !ok {
			t.Fatalf("path component %q = %T, want *OrderedObject", name, value)
		}
	}
	return object
}

func orderedValueAsPlain(value any) any {
	switch typed := value.(type) {
	case *OrderedObject:
		result := make(map[string]any, len(typed.fields))
		for _, field := range typed.fields {
			result[field.Name] = orderedValueAsPlain(field.Value)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = orderedValueAsPlain(item)
		}
		return result
	default:
		return value
	}
}

func loadJSONFixture(t testing.TB, path string) jsonFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture jsonFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}
