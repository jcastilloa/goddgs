# MEMORY.md — goddgs

Persistent project state. Read before changing behavior. Update after every
material decision, completed OpenSpec task, source-baseline change, blocker, or
verification result.

## Current state — 2026-07-25

- Target: Go library port of DDGS only. No API server, CLI, MCP, DHT, cache,
  Docker service, or executable entrypoint.
- Module scaffold exists. Root package is `ddgs`; module path is provisional:
  `github.com/jcastillo/goddgs`.
- Active OpenSpec change: `openspec/changes/port-ddgs-python-library/`.
- Public façade/configuration, normalizers, ordered result aggregation, ranker,
  backend selection/static registry, isolated fixture-tested scheduler core,
  offline HTML/XPath/JSON parser adapter, isolated base transport,
  request-local DDG text standard-HTTP/2 transport, and fixture-backed
  DuckDuckGo, Grokipedia, Wikipedia, Brave, Google, Mojeek, Startpage, Yahoo,
  and Yandex text adapters; Bing/DuckDuckGo image adapters; Bing/DuckDuckGo/
  Yahoo News adapters; DuckDuckGo Videos; and Anna's Archive Books are complete.
  Task 7.7 composes all active frozen adapters into the public client with lazy
  per-client caching, fresh isolated transports, source-order construction before
  scheduler work, ordered keyword forwarding, and non-normalizing results. No
  live search-engine proof, source TLS/H2 fingerprint parity, renderer, or
  extraction implementation exists yet; those gates remain intentional.
- Tasks 2.1–2.7 are complete. The isolated Python oracle lives temporarily at
  `/tmp/goddgs-reference-a12929a`; exact resolved packages and rebuild steps
  are in `docs/reference-environment.md`. It made no external engine request.
- Fixture corpus has 368 deterministic synthetic/offline contracts: 138 pure,
  179 engine-visible, 9 extract, 24 parser, and 18 transport contracts under
  their
  respective `testdata/contracts/` directories. `tools/reference_capture.py --check`
  validates frozen SHA, resolved-package provenance, result/error shape, trace
  order, kind-specific redaction, output separation, a dynamic matrix for every
  active engine (option-bearing success, empty 200, malformed 200, 503/None),
  and fixture sanitation. Extract/transport capture uses only ephemeral
  loopback with synthetic HTML/bytes and rewrites its URL before output.
  Parser, base transport, and request-local DDG HTTP/2 transport have
  independently consumed their relevant offline fixtures; the doubles/loopback
  still do not prove renderer or browser/TLS-H2 fingerprint parity.
- Task 7.3 is intentionally RED-only: `internal/extract` has a context-first
  fetcher/renderer boundary and fixture tests for raw bytes/text, selected
  render representation, non-200, config forwarding and cancellation. The
  focused test fails only at the unavailable extractor implementation. Task
  7.4 rejected three Go renderer candidates; source-compatible extraction must
  not be implemented until a reviewed renderer passes the frozen corpus.
- Task 5.5 is complete as an evidence gate, not a parity approval:
  `docs/fingerprint-gate.md` records one sanitized, tagged TLS/HTTP2
  observation per Python and Go client. Both Go clients negotiated HTTP/2 but
  differed from `primp` and `HttpClient2` hashes, so every active engine stays
  explicitly incomplete for browser-fingerprint parity.
- Task 6.5 design decision: source image filters cross the Go façade as ordered
  source-keyword options (`size`, `color`, `type_image`, `layout`,
  `license_image`), while public `max_results` remains scheduler/slicing-only.
  Direct Bing adapter fixtures may still set engine-only `max_results`; this is
  required to retain frozen `_search_sync` data flow.
- Task 6.5 complete: image adapters use immutable ordered parameters and an
  injected request port. Bing preserves engine-only Python `int`, literal
  long-form timelimit, metadata/dimension errors, and source provider collision;
  DuckDuckGo preserves header order, VQD/bootstrap/error order, six filter
  slots, raw image values, and result-root errors. They are composed through
  task 7.7 but remain without browser/TLS-H2 fingerprint proof.
- Task 6.6 complete: News adapters use injected request ports and per-instance
  UTC clock seams. Bing preserves payload mapping, source local-zone absolute
  date conversion, localized relative dates, and image truncation; DuckDuckGo
  preserves VQD-before-safe validation and raw dynamic `results` iteration;
  Yahoo preserves fixed-relative-date units, URL/image/source cleanup, and its
  broad partial postprocess failure. They are composed through task 7.7 but
  remain without browser/TLS-H2 fingerprint proof.
- Task 6.7 complete: DuckDuckGo Videos uses an injected request port and
  preserves VQD-before-safe validation, mandatory four-slot filters, page
  stride 60, nested heterogeneous video values, and raw iterable/attribute
  errors. Video filters cross the façade as ordered `resolution`, `duration`,
  and `license_videos` keywords; public `max_results` remains scheduler-only.
  It is composed through task 7.7 but remains without browser/TLS-H2
  fingerprint proof.
- Task 6.8 complete: Anna's Archive uses a private process-lifetime,
  synchronized TLD selector and an immutable per-adapter search URL. Fixtures
  preserve its ordered GET pagination, HTML comment delimiter removal,
  nil-versus-empty response boundary, and the intentional base prefix on an
  already-absolute URL. It is composed through task 7.7 but remains without
  browser/TLS-H2 fingerprint proof.
- Task 6.9 complete: disabled text Bing is captured as source metadata outside
  the active text registry. Explicit `backend="bing"` runs the source invalid
  backend fallback to `auto` and performs two shuffle calls; no active Go Bing
  adapter was added.
- Local target repo had no commits when work began. Existing `.codex`,
  `.claude`, `.opencode`, and `openspec` tooling belong to project setup;
  preserve them unless task explicitly changes them.

## Frozen source baseline

| Field | Value |
| --- | --- |
| Local checkout | `/home/jcastillo/Proyectos/ddgs` |
| Upstream | `https://github.com/deedy5/ddgs` |
| Branch/HEAD | `main` / `a12929a72429a39a0841c3d7caacb20ee17acd4d` |
| Describe | `v9.14.4-2-ga12929a` |
| Commit date | `2026-05-24T00:32:01+03:00` |
| Package `__version__` | `9.14.4` |
| License | MIT, Copyright (c) 2022 deedy5 |

