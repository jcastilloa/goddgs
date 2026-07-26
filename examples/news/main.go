package main

import (
	"context"
	"time"

	ddgs "github.com/jcastilloa/goddgs"
	"github.com/jcastilloa/goddgs/examples/internal/output"
)

const requestTimeout = 15 * time.Second

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	client := ddgs.New(ddgs.WithTimeout(requestTimeout))
	results, err := client.News(
		ctx,
		"open source",
		ddgs.WithBackend("startpage"),
		ddgs.WithMaxResults(1),
	)
	if err != nil {
		output.Fail(err)
	}
	if err := output.Write(results); err != nil {
		output.Fail(err)
	}
}
