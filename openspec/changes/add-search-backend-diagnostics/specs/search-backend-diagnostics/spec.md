## ADDED Requirements

### Requirement: Opt-in search completion diagnostics
The library SHALL provide an opt-in public search option that receives one value-only diagnostic event for every engine search completion. Each event SHALL include engine name, provider name, result count, whether results were nonempty, and any returned error.

#### Scenario: Engine returns results
- **WHEN** a configured diagnostic callback observes an engine completing with two results
- **THEN** it SHALL receive that engine and provider name, result count `2`, `has_results=true`, and no error

#### Scenario: Engine fails or returns no results
- **WHEN** an engine completes with an error or an empty result list
- **THEN** the callback SHALL receive an event describing that completion without changing scheduler fallback behavior

### Requirement: Diagnostics are opt-in and concurrent
The library SHALL not alter public raw results, errors, scheduling, or ranking when no diagnostic callback is supplied. Diagnostic callbacks SHALL execute at engine completion and callers SHALL treat event order as concurrent completion order.

#### Scenario: Default search behavior
- **WHEN** caller does not configure diagnostics
- **THEN** the search result and error behavior SHALL remain identical to the existing source-compatible path

#### Scenario: Concurrent completions
- **WHEN** multiple scheduled engines complete concurrently
- **THEN** each completion SHALL produce one callback event and the library SHALL retain no mutable diagnostic state after the operation returns
