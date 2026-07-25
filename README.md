# goddgs

Go library port of [deedy5/ddgs](https://github.com/deedy5/ddgs), focused only on embeddable metasearch functionality.

Status: public search composition and extraction are implemented against frozen
offline source contracts. Rendered extraction formats are a documented practical
exception; browser TLS/HTTP2 fingerprint parity remains unproven. Do not
describe this module as a complete 1:1 port or production-ready release.

## Scope

- Go module for importing from Go programs.
- Text, image, news, video, book search, and source-compatible content fetching
  with practical HTML rendering.
- Engine request construction, parsing, normalization, aggregation, ranking, proxy, TLS, and timeout behavior.

Excluded: HTTP API server, CLI, MCP server, DHT/cache service, Docker service, and unrelated application wiring.

## Source baseline

- Repository: `https://github.com/deedy5/ddgs`
- Local source: `/home/jcastillo/Proyectos/ddgs`
- Commit: `a12929a72429a39a0841c3d7caacb20ee17acd4d`
- Describe: `v9.14.4-2-ga12929a`

Source code, not a stale README entry, defines runtime behavior. Read
[docs/source-audit.md](docs/source-audit.md),
[docs/source-quirks.md](docs/source-quirks.md),
[docs/engine-contracts.md](docs/engine-contracts.md),
[docs/reference-environment.md](docs/reference-environment.md), [MEMORY.md](MEMORY.md), and
OpenSpec change [`port-ddgs-python-library`](openspec/changes/port-ddgs-python-library/).

## Layout

```text
.
├── doc.go                 # Public package: ddgs
├── internal/
│   ├── engine/            # Source-engine adapters and registry
│   ├── extract/           # Source fetch lifecycle + practical HTML rendering
│   ├── normalize/         # URL, text, and date normalization
│   ├── parser/            # HTML/XPath and JSON extraction
│   ├── search/            # Search orchestration, aggregation, ranking
│   └── transport/         # HTTP, cookies, proxy, TLS/fingerprint behavior
├── testdata/
│   ├── contracts/         # Python-vs-Go behavioral goldens
│   └── fixtures/          # Captured engine responses
├── docs/
└── openspec/
```

Module path: `github.com/jcastilloa/goddgs`.

## Commands

```bash
make test
make test-race
make vet
make verify
make integration # networked tests; opt-in only
```

See [docs/integration.md](docs/integration.md) before `make integration`: it
serializes five minimal live smoke requests and remains separate from default
test commands.

`make verify` is fully offline and makes no external engine requests. See
[docs/dependency-decisions.md](docs/dependency-decisions.md) for the approved
practical extraction-rendering exception and remaining fingerprint limitation.

## License and attribution

The upstream source is MIT-licensed. See [LICENSE](LICENSE) and [NOTICE.md](NOTICE.md). Attribution does not imply upstream endorsement.
