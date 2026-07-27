# MEMORY.md — goddgs

Persistent project state. Read before changing behavior. Update after every
material decision, completed OpenSpec task, source-baseline change, blocker, or
verification result.

## Current state — 2026-07-26

- **Search backend diagnostics (2026-07-27):** opt-in `WithSearchDiagnostics` reports every completed engine execution with backend name, provider, result count, and error. It is a Go-only observability extension for downstream orchestration; default raw result shape and frozen selection/scheduling/aggregation/ranking behavior remain unchanged. Completion callbacks run on scheduler workers and are completion-ordered, so callers collecting events must synchronize. Focused RED/GREEN tests plus `go test ./...`, `go test -race ./...`, `go vet ./...`, and `make verify` passed without external engine requests. Change was synchronized into canonical OpenSpec requirements and archived at `openspec/changes/archive/2026-07-27-add-search-backend-diagnostics/`; all nine canonical specs pass strict validation.

- **Handoff status:** historical changes `port-ddgs-python-library` (**56/56**)
  and `randomize-browser-profiles` (**15/15**) remain complete. Completed
  `add-live-api-examples` (**6/6**) synced its three canonical requirements and
  is archived at `openspec/changes/archive/2026-07-26-add-live-api-examples/`.
  `port-ddg-session-fingerprint` (**12/12**) remains synced and archived at
  `openspec/changes/archive/2026-07-25-port-ddg-session-fingerprint/`.
- **Next concrete action:** no migration development task remains. New examples
  are ready for user-run requests.
- **Live API examples (2026-07-26):** `examples/{text,images,news,videos,books,extract}`
  are standalone programs using public `ddgs` APIs, bounded contexts/client
  timeouts, explicit active backends, compact limits, and JSON output. They
  are documentation source, not a library CLI or default test. Shared
  `examples/internal/output` preserves heterogeneous `RawResult` values via
  JSON and reports errors to standard error.
- **Example verification (2026-07-26):** `gofmt`, `git diff --check`, `go build
  ./examples/...`, and `make verify` passed. Test design/TDD RED→GREEN→REFACTOR
  are genuinely N/A: this change adds no production behavior or deterministic
  test contract, and live output cannot be a Go `Example` assertion. `golang-pro`,
  `clean-code`, `go-code-simplification`, and `golang-testing` reviews found
  focused, idiomatic documentation code; `go-clean-ddd-hexagonal` is N/A
  (no API/package boundary change), while `go-concurrency-patterns` and
  `go-debugger-pro` are N/A for the new code (no owned goroutine, shared state,
  response body, or transport lifecycle). Full existing `make verify` race
  coverage passed.
- **Live observations (2026-07-26):** user-authorized `go run ./examples/text`
  and `go run ./examples/news` both made external DuckDuckGo requests and
  returned `No results found.`. User-authorized `make integration` ran all
  five categories: text/images also returned no results; Anna's Archive tried
  `127.0.0.1:443` and was refused; News and Videos passed. These are current
  endpoint/network observations only, not offline fixture, parity, or code
  failures. Do not make an external result look deterministic.
- **Known scoped exceptions:** `Extract` Markdown/plain/rich rendering is the
  user-authorized practical renderer exception; raw fetch/bytes/text/errors
  remain frozen-source contracts. DDG `HttpClient2` now has local TLS/H2
  lifecycle parity evidence, but no newly authorized external JA3/JA4/Akamai
  observation; do not claim external byte/fingerprint equality. Base
  `primp.HttpClient` uses the complete 23×5 coherent profile catalog.
- **Final offline verification (2026-07-25):** passed `gofmt -d`, `git diff
  --check`, `go vet ./...`, `go test -count=1 -timeout=120s ./...`, `go test
  -race -count=1 -timeout=180s ./...`, `CGO_ENABLED=0 go test -count=1
  -timeout=120s ./...`, integration-tag compilation and opt-out smoke,
  `tools/reference_capture.py --check`,
  `tools/capture_browser_profiles.py --verify-http-connect`, `make verify`,
  strict validation of both OpenSpec changes, and 80.2% line coverage for
  `internal/transport`. Controlled diagnostic only (no engine) matched
  explicit Python/Go Chrome, Edge, Opera, Safari and Firefox observations.
