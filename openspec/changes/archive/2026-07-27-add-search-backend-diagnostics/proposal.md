## Why

Consumers that orchestrate multiple searches cannot tell which engines actually ran or returned data. They need opt-in diagnostics without changing the established raw result shape or source-compatible scheduling.

## What Changes

- Add an opt-in search diagnostic callback that reports each engine execution after it finishes.
- Report engine name, provider, whether it produced results, result count, and failure text when present.
- Preserve normal `DDGS` result values, ordering, errors, selection, and scheduling when no callback is configured.

## Capabilities

### New Capabilities
- `search-backend-diagnostics`: Opt-in execution diagnostics for a completed search operation.

### Modified Capabilities
- `search-engine-parity`: Diagnostics must not alter frozen search selection, scheduling, aggregation, ranking, or public raw result behavior.
- `parity-verification`: Add deterministic tests proving diagnostic execution information matches scheduler outcomes while the default path remains unchanged.

## Impact

- Public Go search options in `api.go`.
- Search composition and scheduler execution reporting.
- Offline unit and race tests; no source baseline or external network behavior changes.
