# DDGS Python source audit

Audit date: 2026-07-20. This document is implementation evidence for the
`port-ddgs-python-library` OpenSpec change, not a replacement for source code
or contract fixtures.

## Frozen target

| Field | Value |
| --- | --- |
| Source repository | `https://github.com/deedy5/ddgs` |
| Local checkout | `/home/jcastillo/Proyectos/ddgs` |
| Commit | `a12929a72429a39a0841c3d7caacb20ee17acd4d` |
| Describe | `v9.14.4-2-ga12929a` |
| Python version field | `9.14.4` |
| License | MIT, Copyright (c) 2022 deedy5 |

The source commit is two commits after `v9.14.4`. Those commits remove the
earlier DHT/cache/API coupling, so a tag-only port would target wrong behavior.
The source code is authoritative over its README. Example: README advertises
text `bing`; `ddgs/engines/bing.py` sets `disabled = True`, so dynamic registry
excludes it.

## Scope split

Port:

- `ddgs/__init__.py`, `ddgs/ddgs.py`, `base.py`, `results.py`, `similarity.py`,
  `utils.py`, `exceptions.py`, `http_client.py`, `http_client2.py`, and active
  `engines/*.py` behavior.

Do not port:

- `cli.py` / Click output and download helpers.
- `api_server/` / FastAPI, Uvicorn, REST endpoints, MCP tools.
- Previously removed DHT/cache/network service behavior.
- Docker and operational service files.

## Core behavior map

| Source file | Observed responsibility | Port concern |
| --- | --- | --- |
| `__init__.py` | lazy `DDGS` import, version, null logging handler | Go has eager packages; preserve public behavior, not Python metaclass mechanics |
| `ddgs.py` | proxy configuration, engine cache, bounded concurrent dispatch, aggregation, ranker, extraction façade | high-risk state/concurrency boundary |
| `base.py` | engine protocol, request dispatch, lxml tree and XPath extraction | parser compatibility gate |
| `results.py` | category result shapes, setters normalize fields, count-based aggregator | dynamic `dict[str, Any]` output must not be narrowed silently |
| `similarity.py` | deterministic simple ranker | port exact bucket order, not generic relevance |
| `utils.py` | VQD extraction, URL/text/date normalization, proxy alias | edge-case contract tests needed |
| `http_client.py` | `primp.Client`, random impersonation, rendered response properties | transport and extraction gates |
| `http_client2.py` | temporary DDG text `httpx`/HTTP2 client and monkey patch | special fingerprint/race gate |

## Public Python surface to preserve semantically

`DDGS(proxy=None, timeout=5, *, verify=True)` exposes class variable `threads`
and methods:

| Method | Source return shape | Important inputs |
| --- | --- | --- |
| `text()` | `list[dict[str, Any]]` | query, region, safesearch, timelimit, max_results, page, backend |
| `images()` | `list[dict[str, Any]]` | common inputs plus size/color/type_image/layout/license_image |
| `news()` | `list[dict[str, Any]]` | common inputs |
| `videos()` | `list[dict[str, Any]]` | common inputs plus resolution/duration/license_videos |
| `books()` | `list[dict[str, Any]]` | query, max_results, page, backend |
| `extract()` | `{"url": str, "content": str | bytes}` | URL and format: markdown/plain/rich/raw text/raw bytes |

The Go API cannot literally copy Python keyword signatures. Before exposing it,
specify an idiomatic context/options façade plus a parity-preserving raw result
form. `context.Context` is required at Go I/O boundaries. A convenient typed
API cannot erase fields or change source error/result semantics.

## Active registry

Python uses runtime subclass discovery. Static Go registration must match this
frozen output exactly.