Important: HEAD is two commits after tag `v9.14.4`; it removes prior DHT/API
coupling. Port this exact HEAD, not tag-only behavior. The local source worktree
was clean during audit.

## Scope contract

In scope:

- `DDGS` search behavior: text, images, news, videos, books, extraction.
- Engine selection, provider de-duplication, concurrency, aggregation,
  ranking, parsing, normalization, errors, proxy/TLS/timeout behavior.
- All active source engines and disabled-source status.
- Differential fixtures and opt-in live integration verification.

Out of scope:

- `ddgs/cli.py`, `ddgs/api_server/`, FastAPI/Uvicorn, MCP, Click, DHT,
  background cache/network service, Docker, compose, deployment wiring.

## Source architecture facts

| Python area | Role | Go destination |
| --- | --- | --- |
| `ddgs/ddgs.py` | façade, engine cache, bounded fan-out, aggregation/ranking, extract | public `ddgs` + `internal/search` |
| `ddgs/base.py` | engine template, HTML/XPath extraction | `internal/engine`, `internal/parser` |
| `ddgs/engines/*.py` | request payloads and engine-specific processing | `internal/engine` |
| `ddgs/http_client.py` | `primp` transport/response rendering | `internal/transport`, `internal/extract` |
| `ddgs/http_client2.py` | temporary DDG text HTTP/2 fingerprint client | `internal/transport` |
| `ddgs/results.py` | result shapes, normalizers, dedupe/count | `internal/search`, `internal/normalize` |
| `ddgs/similarity.py` | Wikipedia/token ranker | `internal/search` |
| `ddgs/utils.py` | VQD, URL/text/date/proxy helpers | `internal/normalize`, `internal/engine` |

## Runtime engine registry at frozen source

Python dynamically discovers subclasses, then excludes `disabled=True`. Go
must use an equivalent static registry. `Bing` text is present but disabled.

| Category | Active engine names | Provider labels used for de-duplication |
| --- | --- | --- |
| text | brave, duckduckgo, google, grokipedia, mojeek, startpage, wikipedia, yahoo, yandex | brave; bing (duckduckgo, yahoo); google (google, startpage); grokipedia; mojeek; wikipedia; yandex |
| images | bing, duckduckgo | bing (both) |
| news | bing, duckduckgo, yahoo | bing (bing, duckduckgo); yahoo |
| videos | duckduckgo | bing |
| books | annasarchive | annasarchive |

Source README names `bing` as text backend despite its disabled registry state.
Treat registry output as truth.

## Crucial behavioral invariants

- Constructor resolves `proxy`: explicit value (`tb` means
  `socks5h://127.0.0.1:9150`) then `DDGS_PROXY`; default timeout is 5;
  `verify` supports bool or PEM path.
- `backend` accepts comma-separated names. `auto`/`all` shuffles engines;
  text preference injects Wikipedia and Grokipedia, then priority/random sort
  determines final order. Invalid names warn; no valid engines falls back to
  `auto`.
- Engines are cached per DDGS instance. Search tracks providers only after
  completed nonempty results (not at submit time), so it does not guarantee one
  in-flight source per provider. It runs bounded concurrent work and uses
  source worker formula:
  `min(uniqueProviders, ceil(maxResults/10)+1)` unless `maxResults` is falsy;
  class `DDGS.threads` may lower it.
- Aggregate dedupe chooses source field order: text `href`, images `image`,
  news `url`, videos `embed_url`, books `url`; counts occurrences and returns
  most-common order. Longer `body` replaces cached duplicate content.
- Ranker puts Wikipedia hits first, drops titles containing both `Category:`
  and `Wikimedia`, then buckets title/body token matches. Do not replace it
  with generic relevance scoring.
- Normalizers matter: regex tag removal, entity unescape, NFC, Unicode-C
  deletion, collapsed whitespace, percent decode plus spaces to `+`, and
  Python-style ISO date strings.
- Search result values are heterogeneous dictionaries; video maps and
  statistics can contain non-string values. Preserve shape before designing a
  typed convenience layer.
- Read `docs/source-quirks.md` for scheduler/tie-order bugs, engine option
  mismatches, source error heuristics, module-lifetime random values, and
  parser/transport corner cases. These are not cleanup candidates.
- Read `docs/engine-contracts.md` before any engine-adapter task. It is the
  source request/parser/post-processing inventory to turn into fixtures.

## Dependencies and implementation gates

| Source dependency | Why it matters | Go status |
| --- | --- | --- |
| `primp>=1.2.3` | randomized browser impersonation, TLS/HTTP behavior, proxy/certs, HTML render properties | hard compatibility gate; `net/http` alone is not assumed sufficient |
| `lxml>=4.9.4` | tolerant HTML parse + XPath | Helium v0.6.0 internal adapter passes 14 frozen lxml fixtures; JSON decoder preserves `json.Number`/raw mixed values |
| `httpx[http2,socks,brotli]`, `httpcore`, `h2` | DDG text temporary client; random HTTP/2/TLS behavior | standard request-local H2/no-redirect/header behavior fixture-tested; randomized TLS/H2 fingerprint remains hard gate |
| `fake-useragent>=2.2.0` | DDG text random user agent | capture/preserve acceptable UA behavior |
| Click/FastAPI/Uvicorn/MCP | CLI/service only | explicitly excluded |

Go parser `github.com/lestrrat-go/helium v0.6.0` is approved and implemented
behind `internal/parser` after a 14/14 lxml corpus/race probe. `htmlquery
v1.3.6` is rejected for XPath-union and malformed-HTML divergence. Parser JSON
uses `UseNumber`, rejects a second top-level value, and leaves source field
ordering to engine adapters.

Base transport imports `golang.org/x/net/proxy v0.57.0` only to preserve the
frozen SOCKS5-local versus SOCKS5H-remote DNS distinction. It materializes and
closes native bodies and has per-client cookie/header state. It does **not**
establish `primp` browser fingerprint/TLS parity. `DuckDuckGoTextClient`
separately proves standard request-local H2, no-redirect, header/jar isolation,
and lifecycle behavior; source randomized TLS/H2 settings stay gated by 5.5.

