## Context

The frozen Python API returns raw result dictionaries and intentionally does not expose engine provenance. `goddgs-server` needs operational visibility for research requests, but adding a field to each raw result would break its parity-preserving result shape. The scheduler already owns the authoritative engine completion data.

## Goals / Non-Goals

**Goals:**

- Provide opt-in, per-search completion events with engine and provider identity, nonempty-result status, count, and error text.
- Preserve default behavior and all frozen search semantics.
- Keep diagnostics race-safe for callers collecting events from concurrent engine work.

**Non-Goals:**

- Change frozen Python output, selection, worker count, provider de-duplication, ranking, or errors.
- Promise that every selectable backend was tried or that events are in selection order.
- Add tracing, metrics exporters, HTTP APIs, or transport-level timings.

## Decisions

### Expose a callback option rather than mutate raw results

`WithSearchDiagnostics` accepts a callback on the public search option. Events are emitted only when an engine call completes. This preserves raw map contents and lets consumers attach diagnostics to one operation. A synthetic result field was rejected because it violates result-shape parity; process-global hooks were rejected because they create cross-request state and races.

### Emit from the scheduler at completion time

The scheduler has the exact `ScheduledEngine`, result slice, and error. It will call the configured callback after an engine search returns, before its completion is consumed. The callback receives a value-only event. Events are completion-ordered and may run concurrently, matching the scheduler's existing worker model. This captures attempted engines, including empty and failed attempts, rather than inferring provenance from aggregated output.

### Keep callback opt-in and side-effect isolated

With no callback, no diagnostic allocation or callback invocation occurs. Callback panics are not recovered: the caller supplied code executes in library-owned workers and must obey normal Go callback rules. The server callback will be mutex-protected and only append immutable event data.

### Preserve source scheduler behavior

Diagnostics are observational. Existing pending, provider-seen, completion, aggregation, cancellation, and error paths remain unchanged. Tests use controlled engines to verify event contents and that a nil diagnostic callback leaves output unchanged.

## Risks / Trade-offs

- [Callback can block scheduler workers] → Document synchronous completion callback behavior; server callback performs only a protected append.
- [Concurrent event order is nondeterministic] → Expose completion events as a set/list, not a selection-order trace.
- [No source equivalent] → Keep opt-in and document it as a Go diagnostic extension, isolated from default parity behavior.
- [Dependency update coordination] → Commit and push library change before updating server dependency to the new pseudo-version.

## Migration Plan

1. Add library option and scheduler event with RED/GREEN/REFACTOR and race verification.
2. Commit and push `goddgs`; update `goddgs-server` to the resulting module version.
3. Have research collect unique attempted/successful engine names and phase durations in its response.
4. Deploy both commits together. Rollback server first or pin it to the prior library version.

## Open Questions

None. “Used” is represented as attempted engines, with successful engines separately identified, avoiding ambiguity when an engine returns no results.
