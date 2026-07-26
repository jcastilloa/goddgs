# goddgs

`goddgs` is a Go port of [`deedy5/ddgs`](https://github.com/deedy5/ddgs), the
metasearch library. It runs text, image, news, video and book searches across
many engines behind a single client, and fetches page content — all as an
embeddable Go module. No CLI, HTTP server or MCP layer.

The port reproduces the behavior of a frozen upstream commit: request payloads,
engine selection, aggregation, ranking, normalization, and a browser-realistic
TLS/HTTP2 fingerprint. Result values keep their original dynamic types, so no
information is silently narrowed .

## Install

```bash
go get github.com/jcastilloa/goddgs
```

Requires Go 1.26.1 or newer. The build intentionally rejects Go 1.27+ until a
Unicode-table (`x/text`) rebaseline is reviewed .

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"time"

	ddgs "github.com/jcastilloa/goddgs"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client := ddgs.New(ddgs.WithTimeout(15 * time.Second))

	results, err := client.Text(ctx, "open source", ddgs.WithMaxResults(5))
	if err != nil {
		panic(err)
	}

	for _, r := range results {
		fmt.Println(r["title"], "-", r["href"])
	}
}
```

Every search method returns `[]ddgs.RawResult`, where `RawResult` is a
`map[string]any`. Fields and their types (strings, numbers, nested maps, nulls)
are preserved exactly as the source engines return them .

## Client configuration

`ddgs.New` accepts these options; the client's identity and transport are fixed
at construction:

| Option | Description | Default |
| --- | --- | --- |
| `WithTimeout(d)` | Request timeout | `5s` |
| `WithoutTimeout()` | No timeout | — |
| `WithProxy(p)` | Proxy URL; `"tb"` expands to the Tor Browser SOCKS proxy (`socks5h://127.0.0.1:9150`) | env `DDGS_PROXY` |
| `WithTLSVerification(b)` | Enable/disable certificate verification | `true` |
| `WithTLSRootCAFile(path)` | Use a PEM root certificate file | — |

An empty proxy value falls through to the `DDGS_PROXY` environment variable;
only a non-empty explicit proxy overrides it .

## Search operations

All search methods share the same signature and options:

```go
func (d *DDGS) Text  (ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
func (d *DDGS) Images(ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
func (d *DDGS) News  (ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
func (d *DDGS) Videos(ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
func (d *DDGS) Books (ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
```

An empty `query` returns an error, and a cancelled/expired `ctx` is honored
before any request .

### Common options

| Option | Description | Default |
| --- | --- | --- |
| `WithRegion(r)` | Source region, e.g. `"us-en"` | `"us-en"` |
| `WithSafeSearch(s)` | `"on"`, `"moderate"`, `"off"` | `"moderate"` |
| `WithTimeLimit(t)` | Time filter, e.g. `"d"`, `"w"`, `"m"`, `"y"` | unset |
| `WithMaxResults(n)` | Maximum results to return | `10` |
| `WithoutMaxResults()` | Unlimited-result semantics | — |
| `WithPage(p)` | Result page | `1` |
| `WithBackend(b)` | Engine(s), comma-separated, or `"auto"` / `"all"` | `"auto"` |

### Image options

`WithImageSize`, `WithImageColor`, `WithImageType`, `WithImageLayout`,
`WithImageLicense` .

### Video options

`WithVideoResolution`, `WithVideoDuration`, `WithVideoLicense` .

## Backends by category

Target a specific engine with `WithBackend`, or leave the default `"auto"` to
fan out across every engine in the category .

| Category | Backends |
| --- | --- |
| Text | `brave`, `duckduckgo`, `google`, `grokipedia`, `mojeek`, `startpage`, `wikipedia`, `yahoo`, `yandex` |
| Images | `bing`, `duckduckgo` |
| News | `bing`, `duckduckgo`, `yahoo` |
| Videos | `duckduckgo` |
| Books | `annasarchive` |

Text `bing` exists upstream but is disabled at the source; requesting it
explicitly follows the invalid-backend fallback path rather than activating it .

## Content extraction

`Extract` fetches a URL and returns its content in the requested format:

```go
res, err := client.Extract(ctx, "https://example.com",
	ddgs.WithExtractFormat("text_markdown"),
)
// res.URL     string
// res.Content any  // string for rendered/raw text, []byte for raw content
```

Formats: `text_markdown` (default), `text_plain`, `text_rich`, plus raw text and
raw bytes . The rendered Markdown/plain/rich output uses a practical Go
renderer and is **not** byte-identical to the upstream `primp` rendering; raw
fetch, bytes and error behavior are faithful .

## Error handling

Errors are classifiable with `errors.Is`:

```go
if errors.Is(err, ddgs.ErrTimeout) {
	// ...
}
```

Sentinels: `ddgs.ErrDDGS`, `ddgs.ErrTimeout`, `ddgs.ErrRateLimit` .

## Behavior notes

- **Fingerprint:** each client picks one coherent browser/OS identity (from 115
  frozen profiles) covering TLS, headers and HTTP/2 as a single bundle, chosen
  once at construction — never randomized per request. This holds across direct,
  PEM, disabled-verify, HTTP(S) CONNECT and SOCKS routes. Plain HTTP stays
  standard. DuckDuckGo text uses a separate, distinct client .
- **Context:** a `context.Context` is required at every I/O boundary and is
  honored for cancellation and timeouts .
- **Frozen semantics:** engine quirks (random tokens, cookie scoping, source
  error classes) are preserved on purpose rather than "fixed" .

## License and attribution

Ported from the MIT-licensed upstream `deedy5/ddgs`. See [LICENSE](LICENSE) and
[NOTICE.md](NOTICE.md) for third-party dependency notices. Attribution does not
imply upstream endorsement.