The frozen Python repository has no dependency lockfile; its `pyproject.toml`
only declares lower bounds. Before fixture capture, task 2.1 must record exact
resolved runtime package versions (and preferably wheel hashes) as provenance.

## Open blockers / risks

1. **Browser fingerprint parity — critical.** Source uses `primp` random
   impersonation and custom HTTP/2 settings. Base `net/http`/SOCKS behavior
   and DDG standard H2/no-redirect behavior are proven offline only; source
   randomized TLS/H2 fingerprint and each `primp`-dependent engine still need
   controlled evidence.
2. **Engine parser integration — high.** Internal Helium/JSON parser contracts
   are complete, but no engine adapter has consumed them with a real transport
   response or source-specific post-processing yet.
3. **`extract()` rendering — critical.** `primp` provides `text_markdown`,
   `text_plain`, and `text_rich`; no Go equivalent selected. Raw HTML/bytes
   are easy, rendered output needs differential fixtures.
4. **Public Go API — high.** Python signatures and dynamic dict results cannot
   be copied literally. Design must retain raw parity and document Go-native
   `context.Context`/typed options without hiding source behavior.
5. **State and concurrency — high.** Python cached engines mutate URLs,
   language, cookies, and a globally monkey-patched HTTP/2 method. Go must
   avoid races/leaks while preserving user-visible semantics.
6. **Live-engine volatility — high.** Upstream markup and anti-bot policy
   change. Offline contracts are primary; integration checks are tagged and
   rate-limited.
7. **Source baseline drift — medium.** Source `__version__` and HEAD differ.
   Any upstream update requires a new audit/diff and explicit OpenSpec change.
8. **Module path — medium.** Confirm final Git remote/import path before first
   release.
9. **Scheduler composition — high.** The tested scheduler core currently
   receives only common fields and is not wired to public `DDGS` timeout or
   category-specific source `**kwargs`. Capture a lossless immutable
   per-category request contract before connecting public search to engines.

## Required evidence before an engine is complete

1. Frozen source request sequence captured: method, URL, query/body, relevant
   headers/cookies, token/bootstrap calls, and random inputs controlled.
2. Offline Go payload/selector/result/error golden tests, including edge cases.
3. Differential comparison with frozen Python behavior for same fixture.
4. Race-safe concurrent Go test; no leaked responses or goroutines.
5. Opt-in live integration result when practical, recorded as an observation,
   never sole proof of parity.

## Next implementation order

1. Build isolated Python reference environment and pure fixture schema/capture
   harness. **Completed 2026-07-20 (tasks 2.1–2.3).**
2. Capture engine-visible request behavior before each relevant engine adapter;
   never write an engine without its fixture evidence.
3. Capture engine-visible request behavior and define the lossless per-category
   scheduler request shape; only then compose the public façade with engines.
4. DDG text, Grokipedia, Wikipedia, Brave, Google, and Mojeek adapter gates
   are complete. Next implement Startpage, Yahoo, and Yandex with their
   captured bootstrap/random-path/search-id/URL-decode behavior; keep every
   `primp`/randomized TLS-H2 fingerprint explicitly incomplete pending task
   5.5.

## Verification baseline

Current scaffold verification must stay green:

```bash
make verify
make integration  # only after tagged tests exist and live checks are intended
```

On 2026-07-20, available toolchain: Go `1.26.1`, Python `3.12.3`, `uv`.
Python runtime dependencies were not installed locally; do not mistake that
absence for source behavior or skip differential-harness setup.

Verification recorded on 2026-07-20:

- `openspec validate port-ddgs-python-library --strict --no-interactive` — pass.
- `make verify` — pass (`gofmt` check, `go vet`, unit, and race checks).
- `make integration` — pass mechanically; no tagged integration tests exist
  yet, so it made no external requests and is not engine evidence.
- Python oracle setup — pass: `uv pip check` on the isolated source environment;
  see `docs/reference-environment.md` for exact resolved packages and pure
  probe outputs.
- Fixture schema/capture — pass: `tools/reference_capture.py --write` and
  `--check` initially generated/verified 23 files; later pure-core work
  expanded the corpus to 129 files; Python syntax and JSON syntax checks,
  generated-contract invariants, `git diff --check`, OpenSpec strict validation,
  `make verify`, and mechanical `make integration` all passed. Each fixture
  now records resolved package versions; `cause_type: null` is schema-valid.

## Decision log

