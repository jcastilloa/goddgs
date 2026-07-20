## ADDED Requirements

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