- **DDG session-fingerprint acceptance (2026-07-25):** passed focused RED/GREEN
  tests, `go test ./internal/transport -covermode=atomic` (**80.2%**),
  `go test ./... -count=1`, `go test -race ./... -count=1`,
  `CGO_ENABLED=0 go test ./... -count=1`, `make verify`, exact-source
  `tools/capture_duckduckgo_session_profiles.py --verify`, strict OpenSpec,
  integration-tag compilation and endpoint-absent fingerprint skips. Repeated
  `-race -count=20` passed concurrent clients, reuse/reconnect and cancellation.
  The concurrency/debugger review found no shared mutable profile, response-body,
  cancellation or goroutine-lifecycle hazard. No external request ran.
- Target: Go library port of DDGS only. No API server, supported CLI, MCP,
  DHT, cache, Docker service, or application entrypoint. Standalone `examples/`
  `main` packages are documentation source, not a product command surface.
- Module scaffold exists. Root package is `ddgs`; final module path is
  `github.com/jcastilloa/goddgs`.
- All OpenSpec work is archived. Canonical live-example requirements live in
  `openspec/specs/live-api-examples/`; DDG session transport specs live in
  `openspec/specs/duckduckgo-text-session-transport/` and
  `openspec/specs/duckduckgo-text-fingerprint-verification/`.
- Public façade/configuration, normalizers, ordered result aggregation, ranker,
  backend selection/static registry, isolated fixture-tested scheduler core,
  offline HTML/XPath/JSON parser adapter, isolated base transport,
  request-local DDG text standard-HTTP/2 transport, and fixture-backed
  DuckDuckGo, Grokipedia, Wikipedia, Brave, Google, Mojeek, Startpage, Yahoo,
  and Yandex text adapters; Bing/DuckDuckGo image adapters; Bing/DuckDuckGo/
  Yahoo News adapters; DuckDuckGo Videos; and Anna's Archive Books are complete.
  Task 7.7 composes all active frozen adapters into the public client with lazy
  per-client caching, fresh isolated transports, source-order construction before
  scheduler work, ordered keyword forwarding, and non-normalizing results.
  `Extract` preserves frozen fetch/bytes/text/error lifecycle with a
  user-authorized practical renderer; its rendered strings are not source
  `primp` claims. Base HTTPS clients now select one complete frozen `primp`
  browser/OS bundle from the 23×5 source outcome space at construction. The
  choice binds ClientHello semantics, ALPN, HTTP/2 SETTINGS/window/priority,
  pseudo-header order, regular-header order and default headers for that
  client's whole lifetime across direct, PEM, disabled-verification, HTTP(S)
  CONNECT and SOCKS HTTPS target routes. It is never a superficial User-Agent
  or header shuffle. Python binding chronology is OS first (Android, iOS,
  Linux, macOS, Windows), then browser variant. Local wire tests cover all 115
  pairs; controlled diagnostics match explicit Chrome, Edge, Opera, Safari and
  Firefox source observations. DuckDuckGo now has its own source-shaped local
  TLS/H2 profile path; no live search-engine request ran during that transport
  work before the separately user-authorized example observations above.
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
  do not prove browser/TLS-H2 fingerprint parity. Extraction rendering is a
  documented practical exception, not source-render equivalence.
- Tasks 7.3–7.5 are complete. `internal/extract` has a context-first
  fetcher/renderer boundary and frozen tests for raw bytes/text, selected
  representation, non-200, config forwarding, cancellation, and fetch errors,
  plus dedicated Go output tests for the practical renderer. Root `DDGS.Extract`
  creates an isolated transport per call; rendered Markdown/plain/rich remains
  explicitly excepted by the user decision below.
- Task 5.5 is complete as the evidence gate. The completed
  `randomize-browser-profiles` change replaces its historical fixed Chrome
  proof with all 23×5 source browser/OS bundles. `docs/fingerprint-gate.md`
  records matching explicit Python/Go Chrome, Edge, Opera, Safari and Firefox
  diagnostic observations; TLS random/key-share/GREASE entropy remains fresh
  by design. DuckDuckGo's temporary source client remains distinct.
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
  task 7.7 and use the completed base `primp` 23×5 browser-profile transport.
