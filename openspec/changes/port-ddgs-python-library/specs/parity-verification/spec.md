## ADDED Requirements

### Requirement: Frozen Python differential corpus
Project SHALL maintain sanitized reproducible fixtures generated from frozen
Python source commit. Fixture SHALL identify source SHA, category, engine,
controlled inputs, request sequence, relevant request metadata, sanitized
response/error input, and expected normalized output or error classification.

#### Scenario: Engine behavior is added
- **WHEN** Go engine adapter is proposed for completion
- **THEN** repository SHALL contain differential fixtures demonstrating frozen Python request and output behavior

#### Scenario: Sensitive request state is captured
- **WHEN** source capture contains proxy credentials, session cookies, private URL data, or other secrets
- **THEN** capture tooling SHALL redact or omit them before fixture storage

### Requirement: Offline contract tests precede engine implementation
For each new behavior, project SHALL add deterministic offline Go contract tests
before production implementation, execute RED before GREEN, and retain fixture
assertion after refactoring. Tests SHALL verify observable behavior rather than
private implementation structure.

#### Scenario: New engine mapping is implemented
- **WHEN** engine payload or post-processing rule is added
- **THEN** named table-driven offline test SHALL first fail against missing behavior and later pass

#### Scenario: Parser behavior changes
- **WHEN** parser library, selector adapter, or renderer changes
- **THEN** all affected frozen fixtures SHALL remain green without editing expected output to hide regression

### Requirement: Live engine tests are explicit and rate-safe
Tests contacting external engines SHALL use `//go:build integration`, require
explicit opt-in, serialize or rate-limit calls, and state they are connectivity
observations rather than deterministic ranking goldens.

#### Scenario: Default test run
- **WHEN** contributor runs `go test ./...` or `make test`
- **THEN** no real external search engine request SHALL be made

#### Scenario: Integration test is requested
- **WHEN** contributor runs `make integration` in approved environment
- **THEN** tagged tests SHALL run with documented rate limits and no credentials committed

### Requirement: Concurrency and resource verification
Any package starting engine goroutines or owning response bodies SHALL test
cancellation, resource closure, and race safety. Change verification SHALL run
`go vet ./...`, `go test ./...`, and `go test -race ./...`.

#### Scenario: Fan-out code changes
- **WHEN** scheduler or transport fan-out code changes
- **THEN** race-enabled tests SHALL pass and instrumentation SHALL show no operation-owned goroutine leak

#### Scenario: Verification cannot run
- **WHEN** required verification command is unavailable or fails
- **THEN** task status SHALL remain incomplete and handoff SHALL report exact command, failure, and blocker

### Requirement: Source baseline updates are audited
Project SHALL not change frozen Python baseline merely by pulling upstream.
Baseline update SHALL include source diff, fixture update, OpenSpec decision,
and memory/audit update.

#### Scenario: New upstream release appears
- **WHEN** contributor wants newer upstream commit
- **THEN** contributor SHALL create or update approved OpenSpec change before changing source SHA or contracts
