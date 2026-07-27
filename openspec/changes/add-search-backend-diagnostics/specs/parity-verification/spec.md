## ADDED Requirements

### Requirement: Diagnostics have deterministic offline and race evidence
The diagnostic extension SHALL have offline tests for successful, empty, and failed engine completion events, plus race-enabled verification for concurrent callbacks.

#### Scenario: Diagnostic scheduler code changes
- **WHEN** scheduler completion diagnostics are added or modified
- **THEN** focused offline tests and `go test -race ./...` SHALL pass without external engine requests
