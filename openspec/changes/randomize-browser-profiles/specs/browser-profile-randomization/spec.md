## ADDED Requirements

### Requirement: Source-shaped browser and OS selection
For every transport client that can issue an HTTPS request, the module SHALL independently
select one of the 23 frozen `primp` browser-version variants and one of the
five frozen operating-system variants exactly once when that transport client
is constructed.  The selected identity SHALL remain stable for all of that
client's requests and connection reuse.

The deterministic and production selectors SHALL preserve the Python binding's
selection chronology: operating-system choice first, then browser-version
choice.

#### Scenario: Deterministic private selection chooses one complete profile
- **WHEN** a test injects a browser-version index and an operating-system
  index while constructing an eligible client
- **THEN** every request from that client SHALL use the headers, TLS profile
  and HTTP/2 shape belonging to that exact source pair.

#### Scenario: Source selection chronology is retained
- **WHEN** a test records the deterministic selector limits during client
  construction
- **THEN** it SHALL observe the operating-system outcome space (`5`) before
  the browser-version outcome space (`23`).

#### Scenario: Separate clients may receive separate identities
- **WHEN** two eligible clients are constructed with distinct injected source
  selections
- **THEN** each client SHALL retain its own selected identity without shared
  mutable profile state.

#### Scenario: Requests do not rotate an established identity
- **WHEN** an eligible client performs multiple requests to one origin or to
  different origins
- **THEN** its selected browser-version and operating-system identity SHALL
  remain unchanged for its lifetime.

### Requirement: Complete source profile bundles
Every selectable source browser-version and operating-system pair SHALL map to
one immutable bundle containing mutually compatible default headers, TLS
ClientHello semantics, ALPN, HTTP/2 settings/order, pseudo-header order,
regular-header order, priority settings and initial stream ID.

#### Scenario: Captured profile bundle matches its source fixture
- **WHEN** a source profile/OS bundle is selected in a loopback TLS and HTTP/2
  wire test
- **THEN** its observable stable fields SHALL match the frozen `primp`
  fixture, excluding only entropy-bearing TLS random, key-share and GREASE
  payload bytes.

#### Scenario: Entropy-bearing handshake values remain fresh
- **WHEN** an eligible profile opens two independent TLS connections
- **THEN** the client SHALL preserve the profile's captured structural
  semantics while allowing TLS random, key shares and GREASE values to vary
  per handshake.

### Requirement: Source selection distribution is preserved
The production selector SHALL preserve the frozen source's independent uniform
selection over browser-version variants and operating-system variants.  It
MUST NOT weight variants by browser family, collapse coincident output bundles
or rotate identity on each request.

#### Scenario: Enumerated selector space contains all frozen outcomes
- **WHEN** the selector's test-only deterministic outcome space is enumerated
- **THEN** it SHALL contain 23 browser-version choices and five operating-
  system choices with no omitted source variant.

### Requirement: HTTPS target routes preserve a complete profile
The module SHALL apply the selected complete browser profile to every HTTPS
search origin reached through direct HTTPS, custom-root, disabled-verification,
HTTP(S) CONNECT proxy and SOCKS5/SOCKS5H target routes. Plain HTTP SHALL
retain the tested standard transport and SHALL NOT receive browser default
headers without a matching TLS and HTTP/2 bundle.

#### Scenario: Proxy tunnel carries the browser identity to the target
- **WHEN** a client reaches an HTTPS target through an HTTP(S) CONNECT or
  SOCKS proxy
- **THEN** the target observes the selected TLS ClientHello, HTTP/2 shape and
  default headers after the tunnel is established.

#### Scenario: Custom TLS route retains browser identity
- **WHEN** a client uses a PEM root or disabled verification for an HTTPS
  target
- **THEN** the target observes the selected browser bundle while the caller's
  TLS verification policy remains in force.

### Requirement: Profile capture data is reproducible and safe
Generated browser-profile compatibility data SHALL state its frozen source
baseline, generator, checksum and source dependency provenance.  It SHALL be
created with loopback-only capture and SHALL NOT contain cookies, credentials,
external page content or proxy URLs.

#### Scenario: Profile data integrity is verified offline
- **WHEN** the offline fixture and generated-data verification runs
- **THEN** it SHALL reject data whose checksum, profile count or source
  provenance differs from the frozen expected values.
