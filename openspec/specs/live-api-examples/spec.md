# live-api-examples Specification

## Purpose
TBD - created by archiving change add-live-api-examples. Update Purpose after archive.
## Requirements
### Requirement: Standalone live API examples
The repository SHALL provide standalone Go programs under `examples/` that
demonstrate public `ddgs.DDGS` text, image, news, video, book, and extraction
operations. Each program SHALL use a caller-owned bounded context, a bounded
client timeout, one frozen active explicit backend where the operation is a
search, and a small maximum-result limit. The examples SHALL use only public
library APIs for the demonstrated operation.

#### Scenario: User runs a search category example
- **WHEN** a user runs a documented search example with `go run`
- **THEN** it SHALL issue the shown real external search request and render
  the returned raw result data or a process-visible error

#### Scenario: User compiles examples without executing them
- **WHEN** a contributor runs `go build ./examples/...`
- **THEN** every example package SHALL compile without contacting an external
  search engine

### Requirement: Dynamic result output remains visible
Search examples SHALL emit returned `[]ddgs.RawResult` as JSON without
coercing source result fields or nested dynamic values into an example-specific
typed schema. Extraction examples SHALL emit both returned URL and content in
a JSON-encodable form. An example failure SHALL be written to standard error
and return a nonzero process status.

#### Scenario: Engine returns nested result data
- **WHEN** an example receives a raw result containing nested or non-string
  fields
- **THEN** its output SHALL preserve those values through JSON serialization
  rather than discarding or stringifying them

### Requirement: Network behavior is explicit documentation
`examples/README.md` SHALL list runnable commands and state that they send
real external requests only when manually invoked. It SHALL state that live
output is a current connectivity observation, not a deterministic parity or
ranking assertion, and SHALL direct contributors to the opt-in integration
documentation for the project test policy.

#### Scenario: Reader inspects example documentation
- **WHEN** a reader opens `examples/README.md`
- **THEN** they SHALL be able to select and run one category or extraction
  example while understanding its external network effect and test boundary
