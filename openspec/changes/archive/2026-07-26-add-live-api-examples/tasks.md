## 1. Runnable API documentation

- [x] 1.1 Add a small example-only JSON output helper that preserves raw search
  values and reports errors to standard error. Evidence: `examples/internal/output`
  indents JSON to standard output and reports failures to standard error.
- [x] 1.2 Add standalone real-request programs for text, images, news,
  videos, books, and extraction using public `ddgs` APIs, bounded context,
  compact result limits, explicit active backends, and representative
  category-specific options. Evidence: six `examples/*/main.go` packages use
  only public APIs and compiled with `go build ./examples/...`.
- [x] 1.3 Add `examples/README.md` with every run command, network warning,
  output description, and parity/integration-test boundary. Evidence: README
  lists each command and distinguishes live observations from offline parity.

## 2. Verification and evidence

- [x] 2.1 Format and compile examples without network access; run project
  static analysis, offline unit tests, and race tests. Evidence: final
  `go build ./examples/...` and `make verify` passed on 2026-07-26.
- [x] 2.2 Run one documented example and the serialized opt-in integration
  smoke suite in a permitted rate window; record live availability as an
  observation rather than a parity assertion. Evidence: `go run
  ./examples/text` and `go run ./examples/news` made live DuckDuckGo requests
  and returned `No results found.`; `make integration` ran every category but
  failed text/images with the same response and books because
  `annas-archive.pk` dialed `127.0.0.1:443` and was refused. News and videos
  passed in that run. These are volatile external observations, not fixture or
  parity failures.
- [x] 2.3 Update `MEMORY.md` with scope, skill applicability decisions,
  exact verification commands, live observations, and remaining risks.
  Evidence: `Current state — 2026-07-26` records all six areas and preserves
  external availability as non-parity evidence.
