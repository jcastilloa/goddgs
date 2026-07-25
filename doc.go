// Package ddgs is a Go module for querying frozen-source DDGS metasearch
// backends with raw source-shaped results.
//
// Search composition and extraction fetch lifecycle are fixture-tested.
// Extraction HTML rendering is a documented practical exception, while browser
// TLS/HTTP2 fingerprint parity remains explicitly unproven. See
// docs/fingerprint-gate.md and MEMORY.md before a public release.
package ddgs
