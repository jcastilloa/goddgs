//go:build go1.27

package normalize

// Frozen Python normalization uses Unicode 15.0.0. Go 1.27 changes standard
// library Unicode tables, so this package deliberately requires an explicit
// source-baseline review before compiling with that toolchain or later.
const unsupportedUnicodeToolchain = uint(0) / uint(0)
