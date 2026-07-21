## Context

`goddgs` starts as an empty Go 1.26 module. It must port library behavior from
frozen `ddgs` Python source `a12929a72429a39a0841c3d7caacb20ee17acd4d`
(`v9.14.4-2-ga12929a`), not merely offer generic web search.

Source core and engine modules are roughly 2,256 lines. Active engines vary in
cookies, bootstrap requests, random values, lxml XPath, JSON types,
provider-level de-duplication, and two HTTP stacks. `primp` browser
impersonation and temporary `httpx`/`h2` behavior affect engine compatibility.
`ddgs.py` fans out concurrently and caches mutable engines, so direct Go
translation would risk races.

Target is reusable library only. No `cmd/`, server, CLI, MCP, DHT/cache, or
container behavior belongs in this change.

## Goals / Non-Goals

**Goals:**

- Provide context-aware Go façade for text, images, news, videos, books, and extract.
- Preserve active/disabled registry, request sequence, results/types,
  normalization, aggregation, ranking, error, proxy/TLS/timeout, and fan-out behavior.
- Make parity reproducible through sanitized offline fixtures from frozen Python source.
- Keep parser/transport details internal; make concurrency cancellation-safe,
  response-safe, race-free, and leak-free.

**Non-Goals:**

- Python ABI, CLI, FastAPI/MCP/DHT/cache/Docker behavior, or any daemon.
- New ranking, fallbacks, retries, caching, telemetry, or simplified schemas.
- Claims of deterministic live order where source uses randomization/concurrency.
- Selecting parser, fingerprint transport, or renderer before evidence proves parity.

## Decisions

### Pin source behavior by immutable commit and fixture corpus

**Proven:** package `__version__` says `9.14.4`, while HEAD is two commits
after tag `v9.14.4` and removes DHT/API coupling. Text Bing is README-listed
but disabled in runtime registry.

**Decision:** all contracts name full SHA. Frozen source is test input, not
runtime dependency. Each fixture records SHA, engine/category, controlled
input, request sequence, sanitized response, result/error. Tag-only, upstream
main, and README-only baselines are rejected because they lose behavior or
reproducibility.

### Use thin public façade plus lossless raw results

**Proven:** Python returns category-specific `list[dict[str, Any]]`; video
results have nested and non-string values. Python has no context.

**Decision:** public package will offer a `DDGS` client and Go methods `Text`,
`Images`, `News`, `Videos`, `Books`, `Extract`, all I/O methods taking
`context.Context` first. Search outputs are lossless raw `map[string]any` (or
an equivalent); extraction preserves string-or-bytes content. Functional
options preserve omitted vs explicitly supplied defaults, including unlimited
max-results. Final export spelling follows RED API contract tests. Errors wrap
public classifyable DDGS/timeout/rate-limit errors. Typed structs may layer on
later, never replace raw parity.

**Resolved public shape:**

```go
type RawResult map[string]any

type ExtractResult struct {
	URL     string
	Content any // source contract: string or []byte
}

func (d *DDGS) Text(ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
func (d *DDGS) Images(ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
func (d *DDGS) News(ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
func (d *DDGS) Videos(ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
func (d *DDGS) Books(ctx context.Context, query string, opts ...SearchOption) ([]RawResult, error)
func (d *DDGS) Extract(ctx context.Context, url string, opts ...ExtractOption) (ExtractResult, error)
```

`SearchOption` and `ExtractOption` are separate functional-option families;
they retain omitted/default, explicit zero, and explicit unlimited states
without exposing internal engine or transport types. `ErrDDGS`, `ErrTimeout`,
`ErrRateLimit`, and `*DDGSError` provide the Go error-classification boundary.
The public façade uses a private consumer-side executor port so fixture tests
can prove lossless forwarding before a real internal search adapter exists.