| Category | Engine | Provider label | Request/result complications |
| --- | --- | --- | --- |
| text | brave | brave | country cookies, safesearch, date/page mapping, HTML XPath |
| text | duckduckgo | bing | POST HTML; temporary `HttpClient2`; random fake-useragent; filters DDG ad redirect |
| text | google | google | random Android UA, consent cookie, redirect cleanup, HTML XPath |
| text | grokipedia | grokipedia | JSON typeahead; title/snippet transformation |
| text | mojeek | mojeek | region cookies, safe/page mapping, HTML XPath |
| text | startpage | google | bootstrap request obtains `sc`; POST with state, HTML XPath |
| text | wikipedia | wikipedia | JSON open-search then second extract request; priority 2 |
| text | yahoo | bing | random path tokens, redirect decode, HTML XPath |
| text | yandex | yandex | random search id, page mapping, HTML XPath |
| images | bing | bing | HTML XPath then JSON attribute parsing, image dimensions |
| images | duckduckgo | bing | initial VQD fetch then JSON endpoint; filters/pagination |
| news | bing | bing | HTML XPath, locale/relative-date conversion, image cleanup |
| news | duckduckgo | bing | VQD bootstrap then JSON endpoint |
| news | yahoo | yahoo | redirect/image/source/date cleanup, HTML XPath |
| videos | duckduckgo | bing | VQD bootstrap then heterogeneous JSON objects |
| books | annasarchive | annasarchive | randomized TLD, comment removal, relative URL repair, HTML XPath |

Disabled but source-present: text `bing`. Preserve its disabled registry status;
do not expose it as active merely because its adapter can be written.

Provider labels intentionally collapse several engine names. Dispatcher marks a
provider only after a completed future returns a nonempty result; it does not
reserve a provider at submit time. DuckDuckGo and Yahoo text both label as
`bing`; Google and Startpage both label as `google`; images Bing/DDG label as
`bing`; news Bing/DDG label as `bing`. Go scheduler must not strengthen this
into strict one-in-flight-provider behavior without explicit baseline change.

## Orchestration invariants

1. Backend is a comma-separated string. `auto` or `all` starts from shuffled
   registry keys. Text temporarily puts Wikipedia and Grokipedia first, then
   engine instances sort by descending `(priority, random)`. In frozen source
   `random` is a function object in that lambda, not a function call, so ties
   retain shuffled order under Python stable sort.
2. Invalid requested backend names only log a warning. If no valid engine is
   left, source recursively uses `auto`.
3. Engine instances are cached on each `DDGS` object.
4. Worker count is `min(uniqueProviders, ceil(max_results/10)+1)` when
   `max_results` is truthy; otherwise unique provider count. `DDGS.threads`
   can lower it. Do not add an arbitrary global worker limit.
5. Futures run engine searches and source records the last observed error.
   `FIRST_EXCEPTION` waiting and a batch condition based partly on enumerated
   engine position make partial-result timing observable. A result list is
   deduplicated, ranked, then sliced; no result raises timeout only when error
   text contains `timed out`, otherwise generic DDGS error.
6. Result aggregation uses the first cache field encountered in result field
   order: text `href`, images `image`, news `url`, videos `embed_url`, books
   `url`. It replaces duplicate cached item when incoming `body` is longer,
   increments frequency, then returns frequency order.
7. `SimpleFilterRanker` puts `wikipedia.org` hits first; skips titles with both
   `Category:` and `Wikimedia`; then returns both-title-and-body, title-only,
   body-only, and neither token buckets. Token split is `\W+`, lower-cased,
   minimum length 3, matching by substring.

Some source ordering is intentionally random/concurrency-dependent. Do not
claim byte-for-byte live ordering as a deterministic contract. Preserve the
algorithm and test controlled scheduler/fixture cases.

## Normalization and parsing details

- Text setter: if nonempty, regex strips tags (`<.*?>`), HTML-unescapes,
  Unicode NFC-normalizes, removes Unicode general category `C*`, then collapses
  whitespace.
- URL setter: Python `unquote`, then literal spaces become `+`. Do not use Go
  query-unescape if it changes `+` handling.
- Integer date setter converts Unix seconds to UTC Python `isoformat()`;
  engine date helpers also emit UTC strings and use current clock for relative
  values. Go RFC3339 `Z` differs lexically from Python `+00:00`; fixture test
  output exactly.
- HTML engines rely on lxml recovery plus XPath. Retain source XPath strings
  and validate every selector against saved responses before changing them.
- JSON engines assign keys directly. Missing/null/mixed values can survive into
  `dict[str, Any]`; a Go string-only struct would be a semantic loss.
- VQD extraction searches raw bytes for three exact marker variants and throws
  if none appears.

## Source dependencies

