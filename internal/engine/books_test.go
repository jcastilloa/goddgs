package engine

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
)

const annasArchiveFixtureSearchURL = "https://annas-archive.fixture/search"

var annasArchiveSearchURLPattern = regexp.MustCompile(`^https://annas-archive\.(gd|gl|pk)/search$`)

func TestAnnasArchive_SearchMatchesFrozenFixtures(t *testing.T) {
	testCategoryHTMLTextEngineFixturesWithFixture(t, "books", "annasarchive", func(_ *testing.T, client htmlTextTransport, _ htmlTextEngineFixture) Searcher {
		return newAnnasArchiveWithSearchURL(client, annasArchiveFixtureSearchURL)
	})
}

func TestAnnasArchive_FixedURLMatchesFrozenLifetimeFixture(t *testing.T) {
	var fixture struct {
		Input struct {
			FixedSearchURL string `json:"fixed_search_url"`
		} `json:"input"`
		Result struct {
			Output struct {
				FirstSearchURL  string `json:"first_search_url"`
				SecondSearchURL string `json:"second_search_url"`
				SameObjectValue bool   `json:"same_object_value"`
			} `json:"output"`
		} `json:"result"`
	}
	loadHTMLTextFixture(t, "../../testdata/contracts/pure/pure.annasarchive-module-lifetime-search-url.json", &fixture)

	first := newAnnasArchiveWithSearchURL(nil, fixture.Input.FixedSearchURL)
	second := newAnnasArchiveWithSearchURL(nil, fixture.Input.FixedSearchURL)
	if first.searchURL != fixture.Result.Output.FirstSearchURL {
		t.Fatalf("first search URL = %q, want %q", first.searchURL, fixture.Result.Output.FirstSearchURL)
	}
	if second.searchURL != fixture.Result.Output.SecondSearchURL {
		t.Fatalf("second search URL = %q, want %q", second.searchURL, fixture.Result.Output.SecondSearchURL)
	}
	if (first.searchURL == second.searchURL) != fixture.Result.Output.SameObjectValue {
		t.Fatalf("same search URL = %t, want %t", first.searchURL == second.searchURL, fixture.Result.Output.SameObjectValue)
	}
}

func TestNewAnnasArchive_ReusesOneProcessLifetimeSearchURL(t *testing.T) {
	first, err := NewAnnasArchive(&scriptedHTMLTextTransport{})
	if err != nil {
		t.Fatalf("NewAnnasArchive(first): %v", err)
	}
	second, err := NewAnnasArchive(&scriptedHTMLTextTransport{})
	if err != nil {
		t.Fatalf("NewAnnasArchive(second): %v", err)
	}
	if first.searchURL != second.searchURL {
		t.Fatalf("search URLs differ: %q != %q", first.searchURL, second.searchURL)
	}
	if !annasArchiveSearchURLPattern.MatchString(first.searchURL) {
		t.Fatalf("search URL = %q, want frozen source shape", first.searchURL)
	}
}

func TestNewAnnasArchive_ConcurrentConstructionReusesProcessLifetimeSearchURL(t *testing.T) {
	const constructors = 32

	urls := make(chan string, constructors)
	errorsByConstructor := make(chan error, constructors)
	var group sync.WaitGroup
	for range constructors {
		group.Add(1)
		go func() {
			defer group.Done()
			adapter, err := NewAnnasArchive(&scriptedHTMLTextTransport{})
			if err != nil {
				errorsByConstructor <- err
				return
			}
			urls <- adapter.searchURL
		}()
	}
	group.Wait()
	close(errorsByConstructor)
	close(urls)

	for err := range errorsByConstructor {
		t.Errorf("NewAnnasArchive: %v", err)
	}
	var first string
	for url := range urls {
		if first == "" {
			first = url
			continue
		}
		if url != first {
			t.Fatalf("search URLs differ: %q != %q", url, first)
		}
	}
	if first == "" {
		t.Fatal("NewAnnasArchive returned no search URL")
	}
}

func TestAnnasArchive_SearchHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results, err := newAnnasArchiveWithSearchURL(&scriptedHTMLTextTransport{}, annasArchiveFixtureSearchURL).Search(ctx, SearchRequest{Query: "fixture"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search error = %v, want context.Canceled", err)
	}
	if results != nil {
		t.Fatalf("Search results = %#v, want nil", results)
	}
}

func TestAnnasArchive_SearchPropagatesTransportError(t *testing.T) {
	sourceError := errors.New("fixture transport failure")
	results, err := newAnnasArchiveWithSearchURL(&scriptedHTMLTextTransport{err: sourceError}, annasArchiveFixtureSearchURL).Search(
		context.Background(),
		SearchRequest{Query: "fixture"},
	)
	if !errors.Is(err, sourceError) {
		t.Fatalf("Search error = %v, want wrapped source error", err)
	}
	if results != nil {
		t.Fatalf("Search results = %#v, want nil", results)
	}
}

func TestAnnasArchive_SearchIsConcurrentSafe(t *testing.T) {
	fixture := loadHTMLTextEngineFixture(t, "../../testdata/contracts/engine/engine.books.annasarchive-happy-comment-and-relative-url.json")
	client := routingHTMLTextTransportFromFixture(t, fixture)
	adapter := newAnnasArchiveWithSearchURL(client, annasArchiveFixtureSearchURL)

	const calls = 32
	errorsByCall := make(chan error, calls)
	var group sync.WaitGroup
	for range calls {
		group.Add(1)
		go func() {
			defer group.Done()
			results, err := adapter.Search(context.Background(), htmlTextSearchRequest(fixture))
			if err != nil {
				errorsByCall <- err
				return
			}
			if len(results) != 1 {
				errorsByCall <- errors.New("unexpected book result count")
			}
		}()
	}
	group.Wait()
	close(errorsByCall)
	for err := range errorsByCall {
		t.Errorf("concurrent Search: %v", err)
	}
	if got, want := client.RequestCount(), calls; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
}
