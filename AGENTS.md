# AGENTS.md — goddgs

## Mission

Build `goddgs`: an importable Go module that ports the **library behavior** of
[`deedy5/ddgs`](https://github.com/deedy5/ddgs) accurately. Search engines are
the product. Request shape, transport behavior, parsing, normalization,
deduplication, ranking, result shape, and errors are compatibility contracts.

This is not a service project. Do not add an HTTP API, CLI, MCP server, DHT,
cache daemon, Docker service, configuration server, or application `cmd/`
tree unless a later OpenSpec change explicitly expands scope.

## Source of truth

| Item | Value |
| --- | --- |
| Source checkout | `/home/jcastillo/Proyectos/ddgs` |
| Upstream | `https://github.com/deedy5/ddgs` |
| Frozen commit | `a12929a72429a39a0841c3d7caacb20ee17acd4d` |
| Source describe | `v9.14.4-2-ga12929a` |
| Python package version field | `9.14.4` |
| Active OpenSpec change | `port-ddgs-python-library` |

Authority order:

1. Frozen Python source at that commit.
2. Its executable behavior and captured differential fixtures.
3. Its tests.
4. Its README only where it agrees with source.
5. New upstream versions only after an explicit baseline-update decision.

The source README lists text `bing`, but source registry disables it. Runtime
source behavior wins. Record every discovered discrepancy in `MEMORY.md` and
the relevant OpenSpec artifact.

## Mandatory skills

Read this file and `MEMORY.md` first. Local Go skills in `.codex/skills/` are
mandatory quality gates, not a menu or optional suggestions. Before accepting
any Go change, explicitly assess every listed Go skill and use every one whose
declared scope applies. Record any genuine `N/A` decision in the task evidence
or `MEMORY.md`; "small change" is never a reason to skip one.

| Situation | Required skill(s) |
| --- | --- |
| Every user-facing response | `caveman` — default full mode; code and precise warnings remain normal/clear. |
| Every Go design or implementation | `golang-pro`, `clean-code`, `go-code-simplification` review. |
| Public API, module layout, ports/adapters | `go-clean-ddd-hexagonal`; apply its dependency rules but do not force service-oriented DDD into this library. |
| Any production behavior change | `tdd-workflows-tdd-cycle`, then strict `tdd-workflows-tdd-red` → `tdd-workflows-tdd-green` → `tdd-workflows-tdd-refactor`. |
| Any test design/change | `golang-testing`. |
| Simplifying or refactoring working Go code | `go-code-simplification` and `clean-code`, preserving observable behavior. |
| Goroutines, fan-out, cancellation, worker limits, shared state, or transport lifecycle | `go-concurrency-patterns` and a `go-debugger-pro` safety review; run race/leak-oriented checks appropriate to the code. |
| Race, leak, deadlock, timeout, profile, flaky concurrent test, or resource-growth investigation | `go-debugger-pro` before proposing a fix. |
| OpenSpec exploration, proposal, implementation, archive | matching `openspec-*` skill. |

### Required Go delivery gate

For each Go task, apply this sequence and leave evidence in the OpenSpec task
or `MEMORY.md`:

1. **Design:** `golang-pro`; also `go-clean-ddd-hexagonal` whenever an API,
   package boundary, port, adapter, or dependency direction is involved.
2. **Specification:** `golang-testing` and TDD RED. Run the new test and show
   it fails for the missing behavior, not because of setup or compilation
   damage.
3. **Implementation:** TDD GREEN plus `clean-code`. Add only code needed by
   the red contract.
4. **Refactor:** TDD REFACTOR plus `go-code-simplification` and `clean-code`.
   Preserve errors, nil/empty values, ordering, public API, context behavior,
   and concurrency semantics exactly.
5. **Concurrency/transport gate:** for every applicable task, use
   `go-concurrency-patterns`, review cancellation/ownership/response closure,
   run `go test -race ./...`, and use `go-debugger-pro` to inspect race, leak,
   deadlock, timeout, or lifecycle risk. Pure deterministic code may mark
   these two skills `N/A` only with a reason.
6. **Acceptance:** `gofmt`, `go vet ./...`, focused tests, full tests, race
   tests where applicable, and coverage against the project thresholds.

Skill scope still matters: do not misuse a debugger skill to write unrelated
new pure code, but do not omit its required diagnostic/safety review when its
scope applies. Do not skip source inspection or tests because a skill suggests
a generic shortcut.

## OpenSpec workflow

1. Run `openspec list --json` and read active change artifacts before work.
2. Use `openspec status --change <name> --json` and its generated
   instructions. Do not guess artifact requirements.
3. Keep implementation tied to a checked task. Update a task checkbox only
   after its focused tests and required verification pass.
4. If source evidence changes architecture, parser semantics, public API,
   transport, or scope, update proposal/design/specs/tasks before coding.
5. Update `MEMORY.md` after a material decision, source-baseline change,
   completed task, blocker, or verification result.

## Module architecture

Use a small library architecture, not skeleton service baggage:

```text
public package ddgs
        ↓
internal/search       orchestration, aggregation, ranking
        ↓
internal/engine       source-specific request and result adapters
        ↓
internal/transport    HTTP, cookies, proxy, TLS/fingerprint
internal/parser       HTML/XPath and JSON extraction
internal/normalize    text, URL, date compatibility
internal/extract      source-compatible content rendering
```

Dependencies point inward. Public package must not expose third-party parser
or HTTP client types. Define small interfaces at consumers, inject them for
tests, and avoid interfaces with one implementation unless they form a real
test or architectural boundary.

`github.com/jcastillo/goddgs` is provisional. Confirm final module path
before publishing; changing it later is a breaking import-path change.

## Non-negotiable parity rules

- No invented fallback engine, payload, selector, header, parser cleanup,
  retry policy, ranking tweak, field coercion, or error swallowing.
- Port every active source engine and retain disabled-engine status exactly.
  A missing, blocked, or incompatible engine remains visible as a blocker; it
  is never silently omitted from an `auto` result.
- Preserve source-specific request sequences: bootstrap requests, cookies,
  tokens, redirects, URL unwraps, random values, paging, safe-search,
  time-limit mapping, and post-processing.
- Preserve result fields and dynamic value types. Python returns dictionaries
  with category-specific fields; do not narrow values to Go strings merely for
  a prettier API. Any typed façade needs a parity-preserving raw form and an
  approved specification.
- Preserve source normalizers: HTML stripping, entity decoding, NFC, Unicode
  category-C removal, whitespace collapse, URL unquoting/space replacement,
  and date lexical output. Test edge cases with golden fixtures.
- Preserve aggregation keys, occurrence-count ordering, Wikipedia priority,
  category-page removal, token ranking, backend selection, provider
  de-duplication timing, and `max_results` behavior. Provider is marked only
  after a completed nonempty source result; do not "improve" it into a
  submit-time reservation without a source-baseline change.
- Read `docs/source-quirks.md` before changing scheduler, options, errors,
  result normalization, parser, or engine code. Frozen bugs and oddities are
  compatibility behavior until explicitly superseded.
- Read `docs/engine-contracts.md` before creating or changing an engine
  adapter. It records frozen request, selector, and post-processing behavior;
  differential fixtures remain the acceptance evidence.
- Do not make a network result look deterministic. External engines change.
  Fixture/contract tests prove semantics; live tests prove current
  connectivity only.

## Transport and concurrency

`primp` impersonation and the temporary `httpx`/`h2` client are core source
behavior, not replaceable with a casual `net/http.Client`. Browser TLS and
HTTP/2 fingerprint compatibility is an explicit acceptance gate for each
affected engine. No claim of 1:1 parity until it has evidence.

- Support HTTP(S) and SOCKS proxy behavior, `DDGS_PROXY`, `tb` alias,
  timeout, TLS verification off, and custom PEM roots according to source
  contracts.
- Close every response body. Keep cookie and per-engine state isolated.
- Use `context.Context` as first parameter at Go I/O boundaries. Cancellation
  must stop work and leave no goroutine blocked.
- Never mutate shared engine request state while concurrent calls may read it.
  Source engines mutate URLs, language, cookies, and temporary HTTP/2 state;
  design Go request state as per-call or properly synchronized without
  changing externally observable behavior.
- Run `go test -race ./...` for all fan-out/transport changes. Add leak checks
  for packages that start goroutines.
- Do not use cgo or a fingerprinting dependency without a design decision,
  license review, reproducibility check, and tests against affected engines.

## Test discipline

TDD is mandatory for implementation work: RED → GREEN → REFACTOR. Tests
describe source behavior, not a new implementation's internals.

- Unit, parser, normalization, request-payload, and aggregation tests are
  deterministic and offline.
- Table tests have named subtests. Independent tests use `t.Parallel()` only
  when fixtures/state are isolated.
- Networked tests have `//go:build integration`; opt in via
  `make integration`. Serialize/rate-limit live engine tests so they do not
  trigger bans.
- Build a Python-vs-Go differential fixture harness before declaring an
  engine complete. Capture request sequence, method, query/body, relevant
  headers/cookies, response fixture, normalized result, and error shape.
- Critical parity paths need 100% behavior coverage. General changed code
  targets at least 80% line coverage and 75% branch coverage; coverage never
  excuses absent contract cases.
- Before handoff run `make verify`; run `make integration` only when network
  conditions and rate limits permit. Report skipped live checks explicitly.

## Dependency and security policy

- Prefer standard library only where it demonstrably preserves source
  behavior. Simplicity is not a reason to lose engine compatibility.
- Before adding a module: document purpose, license, version pin, update
  strategy, supply-chain risk, and why existing code cannot provide parity.
- Never commit credentials, proxy URLs with credentials, captured private
  content, cookies, or generated local environments.
- Treat `verify=false` as a caller-controlled compatibility feature; never
  make it a default or broaden its effect.
- Keep upstream MIT notice and attribution. Preserve third-party notices for
  copied or adapted code.

## Quality and handoff

- Format with `gofmt`; run `go vet ./...`, `go test ./...`, and
  `go test -race ./...`.
- Use errors that callers can classify with `errors.Is`/`errors.As`; do not
  log and return the same error from library code.
- Public exports require Godoc and a compatibility rationale. Do not change
  exported API without an OpenSpec requirement and migration note.
- Keep functions focused; preserve behavior before applying clean-code
  refactors. No globals with mutable request, random, clock, or transport
  state unless synchronization and source semantics demand them.
- At session end, update `MEMORY.md` with progress, exact commands/results,
  remaining risks, and next concrete task.