- Task 6.6 complete: News adapters use injected request ports and per-instance
  UTC clock seams. Bing preserves payload mapping, source local-zone absolute
  date conversion, localized relative dates, and image truncation; DuckDuckGo
  preserves VQD-before-safe validation and raw dynamic `results` iteration;
  Yahoo preserves fixed-relative-date units, URL/image/source cleanup, and its
  broad partial postprocess failure. They are composed through task 7.7 and use
  the completed base `primp` 23×5 browser-profile transport.
- Task 6.7 complete: DuckDuckGo Videos uses an injected request port and
  preserves VQD-before-safe validation, mandatory four-slot filters, page
  stride 60, nested heterogeneous video values, and raw iterable/attribute
  errors. Video filters cross the façade as ordered `resolution`, `duration`,
  and `license_videos` keywords; public `max_results` remains scheduler-only.
  It is composed through task 7.7 and uses the completed base `primp` 23×5
  browser-profile transport.
- Task 6.8 complete: Anna's Archive uses a private process-lifetime,
  synchronized TLD selector and an immutable per-adapter search URL. Fixtures
  preserve its ordered GET pagination, HTML comment delimiter removal,
  nil-versus-empty response boundary, and the intentional base prefix on an
  already-absolute URL. It is composed through task 7.7 and uses the completed
  base `primp` 23×5 browser-profile transport.
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
| `primp>=1.2.3` | randomized browser impersonation, TLS/HTTP behavior, proxy/certs, HTML render properties | base HTTPS uses a frozen 23×5 coherent browser/OS catalog across direct and tunnel target routes; DDG temporary fingerprint and renderer strings remain documented limits |
| `lxml>=4.9.4` | tolerant HTML parse + XPath | Helium v0.6.0 internal adapter passes 14 frozen lxml fixtures; JSON decoder preserves `json.Number`/raw mixed values |
| `httpx[http2,socks,brotli]`, `httpcore`, `h2` | DDG text temporary client; random HTTP/2/TLS behavior | local source-shaped TLS policy/cipher session selection and per-connection H2 initialization are loopback fixture-tested; external fingerprint equality remains unobserved |
| `fake-useragent>=2.2.0` | DDG text random user agent | full frozen weighted pool is embedded, checksum-verified, and selected once per process |
| Click/FastAPI/Uvicorn/MCP | CLI/service only | explicitly excluded |

Go parser `github.com/lestrrat-go/helium v0.6.0` is approved and implemented
behind `internal/parser` after a 14/14 lxml corpus/race probe. `htmlquery
v1.3.6` is rejected for XPath-union and malformed-HTML divergence. Parser JSON
uses `UseNumber`, rejects a second top-level value, and leaves source field
ordering to engine adapters.

Base transport imports `golang.org/x/net/proxy v0.57.0` only to preserve the
frozen SOCKS5-local versus SOCKS5H-remote DNS distinction. It materializes and
closes native bodies and has per-client cookie/header state. All HTTPS target
routes additionally use `sardanioss/utls` with one selected frozen `primp`
ClientHello/H2/header bundle per isolated client. The generated catalog has 23
browser variants × 5 operating systems and is captured only via local loopback;
details and checksum are in `docs/browser-profiles.md`. `DuckDuckGoTextClient`
uses a separate five-policy local source capture: enabled-verification policy
and legacy cipher order are selected once per client, while H2 values are
sampled per connection. It retains no-redirect/header/jar/lifecycle behavior;
external fingerprint equality remains deliberately unobserved.

The frozen Python repository has no dependency lockfile; its `pyproject.toml`
only declares lower bounds. Before fixture capture, task 2.1 must record exact
resolved runtime package versions (and preferably wheel hashes) as provenance.

## Open blockers / risks

