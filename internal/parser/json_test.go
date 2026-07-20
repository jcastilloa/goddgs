package parser

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type jsonFixture struct {
	FixtureID string `json:"fixture_id"`
	Contract  struct {
		Operation string `json:"operation"`
	} `json:"contract"`
	Input struct {
		JSON string `json:"json"`
	} `json:"input"`
	Result struct {
		Status string          `json:"status"`
		Output json.RawMessage `json:"output"`
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

	for _, path := range paths {
		fixture := loadJSONFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			if fixture.Contract.Operation != "json_loads" {
				t.Fatalf("operation = %q, want json_loads", fixture.Contract.Operation)
			}

			got, err := DecodeJSON([]byte(fixture.Input.JSON))
			if fixture.Result.Status == "error" {
				if err == nil {
					t.Fatal("DecodeJSON error = nil, want malformed Python source input rejection")
				}
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
