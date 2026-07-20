package engine

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type registryFixture struct {
	Result struct {
		Output registryFixtureOutput `json:"output"`
	} `json:"result"`
}

type registryFixtureOutput struct {
	Active   []Category `json:"active"`
	Disabled []Metadata `json:"disabled"`
}

func TestFrozenRegistry_MatchesFrozenSourceFixture(t *testing.T) {
	fixture := loadRegistryFixture(t, "../../testdata/contracts/pure/pure.engine-registry-active-and-disabled.json")
	registry := FrozenRegistry()

	got := registryFixtureOutput{
		Active:   registry.Categories(),
		Disabled: registry.Disabled(),
	}
	if !reflect.DeepEqual(got, fixture.Result.Output) {
		t.Fatalf("FrozenRegistry() = %#v, want %#v", got, fixture.Result.Output)
	}
}

func TestRegistry_ReturnsIndependentMetadataCopies(t *testing.T) {
	registry := FrozenRegistry()
	first, ok := registry.Active("text")
	if !ok || len(first) == 0 {
		t.Fatalf("Active(text) = %#v, %t; want frozen text metadata", first, ok)
	}
	first[0].Name = "mutated"

	second, ok := registry.Active("text")
	if !ok || second[0].Name != "brave" {
		t.Fatalf("second Active(text) = %#v, %t; registry leaked mutable metadata", second, ok)
	}

	categories := registry.Categories()
	categories[0].Engines[0].Name = "mutated"
	if got, ok := registry.Active("books"); !ok || got[0].Name != "annasarchive" {
		t.Fatalf("Active(books) after Categories mutation = %#v, %t; registry leaked mutable category", got, ok)
	}
}

func loadRegistryFixture(t *testing.T, path string) registryFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()

	var fixture registryFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}
