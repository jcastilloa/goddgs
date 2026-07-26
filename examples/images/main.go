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
	results, err := client.Images(
		ctx,
		"northern lights landscape",
		ddgs.WithBackend("duckduckgo"),
		ddgs.WithMaxResults(3),
		ddgs.WithImageSize("Large"),
		ddgs.WithImageColor("Blue"),
		ddgs.WithImageType("photo"),
		ddgs.WithImageLayout("Wide"),
		ddgs.WithImageLicense("any"),
	)
	if err != nil {
		output.Fail(err)
	}
	if err := output.Write(results); err != nil {
		output.Fail(err)
	}
}
