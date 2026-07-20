package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type xpathFixture struct {
	FixtureID string `json:"fixture_id"`
	Contract  struct {
		Operation string `json:"operation"`
	} `json:"contract"`
	Input struct {
		HTML           string            `json:"html"`
		ItemsXPath     string            `json:"items_xpath"`
		ElementsXPath  map[string]string `json:"elements_xpath"`
		ElementsOrder  []string          `json:"elements_order"`
		PreProcessHTML string            `json:"pre_process_html"`
		XPath          string            `json:"xpath"`
	} `json:"input"`
	Result struct {
		Output json.RawMessage `json:"output"`
	} `json:"result"`
}

type xpathGenericOutput struct {
	ItemCount int                `json:"item_count"`
	Items     []xpathGenericItem `json:"items"`
}

type xpathGenericItem struct {
	Fields map[string]xpathValues `json:"fields"`
}

type xpathValues struct {
	Raw    []string `json:"raw"`
	Joined string   `json:"joined"`
}

func TestParser_MatchesFrozenLXMLXPathFixtures(t *testing.T) {
	paths, err := filepath.Glob("../../testdata/contracts/parser/*-xpath*.json")
	if err != nil {
		t.Fatalf("find parser fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no parser fixtures")
	}

	for _, path := range paths {
		fixture := loadXPathFixture(t, path)
		t.Run(fixture.FixtureID, func(t *testing.T) {
			html := fixture.Input.HTML
			if fixture.Input.PreProcessHTML == "remove_comment_delimiters" {
				html = RemoveCommentDelimiters(html)
			}

			document, err := ParseHTML(t.Context(), html)
			if err != nil {
				t.Fatalf("ParseHTML: %v", err)
			}

			switch fixture.Contract.Operation {
			case "xpath_document_values":
				assertDocumentValuesFixture(t, document, fixture)
			case "xpath_generic_extraction":
				assertGenericExtractionFixture(t, document, fixture)
			default:
				t.Fatalf("unsupported fixture operation %q", fixture.Contract.Operation)
			}
		})
	}
}

func assertDocumentValuesFixture(t *testing.T, document *Document, fixture xpathFixture) {
	t.Helper()

	got, err := document.Values(t.Context(), fixture.Input.XPath)
	if err != nil {
		t.Fatalf("Values(%q): %v", fixture.Input.XPath, err)
	}
	var want []string
	if err := json.Unmarshal(fixture.Result.Output, &want); err != nil {
		t.Fatalf("decode %s output: %v", fixture.FixtureID, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Values(%q) = %#v, want %#v", fixture.Input.XPath, got, want)
	}
}

func assertGenericExtractionFixture(t *testing.T, document *Document, fixture xpathFixture) {
	t.Helper()

	queries := fieldQueries(t, fixture)

	got, err := document.Extract(t.Context(), fixture.Input.ItemsXPath, queries)
	if err != nil {
		t.Fatalf("Extract(%q): %v", fixture.Input.ItemsXPath, err)
	}

	var want xpathGenericOutput
	if err := json.Unmarshal(fixture.Result.Output, &want); err != nil {
		t.Fatalf("decode %s output: %v", fixture.FixtureID, err)
	}
	if len(got) != want.ItemCount {
		t.Fatalf("Extract item count = %d, want %d", len(got), want.ItemCount)
	}
	if len(want.Items) != want.ItemCount {
		t.Fatalf("fixture item count = %d, items = %d", want.ItemCount, len(want.Items))
	}

	for itemIndex, wantItem := range want.Items {
		gotItem := got[itemIndex]
		if len(gotItem.Fields) != len(fixture.Input.ElementsOrder) {
			t.Fatalf("item %d field count = %d, want %d", itemIndex, len(gotItem.Fields), len(fixture.Input.ElementsOrder))
		}
		for fieldIndex, name := range fixture.Input.ElementsOrder {
			gotField := gotItem.Fields[fieldIndex]
			if gotField.Name != name {
				t.Fatalf("item %d field %d name = %q, want %q", itemIndex, fieldIndex, gotField.Name, name)
			}
			wantField, exists := wantItem.Fields[name]
			if !exists {
				t.Fatalf("fixture item %d misses field %q", itemIndex, name)
			}
			if !reflect.DeepEqual(gotField.Raw, wantField.Raw) {
				t.Fatalf("item %d %s raw = %#v, want %#v", itemIndex, name, gotField.Raw, wantField.Raw)
			}
			if gotField.Joined != wantField.Joined {
				t.Fatalf("item %d %s joined = %q, want %q", itemIndex, name, gotField.Joined, wantField.Joined)
			}
		}
	}
}

func fieldQueries(t testing.TB, fixture xpathFixture) []FieldQuery {
	t.Helper()

	queries := make([]FieldQuery, 0, len(fixture.Input.ElementsOrder))
	for _, name := range fixture.Input.ElementsOrder {
		expression, exists := fixture.Input.ElementsXPath[name]
		if !exists {
			t.Fatalf("fixture order names unknown field %q", name)
		}
		queries = append(queries, FieldQuery{Name: name, XPath: expression})
	}
	return queries
}

func loadXPathFixture(t testing.TB, path string) xpathFixture {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture xpathFixture
	if err := json.Unmarshal(contents, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return fixture
}