| Date | Decision | Reason |
| --- | --- | --- |
| 2026-07-20 | Freeze source at `a12929a`; use source over README discrepancies | reproducible behavioral target |
| 2026-07-20 | Restrict epic to Go library | explicit user scope |
| 2026-07-20 | Use root public `ddgs` package plus small internal library layers | Go-module adaptation of skeleton; avoids service boilerplate |
| 2026-07-20 | Do not implement an engine before differential contracts | search-engine parity is product-critical |
| 2026-07-20 | Keep module path provisional | target repository has no configured remote |
| 2026-07-20 | Complete OpenSpec artifacts; mark scaffold/governance tasks done | epic ready for evidence-first implementation |
| 2026-07-20 | Record isolated Python oracle before fixture work | source lacks lockfile; fixtures need reproducible dependency provenance |
| 2026-07-20 | Make all applicable local Go skills mandatory delivery gates | user requires full Go, clean-code, simplification, concurrency, debugger, testing, and TDD discipline; `AGENTS.md` defines evidence and N/A rules |
| 2026-07-20 | Complete fixture schema and pure capture corpus | 23 synthetic/offline contracts cover normalizers, VQD, proxy, aggregation, ranker, backend selection, scheduler quirks, error classification, and lazy extraction selection; no engine HTTP occurred |
| 2026-07-20 | Complete API/configuration TDD slice (tasks 3.1–3.2) | RED fixtures/tests preceded `ddgs.go`/`api.go`; constructor preserves absent vs empty proxy, `tb`, timeout nil/zero/default, TLS bool/PEM; façade preserves raw maps/content kinds and context cancellation through a private executor port |
| 2026-07-20 | Complete normalizer TDD slice (tasks 3.3–3.4) | 72 fixture corpus proves text/URL/date/VQD/proxy parity, including malformed percent bytes, all Python HTML5 entities, Python-only `nGt;`/`nLt;`, VQD repr, and date error boundaries |
| 2026-07-20 | Pin `golang.org/x/text` at `v0.40.0` with Go 1.26.1 minimum | Helium requires it; its `!go1.27` NFC table remains Unicode 15.0.0, while project guard continues rejecting Go 1.27+ before Unicode 17 can drift behavior |
| 2026-07-20 | Model date normalization as `(value, error)` internally | Frozen CPython/Linux raises `ValueError`/`OSError` for out-of-range timestamps; formatting Go-only years would violate parity. Future JSON adapters must retain `json.Number` until integer/float distinction is resolved |
| 2026-07-20 | Keep ordered internal `engine.Result` fields until the public map boundary | Python aggregation selects the first eligible field in object insertion order, including dynamic fields; adapters must own the source-shaped result before `internal/search` aggregates it; Go map iteration cannot carry this contract |
| 2026-07-20 | Preserve raw body `len()` failures in duplicate aggregation | Frozen Python raises `TypeError` for falsy `None`/bool/numeric body values on the second duplicate; coercion or a friendly zero length would alter behavior |
| 2026-07-20 | Preserve raw ranker membership/lower ordering | Frozen `SimpleFilterRanker` first applies membership to heterogeneous raw fields, then calls `.lower()`; fixtures cover list/dict membership and null/scalar errors so Go cannot pre-coerce documents |
| 2026-07-20 | Complete isolated ranker/backend/registry/scheduler core | Frozen fixtures prove ranking, active/disabled metadata, backend order/fallback, bounded batch scheduling, provider timing, error classification, and final slicing; no engine/transport/public composition is implied |
| 2026-07-20 | Snapshot scheduler inputs at operation entry | Python core consumes immutable scalar/list values. Go clones optional request pointers and engine metadata slice before concurrent dispatch, then gives workers independent request values to avoid caller/sibling aliasing |
| 2026-07-20 | Complete synthetic engine-visible capture adapters (task 2.4) | Nine sanitized fixtures cover DDG VQD media bootstraps, repeat Startpage `sc`, Wikipedia enrichment, Brave/Google cookies, Google/Yahoo redirect cleanup, and Yahoo/Yandex request-time randomness; fake `ddgs.base.HttpClient` forbids external requests |
| 2026-07-20 | Complete active-engine request/response matrix (task 2.5) | 79 sanitized synthetic fixtures cover all 16 active category/engine pairs with option-bearing success, empty/malformed 200, and 503/`None`; capturer rejects frozen-registry pair missing required path |
| 2026-07-20 | Complete extraction fixture corpus (task 2.6) | Nine sanitized loopback-only fixtures freeze raw bytes/text, Markdown/plain/rich output, unknown-format fallback, 503 error, selected response property, and invalid-UTF-8 behavior; no Go renderer is approved yet |
| 2026-07-20 | Make fixture sanitation executable (task 2.7) | Capturer audits/rejects URL userinfo, unapproved loopback, local paths, auth headers, and secret/session/token-like cookies; manual corpus audit found only synthetic/public values and no live payload or credentials |
| 2026-07-20 | Approve Helium for internal XPath adapter (task 4.1) | `github.com/lestrrat-go/helium v0.6.0` matched all 14 frozen lxml fixtures and upstream `html`/`xpath1` race tests; pure Go core, MIT license, no enabled cgo path. Reject htmlquery and cgo libxml2 binding; adapter remains TDD pending |
| 2026-07-20 | Complete parser TDD gate (tasks 4.2–4.5) | `internal/parser` preserves 14 lxml XPath contracts plus 7 JSON contracts with `UseNumber`; cgo-off, race x20, concurrent-document reads, and representative 100x benchmarks pass. Parser remains an offline syntax/XPath boundary, not transport or engine proof. |
| 2026-07-21 | Complete base transport TDD gate (tasks 5.1–5.3) | `internal/transport` uses an isolated cookie jar/header state, materializes and closes native bodies, preserves response bytes/text/status, follows source base HTTP behavior, and distinguishes SOCKS5-local from SOCKS5H-remote DNS. RED exposed bare-domain Google cookies and a first-use client-init race; both are fixture/test-proven fixed. Full race plus transport stress x50 pass; fingerprint/DDG H2 are still not claimed. |
| 2026-07-21 | Complete request-local DDG transport gate (task 5.4) | Five frozen `HttpClient2` fixtures and local TLS/H2 tests prove `DuckDuckGoTextClient` request shape, standard H2, no redirect follow, copied UA/header state, isolated jars/transports, cancellation, and no mutation of `http.DefaultTransport`. Source randomized TLS/H2 settings and browser fingerprint are deliberately not claimed and remain task 5.5. |
| 2026-07-21 | Complete DuckDuckGo text adapter gate (task 6.2) | `internal/engine` now owns ordered source results and `DuckDuckGoText` uses the special DDG transport port. Eight frozen fixtures prove POST form order, page/time conditions, ignored safesearch, `nil` versus `[]`, HTML extraction/result normalization, and `y.js` filtering; concurrent adapter stress passes under race detection. It is still not public-client composition or fingerprint proof. |
| 2026-07-21 | Complete Grokipedia and Wikipedia adapter gate (task 6.1) | Ordered JSON now preserves source object iteration where Wikipedia selects `next(iter(query.pages.values()))`; Grokipedia preserves Python JSON non-finite and f-string-visible slug behavior only at its source operation. Fixture RED/GREEN/REFACTOR proves missing/null/type errors, exact request sequences, `nil`/empty states, source error classes/messages, ordered pages, and case-sensitive disambiguation. Race stress, 288-fixture oracle, full tests/race, `make verify`, and OpenSpec validation pass. Browser fingerprint proof and public composition remain intentionally open. |
| 2026-07-21 | Complete Brave, Google, and Mojeek adapter gate (task 6.3) | Ordered request/cookie fixtures prove Brave Python-dict replacement order, Google process-lifetime UA/consent/case/page/redirect behavior, and Mojeek exact safe/page/time behavior. The adapters use injected request-local ports and immutable request values; `cookie_order` is now part of fixture traces. Full tests/race, adapter race stress x50, cgo-off, `make verify`, strict OpenSpec validation, and the 300-fixture oracle pass. Browser fingerprint proof and public composition remain intentionally open. |
| 2026-07-21 | Complete Startpage, Yahoo, and Yandex adapter gate (task 6.4) | Ordered request/form fixtures prove Startpage bootstrap sequencing/raw-status/empty-text behavior, Yahoo request-time token path and double URL post-processing, and Yandex request-time search-id/ignored-option branches. Adapters use injected request-local ports and immutable request values; full tests/race, adapter race stress x50, cgo-off, `make verify`, strict OpenSpec validation, and the 314-fixture oracle pass. Browser fingerprint proof and public composition remain intentionally open. |
| 2026-07-21 | Complete Bing and DuckDuckGo image adapter gate (task 6.5) | Ordered source-parameter fixtures prove Bing engine-only `max_results`/Python `int`, long-form-only timelimit, metadata/dimension errors, and DuckDuckGo VQD/header/filter/result-root/error ordering. Public image options retain source keyword ordering while scheduler `max_results` stays outside engine kwargs. Full tests/race, adapter race stress x50, cgo-off, `make verify`, strict OpenSpec validation, and the 337-fixture oracle pass. Browser fingerprint proof and public composition remain intentionally open. |
| 2026-07-21 | Complete Bing, DuckDuckGo, and Yahoo News adapter gate (task 6.6) | Fixture RED/GREEN/REFACTOR proves source-ordered News payloads, VQD sequencing, nil/empty/error distinctions, Bing local-zone and relative dates/image truncation, DuckDuckGo raw dynamic `results` iteration, and Yahoo broad partial cleanup with clock-injected dates. Adapters own no goroutine or response body; injected ports and immutable per-call data pass race stress. Full tests/race, cgo-off, `make verify`, strict OpenSpec validation, and the 353-fixture oracle pass. Browser fingerprint proof and public composition remain intentionally open. |
| 2026-07-21 | Complete DuckDuckGo Videos adapter gate (task 6.7) | Fixture RED/GREEN/REFACTOR proves public video keyword forwarding, VQD-before-safe error order, exact four-slot `f`, page stride 60, nested/dynamic result values, and raw results-root/item errors. Images, News, and Videos share only the fixture-proven JSON-iterable helper. Full tests/race, video race stress x20, cgo-off, `make verify`, strict OpenSpec validation, and the 361-fixture oracle pass. Browser fingerprint proof and public composition remain intentionally open. |
| 2026-07-21 | Complete Anna's Archive adapter gate (task 6.8) | Fixture RED/GREEN/REFACTOR proves one process-lifetime archive TLD, immutable adapter URL, ordered `q,page` GET (including page zero), comment delimiter removal, nil/empty/malformed distinctions, and source base-prefix repair even for absolute URLs. Full tests/race, adapter race stress x50, cgo-off, `make verify`, strict OpenSpec validation, and the 363-fixture oracle pass. Browser fingerprint proof and public composition remain intentionally open. |
| 2026-07-21 | Complete disabled text Bing regression gate (task 6.9) | Direct frozen-source fixture proves `Bing.disabled=True` metadata, its absence from active text names, and explicit `backend="bing"` fallback to active `auto` after two shuffle calls. This task intentionally added no production code or active adapter; test-only TDD GREEN is existing behavior, with RED/GREEN production phases N/A. Focused race x20, full acceptance, and the 364-fixture oracle pass. |
| 2026-07-25 | Complete category selector/scheduler differential gate (task 6.10) | Frozen synthetic engines drive every active category through explicit/comma, `auto`, `all`, invalid fallback, provider collisions, empty/error recovery, timeout/no-result, and max-result branches. RED caught missing common/category-keyword forwarding and source timeout-cause shape; scheduler now owns immutable ordered source parameters, copied again per worker. Focused race x50, full acceptance, and the 365-fixture oracle pass. Real adapter/public-client composition remains intentionally open. |
| 2026-07-25 | Complete extraction RED contract (task 7.3) | `internal/extract/extract_test.go` fails only against the unavailable extractor, after proving 9 frozen extract fixtures plus constructor forwarding, GET, raw byte/text ownership, lazy rendering, unknown fallback, cancellation, and fetch-error propagation. It is intentionally not green until renderer gate resolves. |
| 2026-07-25 | Reject current Go extract renderers (task 7.4 remains blocked) | Resolved `primp 1.3.1` calls Rust `html2text 0.16.7` width 100/default/Trivial/Rich decorators. Three MIT Go candidates diverge on frozen heading/link/list output; the exact MIT Rust crate is ~11.5k lines and this environment has no cargo. No cgo/subprocess/best-effort fallback is approved. |
| 2026-07-25 | Complete browser-fingerprint evidence gate (task 5.5) | Endpoint-explicit tagged observations show both Go transports negotiate HTTP/2 but expose the same standard Go JA3/JA4/Akamai hashes, distinct from observed source `primp` and `HttpClient2` values. `docs/fingerprint-gate.md` maps every active engine to its still-unproven source client; no dependency or parity claim was added. |
| 2026-07-25 | Complete public composition gate (task 7.7) | Root composition selects frozen metadata lazily, eagerly constructs all selected adapters before scheduler workers, caches adapters per public client under a narrow lock, and gives every engine a fresh isolated transport. DuckDuckGo text gets its separate H2 client plus the frozen full weighted `fake-useragent 2.2.0` pool, decoded/checksummed once per Go process. Public boundary preserves ordered keyword forwarding and maps scheduler timeout/generic failures to classifiable `DDGSError` values. |

