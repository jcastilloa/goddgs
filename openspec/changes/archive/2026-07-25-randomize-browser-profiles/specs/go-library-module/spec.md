## MODIFIED Requirements

### Requirement: Practical browser-profile transport

HTTPS target requests made through the base source-client transport SHALL use
the reviewed browser-profile capability rather than the standard Go TLS and
HTTP/2 fingerprint. The client SHALL select one coherent frozen-source browser
profile per transport client from the `primp` browser-version and operating-
system outcome space. The selected identity SHALL remain stable for that
client's lifetime and bundle compatible headers, TLS and HTTP/2 behaviour. The
complete selection and route contract is defined by
`browser-profile-randomization`.

#### Scenario: Browser-profile HTTPS request is opened
- **WHEN** a source engine performs a direct HTTPS request through its isolated base client
- **THEN** the client SHALL select one coherent frozen `primp` browser/OS
  bundle and send that bundle's TLS semantics, headers, HTTP/2 SETTINGS order,
  connection window, and pseudo-header order

#### Scenario: Eligible client retains one selected browser identity
- **WHEN** an HTTPS-capable client makes multiple requests
- **THEN** all requests SHALL retain the browser-version and operating-system
  identity selected when that client was constructed

#### Scenario: Browser-profile connection is no longer needed
- **WHEN** `CloseIdleConnections` is called on the owning transport client
- **THEN** cached browser-profile connections SHALL be closed without affecting another engine client

#### Scenario: Tunnel does not split browser identity
- **WHEN** a caller configures a proxy, custom TLS root or disabled TLS verification
- **THEN** the target SHALL receive the selected profile while that path's
  proxy and verification contract is retained

#### Scenario: DuckDuckGo temporary fingerprint is considered
- **WHEN** a caller uses DuckDuckGo text's temporary HTTP/2 client
- **THEN** the module SHALL document that it is a distinct source transport and
  does not inherit the base-client browser-profile claim
