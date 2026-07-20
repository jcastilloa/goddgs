# MEMORY.md — goddgs

Persistent project state. Read before changing behavior. Update after every
material decision, completed OpenSpec task, source-baseline change, blocker, or
verification result.

## Current state — 2026-07-20

- Target: Go library port of DDGS only. No API server, CLI, MCP, DHT, cache,
  Docker service, or executable entrypoint.
- Module scaffold exists. Root package is `ddgs`; module path is provisional:
  `github.com/jcastillo/goddgs`.
- Active OpenSpec change: `openspec/changes/port-ddgs-python-library/`.
- Public façade/configuration, normalizers, ordered result aggregation, ranker,
  backend selection/static registry, isolated fixture-tested scheduler core,
  and offline HTML/XPath/JSON parser adapter are complete. No live search
  engine, transport, renderer, extraction implementation, or
  public-client-to-engine composition exists yet; those internal package
  boundaries remain intentional.
- Tasks 2.1–2.7 are complete. The isolated Python oracle lives temporarily at
  `/tmp/goddgs-reference-a12929a`; exact resolved packages and rebuild steps
  are in `docs/reference-environment.md`. It made no external engine request.
- Fixture corpus has 238 deterministic synthetic/offline contracts: 129 pure,
  79 engine-visible, 9 extract, and 21 parser contracts under their
  respective `testdata/contracts/` directories. `tools/reference_capture.py --check`
  validates frozen SHA, resolved-package provenance, result/error shape, trace
  order, kind-specific redaction, output separation, a dynamic matrix for every
  active engine (option-bearing success, empty 200, malformed 200, 503/None),
  and fixture sanitation. Extract capture uses only ephemeral loopback with
  synthetic HTML/bytes and rewrites its URL before output. Neither doubles nor
  loopback prove Go parser, renderer, TLS, or transport parity.
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
| `httpx[http2,socks,brotli]`, `httpcore`, `h2` | DDG text temporary client; random HTTP/2/TLS behavior | hard compatibility gate |
| `fake-useragent>=2.2.0` | DDG text random user agent | capture/preserve acceptable UA behavior |
| Click/FastAPI/Uvicorn/MCP | CLI/service only | explicitly excluded |

Go parser `github.com/lestrrat-go/helium v0.6.0` is approved and implemented
behind `internal/parser` after a 14/14 lxml corpus/race probe. `htmlquery
v1.3.6` is rejected for XPath-union and malformed-HTML divergence. Parser JSON
uses `UseNumber`, rejects a second top-level value, and leaves source field
ordering to engine adapters.

The frozen Python repository has no dependency lockfile; its `pyproject.toml`
only declares lower bounds. Before fixture capture, task 2.1 must record exact
resolved runtime package versions (and preferably wheel hashes) as provenance.

## Open blockers / risks

1. **Browser fingerprint parity — critical.** Source uses `primp` random
   impersonation and custom HTTP/2 settings. Need a Go transport strategy,
   proof per affected engine, and license/reproducibility review.
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
4. Implement transport capability and first engine vertically; then engine
   groups, extraction, race/live integration gates.

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
| 2026-07-20 | Keep ordered internal `search.Result` fields until the public map boundary | Python aggregation selects the first eligible field in object insertion order, including dynamic fields; Go map iteration cannot carry this contract |
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
