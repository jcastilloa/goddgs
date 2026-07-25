// Package ddgs is a Go module for querying frozen-source DDGS metasearch
// backends with raw source-shaped results.
//
// Search composition and extraction fetch lifecycle are fixture-tested.
// Extraction HTML rendering is a documented practical exception. HTTPS base
// clients choose one coherent frozen primp browser/OS bundle for their
// lifetime; the DuckDuckGo temporary client remains a separate capability.
// See docs/browser-profiles.md, docs/fingerprint-gate.md and MEMORY.md before
// a public release.
package ddgs
