# Live API examples

Every program below sends a real external request only when you run it. It
prints indented JSON to standard output; errors go to standard error and end
with a nonzero status. Results, ordering, availability, and engine acceptance
can change at any time, so this output is a live observation, not a parity or
ranking assertion.

Run one example at a time in a permitted rate window:

```bash
go run ./examples/text
go run ./examples/images
go run ./examples/news
go run ./examples/videos
go run ./examples/books
go run ./examples/extract
```

The search examples use explicit frozen active backends and request at most
three results. They show `context.Context`, `ddgs.New`, client timeout,
search options, category-specific image/video options, and lossless
`[]ddgs.RawResult` JSON output. The extraction example fetches `go.dev` and
renders practical `text_plain` content; it does not claim `primp`-identical
rendering.

Compile every example without making a network request:

```bash
go build ./examples/...
```

These programs are documentation source, not a supported CLI. Default tests
must stay offline. For the project's serialized, rate-limited opt-in live
smoke suite, see [../docs/integration.md](../docs/integration.md). Frozen
offline fixtures, rather than any example output, remain the source-parity
evidence.
