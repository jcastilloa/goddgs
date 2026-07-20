package parser

import (
	"context"
	"sync"
	"testing"
)

func TestParser_ConcurrentIndependentDocuments(t *testing.T) {
	fixture := loadXPathFixture(t, "../../testdata/contracts/parser/parser.news.yahoo-generic-xpath.json")
	queries := fieldQueries(t, fixture)

	const workers = 32
	start := make(chan struct{})
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			<-start
			document, err := ParseHTML(context.Background(), fixture.Input.HTML)
			if err != nil {
				errors <- err
				return
			}
			if _, err := document.Extract(context.Background(), fixture.Input.ItemsXPath, queries); err != nil {
				errors <- err
			}
		})
	}

	close(start)
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
}

func TestParser_ConcurrentReadOnlyDocument(t *testing.T) {
	fixture := loadXPathFixture(t, "../../testdata/contracts/parser/parser.news.yahoo-generic-xpath.json")
	queries := fieldQueries(t, fixture)
	document, err := ParseHTML(context.Background(), fixture.Input.HTML)
	if err != nil {
		t.Fatalf("ParseHTML: %v", err)
	}

	const workers = 32
	start := make(chan struct{})
	errors := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			<-start
			if _, err := document.Extract(context.Background(), fixture.Input.ItemsXPath, queries); err != nil {
				errors <- err
			}
		})
	}

	close(start)
	group.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
}
