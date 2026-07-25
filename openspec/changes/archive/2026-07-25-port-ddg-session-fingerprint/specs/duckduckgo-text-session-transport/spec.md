## ADDED Requirements

### Requirement: DuckDuckGo text TLS profile follows client lifetime
The system SHALL construct one isolated DuckDuckGo text TLS profile for each
DuckDuckGo text client. When source verification is enabled, that profile SHALL
sample source-compatible cipher ordering and exactly one source SSL-policy
branch at construction, retain it for all HTTPS connection attempts made by
that client, and not share it with another client. When verification is
disabled, it SHALL preserve source's branch that bypasses randomized SSL
context construction. No public API SHALL expose the profile.

#### Scenario: Reused client creates multiple HTTPS connections
- **WHEN** a DuckDuckGo text client opens a connection, loses it, and opens a
  replacement connection
- **THEN** both ClientHello profiles retain the client's selected TLS policy
  and cipher ordering while connection-local entropy remains fresh

#### Scenario: Separate DuckDuckGo clients run concurrently
- **WHEN** two isolated DuckDuckGo text clients execute concurrent requests
- **THEN** headers, cookies, TLS profiles, connections, and initialization
  state remain client-local without a data race or package-global mutation

#### Scenario: Disabled verification uses source branch
- **WHEN** a caller requests source-compatible disabled certificate
  verification
- **THEN** the transport skips the source randomized SSL-context branch while
  retaining HTTP/2, request, proxy, cancellation, and response-lifecycle
  behavior

### Requirement: DuckDuckGo text HTTP/2 settings follow connection lifetime
The system SHALL sample the frozen source's seven HTTP/2 settings only when a
new DuckDuckGo text HTTP/2 connection initializes. It SHALL send the settings
in source order and ranges followed by the `2**24` connection window increment.
It SHALL not resample or send an initialization sequence while reusing an open
connection.

#### Scenario: First HTTP/2 connection initializes
- **WHEN** a DuckDuckGo text request negotiates HTTP/2 on a new connection
- **THEN** local wire capture observes source-ranged header-table,
  enable-push, initial-window, max-frame, enable-connect,
  max-concurrent-streams, and max-header-list settings in source order and the
  source connection window increment

#### Scenario: Existing HTTP/2 connection is reused
- **WHEN** two sequential requests use a reusable DuckDuckGo text HTTP/2
  connection
- **THEN** only the first request's connection initialization emits settings
  and the second request uses the existing connection

#### Scenario: Close idle forces a new initialization
- **WHEN** `CloseIdleConnections` releases a DuckDuckGo text connection before
  another request
- **THEN** the later request creates a new connection and samples a new
  source-ranged HTTP/2 initialization profile without changing its TLS session
  profile

### Requirement: Existing DuckDuckGo text transport behavior remains stable
The system SHALL retain frozen DuckDuckGo text request construction, HTTP/2
negotiation, disabled redirects, proxy/timeout/PEM/verification behavior,
header and cookie isolation, context cancellation, response materialization
and closure, and error classification.

#### Scenario: Existing deterministic transport fixture executes
- **WHEN** a frozen DuckDuckGo text constructor, request, redirect, error, or
  cancellation fixture runs through the upgraded transport
- **THEN** its source-visible request, response, and error contract remains
  unchanged
