# goddgs

Go library port of [deedy5/ddgs](https://github.com/deedy5/ddgs), focused only on embeddable metasearch functionality.

Status: scaffold and parity epic created. No search behavior is implemented yet.

## Scope

- Go module for importing from Go programs.
- Text, image, news, video, book search, and content extraction parity with source `ddgs`.
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
│   ├── extract/           # HTML/content rendering parity
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

`github.com/jcastillo/goddgs` is a provisional module path. Confirm it before the first published release.

## Commands

```bash
make test
make test-race
make vet
make verify
make integration # networked tests; opt-in only
```

## License and attribution

The upstream source is MIT-licensed. See [LICENSE](LICENSE) and [NOTICE.md](NOTICE.md). Attribution does not imply upstream endorsement.