1. **Browser fingerprint limits — narrowed.** Base HTTPS uses the full source
   23×5 coherent selection space on direct, PEM, disabled-verification and
   tunnel target routes. TLS entropy/GREASE bytes remain intentionally fresh.
   DuckDuckGo's external `HttpClient2` fingerprint remains unobserved after
   the local port; do not infer JA3/JA4/Akamai equality.
2. **Rendered extraction divergence — accepted exception.** `primp` output is
   not reproduced for Markdown/plain/rich. `html-to-markdown` plus a practical
   plain-text projection is tested and documented; do not advertise it as 1:1.
3. **Live-engine volatility — high.** Upstream markup and anti-bot policy
   change. Offline contracts are primary; integration checks are tagged and
   rate-limited.
4. **Source baseline drift — medium.** Source `__version__` and HEAD differ.
   Any upstream update requires a new audit/diff and explicit OpenSpec change.
5. **Module path — resolved.** Final Git remote/import path is
   `github.com/jcastilloa/goddgs`; changing it later breaks every importing
   program.

## Required evidence before an engine is complete

1. Frozen source request sequence captured: method, URL, query/body, relevant
   headers/cookies, token/bootstrap calls, and random inputs controlled.
2. Offline Go payload/selector/result/error golden tests, including edge cases.
3. Differential comparison with frozen Python behavior for same fixture.
4. Race-safe concurrent Go test; no leaked responses or goroutines.
5. Opt-in live integration result when practical, recorded as an observation,
   never sole proof of parity.

## Next implementation order

