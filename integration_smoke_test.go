//go:build integration

package ddgs_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jcastilloa/goddgs"
)

const (
	integrationEnabledEnvironment = "GODDGS_INTEGRATION"
	integrationRequestTimeout     = 15 * time.Second
	integrationDelay              = 3 * time.Second
)

func TestIntegrationSmokeCategories(t *testing.T) {
	if os.Getenv(integrationEnabledEnvironment) != "1" {
		t.Skipf("set %s=1 to permit live source-engine smoke requests", integrationEnabledEnvironment)
	}

	client := ddgs.New(ddgs.WithTimeout(integrationRequestTimeout))
	cases := []struct {
		name   string
		search func(context.Context) ([]ddgs.RawResult, error)
	}{
		{
			name: "text-duckduckgo",
			search: func(ctx context.Context) ([]ddgs.RawResult, error) {
				return client.Text(ctx, "open source", ddgs.WithBackend("duckduckgo"), ddgs.WithMaxResults(1))
			},
		},
		{
			name: "images-duckduckgo",
			search: func(ctx context.Context) ([]ddgs.RawResult, error) {
				return client.Images(ctx, "open source", ddgs.WithBackend("duckduckgo"), ddgs.WithMaxResults(1))
			},
		},
		{
			name: "news-duckduckgo",
			search: func(ctx context.Context) ([]ddgs.RawResult, error) {
				return client.News(ctx, "open source", ddgs.WithBackend("duckduckgo"), ddgs.WithMaxResults(1))
			},
		},
		{
			name: "videos-duckduckgo",
			search: func(ctx context.Context) ([]ddgs.RawResult, error) {
				return client.Videos(ctx, "open source", ddgs.WithBackend("duckduckgo"), ddgs.WithMaxResults(1))
			},
		},
		{
			name: "books-annasarchive",
			search: func(ctx context.Context) ([]ddgs.RawResult, error) {
				return client.Books(ctx, "open source", ddgs.WithBackend("annasarchive"), ddgs.WithMaxResults(1))
			},
		},
	}

	for index, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), integrationRequestTimeout+5*time.Second)
			defer cancel()

			results, err := testCase.search(ctx)
			if err != nil {
				t.Fatalf("live smoke search: %v", err)
			}
			if len(results) == 0 {
				t.Fatal("live smoke search returned no results")
			}
			t.Logf("received %d result(s)", len(results))
		})
		if index < len(cases)-1 {
			time.Sleep(integrationDelay)
		}
	}
}
