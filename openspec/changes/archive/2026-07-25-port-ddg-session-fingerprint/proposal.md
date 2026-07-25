## Why

Frozen DDGS `HttpClient2` gives DuckDuckGo text a transport identity that
differs from the completed base `primp.HttpClient` profile catalog. It selects
TLS cipher/version/options once when the cached DuckDuckGo engine constructs
its client, and initializes randomized HTTP/2 settings for each newly opened
HTTP/2 connection. The current Go client preserves request behavior but still
uses the standard Go TLS and HTTP/2 fingerprint.

This change closes that remaining DuckDuckGo text transport parity gap without
changing its public search API, request payloads, parsing, or result behavior.
Source baseline remains `a12929a72429a39a0841c3d7caacb20ee17acd4d`.

## What Changes

- Add an isolated DuckDuckGo text transport with one randomized TLS policy per
  constructed client/session and randomized source-shaped HTTP/2 settings per
  newly initialized connection.
- Preserve `HttpClient2` observable behavior: HTTP/2, redirects disabled,
  proxy/timeout/verification support, isolated cookie and header state,
  context cancellation, response-body closure, and classifiable errors.
- Prohibit source-style package-global HTTP/2 monkey-patching; concurrent
  DuckDuckGo clients and requests must not share mutable TLS, HTTP/2, cookie,
  header, or connection-init state.
- Capture sanitized, deterministic differential evidence and local wire tests
  for the source lifetime boundaries. No search-engine request is required for
  acceptance.

## Capabilities

### New Capabilities

- `duckduckgo-text-session-transport`: source-compatible isolated TLS-session
  and HTTP/2-connection behavior for the DuckDuckGo text adapter.
- `duckduckgo-text-fingerprint-verification`: offline and controlled endpoint
  evidence that verifies the DuckDuckGo text transport without claiming raw
  entropy-byte equality.

### Modified Capabilities

<!-- None. No existing OpenSpec capability specification defines this behavior. -->

## Impact

- Affected code: `internal/transport/duckduckgo_text.go`, its private
  transport implementation/tests, public-client composition only if required
  to preserve per-`DDGS` engine caching.
- Public API: no exported signature or result-shape change.
- Dependencies: any addition requires documented purpose, version, license,
  cgo status, supply-chain review, reproducibility evidence, and source-wire
  parity justification.
- Exclusions: other engines, HTML parsing, scheduler behavior, rendering, and
  live DuckDuckGo searches remain outside this change.