| Dependency | Declared role | Port implication |
| --- | --- | --- |
| `primp>=1.2.3` | default HTTP client, random browser/OS impersonation, certificates/proxy, rendered text | `net/http` has a different TLS/HTTP fingerprint; prove replacement per engine |
| `lxml>=4.9.4` | HTML parsing and XPath | choose only after lxml-vs-Go selector corpus passes |
| `httpx[http2,socks,brotli]>=0.28.1` | temporary DuckDuckGo text client | HTTP/2, SOCKS, compression and redirect behavior need an equivalent |
| `httpcore`, `h2` | imported by `http_client2.py` patch | source overrides H2 initial settings per request; Go solution must not globally patch shared state |
| `fake-useragent>=2.2.0` | random DDG text UA | decide/capture compatible UA pool and behavior |
| `click` | CLI only | excluded |
| `fastapi`, `uvicorn`, `mcp` | service/MCP extras | excluded |

The frozen source has no committed dependency lockfile. `pyproject.toml` gives
lower bounds only, so an exact Python reference environment cannot be recreated
from Git alone today. Task 2.1 must resolve, record, and retain the exact
installed runtime versions (and ideally wheel hashes) used to generate every
fixture. This record is fixture provenance, not a new upstream baseline.

Parser evaluation (task 4.1) selected pure-Go
`github.com/lestrrat-go/helium v0.6.0` behind `internal/parser`: its isolated
adapter probe matched all 14 frozen lxml XPath fixtures and its HTML/XPath
packages passed under `-race`. `github.com/antchfx/htmlquery v1.3.6` was
rejected after two semantic mismatches: Yahoo News XPath union returned
`primary.jpg` instead of lxml `fallback.jpg`, and malformed Startpage recovery
included `Body` in title. Details, license, cgo review, and update rule are in
`docs/dependency-decisions.md`. Tasks 4.2–4.5 then implemented the internal
adapter with all source XPath strings unchanged, `json.Decoder.UseNumber` for
JSON engines, race checks for independent/shared read-only documents, and
representative offline benchmarks. This does not prove engine request,
transport, TLS/fingerprint, or engine-specific post-processing parity.

## Implementation blockers and required responses

| Severity | Blocker | Required response |
| --- | --- | --- |
| critical | Browser TLS/HTTP2 fingerprint differences can cause engine blocks | design/prove a Go transport; no fake parity claim with default `net/http` |
| critical | `primp` rendered markdown/plain/rich output lacks a selected Go equivalent | create differential fixtures and select/implement renderer before `extract()` completion |
| critical | lxml XPath/recovery may differ | fixture corpus for all selectors; no hand-rewritten selectors without evidence |
| high | Cached mutable engines and global `HttpClient2.Patch` are unsafe under concurrent calls | isolate request state, close bodies, use contexts, race-test; document any semantic decision |
| high | Live search pages and anti-bot defenses change independently of code | offline golden tests primary; tagged slow/rate-limited integration secondary |
| high | Dynamic Python result values do not fit naïve Go structs | retain raw heterogeneous values and specify typed conversions explicitly |
| medium | Source version field differs from source commit | pin SHA and update only with an explicit source-diff audit |
| medium | Final Go module path unknown | decide before publishing first public version |
| medium | Random values/UAs/TLDs/tokens affect requests | inject deterministic random source in tests; use source-equivalent secure randomness at runtime |

## Verification plan

1. Create an isolated Python capture harness from frozen source. It must record
   request sequence, method, URL, query/body, selected headers/cookies, mock or
   captured response, normalized result, and exception type/message.
2. Store sanitized, reproducible fixtures under `testdata/`; never store proxy
   credentials, session cookies, or private responses.
3. Write Go RED tests from fixture contracts. Cover normalizers, payloads,
   selectors, post-processing, aggregation/ranking, errors, proxy aliases, and
   cancellation before engine implementation.
4. For every engine, compare Go behavior to frozen Python fixture output.
5. Use `//go:build integration` for live engines. Serialize and rate-limit
   requests; a successful live result validates connectivity, not full parity.
6. Run `go test ./...`, `go test -race ./...`, `go vet ./...`, and leak checks
   where goroutines are introduced.

Current local environment has Go `1.26.1`, Python `3.12.3`, and `uv`; Python
runtime dependencies were not installed at the start of audit. An isolated
reference environment was subsequently recorded in
[reference-environment.md](reference-environment.md); it must be compared
before collecting fixtures or regenerating them. See
[source-quirks.md](source-quirks.md) for known observable edge behavior absent
from upstream smoke tests.
