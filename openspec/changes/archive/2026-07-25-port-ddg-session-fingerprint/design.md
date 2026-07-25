## Context

`DuckDuckGoTextClient` currently uses an isolated standard-library HTTP/2
transport. It correctly preserves its request shape, redirect policy, headers,
cookie jar, cancellation, error classification, and response lifecycle, but it
cannot emit the frozen Python `HttpClient2` TLS or HTTP/2 initialization.

Frozen source creates `HttpClient2` while constructing `Duckduckgo`; `DDGS`
caches that engine per public client. When verification is enabled, the
constructor samples the TLS cipher order and one of four SSL policies once. A
request temporarily installs a global `httpcore` callback, but the callback's
seven HTTP/2 settings are sampled only when a new HTTP/2 connection is
initialized. Reusing a connection emits no new settings. With `verify=False`,
source bypasses `_get_random_ssl_context`, while retaining the HTTP/2
connection initialization behavior.

The local source checkout and exact reference environment are authoritative:
source commit `a12929a72429a39a0841c3d7caacb20ee17acd4d`, CPython 3.12.3,
`httpx` 0.28.1, `httpcore` 1.0.9, and `h2` 4.3.0. Parser/XPath, result shape,
engine scheduling, and extraction rendering do not participate in this
transport-only change.

## Goals / Non-Goals

**Goals:**

- Give each constructed DuckDuckGo text transport one immutable,
  source-shaped TLS session profile; do not select it per request.
- Create an independent, source-ranged HTTP/2 initialization profile for each
  new connection, including after a failed connection or
  `CloseIdleConnections`.
- Preserve all existing DuckDuckGo text request, proxy, verification,
  redirect, cancellation, error, cookie, header, and response-closure
  contracts.
- Prove lifetime boundaries through sanitized Python-vs-Go loopback fixtures,
  local TLS/H2 wire tests, concurrency/race checks, and an opt-in controlled
  fingerprint diagnostic.

**Non-Goals:**

- No public API, engine adapter, parser, result, scheduler, renderer, or
  base-`primp` browser-profile modification.
- No source-style global monkey-patch, cgo, service/CLI, or live search-engine
  request.
- No claim that entropy-bearing TLS bytes are identical across languages or
  connections.

## Decisions

### Keep the DuckDuckGo transport a private adapter

`DuckDuckGoTextClient` remains the sole consumer-facing transport port. Its
private round tripper receives immutable client settings and owns its origin
connections. The public `DDGS` cache already constructs one adapter/client per
selected engine in source order, which matches the source lifetime; no public
option or generic fingerprint abstraction is added.

Alternative: add a configurable public impersonation API. Rejected: frozen
Python exposes no such API and it would let callers create unsupported
cross-engine behavior.

### Model TLS at client construction and HTTP/2 at connection creation

An internal `ddgSessionProfile` is sampled once at DuckDuckGo client
construction. For enabled verification it retains the source cipher ordering
and one selected SSL policy: default, TLS-1.2 maximum, TLS-1.3 minimum, or
no-session-ticket. For disabled verification it retains the captured
non-random source branch. A fresh uTLS specification is built from that
immutable profile for each connection, so key shares, random bytes, and SNI
are connection-local while policy/cipher selection stays session-local.

An internal profile factory samples the seven source HTTP/2 settings when a
connection is actually initialized: header-table size, enable-push, initial
window, max-frame size, enable-connect protocol, max-concurrent streams, and
max-header-list size; it also emits the source `2**24` connection window
increment. The factory is not called for an already reusable connection.

Alternative: sample all values in `Do`. Rejected: it mis-models reuse and
would claim a request lifetime that source does not have. Alternative:
recreate Python's global patch. Rejected: it races across clients and violates
Go transport isolation.

### Extend existing private wire primitives, not dependencies

`github.com/sardanioss/utls v1.10.3` is already approved, pure Go, and
isolated in `internal/transport`. Extend its private raw-ClientHello and
HTTP/2 framing path so a profile factory can produce one H2 wire profile per
connection. Keep the base browser-profile path behavior unchanged. Reuse the
existing target dialer for direct, HTTP(S) CONNECT, and SOCKS target routes;
certificate/PEM policy remains caller-controlled.

Alternative: standard `net/http.Transport`. Rejected: it cannot control the
source TLS and H2 initialization. Alternative: a new TLS client module.
Rejected unless existing uTLS proves insufficient: it broadens supply chain
and requires a separate license/cgo/reproducibility decision.

### Capture source templates and ranges, never production traffic

Commit a compact, checksum-verified asset generated only against an ephemeral
loopback endpoint using the exact reference environment. It records source
ClientHello templates for each verification/TLS-policy branch and the ordered
H2 setting schema/ranges, not credentials, cookies, search requests, or
responses. Tests inject deterministic randomness; production uses
`crypto/rand` and preserves source ranges/selection chronology.

Controlled endpoint observations remain opt-in integration tests. They record
only protocol version and sanitized fingerprint hashes, never a full response.

## Risks / Trade-offs

- [uTLS template interpretation differs from OpenSSL] → parse a fresh template
  per connection; assert ClientHello semantics, ALPN, settings, window, frame
  order, header order, reuse, and reconnect behavior against local captures.
- [Global mutable state leaks between concurrent requests] → profile values are
  immutable after construction; origin/connection ownership is lock-protected;
  tests run repeated `-race` stress and cancellation paths.
- [Reference environment drift] → capture tool checks frozen SHA and exact
  CPython/dependency versions before accepting regenerated assets.
- [Connection reuse hides a bad lifetime boundary] → local tests force reuse,
  close-idle reconnect, failed initial dial retry, and distinct concurrent
  clients with deterministic random seams.
- [Scope expansion changes HTTP semantics] → retain existing transport
  fixtures unchanged; request payload, result parsing, scheduler, parser, and
  extraction are explicit N/A gates for this change.

## Migration Plan

1. Add source-loopback capture and deterministic RED fixtures before changing
   production transport code.
2. Introduce the private session/connection profiles behind
   `DuckDuckGoTextClient`; retain its exported constructor and methods.
3. Run focused wire/lifecycle/race tests, full verification, and opt-in
   diagnostic only with explicit endpoint authorization.
4. Roll back by removing the private round tripper and asset; the public API
   and existing fixtures require no caller migration.

## Open Questions

None. The frozen source lifecycle, disabled-verification branch, and exact
reference environment are now captured. Any new TLS dependency or a mismatch
in controlled evidence reopens this design rather than changing behavior
silently.