## Core TDD evidence — 2026-07-20

- **RED 3.1–3.2:** public constructor/API contract tests failed before
  `New`, options, façade methods, error classes, and private executor seam
  existed. Failures were missing behavior/declarations, not source fixture or
  compilation setup damage after the declaration phase.
- **GREEN/REFACTOR 3.1–3.2:** constructor/search/extract fixture tests pass.
  `RawResult` remains `map[string]any`; no source field is coerced. Client
  with no executor reports explicit unavailable behavior; this is not an
  engine implementation claim.
- **RED 3.3:** differential normalizer tests failed against stub behavior for
  text, URL, date, VQD, and proxy. Expanded edge fixtures exposed Go's missing
  HTML5 `nGt;`/`nLt;` and Go-only out-of-range date formatting.
- **GREEN/REFACTOR 3.3–3.4:** package preserves frozen URL replacement,
  NFC/category-C behavior, HTML entity behavior, strict VQD bytes/error repr,
  source proxy alias, and captured CPython/Linux date exception
  class/message. It uses only an internal date error boundary; engines must
  propagate it when result construction is added.
- **RED 3.5:** frozen result/aggregation fixtures and `internal/search`
  tests failed before `Result`, category shapes, ordered fields, and the
  aggregator existed. Subsequent RED cases exposed `json.Number` date
  conversion, dynamic-field ordering, and source `len()` error messages.
