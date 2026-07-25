# Opt-in live engine smoke tests

`make integration` is the only project target that permits live search-engine
requests. It sets `GODDGS_INTEGRATION=1`, compiles tests tagged
`integration`, runs them serially (`-p=1`), and waits three seconds between
categories. Default unit, race, coverage, and fixture commands never include
these tests.

The smoke suite sends one query (`open source`) with `max_results=1` to these
explicit backends, in this order: DuckDuckGo text, images, news, videos, then
Anna's Archive books. It does not use `auto` or `all`, parallel tests, captured
cookies, credential headers, response persistence, or result logging.

Run it only with a permitted network, a suitable rate window, and a human
reviewer prepared to assess current endpoint availability:

```bash
make integration
```

Each category is expected to return at least one raw result. A timeout,
transport error, or empty result is a current live-availability failure to
record; it does not invalidate offline differential fixtures. In particular,
the browser TLS/HTTP2 fingerprint gate remains unapproved, so an engine may
reject the Go transport even when its fixture contract passes. See
[`fingerprint-gate.md`](fingerprint-gate.md).

The integration build tag alone does not authorize network access: without
`GODDGS_INTEGRATION=1`, the smoke test skips before constructing a client.
This permits a compile-only safety check without external requests:

```bash
go test -tags=integration -run '^$' ./...
GODDGS_INTEGRATION=0 go test -tags=integration -run TestIntegrationSmokeCategories .
```

No integration run claims browser-fingerprint parity. Extraction rendering uses
the documented practical renderer exception; it is not a `primp` output claim.
