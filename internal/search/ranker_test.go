package search

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type rankerFixture struct {
	FixtureID string `json:"fixture_id"`
	Input     struct {
		Query     string           `json:"query"`
		Queries   []string         `json:"queries"`
		Documents []map[string]any `json:"documents"`
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

func TestSimpleFilterRanker_MatchesFrozenFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/pure/pure.ranker-*.json")
	if err != nil {
		t.Fatalf("find ranker fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no ranker fixtures")
	}

	for _, path := range paths {
		fixture := loadRankerFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			ranker := NewSimpleFilterRanker()
			if fixture.FixtureID == "pure.ranker-unicode-word-tokens" {
				assertRankerUnicodeFixture(t, ranker, fixture)
				return
			}

			got, err := ranker.Rank(fixture.Input.Documents, fixture.Input.Query)
			if fixture.Result.Status == "error" {
				if err == nil {
					t.Fatalf("Rank error = nil, want %s: %q", fixture.Result.Error.Type, fixture.Result.Error.Message)
				}
				if actual := err.Error(); actual != fixture.Result.Error.Message {
					t.Fatalf("Rank error = %q, want %q", actual, fixture.Result.Error.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("Rank: %v", err)
			}
			want := decodeRankerDocuments(t, fixture.FixtureID, fixture.Result.Output)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Rank = %#v, want %#v", got, want)
			}
		})
	}
}

func loadRankerFixture(t *testing.T, path string) rankerFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.UseNumber()

	var fixture rankerFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}

func decodeRankerDocuments(t *testing.T, fixtureID string, encoded json.RawMessage) []map[string]any {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var documents []map[string]any
	if err := decoder.Decode(&documents); err != nil {
		t.Fatalf("decode %s ranker output: %v", fixtureID, err)
	}
	return documents
}

func assertRankerUnicodeFixture(t *testing.T, ranker *SimpleFilterRanker, fixture rankerFixture) {
	t.Helper()

	var output struct {
		Tokens map[string][]string `json:"tokens"`
		Ranked []map[string]any    `json:"ranked"`
	}
	decoder := json.NewDecoder(bytes.NewReader(fixture.Result.Output))
	decoder.UseNumber()
	if err := decoder.Decode(&output); err != nil {
		t.Fatalf("decode %s unicode output: %v", fixture.FixtureID, err)
	}
	for _, query := range fixture.Input.Queries {
		if got, want := ranker.Tokens(query), output.Tokens[query]; !reflect.DeepEqual(got, want) {
			t.Fatalf("Tokens(%q) = %#v, want %#v", query, got, want)
		}
	}
	got, err := ranker.Rank(fixture.Input.Documents, fixture.Input.Queries[0])
	if err != nil {
		t.Fatalf("Rank unicode fixture: %v", err)
	}
	if !reflect.DeepEqual(got, output.Ranked) {
		t.Fatalf("Rank unicode fixture = %#v, want %#v", got, output.Ranked)
	}
}
