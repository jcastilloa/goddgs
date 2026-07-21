# Dependency decisions

Each entry records purpose, pin, license, supply-chain assessment, and update
rule before a Go module enters `go.mod`.

## `golang.org/x/text` `v0.40.0` — approved 2026-07-20

| Item | Decision |
| --- | --- |
| Purpose | NFC normalization and Python-compatible UTF-8 replacement during URL percent decoding in `internal/normalize`. Go standard library has no Unicode normalization API. |
| Imported packages | `golang.org/x/text/unicode/norm` for NFC; `golang.org/x/text/encoding/unicode` and `golang.org/x/text/transform` for Python-compatible UTF-8 replacement during URL percent decoding. |
| Version | `v0.40.0`; checksum and module checksum are retained in `go.sum`. |
| Compatibility | Module declares Go `1.25`; project uses Go `1.26.1`. Its `norm` table is Unicode `15.0.0` under `!go1.27`; frozen Python 3.12 `unicodedata.unidata_version` is `15.0.0`. Version `v0.40.0` selects Unicode 17 under Go 1.27, so the existing build guard intentionally rejects Go 1.27+ pending rebaseline. |
| License | BSD 3-Clause, verified from cached module `LICENSE`. Compatible with project/upstream MIT distribution; retain upstream module metadata through Go modules. |
| cgo | None. Pure Go Unicode tables. |
| Maintenance/provenance | Official Go subrepository, source origin `https://go.googlesource.com/text`, tag `v0.40.0`, module checksum verified through Go module tooling. |
| Supply-chain risk | Extra Unicode-table code only; no network, filesystem, transport, or runtime initialization behavior. Pin exact version and retain `go.sum`; review release/security advisories before any update. |
| Why not standard library | `unicode` classifies runes but does not implement NFC. Omitting NFC contradicts frozen fixture `pure.normalize-text-nfc`. |
| Update rule | Do not update casually. Re-run all normalizer differentials and inspect Go build tags/table versions; confirm active table remains Python baseline-compatible or update source baseline/OpenSpec explicitly. |

Evidence gathered locally on 2026-07-20: Go `1.26.1`; frozen Python
`3.12.3`; `norm.Version == "15.0.0"` for Go 1.26 build path; Python Unicode
database `15.0.0`.

## `github.com/lestrrat-go/helium` `v0.6.0` — approved 2026-07-20

| Item | Decision |
| --- | --- |
| Purpose | Internal tolerant HTML parsing and XPath 1.0 evaluation for frozen `lxml` source selectors. Only `helium`, `helium/html`, and `helium/xpath1` are used; no third-party type crosses `internal/parser`. |
| Version | `v0.6.0`, source tag hash `5fbefa470739ec67353927cb9cb41033c0250530`; checksums retained in `go.sum`. |
| Compatibility | Module requires Go `1.26.1`; project directive is raised to that exact minimum. It requires `golang.org/x/text v0.40.0`, whose Go 1.26 path remains Unicode 15.0.0 under the existing Go 1.27 build guard. |
| License | MIT, verified from cached module `LICENSE`, copyright (c) 2015 lestrrat. Notice recorded in `NOTICE.md`. |
| cgo | Core imported packages are pure Go. Only optional benchmark files carry `cgo && libxml2bench` tags; project neither enables that tag nor imports those packages. `CGO_ENABLED=0` parser verification is required. |
| Maintenance/provenance | Module metadata resolves tag `v0.6.0` on `https://github.com/lestrrat-go/helium`; Go module tooling records its checksum. It is a broad XML toolkit, so update only as a reviewed parser change. |
| Supply-chain risk | Broad module brings indirect `github.com/dlclark/regexp2`; parser adapter imports only HTML/XPath 1 packages. Default parser policy blocks external network/filesystem resources; adapter must not enable loaders, DTDs, or XInclude. |
| Why selected | Isolated Go probe matched all 14 frozen lxml parser fixtures, including Yahoo News XPath-union document order and malformed Startpage recovery; probe plus `helium/html` and `helium/xpath1` upstream tests passed under `-race`. |
| Rejected alternatives | `github.com/antchfx/htmlquery v1.3.6` matched 12/14: Yahoo News union returned `primary.jpg` instead of lxml `fallback.jpg`, and malformed Startpage recovery consumed `Body` into title. `github.com/lestrrat-go/libxml2` was not selected because it needs cgo/system libxml2, violating the current pure-Go gate. |
| Update rule | Re-run all parser fixtures, `CGO_ENABLED=0` parser tests, normalizer differentials, and race checks. Reassess XPath recovery, Helium release provenance, indirect dependencies, and Unicode tables before changing either Helium or `x/text`. |

