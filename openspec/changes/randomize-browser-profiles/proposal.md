# Proposal: Randomize browser impersonation profiles

## Why

The frozen Python client constructs `primp.Client` with
`impersonate="random"` and `impersonate_os="random"`.  It therefore chooses
one coherent browser identity when the client is created, rather than using a
single fixed Chrome profile for all clients.  A fixed profile leaves a
detectable behavioural gap in a library whose transport is part of the search
engine contract.

## What Changes

- Capture differential transport fixtures for every frozen `primp` browser
  variant and operating-system selection, including the resulting TLS
  ClientHello semantics, HTTP/2 parameters/order, and default headers.
- Replace the fixed direct-HTTPS browser profile with a source-shaped selector:
  select operating system first and browser variant second once per transport client,
  retain that identity for its lifetime, and reuse its connections normally.
- Implement every captured profile through reviewed, pure-Go TLS and HTTP/2
  specifications.  A profile is not eligible for selection until its
  ClientHello, HTTP/2 wire shape, and headers have differential evidence.
- Carry the selected target-facing profile through direct TLS, custom PEM,
  disabled-verification, HTTP(S) CONNECT and SOCKS tunnels. The profile is
  applied after a proxy tunnel is established, so the search origin receives
  the same coherent TLS/HTTP/2 identity. HTTP remains standard HTTP because it
  has no TLS or HTTP/2 browser bundle to accompany browser headers.
- Add deterministic test injection for profile selection without exposing a
  new public option or rotating identity per request.

## Non-goals

- No public API, service, CLI, configuration endpoint, or runtime profile
  download.
- No live search-engine test traffic as acceptance evidence.
- No claim that browser profile randomization makes request outcomes
  deterministic or bypasses every bot-control system.

## Impact

- Affects `internal/transport` and its private composition boundary only.
- Adds generated compatibility data derived from local, loopback-only `primp`
  observations.  It must have provenance, checksum, license review and
  regeneration instructions.
- Changes the browser transport design and the fingerprint acceptance gate.

## Capabilities

### New Capabilities

- `browser-profile-randomization`: source-shaped per-client selection and
  reuse of browser impersonation profiles.
- `browser-profile-parity-verification`: offline differential fixtures and
  wire tests for all selectable profiles.

### Modified Capabilities

- `go-library-module`: direct HTTPS transport no longer uses one fixed
  browser profile; it uses a captured source-compatible profile selected once
  per client.