- **GREEN/REFACTOR 3.5–3.6:** `internal/search/results.go` preserves the five
  dataclass default shapes, source-only named-field normalization, nested
  video types, field insertion/update order, cache-field scan order,
  occurrence ordering, Unicode body length, longer-body replacement, and
  captured body error quirks. `RawResult` remains only the outward map form;
  aggregation never iterates a Go map.
- **RED/GREEN/REFACTOR 3.7–3.9:** ranker, backend selector, and audited static
  registry tests were RED before their implementations. The 129-fixture pure corpus
  proves Wikipedia/category/token buckets, raw ranker type failures,
  Unicode/token behavior, invalid backend fallback, stable shuffled ties,
  provider labels/timing, every frozen active category, and inactive text Bing.
- **RED/GREEN/REFACTOR 7.1–7.2:** scheduler tests were RED before the bounded
  worker pool. The green/refactor core preserves source worker formula
  (including Python IEEE-754 boundary), submission-order completion handling,
  `FIRST_EXCEPTION`, zero-timeout completed-future snapshot, partial results,
  source error heuristic, rank-before-slice, provider timing, and max-result
  forms. It snapshots optional request pointers and the engine slice at entry;
  each worker receives a separate immutable request value.
- **RED/GREEN/REFACTOR 4.2–4.5:** XPath tests first failed against an
  unavailable parser adapter; Helium then passed all source expressions,
  document-order union, attributes, whitespace collapse, malformed recovery,
  and Anna comment removal without selector rewrites. JSON tests first failed
  against an unavailable decoder, then passed Grokipedia/Bing/DDG absent/null,
  nested/mixed, malformed, and trailing-value fixtures. `golang-pro`,
  hexagonal boundary review, `golang-testing`, TDD, clean-code, and
  simplification applied. Parser code is pure/no goroutines, so concurrency and
  debugger were N/A for implementation; task 4.5 nevertheless ran shared and
  independent-document concurrent reads under `-race` x20 plus cgo-off tests.
- **RED/GREEN/REFACTOR 5.1–5.3:** base transport tests were RED for native
  response closure/materialization, source bare-domain cookies, HTTP/HTTPS/
  SOCKS proxy shape, TLS verification/PEM, timeout/cancellation, gzip,
  redirects, non-200 response preservation and first-use concurrent client
  initialization. GREEN uses a per-client jar and header lock, a one-time
  protected native client, copied request data, and `x/net/proxy` only for
  SOCKS. REFACTOR keeps native HTTP types internal. `golang-pro`, hexagonal
  boundary review, testing/TDD, clean-code and simplification applied;
  concurrency/debugger review found and fixed the initialization race.
  `go test -race -count=50 ./internal/transport` and lifecycle tests pass.
- **RED/GREEN/REFACTOR 5.4:** DDG tests were RED for source constructor
  request shape, disabled redirects, local TLS/H2, timeout classification,
  request-local header state, cancellation, and no default-transport mutation.
  GREEN adds a separate `DuckDuckGoTextClient` with a cloned transport and
  `ForceAttemptHTTP2`; no package-global HTTP/2 patch is recreated. REFACTOR
  retained only standard Go H2 behavior, then `-race` x100 and lifecycle tests
  passed. Randomized source TLS/H2 fingerprint remains intentionally open.
- **RED/GREEN/REFACTOR 6.2:** DuckDuckGo text fixture tests were RED before
  the engine port and adapter existed. GREEN parses the frozen HTML with the
  internal parser, builds source-ordered category results, preserves POST form
  order `q,b,l[,s][,df]`, the source `nil` versus parsed-empty distinction,
  and filters `y.js` links. REFACTOR moved canonical ordered result ownership
  from `internal/search` to `internal/engine`, removed mutable global selector
  state, and added trace-order validation. `golang-pro`, hexagonal dependency
  review, testing/TDD, clean-code, simplification, concurrency patterns, and
  debugger review applied; 32 concurrent calls and `-race` x50 pass. The
  adapter reads immutable request data and owns no response body or goroutine.
- **RED/GREEN/REFACTOR 6.1:** Grokipedia/Wikipedia fixture tests were RED
  before either JSON adapter existed. GREEN added only ordered JSON decoding,
  an injected request port, and the two source request/result sequences.
  Subsequent RED cases exposed Python's non-finite JSON acceptance, f-string
  slug rendering, Wikipedia's JSON insertion-order `pages` selection, and
  exact absent/null/type failures. REFACTOR kept ordered-object state inside
  `internal/parser`, adapters request-local, and raw result fields ordered.
  `golang-pro`, hexagonal boundary review, `golang-testing`, strict TDD,
  clean-code, simplification, concurrency patterns, and debugger review
  applied; 56 concurrent calls and `go test -race -count=50 ./internal/engine`
  pass. No adapter owns a response body or goroutine.
- **RED/GREEN/REFACTOR 6.3:** Brave/Google/Mojeek fixture tests were RED
  before the adapters existed. GREEN implemented only their captured GET
  request/cookie/XPath/result sequences. A follow-up RED required explicit
  `cookie_order` in every trace; GREEN extended the schema/capturer and made
  source Python dictionary replacement order observable. REFACTOR kept an
  injected consumer-side transport port, immutable per-call field slices, and
  a synchronized process-lifetime Google UA selector; it does not mutate a
  global transport. `golang-pro`, hexagonal boundary review,
  `golang-testing`, strict TDD, clean-code, simplification, concurrency
  patterns, and debugger review applied. Adapter stress (32 calls) and
  `go test -race -count=50` pass; adapters own no goroutine or response body.
