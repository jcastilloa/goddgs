package parser

import (
	"context"
	"path/filepath"
	"testing"
)

func BenchmarkParser_XPathFixtures(b *testing.B) {
	paths, err := filepath.Glob("../../testdata/contracts/parser/*-xpath*.json")
	if err != nil {
		b.Fatalf("find parser fixtures: %v", err)
	}

	for _, path := range paths {
		fixture := loadXPathFixture(b, path)
		b.Run(fixture.FixtureID, func(b *testing.B) {
			queries := fieldQueries(b, fixture)
			html := fixture.Input.HTML
			if fixture.Input.PreProcessHTML == "remove_comment_delimiters" {
				html = RemoveCommentDelimiters(html)
			}

			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				document, err := ParseHTML(ctx, html)
				if err != nil {
					b.Fatal(err)
				}
				switch fixture.Contract.Operation {
				case "xpath_document_values":
					if _, err := document.Values(ctx, fixture.Input.XPath); err != nil {
						b.Fatal(err)
					}
				case "xpath_generic_extraction":
					if _, err := document.Extract(ctx, fixture.Input.ItemsXPath, queries); err != nil {
						b.Fatal(err)
					}
				default:
					b.Fatalf("unsupported fixture operation %q", fixture.Contract.Operation)
				}
			}
		})
	}
}

func BenchmarkDecodeJSONFixtures(b *testing.B) {
	paths, err := filepath.Glob("../../testdata/contracts/parser/*-json-*.json")
	if err != nil {
		b.Fatalf("find parser JSON fixtures: %v", err)
	}

	for _, path := range paths {
		fixture := loadJSONFixture(b, path)
		if fixture.Result.Status != "ok" {
			continue
		}
		b.Run(fixture.FixtureID, func(b *testing.B) {
			source := []byte(fixture.Input.JSON)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := DecodeJSON(source); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
