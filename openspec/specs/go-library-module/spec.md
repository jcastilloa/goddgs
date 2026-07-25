# go-library-module Specification

## Purpose
TBD - created by archiving change port-ddgs-python-library. Update Purpose after archive.
## Requirements
### Requirement: Importable library-only module
Project SHALL provide an importable Go module whose root public package is
`ddgs`. It SHALL contain library code and test tooling only. It SHALL NOT add
HTTP API server, executable CLI, MCP server, DHT/cache service, Docker service,
or application `cmd/` entrypoint in this change.

#### Scenario: Consumer imports module
- **WHEN** Go program imports configured module path and runs `go test`
- **THEN** it SHALL compile without local daemon, API endpoint, or executable process

#### Scenario: Scope inspection
- **WHEN** reviewer inspects packages added by this change
- **THEN** no package SHALL expose REST routes, CLI commands, MCP tools, or service startup behavior

### Requirement: Context-aware DDGS façade
Public package SHALL expose a `DDGS` client façade for source-equivalent text,
image, news, video, book, and extraction operations. Every operation capable
of I/O SHALL accept `context.Context` as first argument and return error rather
than panic for expected failures.

#### Scenario: Caller cancels search
- **WHEN** caller cancels context supplied to DDGS operation before engine work completes
- **THEN** operation SHALL return context-classifiable error and SHALL NOT leave operation-owned goroutines blocked

#### Scenario: Caller uses all source categories
- **WHEN** caller invokes public text, images, news, videos, books, and extract operations
- **THEN** each operation SHALL route to corresponding source category contract rather than generic web-search substitute

#### Scenario: Caller invokes a composed search category
- **WHEN** caller invokes a public search method with a source backend and
  category-specific keyword options
- **THEN** the façade SHALL select only frozen active metadata, construct the
  corresponding source adapter lazily with isolated per-engine transport
  state, preserve ordered keyword inputs, and route the ordered source result
  fields to scheduler aggregation without a second normalization pass

#### Scenario: Caller configures image source filters
- **WHEN** caller invokes `Images` with image size, color, type, layout, or
  license options
- **THEN** Go SHALL retain source keyword names `size`, `color`, `type_image`,
  `layout`, and `license_image` for the selected engine without exposing an
  HTTP/parser type or coercing empty values

#### Scenario: Image max-results remains scheduler-only
- **WHEN** caller invokes `Images` with a public max-results option and image
  filters
- **THEN** Go SHALL retain max-results separately from forwarded image keyword
  arguments, matching frozen `_search_sync` data flow

#### Scenario: Caller configures video source filters
- **WHEN** caller invokes `Videos` with resolution, duration, or license
  options
- **THEN** Go SHALL retain source keyword names `resolution`, `duration`, and
  `license_videos` for the selected engine without exposing an HTTP/parser type
  or coercing empty values; public max-results SHALL remain scheduler-only

### Requirement: Lossless raw result contract
Public search result representation SHALL preserve source category fields and
dynamic value types without coercing all values to strings. Public extraction
result SHALL preserve URL and content where content can be text or raw bytes.

The public façade SHALL expose context-first `Text`, `Images`, `News`,
`Videos`, and `Books` methods returning `[]RawResult`, where `RawResult` is a
lossless `map[string]any`. It SHALL expose context-first `Extract` returning
an `ExtractResult` whose `Content` retains source `string` or `[]byte` value.
Search and extract option families SHALL be distinct so options cannot be
silently applied to a different operation.

Before conversion to `RawResult`, the implementation SHALL retain source field
insertion order, including dynamically added fields, and SHALL use that order
for every order-dependent source behavior such as aggregation cache-key
selection. It SHALL NOT use Go map iteration for that purpose.

#### Scenario: Video result contains nested values
- **WHEN** differential fixture contains source video result with nested maps or non-string statistics
- **THEN** Go raw result SHALL retain equivalent fields and value kinds

