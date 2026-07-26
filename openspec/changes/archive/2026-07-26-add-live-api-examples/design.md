## Context

`github.com/jcastilloa/goddgs` is an importable Go library with a completed
context-first `DDGS` façade. Its public search operations return lossless
`[]RawResult`, and extraction preserves a dynamic content type. Existing
tests and the integration smoke suite prove contracts and connectivity but do
not act as copy-ready API documentation.

Examples must make live requests because their purpose is to demonstrate the
public client against current endpoints. They therefore cannot be Go example
tests or part of `make verify`: endpoint markup, availability, and bot policy
are nondeterministic and frozen offline fixtures remain the parity authority.

## Goals / Non-Goals

**Goals:**

- Provide standalone programs below `examples/` for every public search
  category and extraction operation.
- Demonstrate bounded context cancellation, client timeout, explicit frozen
  active backend selection, category-specific options, and raw result output.
- Make network effects and run commands clear before a user executes an
  example.
- Keep examples buildable without contacting a network endpoint.

**Non-Goals:**

- No public API, engine request, parser, scheduler, transport, TLS, or result
  shape change.
- No executable library CLI, application `cmd/` tree, benchmark, fixture, or
  integration-test expansion.
- No claim that a live result proves deterministic source parity, endpoint
  health, ranking, or browser-fingerprint equivalence.

## Decisions

### Use one standalone `main` package per operation

Each operation gets a small `examples/<operation>/main.go` program. A reader
can run one focused command and see its exact API call without a category
switch or runtime argument parser. These programs are source examples only,
not a supported command-line product or a `cmd/` package.

### Use explicit live backends and compact result limits

Examples select one frozen active backend and request a small maximum result
count. Explicit selection makes request scope clear and avoids variable
fan-out from `auto` selection. Query values are illustrative real searches,
not golden inputs or a promise of any result.

### Serialize dynamic values as JSON through a local example helper

A shared unexported-in-practice helper beneath `examples/internal/` writes
results to standard output with `encoding/json`. This preserves the raw map
and nested dynamic values visibly instead of projecting unstable engine fields
into a new typed example API. Errors stay on standard error and terminate the
example with a nonzero status.

### Use caller-owned timeout context and client timeout

Every program owns a `context.WithTimeout` cancellation function and
constructs `ddgs.New` with the same bounded timeout. This shows the I/O
contract and ensures a user can interrupt a stalled live run without changing
library defaults. The example does not alter transport ownership or start
additional goroutines.

### Document network effects separately from tests

`examples/README.md` lists each `go run` command, states that it sends a live
external request only when invoked, and distinguishes those observations from
the opt-in, rate-limited integration suite and frozen offline parity fixtures.
The acceptance check is `go build ./examples/...`, which compiles without
making an external request.

## Risks / Trade-offs

- [Endpoint rejects or returns no result] → Present errors exactly to the
  interactive user and document that this is live availability, not an
  offline-contract failure.
- [Raw result maps contain heterogeneous values] → JSON encode the maps; do
  not invent stable result structs or fields.
- [Example programs resemble a CLI] → Keep them under `examples/`, omit flag
  parsing and reusable command behavior, and state their documentation-only
  purpose.
- [A network example is accidentally added to default tests] → Use no
  `_test.go` files or Go `Example` output blocks; build examples separately.

## Migration Plan

1. Add programs and run instructions without modifying existing imports or
   test targets.
2. Validate formatting, static analysis, offline tests, race tests, and
   compilation of all example packages without executing them.
3. A user may run a listed command in a permitted rate window. Removing the
   directory reverts the documentation-only addition without data migration.

## Open Questions

- None.
