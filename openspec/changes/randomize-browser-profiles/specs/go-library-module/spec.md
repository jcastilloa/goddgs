## MODIFIED Requirements

### Requirement: Importable Go module and supported runtime
The repository SHALL provide the Go module `github.com/jcastilloa/goddgs` and
an importable root package `ddgs`, compatible with the Go version declared in
`go.mod`. It SHALL expose a context-first public library API for the DDGS
operations, rather than an HTTP service, CLI, or executable API server. Its
HTTPS target transport SHALL select one coherent frozen-source browser profile
per transport client from the `primp` random browser-version and operating-
system outcome space. The selected identity SHALL remain stable for that
client's lifetime and SHALL bundle compatible headers, TLS and HTTP/2
behaviour. The same selected target bundle SHALL traverse direct, custom-root,
disabled-verification and HTTP(S)/SOCKS tunnel routes; plain HTTP retains its
separately tested standard transport semantics.

#### Scenario: Consumer imports the module
- **WHEN** a Go program imports `github.com/jcastilloa/goddgs`
- **THEN** it can construct and use the public DDGS library API without
starting a server or depending on an executable command.

#### Scenario: Eligible client retains one selected browser identity
- **WHEN** an HTTPS-capable client makes multiple requests
- **THEN** all requests SHALL retain the browser-version and operating-system
identity selected when that client was constructed.

#### Scenario: Tunnel does not split browser identity
- **WHEN** a caller configures a proxy, custom TLS root or disabled TLS
verification
- **THEN** the target SHALL receive the selected profile while that path's
  proxy and verification contract is retained.
