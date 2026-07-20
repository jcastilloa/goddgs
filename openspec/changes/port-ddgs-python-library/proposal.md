## Why

Programs written in Go need to embed DDGS metasearch behavior without running a
Python process or an API service. The valuable part of DDGS is its direct
search-engine integration, whose request, anti-bot, parsing, normalization,
aggregation, and ranking behavior must be preserved rather than approximated.

## What Changes

- Add an importable Go module that ports frozen upstream DDGS library behavior
  from commit `a12929a72429a39a0841c3d7caacb20ee17acd4d`.
- Define a Go-native public façade while preserving source result fields,
  options, errors, and observable search semantics.
- Port active text, image, news, video, and book engines; preserve source
  disabled-engine status and source registry/provider behavior.
- Port HTTP/proxy/TLS, cookies, request bootstrapping, HTML/XPath/JSON parsing,
  normalization, deduplication, ranking, bounded fan-out, and extraction.
- Establish a frozen Python differential-fixture harness and tagged live
  integration tests as parity evidence.
- Add source attribution, audit documentation, and module-specific agent rules.
- **BREAKING:** This project deliberately does not provide upstream Python CLI,
  FastAPI API server, MCP server, DHT/cache, or container service behavior.

## Capabilities

### New Capabilities

- `go-library-module`: Importable Go DDGS façade, configuration, errors, and
  module architecture with no service surface.
- `search-engine-parity`: Source-compatible engine registry, transport,
  request construction, parsing, normalization, aggregation, and ranking.
- `content-extraction-parity`: Source-compatible URL fetching and raw/plain/
  rich/Markdown content output.
- `parity-verification`: Offline Python-vs-Go differential fixtures, race-safe
  tests, and opt-in live engine verification.

### Modified Capabilities

None.

## Impact

- Adds root Go module `github.com/jcastillo/goddgs` (provisional path) and
  internal library packages only.
- Adds dependencies only after parser/transport/license evidence proves they
  are necessary for source parity.
- Depends on frozen local Python source at `/home/jcastillo/Proyectos/ddgs` for
  contracts and fixture capture.
- Requires explicit handling of browser TLS/HTTP2 fingerprinting, lxml XPath
  compatibility, and source content-rendering behavior before a 1:1 claim.