**Ordered internal result boundary:** Python result objects retain attribute
insertion order, including dynamically added fields; aggregation uses that
order to select its first eligible cache field. Each `internal/engine` adapter
therefore returns ordered result drafts: its source `__setattr__` assignments
in declared selector/JSON-mapping order, without a Go map boundary.
`internal/search` consumes those already source-shaped results, keeps the
ordered field sequence through aggregation, ranking, and slicing, then
converts to `RawResult` only at the public edge. The engine result constructor
applies the category's frozen defaults and named-field normalizers exactly
once, matching Python's result-object construction.
A Go map cannot promise iteration order, so no source order-dependent
operation may inspect a public or decoded Go map; dynamic field values and
names remain lossless while their source ordering is retained internally for
behavior.

**Rejected:** `map[string]string` and fixed structs lose source data; a public
third-party transport/parser type freezes an implementation detail; no context
breaks normal Go cancellation; a public test-only executor would freeze an
implementation seam.

### Use small library layers, not service skeleton baggage

```text
ddgs public façade
  └── internal/search      options, scheduler, aggregation, ranking
       ├── internal/engine source adapters/static registry
       ├── internal/transport
       ├── internal/parser
       ├── internal/normalize
       └── internal/extract
```

`search` consumes engine contracts; engine consumes narrow parser/transport
contracts; lower layers never import public/search packages. This applies
dependency discipline without irrelevant DI containers, repositories, or
service architecture.

**Engine execution port:** `internal/engine` owns a small `SearchRequest`,
ordered `Field`/`Result` draft, and `Searcher` contract. An adapter defines its
transport interface at its own consumer boundary and imports only
`internal/parser`, `internal/normalize` when source behavior needs it, and
`internal/transport`. It must never import `internal/search`. At composition,
`internal/search` consumes the engine result value without re-normalizing it.
The engine result constructor owns Python dataclass defaults and generic
named-field normalization. The boundary permits direct fixture testing of an
engine request/parser/post-process sequence while preserving inward dependency
direction.

### Make source normalization error-capable and freeze Unicode data

**Proven:** Python `_normalize_date` calls `datetime.fromtimestamp` for every
`int` (including `bool`). In the frozen CPython/Linux reference it raises
`ValueError` for years outside 1–9999 and `OSError` when the underlying C
`tm_year` range overflows. Python text normalization uses Unicode 15.0.0 and
HTML5 entities `nGt;` and `nLt;`, which Go's standard HTML table omits.
Default Go JSON decoding would turn source integer timestamps into `float64`,
silently bypassing this source rule.

**Decision:** internal date normalization returns `(value, error)` and emits a
classifiable internal error preserving captured source type/message for
out-of-range timestamps; result construction propagates it rather than
formatting an invented Go year. Engine JSON adapters must use `UseNumber` and
convert an exact integer to the signed source-integer representation before
date normalization; floats remain unnormalized as in Python. A small,
pre-unescape supplement supplies only Python HTML5's missing `nGt;`/`nLt;`
entities. NFC uses `golang.org/x/text` tables pinned to Unicode 15.0.0. Since
Go's standard Unicode category and printable tables drift with the compiler,
the module intentionally rejects Go 1.27+ until a source-baseline/OpenSpec
review either freezes replacement tables or approves a new Unicode baseline.

**Rejected:** formatting Go years outside source range, default `float64` JSON
decoding, a generic HTML/parser rewrite, or silently accepting a newer Go
Unicode database. Each loses a frozen source behavior.

### Replace Python discovery with audited static registry

**Proven:** Python discovers subclasses and excludes `disabled=True`.

**Decision:** explicit Go registry has exactly active source engines:

- Text: Brave, DuckDuckGo, Google, Grokipedia, Mojeek, Startpage, Wikipedia,
  Yahoo, Yandex.
- Images: Bing, DuckDuckGo.
- News: Bing, DuckDuckGo, Yahoo.
- Videos: DuckDuckGo.
- Books: Anna's Archive.

Text Bing stays inactive. Its explicit selection follows source invalid-backend
fallback, not a newly activated adapter. Tests assert names, categories,
providers, priorities, disabled status, and selection. Plugin discovery,
activating Bing, and dropping provider labels are rejected because each changes
observable behavior.

