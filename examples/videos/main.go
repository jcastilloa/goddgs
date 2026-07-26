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
	results, err := client.Videos(
		ctx,
		"Go conference talk",
		ddgs.WithBackend("duckduckgo"),
		ddgs.WithMaxResults(3),
		ddgs.WithVideoResolution("high"),
		ddgs.WithVideoDuration("short"),
		ddgs.WithVideoLicense("creativeCommon"),
	)
	if err != nil {
		output.Fail(err)
	}
	if err := output.Write(results); err != nil {
		output.Fail(err)
	}
}
