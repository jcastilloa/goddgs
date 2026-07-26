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
	result, err := client.Extract(
		ctx,
		"https://go.dev/doc/",
		ddgs.WithExtractFormat("text_plain"),
	)
	if err != nil {
		output.Fail(err)
	}
	if err := output.Write(map[string]any{
		"url":     result.URL,
		"content": result.Content,
	}); err != nil {
		output.Fail(err)
	}
}
