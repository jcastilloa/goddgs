package search

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/jcastillo/goddgs/internal/engine"
)

type backendFixture struct {
	Input struct {
		Cases    []backendFixtureCase     `json:"cases"`
		Registry []backendFixtureCategory `json:"registry"`
		Category string                   `json:"category"`
		Backend  string                   `json:"backend"`
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

type backendFixtureCase struct {
	Label    string `json:"label"`
	Category string `json:"category"`
	Backend  string `json:"backend"`
}

type backendFixtureCategory struct {
	Category string                   `json:"category"`
	Engines  []backendFixtureMetadata `json:"engines"`
}

type backendFixtureMetadata struct {
	Name     string  `json:"name"`
	Provider string  `json:"provider"`
	Priority float64 `json:"priority"`
	Disabled bool    `json:"disabled"`
}

type selectedEngine struct {
	Name     string  `json:"name"`
	Provider string  `json:"provider"`
	Priority float64 `json:"priority"`
}

func TestBackendSelector_MatchesFrozenBackendFixture(t *testing.T) {
	fixture := loadBackendFixture(t, "../../testdata/contracts/pure/pure.backend-auto-priority-stable-shuffle.json")
	output := decodeBackendOutput(t, fixture)

	var shuffleInputs [][]string
	selector := NewBackendSelector(categoriesFromBackendFixture(fixture.Input.Registry), func(keys []string) {
		shuffleInputs = append(shuffleInputs, append([]string(nil), keys...))
		reverseStrings(keys)
	})

	for _, testCase := range fixture.Input.Cases {
		t.Run(testCase.Label, func(t *testing.T) {
			start := len(shuffleInputs)
			got, err := selector.Select(testCase.Category, testCase.Backend)
			if err != nil {
				t.Fatalf("Select(%q, %q): %v", testCase.Category, testCase.Backend, err)
			}

			want := output.Selections[testCase.Label]
			if actual := projectSelection(got); !reflect.DeepEqual(actual, want) {
				t.Fatalf("Select(%q, %q) = %#v, want %#v", testCase.Category, testCase.Backend, actual, want)
			}
			if actual := shuffleInputs[start:]; !reflect.DeepEqual(actual, output.ShuffleInputs[testCase.Label]) {
				t.Fatalf("shuffle calls = %#v, want %#v", actual, output.ShuffleInputs[testCase.Label])
			}
		})
	}
}

func TestBackendSelector_MatchesFrozenUnknownCategoryError(t *testing.T) {
	registryFixture := loadBackendFixture(t, "../../testdata/contracts/pure/pure.backend-auto-priority-stable-shuffle.json")
	errorFixture := loadBackendFixture(t, "../../testdata/contracts/pure/pure.backend-unknown-category-error.json")

	shuffleCalls := 0
	selector := NewBackendSelector(categoriesFromBackendFixture(registryFixture.Input.Registry), func([]string) {
		shuffleCalls++
	})
	_, err := selector.Select(errorFixture.Input.Category, errorFixture.Input.Backend)
	if err == nil {
		t.Fatalf("Select unknown category error = nil, want %s: %q", errorFixture.Result.Error.Type, errorFixture.Result.Error.Message)
	}
	if actual := err.Error(); actual != errorFixture.Result.Error.Message {
		t.Fatalf("Select unknown category error = %q, want %q", actual, errorFixture.Result.Error.Message)
	}
	if shuffleCalls != 0 {
		t.Fatalf("shuffle calls = %d, want 0", shuffleCalls)
	}
}

func TestBackendSelector_UsesFrozenRegistry(t *testing.T) {
	fixture := loadBackendFixture(t, "../../testdata/contracts/pure/pure.backend-frozen-registry-selection.json")
	output := decodeBackendOutput(t, fixture)

	var shuffleInputs [][]string
	selector := NewBackendSelector(engine.FrozenRegistry().Categories(), func(keys []string) {
		shuffleInputs = append(shuffleInputs, append([]string(nil), keys...))
		reverseStrings(keys)
	})

	for _, testCase := range fixture.Input.Cases {
		t.Run(testCase.Label, func(t *testing.T) {
			start := len(shuffleInputs)
			got, err := selector.Select(testCase.Category, testCase.Backend)
			if err != nil {
				t.Fatalf("Select(%q, %q): %v", testCase.Category, testCase.Backend, err)
			}
			if actual, want := projectSelection(got), output.Selections[testCase.Label]; !reflect.DeepEqual(actual, want) {
				t.Fatalf("Select(%q, %q) = %#v, want %#v", testCase.Category, testCase.Backend, actual, want)
			}
			if actual, want := shuffleInputs[start:], output.ShuffleInputs[testCase.Label]; !reflect.DeepEqual(actual, want) {
				t.Fatalf("shuffle calls = %#v, want %#v", actual, want)
			}
		})
	}
}

type backendOutput struct {
	Selections    map[string][]selectedEngine `json:"-"`
	ShuffleInputs map[string][][]string       `json:"shuffle_inputs"`
}

func decodeBackendOutput(t *testing.T, fixture backendFixture) backendOutput {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(fixture.Result.Output))
	decoder.UseNumber()
	var raw map[string]json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		t.Fatalf("decode %s output: %v", fixture.Result.Status, err)
	}

	output := backendOutput{Selections: make(map[string][]selectedEngine)}
	for _, testCase := range fixture.Input.Cases {
		encoded, exists := raw[testCase.Label]
		if !exists {
			t.Fatalf("output misses %s selection", testCase.Label)
		}
		var selection []selectedEngine
		if err := json.Unmarshal(encoded, &selection); err != nil {
			t.Fatalf("decode %s selection: %v", testCase.Label, err)
		}
		output.Selections[testCase.Label] = selection
	}
	if err := json.Unmarshal(raw["shuffle_inputs"], &output.ShuffleInputs); err != nil {
		t.Fatalf("decode shuffle inputs: %v", err)
	}
	return output
}

func categoriesFromBackendFixture(categories []backendFixtureCategory) []engine.Category {
	result := make([]engine.Category, len(categories))
	for index, category := range categories {
		metadata := make([]engine.Metadata, len(category.Engines))
		for metadataIndex, entry := range category.Engines {
			metadata[metadataIndex] = engine.Metadata{
				Name:     entry.Name,
				Category: category.Category,
				Provider: entry.Provider,
				Priority: entry.Priority,
				Disabled: entry.Disabled,
			}
		}
		result[index] = engine.Category{Category: category.Category, Engines: metadata}
	}
	return result
}

func projectSelection(selection []engine.Metadata) []selectedEngine {
	result := make([]selectedEngine, len(selection))
	for index, metadata := range selection {
		result[index] = selectedEngine{
			Name:     metadata.Name,
			Provider: metadata.Provider,
			Priority: metadata.Priority,
		}
	}
	return result
}

func loadBackendFixture(t *testing.T, path string) backendFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()

	var fixture backendFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func reverseStrings(values []string) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}