### Scheduler preserves source outcome rules but gains safe Go cancellation

**Proven:** source shuffles/sorts engines, calculates workers from unique
providers/max-results, batches futures, tolerates engine errors while results
exist, and only errors when no results remain.

**Decision:** scheduler gets test seams for randomness, clock, engine
execution, and fixtures. Its coordinator alone owns pending futures,
provider-seen state, aggregation, ranking, and the last source error; workers
return immutable completions through a bounded channel and never mutate those
structures. A source batch processes completions in submission order, not
arrival order. A `FIRST_EXCEPTION` wakeup or source wait timeout retains
pending work for source-compatible later handling; operation exit joins all
submitted work but does not aggregate a late result that source would leave
pending. Context stops caller-owned work and joins its goroutines, but it must
not become cancel-on-first-engine-error. The consumer-side engine port must
honor its context, return errors rather than panic, and treat request data as
read-only: Go cannot safely force-stop an arbitrary callback, so an adapter
that ignores cancellation fails its port contract. All optional schedule values
and the engine metadata slice are snapshotted at operation entry; request
values are copied again per engine invocation, so no mutable caller or sibling
request storage is shared. Client timeout remains the source-equivalent scheduler wait timeout,
not an implicit per-engine context deadline. Mutable URL/language/token/header
state is per-call. Client cookie/connection state is isolated/synchronized
only where source-visible behavior needs it. All bodies close; all goroutines
end.

The current scheduler port carries only source common search inputs. Python
forwards category-specific `**kwargs`; before public-to-engine wiring, fixture
capture must define a lossless immutable per-category option representation
and pass resolved client timeout, including explicit `None`. The isolated
scheduler remains a tested core until that composition task is complete.

**Rejected:** direct cached mutable engines cause races; serial/unbounded
execution changes behavior/resource use; `errgroup` early cancellation changes
partial-result behavior.

### Transport fingerprint is a hard gate

**Proven:** `primp` randomizes browser/OS impersonation; DDG text uses a
temporary HTTP/2 client with randomized cipher/H2 settings via global patch.

**Decision:** define an internal capability contract first: request/response,
cookies, redirect, HTTP(S)/SOCKS proxy, timeout, TLS verify/custom PEM,
compression, HTTP/2, UA, fingerprint. Choose Go implementation only after
license, maintenance, reproducibility, cgo, and fixture/live evidence review.
`net/http` can serve only proven paths. DDG H2 settings are request-local; no
global monkey patch.

The frozen oracle writes transport contracts separately from engine contracts:
constructor arguments are captured with a local `primp.Client` double and
HTTP behavior is captured only through an ephemeral loopback server with
synthetic payloads. These contracts establish observable source behavior but
do not themselves approve a Go TLS, HTTP/2, SOCKS, or fingerprint adapter.

The base transport port owns an isolated client configuration, cookie jar, and
native response lifecycle. It accepts a context-first request value and returns
a materialized response containing source-visible status, bytes, and text;
the native body is read and closed before return, so engine adapters cannot
leak it. Header/cookie updates are explicit client operations and request
maps/slices are copied before I/O. Rendered Markdown/plain/rich output is a
separate capability because source `Response` delegates those properties to
`primp`; base transport completion must not claim renderer or fingerprint
parity.

DuckDuckGo text gets a distinct internal transport capability rather than a
mutable switch on the base client. Its constructor snapshots source-compatible
proxy, timeout, verification, and engine User-Agent/header values into an
isolated jar, native `http.Client`, and cloned `http.Transport`. It requests
HTTP/2 with `ForceAttemptHTTP2`, preserves the source's
`follow_redirects=False` through `http.ErrUseLastResponse`, and copies every
request before I/O. It never patches package-global HTTP/2/TLS state; two DDG
clients therefore cannot share mutable header, jar, or transport configuration.

