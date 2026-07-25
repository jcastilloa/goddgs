## 1. Source evidence and test design

- [x] 1.1 Validate local frozen source and exact reference environment; extend the sanitized loopback capture to record deterministic DDG TLS-policy templates, ordered HTTP/2 settings/ranges, header order, and session-versus-connection lifetime evidence.
- [x] 1.2 Document uTLS extension scope, license/cgo/supply-chain review, template provenance/checksum, and explicit no-live-search boundary before implementation.
- [x] 1.3 RED: add deterministic named unit and local TLS/H2 wire tests for TLS policy sampled once per client, disabled-verification source branch, seven ordered H2 settings/ranges per new connection, window increment, reuse, reconnect, cancellation, and isolated concurrent clients; run and record focused failures caused by missing behavior.

## 2. Minimal source-shaped transport

- [x] 2.1 GREEN: add private immutable DDG TLS session-profile selection with crypto-safe production randomness and deterministic test seams; instantiate fresh connection-local uTLS state without exposing a new public API.
- [x] 2.2 GREEN: add private DDG HTTP/2 connection-profile creation and isolated target routing across direct, PEM, disabled-verification, HTTP(S) CONNECT, and SOCKS paths; retain redirects, cookies, headers, errors, cancellation, and body closure.
- [x] 2.3 GREEN: generalize only necessary private HTTP/2 wire lifecycle ownership so profiles are created per connection while the existing 23×5 base browser-profile behavior stays unchanged; make all RED tests green.
- [x] 2.4 Prove deterministic frozen request/error/redirect fixtures and source engine adapter fixtures remain green without changing public API, result values, payload ordering, or scheduler behavior.

## 3. Refactor and documentation

- [x] 3.1 REFACTOR: apply clean-code and Go simplification review to touched transport code; preserve error identity, nil/empty behavior, context cancellation, response closure, ordering, public API, and browser-profile transport semantics.
- [x] 3.2 Update `docs/fingerprint-gate.md`, `docs/dependency-decisions.md`, `docs/source-quirks.md`, source contracts, and `MEMORY.md` with exact lifecycle evidence, remaining entropy limits, N/A parser/result/extract gates, and source checkout/reference-environment provenance.

## 4. Acceptance

- [x] 4.1 Run formatter, diff check, focused transport tests/coverage, full tests, `go vet ./...`, `go test -race ./...`, CGO-free tests, source-capture verification, `make verify`, and strict OpenSpec validation; investigate any failure before completing a task.
- [x] 4.2 Run repeated cancellation/reuse/reconnect/concurrent-client race and leak-oriented checks; record Go concurrency/debugger safety review.
- [x] 4.3 Compile and opt-out smoke tagged fingerprint tests. Run controlled endpoint observation only with explicit endpoint authorization; never run a live search-engine request.
