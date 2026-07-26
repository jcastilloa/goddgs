## Why

The library exposes its completed public search and extraction API, but users
must infer call patterns and source-shaped result handling from tests. Small
programs that perform intentional live requests make the API discoverable
without turning nondeterministic network behavior into default test behavior.

## What Changes

- Add an `examples/` directory with independently runnable Go programs for
  text, image, news, video, book searches, and content extraction.
- Demonstrate client, category, and extraction options with bounded contexts
  and explicit source backends.
- Document that every example performs a real external request only when the
  user runs it, emits raw JSON-shaped data, and is not an offline parity test.

## Capabilities

### New Capabilities

- `live-api-examples`: Runnable, networked documentation programs for the
  public `ddgs` API.

### Modified Capabilities

- None.

## Impact

- Adds source files and documentation below `examples/`; no public API,
  transport, parser, scheduler, engine, dependency, or default test behavior
  changes.
- Uses frozen source baseline `a12929a72429a39a0841c3d7caacb20ee17acd4d`
  (`v9.14.4-2-ga12929a`) and preserves library-only scope: no HTTP API, CLI,
  MCP, service, cache, or Docker addition.
- Offline differential fixtures remain the parity-verification authority.
  Example output and live availability are observations only because engines
  and bot controls change independently of the frozen source contract.