This capability proves only request shape, isolated state, redirect behavior,
context/response lifecycle, and standard-library HTTP/2 negotiation against a
controlled local TLS server. The frozen Python client randomizes cipher suites,
TLS versions/options, and HTTP/2 settings by temporarily monkey-patching
`httpcore`; Go standard `net/http` cannot reproduce those fingerprints. Those
settings remain an explicit task 5.5 per-engine compatibility gate, and no DDG
adapter may claim full transport parity merely because this capability exists.

**Rejected:** default `net/http` everywhere is unproven; blindly importing
fingerprint dependency creates supply-chain/cgo risk.

### Parser and renderer are selected by differential corpus

**Proven:** HTML engines use lxml recovery/XPath; `primp` provides Markdown,
plain, rich extract properties.

**Decision:** preserve source XPath expressions as contract and use
`github.com/lestrrat-go/helium v0.6.0` behind `internal/parser`, never exposing
its types. An isolated pure-Go probe matched all 14 frozen lxml fixtures,
including document-order XPath union and malformed Startpage recovery; its
HTML/XPath packages also passed under `-race`. The parser stays on its secure
defaults: no external network/filesystem loader, DTD, XInclude, or cgo path.
`github.com/antchfx/htmlquery v1.3.6` is rejected: it failed the Yahoo News
union and malformed Startpage contracts. Do not rewrite selectors to fit a
candidate. Capture all extract formats and accept renderer only when corpus
matches; unknown format defaults to source Markdown.

JSON-backed engines decode one complete JSON value through `internal/parser`
with `json.Decoder.UseNumber`. This preserves integer-versus-float lexical
form for later date normalization and retains `null`, missing map keys, nested
objects, and lists without string coercion. A second JSON value is rejected,
as `json.loads` does. JSON object insertion order is intentionally not a
parser contract: engine adapters must pass their frozen source field mapping
in declaration order to the ordered result boundary.

### Fixtures precede engine work; live testing is secondary

**Decision:** build Python capture tooling and sanitized `testdata` fixtures.
Go tests assert offline request/payload/result/error behavior. Live tests carry
`//go:build integration`, require opt-in, serialize/rate-limit, and never use
volatile rank ordering as golden output. Concurrent packages get `-race` and
leak checks.

**Rejected:** live-only tests are flaky/rate-limited; mocks without Python
oracle encode guesses.

## Risks / Trade-offs

| Risk | Mitigation |
| --- | --- |
| Fingerprint mismatch blocks engines | Transport evidence gate; mark unproven engine incomplete. |
| lxml/XPath mismatch | Capture corpus before parser choice; approve only passing candidate. |
| Rendered extract mismatch | Differential formats; no completion without compatible renderer. |
| Context timing differs from Python | Test/categorize caller cancellation separately from source timeout. |
| Go race from source mutable state | Per-call state, isolated clients, race/leak tests. |
| Engines change externally | Offline contracts primary; tagged live smoke tests secondary. |
| Dynamic values lost | Raw result stays first-class. |
| Dependency supply-chain risk | Review/pin/license document each dependency. |
| Module path changes | Confirm before release, no early publish. |

## Migration Plan

1. Keep releases pre-1.0; do not publish until module path and critical gates resolve.
2. Implement vertical fixture-backed slices: core, parser/transport, engines,
   extract, integration/race gates.
3. Put source SHA in release notes. Upstream update requires diff, fixtures,
   OpenSpec, and audit update.
4. No existing Go consumers exist; release rollback means patch/pin, never a
   silent source baseline change.

## Open Questions

1. What final Git remote/import path replaces `github.com/jcastillo/goddgs`?
2. Which reviewed Go transport proves required fingerprinting without bad cgo,
   license, security, or maintenance trade-offs?
3. Which renderer matches `primp` Markdown/plain/rich output?
4. How should source-wide `DDGS.threads` map to Go without mutable global state?
5. Can all source random choices be internal/injectable for deterministic tests?
