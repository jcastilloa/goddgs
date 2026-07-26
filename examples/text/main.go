package main

import (
	"context"
	"time"

	"github.com/jcastilloa/goddgs"
	"github.com/jcastilloa/goddgs/examples/internal/output"
)

const requestTimeout = 15 * time.Second

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	client := ddgs.New(ddgs.WithTimeout(requestTimeout))
	results, err := client.Text(
		ctx,
		"Go context package",
		ddgs.WithBackend("duckduckgo"),
		ddgs.WithMaxResults(3),
		ddgs.WithRegion("us-en"),
		ddgs.WithSafeSearch("moderate"),
		ddgs.WithTimeLimit("m"),
	)
	if err != nil {
		output.Fail(err)
	}
	if err := output.Write(results); err != nil {
		output.Fail(err)
	}
}