Implementation scope is complete. The next action is user-run functional
engine smoke testing under an appropriate rate limit, recording connectivity
observations separately from the frozen offline contracts. Any request to close
the remaining strict fingerprint or rendered-output gaps needs a new OpenSpec
change and source evidence.

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
| 2026-07-25 | Start `port-ddg-session-fingerprint` from local source checkout | User supplied `/home/jcastillo/Proyectos/ddgs`; it is clean at `a12929a72429a39a0841c3d7caacb20ee17acd4d`. Exact editable probe is CPython 3.12.3 with ddgs 9.14.4, httpx 0.28.1, httpcore 1.0.9, h2 4.3.0, hpack 4.2.0 and hyperframe 6.1.0. |
| 2026-07-25 | Correct DDG randomness lifetime | `_get_random_ssl_context` runs once at `HttpClient2` construction only when verification is enabled; policy and legacy cipher order are DDGS-session/client lifetime. `Patch` is request-scoped but samples seven H2 values only when a new h2 connection sends its initialization. `verify=false` bypasses TLS draws but not new-connection H2 draws. |
| 2026-07-25 | Capture deterministic sanitized DDG source templates | `tools/capture_duckduckgo_session_profiles.py` passes twice with byte-identical artifact SHA-256 `5111e75b502d8991f8f98e70231fb3ca169fec94342120e2d4cbd4a5a00e87df`; `--verify` passes. It uses loopback only and strips handshake random/session/key-share entropy. |
| 2026-07-25 | Complete and archive DDG session-fingerprint OpenSpec (12/12) | RED `go test ./internal/transport -run '^TestDuckDuckGoTextClient_EmitsSourceShapedHTTP2Initialization$' -count=1` failed only because standard Go emitted connection window `1073741824`, not source `16777216`. GREEN/REFACTOR adds private session TLS policy/cipher shuffle, connection H2 factory, all target routes, reuse and cancellation. `gofmt`, diff, 80.2% transport coverage, full/race/CGO-off, `make verify`, capture verify, strict OpenSpec, tagged compile/skip and `-race -count=20` pass. Sync created five main-spec requirements; archive path is `openspec/changes/archive/2026-07-25-port-ddg-session-fingerprint/`. Parser/result/extract are N/A; no endpoint or live engine was contacted. |
| 2026-07-20 | Restrict epic to Go library | explicit user scope |
| 2026-07-20 | Use root public `ddgs` package plus small internal library layers | Go-module adaptation of skeleton; avoids service boilerplate |
| 2026-07-20 | Do not implement an engine before differential contracts | search-engine parity is product-critical |
| 2026-07-25 | Confirm final module path | User confirmed remote `https://github.com/jcastilloa/goddgs`; module/imports now use `github.com/jcastilloa/goddgs` |
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
| 2026-07-20 | Complete extraction fixture corpus (task 2.6) | Nine sanitized loopback-only fixtures freeze raw bytes/text, Markdown/plain/rich output, unknown-format fallback, 503 error, selected response property, and invalid-UTF-8 behavior |
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
| 2026-07-25 | Complete extraction TDD (tasks 7.3–7.5) | RED fixture tests preceded `Extractor`; GREEN retains source GET/config, bytes/text/status/cancellation/error behavior and practical HTML renderer; root uses fresh isolated transport per `Extract` call. Focused and full race/CGO-off tests pass. |
| 2026-07-25 | Accept practical Go extraction renderer | Source `primp` Rust output differs. User chose `html-to-markdown v1.6.0` after review; Go contract covers its Markdown/plain/rich output, never claims source formatting parity. |
| 2026-07-25 | Complete browser-fingerprint evidence gate (task 5.5) | Endpoint-explicit tagged observations show both Go transports negotiate HTTP/2 but expose the same standard Go JA3/JA4/Akamai hashes, distinct from observed source `primp` and `HttpClient2` values. `docs/fingerprint-gate.md` maps every active engine to its still-unproven source client; no dependency or parity claim was added. |
| 2026-07-25 | Complete public composition gate (task 7.7) | Root composition selects frozen metadata lazily, eagerly constructs all selected adapters before scheduler workers, caches adapters per public client under a narrow lock, and gives every engine a fresh isolated transport. DuckDuckGo text gets its separate H2 client plus the frozen full weighted `fake-useragent 2.2.0` pool, decoded/checksummed once per Go process. Public boundary preserves ordered keyword forwarding and maps scheduler timeout/generic failures to classifiable `DDGSError` values. |
| 2026-07-25 | Complete completed-package refactor/audit gate (task 7.6) | Applied clean-code/simplification review across every green package. No behavior-preserving production rewrite was justified against the differential corpus; corrected public Godoc/README for practical extraction and fingerprint limits. Full unit/race/CGO-off, oracle, vet, formatting, and OpenSpec pass. |
| 2026-07-25 | Approve practical extraction renderer scope exception | User authorized `github.com/JohannesKaufmann/html-to-markdown v1.6.0` for `Extract` rendered formats after reviewing frozen differences. This unblocks task 7.5. Raw fetch, bytes, decoded text, configuration, cancellation, and error behavior remain source contracts; rendered Markdown/plain/rich are explicitly not claimed as `primp`-identical. |
| 2026-07-25 | Complete practical-release review (task 8.6) | User explicitly accepted a practical release after reviewing renderer and fingerprint limits. All revised OpenSpec tasks are complete; documentation must continue to prohibit claims of a strict `primp` renderer or browser TLS/H2 fingerprint/complete 1:1 parity. The change is releasable as a tested Go module, but must not be marketed as strict source equivalence. |
| 2026-07-25 | Complete practical browser transport and final review (tasks 5.6, 8.7) | Historical fixed-Chrome transport proof: `sardanioss/utls v1.10.3` supplied the first isolated direct-HTTPS Chrome 146 ClientHello/H2 implementation. It is superseded by the completed 23×5 browser-profile change below; DuckDuckGo temporary `HttpClient2` remains a distinct limit. |
| 2026-07-25 | Complete source-shaped browser-profile randomization | `primp-python` chooses operating system first (`android`, `ios`, `linux`, `macos`, `windows`) then one of 23 browser variants. Go now retains the same two independent draws as one immutable 23×5 TLS/ALPN/H2/header identity per base client, across direct, PEM, verify-off, HTTP(S) CONNECT and SOCKS HTTPS target routes. The 115-pair loopback capture asset is checksum/provenance verified; every bundle passes uTLS instantiation and local wire tests. TDD RED exposed the former fixed-profile/wrong-draw-order gap, then GREEN/REFACTOR unified catalog, TLS and H2 ownership. `golang-pro`, hexagonal boundary review, `golang-testing`, TDD RED→GREEN→REFACTOR, clean-code and simplification applied. Concurrency patterns/debugger review covered per-origin cache, cancellation, retry and reuse; focused `-race -count=10` passes. Final acceptance passed formatter, diff check, vet, full tests/race/CGO-off, integration-tag compile/opt-out, Python fixture/profile verification, `make verify`, strict OpenSpec validation and 80.2% transport coverage. Controlled diagnostics match explicit `primp` Chrome 146/Windows, Edge 148/Linux, Opera 131/Android, Safari 26.3/macOS and Firefox 148/iOS observations. No search engine request ran. |
| 2026-07-26 | Add and archive live API examples | Six `examples/` programs document all public categories and extraction without changing library behavior. `go build ./examples/...` and `make verify` pass. User-authorized live requests exposed current DuckDuckGo/Anna's Archive availability failures while integration News/Videos passed; examples preserve those errors and documentation states live output is not parity evidence. `openspec archive -y add-live-api-examples` added the three canonical `live-api-examples` requirements and archived the completed change. |

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
- **Acceptance 7.6 (2026-07-25):** clean-code/simplification review found no
  safe behavior rewrite beyond stale public documentation. `gofmt`,
  `git diff --check`, `go vet ./...`, non-extract unit/race/CGO-off suites,
  the 368-fixture oracle, and strict OpenSpec validation passed. The known
  `internal/extract` RED remains intentionally excluded; it fails only because
  no approved renderer exists.