- **RED/GREEN/REFACTOR 6.4:** Startpage/Yahoo/Yandex fixture tests were RED
  before their adapters existed. GREEN implements only captured bootstrap,
  ordered form/query, XPath, random, and post-processing behavior. Follow-up
  RED fixtures fixed Startpage empty bootstrap text to the frozen `ParserError`
  path and retained malformed nonempty bootstrap HTML as an empty-`sc` POST.
  REFACTOR removed duplicate safesearch conversion, kept randomness injected,
  and generalized the HTML fixture runner to assert ordered forms. `golang-pro`,
  hexagonal boundary review, `golang-testing`, strict TDD, clean-code,
  simplification, concurrency patterns, and debugger review applied; 32
  concurrent calls per adapter and `go test -race -count=50` pass. Adapters
  own no response body or goroutine.
- **RED/GREEN/REFACTOR 6.5:** Bing/DuckDuckGo image fixture tests were RED
  before adapters/options existed. GREEN adds only captured GET/bootstrap/VQD,
  ordered payload/filter fields, XPath/JSON extraction, and source error
  branches. Follow-up RED fixtures exposed Python `int()` Unicode digits,
  underscores/control whitespace, Bing `m=null`, and DuckDuckGo raw
  `results` roots. REFACTOR removed mutable shared header data and retained
  ordered source parameters. `golang-pro`, hexagonal boundary review,
  `golang-testing`, strict TDD, clean-code, simplification, concurrency
  patterns, and debugger review applied; adapter stress and `go test -race
  -count=50` pass. Adapters own no response body or goroutine.
- **RED/GREEN/REFACTOR 6.6:** Bing/DuckDuckGo/Yahoo News fixture tests were
  RED before adapters existed. GREEN adds only captured ordered GET/VQD flows,
  XPath/JSON parsing, source result fields, clock injection, and postprocess
  branches. Follow-up RED fixtures exposed Bing date-layout/local-zone paths,
  all Yahoo relative units and nested URL normalization, plus DuckDuckGo's raw
  dict/string `results` iteration. REFACTOR retains one private News HTML
  path, immutable request fields, and per-instance clocks; it adds no retry,
  global clock, goroutine, or response ownership. `golang-pro`, hexagonal
  boundary review, `golang-testing`, strict TDD, clean-code, simplification,
  concurrency patterns, and debugger review applied; 24–32 concurrent calls
  per adapter and `go test -race -count=20` pass.
- **RED/GREEN/REFACTOR 6.7:** Video filter API and DuckDuckGo Videos fixtures
  were RED before source keyword behavior and the adapter existed. GREEN adds
  only captured VQD/bootstrap GETs, ordered four-slot filters, paging, raw
  JSON result fields, and source error paths. REFACTOR replaces duplicated
  DuckDuckGo dynamic-JSON iterable handling with one fixture-proven private
  helper shared by Images, News, and Videos; it preserves nil/empty/error
  behavior. `golang-pro`, hexagonal boundary review, `golang-testing`, strict
  TDD, clean-code, simplification, concurrency patterns, and debugger review
  applied; 32 concurrent calls and `go test -race -count=20` pass. The adapter
  owns no goroutine or response body.
- **RED/GREEN/REFACTOR 6.8:** Anna's Archive fixtures were RED before the
  adapter existed. GREEN adds only the process-lifetime archive URL selector,
  ordered GET request, source comment delimiter preprocessing, XPath result
  construction, and source URL prefix behavior. REFACTOR removes mutable TLD
  storage while retaining the synchronized `sync.OnceValues` source-lifetime
  selector and a private fixed-URL fixture seam. `golang-pro`, hexagonal
  boundary review, `golang-testing`, strict TDD, clean-code, simplification,
  concurrency patterns, and debugger review applied; 32 concurrent calls and
  `go test -race -count=50` pass. The adapter owns no goroutine or response
  body.
- **RED/GREEN/REFACTOR 7.7:** composition fixture tests were RED before the
  root executor/factory existed. GREEN wires each active frozen registry entry
  through a per-client cache and a fresh transport, retains DDG text's special
  H2 path, forwards source keywords without maps, and classifies scheduler
  timeout/generic errors at the public boundary. Follow-up RED requires all
  selected adapters to exist before scheduler work starts and a concurrent
  cache test proves exactly one adapter construction. REFACTOR retains only
  small consumer-side factory/selector ports. `golang-pro`, hexagonal boundary
  review, `golang-testing`, strict TDD, clean-code, simplification,
  concurrency patterns, and debugger review applied; focused cache stress
  passes `-race -count=50`. The executor owns no response body or goroutine;
  scheduler/transport own their established lifecycles.
- **Regression 6.9:** a new frozen-source fixture directly combines disabled
  Bing metadata, active text names, and `backend="bing"` fallback. The Go
  implementation already matched this frozen behavior, so this was a
  test-only task: production TDD RED/GREEN/REFACTOR is genuinely N/A and no
  production code was changed. `golang-pro`, `golang-testing`, clean-code and
  simplification reviews applied; hexagonal/API review is N/A (no boundary
  change) and concurrency/debugger are N/A (deterministic registry/selector,
  no goroutines or I/O). Focused `-race -count=20` and full `-race` pass.
- **Skills assessed:** `golang-pro`, `go-clean-ddd-hexagonal` (public façade
  port), `golang-testing`, TDD RED/GREEN/REFACTOR, `clean-code`, and
  `go-code-simplification` applied. `go-concurrency-patterns` and
  `go-debugger-pro` were N/A only for prior deterministic normalizer/result
  slices. They were applied to scheduler work: one coordinator owns maps and
  aggregation; workers use bounded jobs/completion channels; cancellation joins
  workers; optional inputs are copied; `testing/synctest` proves cooperative
  cancellation lifecycle. Non-cooperative adapters violate the documented port
  contract because Go cannot safely force-stop arbitrary callbacks.