#### Scenario: Result gains a dynamic field
- **WHEN** a source result updates a declared field and then adds a dynamic field
- **THEN** the updated field SHALL retain its original position and the dynamic field SHALL be appended after declared fields for internal aggregation behavior

#### Scenario: Raw extraction is requested
- **WHEN** caller requests source `content` extraction format
- **THEN** Go extraction result SHALL contain raw bytes rather than lossy string conversion

### Requirement: Source-compatible client configuration
Façade SHALL provide Go-native representation of source proxy, timeout, and
TLS verification/custom PEM settings. It SHALL preserve source `tb` proxy
alias, `DDGS_PROXY` fallback when explicit proxy is absent, source default
timeout, and explicit caller verification control.

#### Scenario: Tor Browser alias is configured
- **WHEN** caller configures proxy value `tb`
- **THEN** requests SHALL use `socks5h://127.0.0.1:9150` as source specifies

#### Scenario: Environment proxy is used
- **WHEN** caller does not configure proxy and `DDGS_PROXY` is set
- **THEN** client configuration SHALL use that environment value

#### Scenario: Explicit proxy wins
- **WHEN** caller configures non-empty proxy and `DDGS_PROXY` is also set
- **THEN** explicit proxy SHALL take precedence

### Requirement: Practical browser-profile transport

HTTPS target requests made through the base source-client transport SHALL use
the reviewed browser-profile capability rather than the standard Go TLS and
HTTP/2 fingerprint. The client SHALL select one coherent frozen-source browser
profile per transport client from the `primp` browser-version and operating-
system outcome space. The selected identity SHALL remain stable for that
client's lifetime and bundle compatible headers, TLS and HTTP/2 behaviour. The
complete selection and route contract is defined by
`browser-profile-randomization`.

#### Scenario: Browser-profile HTTPS request is opened
- **WHEN** a source engine performs a direct HTTPS request through its isolated base client
- **THEN** the client SHALL select one coherent frozen `primp` browser/OS
  bundle and send that bundle's TLS semantics, headers, HTTP/2 SETTINGS order,
  connection window, and pseudo-header order

#### Scenario: Eligible client retains one selected browser identity
- **WHEN** an HTTPS-capable client makes multiple requests
- **THEN** all requests SHALL retain the browser-version and operating-system
  identity selected when that client was constructed

#### Scenario: Browser-profile connection is no longer needed
- **WHEN** `CloseIdleConnections` is called on the owning transport client
- **THEN** cached browser-profile connections SHALL be closed without affecting another engine client

#### Scenario: Tunnel does not split browser identity
- **WHEN** a caller configures a proxy, custom TLS root or disabled TLS verification
- **THEN** the target SHALL receive the selected profile while that path's
  proxy and verification contract is retained

#### Scenario: DuckDuckGo temporary fingerprint is considered
- **WHEN** a caller uses DuckDuckGo text's temporary HTTP/2 client
- **THEN** the module SHALL document that it is a distinct source transport and
  does not inherit the base-client browser-profile claim

### Requirement: Classifiable source errors
Public package SHALL expose errors allowing callers to classify source DDGS
failures, timeouts, and rate-limit failures with `errors.Is` or `errors.As`,
while retaining relevant cause context.

Public error boundary SHALL expose `ErrDDGS`, `ErrTimeout`, `ErrRateLimit`,
and `*DDGSError`. A source timeout or rate-limit error SHALL classify both as
its specific sentinel and as `ErrDDGS`; cancellation SHALL remain
context-classifiable rather than being converted into a source error.

#### Scenario: Engine timeout yields timeout classification
- **WHEN** no source result is available and frozen source classifies observed engine failure as timeout
- **THEN** Go operation SHALL return error classifiable as timeout

#### Scenario: Invalid mandatory query is supplied
- **WHEN** caller supplies source-equivalent empty query
- **THEN** operation SHALL return DDGS-classifiable error rather than dispatch an engine request