- **Acceptance 5.6/8.7 (2026-07-25):** frozen Python `--check` verified 368
  fixtures (138 pure, 179 engine, 9 extract, 24 parser, 18 transport).
  `gofmt`, `git diff --check`, `go vet ./...`, unit tests, full `-race`,
  `CGO_ENABLED=0` tests, integration-tag compilation/opt-out smoke, `make
  verify`, and strict OpenSpec validation passed. Coverage: total 86.0%; root
  90.4%; engine 86.9%; extract 82.2%; normalize 91.7%; parser 83.4%; search
  91.9%; transport 80.0%. Focused browser transport `-race` stress passed
  20–30 repetitions. A controlled TLS diagnostic (not an engine search)
  observed HTTP/2, source Chrome JA4/Akamai H2, and GREASE-varying JA3 values
  including `0482d5…`; no live search-engine request ran.
- **Acceptance randomize-browser-profiles (2026-07-25):** source chronology
  was corrected to the Python binding's OS-first (`5`) then browser-version
  (`23`) draws. The 115-pair `primp 1.3.1` loopback asset is SHA/provenance
  verified and passes target-side HTTP CONNECT verification. All profiles pass
  fresh uTLS instantiation and local TLS/H2 wire tests, including headers,
  SETTINGS/order, windows, priority, pseudo order, reuse, cancellation and
  failed-dial retry. Direct, PEM, disabled-verification, HTTP(S) CONNECT and
  SOCKS target routes preserve the same full selected bundle; plain HTTP stays
  standard. Final full/race/CGO-off tests, fixture verification, `make verify`
  and strict validation pass; transport coverage is 80.2%. The controlled
  diagnostic (not a search engine) matched explicit source Chrome 146/Windows,
  Edge 148/Linux, Opera 131/Android, Safari 26.3/macOS and Firefox 148/iOS.
  Skills applied: `golang-pro`, `go-clean-ddd-hexagonal`, `golang-testing`,
  TDD RED→GREEN→REFACTOR, `clean-code`, `go-code-simplification`,
  `go-concurrency-patterns`, `go-debugger-pro`, and `openspec-apply-change`.
  Published in `ca7c613`.
- **OpenSpec archive (2026-07-26):** synced and archived completed
  `port-ddgs-python-library` and `randomize-browser-profiles` as
  `openspec/changes/archive/2026-07-25-*`. The library change created the
  canonical content-extraction, module, parity-verification, and
  search-engine specs; randomization created its profile spec and updated the
  actual `Practical browser-profile transport` requirement. The historical
  randomization delta named an obsolete module requirement, so it was
  reconciled against that canonical transport requirement before archival; no
  production code changed. `openspec validate --all --strict --no-interactive`
  passed 7/7 and `openspec list --json` returned no active changes. Archive
  sync committed and pushed as `5bd75b3`.
