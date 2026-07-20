# Dependency decisions

Each entry records purpose, pin, license, supply-chain assessment, and update
rule before a Go module enters `go.mod`.

## `golang.org/x/text` `v0.31.0` — approved 2026-07-20

| Item | Decision |
| --- | --- |
| Purpose | NFC normalization and Python-compatible UTF-8 replacement during URL percent decoding in `internal/normalize`. Go standard library has no Unicode normalization API. |
| Imported packages | `golang.org/x/text/unicode/norm` for NFC; `golang.org/x/text/encoding/unicode` and `golang.org/x/text/transform` for Python-compatible UTF-8 replacement during URL percent decoding. |
| Version | `v0.31.0`; checksum and module checksum are retained in `go.sum`. |
| Compatibility | Module declares Go `1.24`; project uses Go `1.26`. Unlike `v0.40.0`, `v0.31.0` keeps its `norm` Unicode `15.0.0` table for Go 1.21+ rather than selecting Unicode 17 under Go 1.27. Frozen Python 3.12 `unicodedata.unidata_version` is `15.0.0`. Standard-library Unicode remains compiler-versioned, so build guard intentionally rejects Go 1.27+ pending rebaseline. |
| License | BSD 3-Clause, verified from cached module `LICENSE`. Compatible with project/upstream MIT distribution; retain upstream module metadata through Go modules. |
| cgo | None. Pure Go Unicode tables. |
| Maintenance/provenance | Official Go subrepository, source origin `https://go.googlesource.com/text`, tag `v0.31.0`, module checksum verified through Go module tooling. |
| Supply-chain risk | Extra Unicode-table code only; no network, filesystem, transport, or runtime initialization behavior. Pin exact version and retain `go.sum`; review release/security advisories before any update. |
| Why not standard library | `unicode` classifies runes but does not implement NFC. Omitting NFC contradicts frozen fixture `pure.normalize-text-nfc`. |
| Update rule | Do not update casually. Re-run all normalizer differentials and inspect Go build tags/table versions; confirm active table remains Python baseline-compatible or update source baseline/OpenSpec explicitly. |

Evidence gathered locally on 2026-07-20: Go `1.26.1`; frozen Python
`3.12.3`; `norm.Version == "15.0.0"` for Go 1.26 build path; Python Unicode
database `15.0.0`.