Evidence gathered locally on 2026-07-20: isolated probe against all 14 parser
fixtures passed with normal and `-race` execution; `go test` and
`go test -race` for `helium/html` and `helium/xpath1` passed while pinned to
the project Unicode baseline.

## `golang.org/x/net` `v0.57.0` — approved for SOCKS only, 2026-07-21

| Item | Decision |
| --- | --- |
| Purpose | Build the base transport's SOCKS5 dial path without leaking a third-party type. Frozen loopback fixtures distinguish `socks5` local DNS (`127.0.0.1`) from `socks5h` remote DNS (`localhost`); standard `net/http.Transport` explicitly treats both schemes alike. |
| Imported package | `golang.org/x/net/proxy` only. The adapter resolves an IPv4 address before passing `socks5` to its dialer; it passes the original hostname for `socks5h`. |
| Version | `v0.57.0`; exact module checksum retained through Go module tooling. |
| Compatibility | Module declares Go `1.25.0`; project uses Go `1.26.1`. |
| License | BSD 3-Clause, verified from cached module `LICENSE`; compatible with project/upstream MIT distribution. |
| cgo | None in the imported proxy path. |
| Maintenance/provenance | Official Go subrepository, tag `v0.57.0`, origin `https://go.googlesource.com/net`, module tooling reports tag hash `b8f09f6f062ceb4531b7af4bd17a5c8fe9c4b2b5`. |
| Supply-chain risk | Adds `x/crypto`, `x/sys`, and `x/term` module dependencies declared by `x/net`; runtime transport imports only `proxy`. Review security releases before updates. |
| Why not standard library | Go 1.26 `net/http.Transport` documents that `socks5` is treated as `socks5h`, contradicting frozen source loopback contracts. |
| Explicit limit | This dependency proves only SOCKS connection behavior. It neither provides browser TLS fingerprinting nor source HTTP/2 settings; those remain task 5.4/5.5 gates. |
| Update rule | Re-run both SOCKS loopback fixtures, cancellation/race tests, and inspect proxy API/license/module graph before an update. |

## Browser-fingerprint candidates — not approved, 2026-07-21

| Candidate | Assessment | Decision |
| --- | --- | --- |
| `github.com/refraction-networking/utls v1.8.2` | Go `1.24`, BSD 3-Clause, no cgo requirement in the inspected module. It can control a TLS ClientHello, but the frozen contract also needs request and HTTP/2 behavior; no differential engine evidence proves that it matches `primp` or DDG's temporary HTTP/2 client. Its module graph also adds brotli/compression and crypto dependencies. | Do not add yet. Reconsider only with controlled per-engine TLS + HTTP/2 evidence and a request-local design. |
| `github.com/bogdanfinn/tls-client v1.15.1` | Go `1.24.1`, broad forked HTTP/QUIC/TLS dependency graph with local `replace` directives in its module metadata. Its BSD-style license contains an advertising clause requiring an acknowledgement naming an unresolved `<organization>`. | Rejected for this module. License obligation and supply-chain surface are not justified by fixture evidence. |

`net/http` plus `x/net/proxy` remains approved only for the base behaviors
covered by the transport corpus: request construction, response lifecycle,
cookies, redirects, compression, TLS verification/PEM, HTTP(S) proxies and
SOCKS resolution. It is **not** evidence of `primp` browser impersonation,
TLS fingerprint parity, or DDG text HTTP/2 settings; those remain explicit
task 5.4/5.5 gates.