- **Acceptance:** `gofmt -d $(rg --files -g '*.go')`, `go vet ./...`,
  `go test -count=1 ./...`, `go test -race -count=1 ./...`, `make verify`,
  `make integration` (mechanical; no tagged live tests),
  `/tmp/goddgs-reference-a12929a/bin/python tools/reference_capture.py --check`,
  `openspec validate port-ddgs-python-library --strict --no-interactive`, and
  `git diff --check` all passed. Scheduler final acceptance also ran
  `go test -race -count=100 -run '^(TestSourceWorkerCount_MatchesFrozenFixtures|TestScheduler_|TestSourceBatch)' ./internal/search`.
  Overall tested Go coverage: 91.3%; public package: 96.4%;
  `internal/normalize`: 91.7%; `internal/search`: 90.3%.
- **Acceptance 6.3 (2026-07-21):** frozen Python `--check` verified 300
  fixtures (130 pure, 119 engine, 9 extract, 24 parser, 18 transport);
  `gofmt`, `git diff --check`, `go vet ./...`, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, focused engine race stress x50,
  `CGO_ENABLED=0 go test -count=1 ./...`, `make verify`, and strict OpenSpec
  validation passed. Total coverage: 86.8%; `internal/engine`: 84.4%; parser:
  83.4%; transport: 83.8%.
- **Acceptance 6.4 (2026-07-21):** frozen Python `--check` verified 314
  fixtures (130 pure, 133 engine, 9 extract, 24 parser, 18 transport);
  `gofmt`, `git diff --check`, `go vet ./...`, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, focused adapter race stress x50,
  `CGO_ENABLED=0 go test -count=1 ./...`, `make verify`, and strict OpenSpec
  validation passed. Total coverage: 87.0%; `internal/engine`: 85.1%.
- **Acceptance 6.5 (2026-07-21):** frozen Python `--check` verified 337
  fixtures (131 pure, 155 engine, 9 extract, 24 parser, 18 transport);
  `gofmt`, `git diff --check`, `go vet ./...`, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, focused image-adapter race stress x50,
  `CGO_ENABLED=0 go test -count=1 ./...`, `make verify`, and strict OpenSpec
  validation passed. Total coverage: 87.4%; public package: 94.7%;
  `internal/engine`: 86.7%; parser: 83.4%; transport: 83.8%. Live checks
  skipped: no external engine request or fingerprint evidence was authorized.
- **Acceptance 6.6 (2026-07-21):** frozen Python `--check` verified 353
  fixtures (131 pure, 171 engine, 9 extract, 24 parser, 18 transport);
  `gofmt`, `git diff --check`, `go vet ./...`, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, focused News-adapter race stress x20,
  `CGO_ENABLED=0 go test -count=1 ./...`, `make verify`, and strict OpenSpec
  validation passed. Total coverage: 87.3%; public package: 94.7%;
  `internal/engine`: 86.6%; parser: 83.4%; transport: 83.8%. Live checks
  skipped: no external engine request or fingerprint evidence was authorized.
- **Acceptance 6.7 (2026-07-21):** frozen Python `--check` verified 361
  fixtures (132 pure, 178 engine, 9 extract, 24 parser, 18 transport);
  `gofmt`, `git diff --check`, `go vet ./...`, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, focused Video-adapter race stress x20,
  `CGO_ENABLED=0 go test -count=1 ./...`, `make verify`, and strict OpenSpec
  validation passed. Total coverage: 87.6%; public package: 94.8%;
  `internal/engine`: 87.1%; parser: 83.4%; transport: 83.8%. Live checks
  skipped: no external engine request or fingerprint evidence was authorized.
- **Acceptance 6.8 (2026-07-21):** frozen Python `--check` verified 363
  fixtures (133 pure, 179 engine, 9 extract, 24 parser, 18 transport);
  `gofmt`, `git diff --check`, `go vet ./...`, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, focused Anna's Archive race stress x50,
  `CGO_ENABLED=0 go test -count=1 ./...`, `make verify`, and strict OpenSpec
  validation passed. Total coverage: 87.5%; public package: 94.8%;
  `internal/engine`: 86.9%; parser: 83.4%; transport: 83.8%. Live checks
  skipped: no external engine request or fingerprint evidence was authorized.
- **Acceptance 6.9 (2026-07-21):** frozen Python `--check` verified 364
  fixtures (134 pure, 179 engine, 9 extract, 24 parser, 18 transport);
  `gofmt`, `git diff --check`, `go vet ./...`, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, focused registry/selector race x20,
  `CGO_ENABLED=0 go test -count=1 ./...`, `make verify`, and strict OpenSpec
  validation passed. Total coverage: 87.4%; public package: 94.8%;
  `internal/engine`: 86.9%; `internal/search`: 91.4%; parser: 83.4%;
  transport: 83.8%. Live checks skipped: no external engine request or
  fingerprint evidence was authorized.
- **Acceptance 6.10 (2026-07-25):** frozen Python `--check` verified 365
  fixtures (135 pure, 179 engine, 9 extract, 24 parser, 18 transport);
  `gofmt`, `git diff --check`, `go vet ./...`, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, focused category differential race x50,
  `CGO_ENABLED=0 go test -count=1 ./...`, `make verify`, and strict OpenSpec
  validation passed. Total coverage: 87.5%; public package: 94.8%;
  `internal/engine`: 86.9%; `internal/search`: 91.9%; parser: 83.4%;
  transport: 83.8%. Live checks skipped: no external engine request or
  fingerprint evidence was authorized.
- **Acceptance 7.7 (2026-07-25):** frozen Python `--check` verified 368
  fixtures (138 pure, 179 engine, 9 extract, 24 parser, 18 transport);
  `gofmt`, `git diff --check`, `go vet ./...`, root composition tests,
  `go test -count=1` and `go test -race -count=1` for every package except the
  intentionally RED-only `internal/extract`, plus CGO-off tests and strict
  OpenSpec validation passed. Root package coverage: 90.9%; transport: 81.4%.
  Full `make verify` remains intentionally blocked only by task 7.3/7.4
  extractor RED, not by composition. Live checks were skipped; fingerprint
  parity remains unapproved.
