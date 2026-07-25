## 1. Frozen-source evidence

- [x] 1.1 Extend the loopback-only Python reference harness to capture all 23 explicit `primp` browser variants across all five source OS choices, including stable TLS, header and HTTP/2 semantics. Evidence: `tools/capture_browser_profiles.py` captures 115 loopback-only pairs from frozen `primp 1.3.1`; `--verify-http-connect` passes every pair through a local CONNECT target without external engine traffic.
- [x] 1.2 Add integrity/provenance checks for generated profile data and record its baseline, counts and checksums in project documentation. Evidence: embedded asset SHA-256, frozen commit, 23×5 cardinality and every pair are checked by `browser_profiles.go`; provenance, regeneration and checksum are in `docs/browser-profiles.md`.
- [x] 1.3 Prove that every captured TLS profile can be instantiated with the reviewed pure-Go uTLS custom-spec adapter; record and resolve unsupported extension cases before selection is enabled. Evidence: all 115 profiles pass fresh-entropy local ClientHello construction and full local TLS/H2 wire tests; Safari padding preservation is fixture-tested.

## 2. Profile model and selection

- [x] 2.1 RED: add deterministic private selector tests for the complete 23-by-5 source outcome space, per-client stability and client isolation. Evidence: RED exposed the prior fixed Chrome/incorrect draw order; `browser_profiles_test.go` now checks every pair, Python binding chronology (OS `5`, then variant `23`), invalid draws, stability and isolated clients.
- [x] 2.2 GREEN: implement immutable profile bundles and private source-shaped random selection without exported configuration or shared mutable random state. Evidence: private crypto-random selection constructs one catalog clone per eligible client; no profile or random selector crosses the public API.
- [x] 2.3 REFACTOR: simplify profile data lookup while preserving source probability, dynamic TLS entropy and immutable ownership. Evidence: one immutable catalog/clone path serves direct and tunnel transports; focused profile tests and `-race -count=10` pass.

## 3. Coherent browser transport bundles

- [x] 3.1 RED: add local TLS/H2 wire tests for every selected browser family/version shape, headers, regular-header order, pseudo-header order, SETTINGS, windows, priority and stream-ID behaviour. Evidence: all 115 source pairs run through local TLS/H2 capture; tests assert ClientHello semantics, preface, SETTINGS/order, windows, priority, stream ID, pseudo order and header order.
- [x] 3.2 GREEN: construct the captured source ClientHello semantics using uTLS custom specifications and bind each to its matching HTTP/2/header bundle. Evidence: `browserRoundTripper` parses captured ClientHello templates with uTLS and uses the matching profile H2/header bundle; no independent UA/header selection exists.
- [x] 3.3 GREEN: preserve profile selection across connection reuse, failed initial dials, cancellation and concurrent requests without poisoning an origin cache. Evidence: local tests cover reuse, 96 KiB flow control, cancelled handshake retry, concurrent origin construction and `CloseIdleConnections`; focused race stress passes.
- [x] 3.4 REFACTOR: remove the fixed Chrome-only profile path and consolidate shared profile/wire code without changing captured behaviour. Evidence: the old fixed path is removed; one profile-driven wire path handles every eligible HTTPS target route.

## 4. Boundary behaviour and documentation

- [x] 4.1 RED/GREEN: retain standard HTTP without a browser header-only identity; prove custom PEM, disabled verification, HTTP(S) CONNECT and SOCKS5/SOCKS5H tunnels deliver the selected full profile to HTTPS targets. Evidence: route tests cover plain HTTP standard behavior plus PEM, verify-off, HTTP/HTTPS CONNECT and SOCKS5/SOCKS5H target-side browser wire; all 115 source pairs pass HTTP CONNECT loopback.
- [x] 4.2 Document profile coverage, generated-data provenance, known entropy limits, dependency/license review and diagnostic evidence without claiming byte-identical handshakes. Evidence: `docs/browser-profiles.md`, `docs/fingerprint-gate.md`, `docs/dependency-decisions.md` and `NOTICE.md` document the data, BSD/MIT/Apache notices, fresh TLS entropy and controlled diagnostics.
- [x] 4.3 Update `MEMORY.md`, AGENTS/OpenSpec references and transport acceptance documentation with the completed selection contract. Evidence: project rules now prohibit standalone header/UA rotation and record source OS-then-browser selection chronology.

## 5. Acceptance

- [x] 5.1 Run formatter, `go vet ./...`, focused profile tests, full tests, `go test -race ./...`, CGO-free tests, fixture verification and OpenSpec validation. Evidence: 2026-07-25 acceptance passed `gofmt`, diff check, vet, full unit/race/CGO-off suites, integration-tag compile/opt-out smoke, Python fixture/profile verification, `make verify`, and strict validation of both changes; `internal/transport` coverage is 80.2%.
- [x] 5.2 Run the controlled fingerprint diagnostic only (not a search engine), record profile-family observations and verify no live engine test is required for acceptance. Evidence: tagged endpoint-only test observed matching explicit Python/Go Chrome 146/Windows, Edge 148/Linux, Opera 131/Android, Safari 26.3/macOS and Firefox 148/iOS H2/JA3/JA4/Akamai values. No search engine request was made.
