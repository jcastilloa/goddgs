# Frozen Python reference environment

This is the provenance record for the Python oracle used by the
`port-ddgs-python-library` OpenSpec change. It is not a Go dependency and its
environment directory is deliberately outside this repository.

## Baseline

| Item | Value |
| --- | --- |
| Source checkout | `/home/jcastillo/Proyectos/ddgs` |
| Frozen commit | `a12929a72429a39a0841c3d7caacb20ee17acd4d` |
| Source version field | `9.14.4` |
| Source worktree at setup | clean |
| `pyproject.toml` SHA-256 | `74024a4ed3917e90f0aaf5fc07e6bf5ef6ab0834c726060bc26abb9d66f08e18` |
| Setup date | 2026-07-20 |
| Python | CPython 3.12.3 |
| Resolver | `uv 0.11.8` |
| Local environment | `/tmp/goddgs-reference-a12929a` |

The upstream checkout has no committed lockfile. The exact resolved package
set below is therefore fixture provenance for this port; it does not change
the frozen upstream baseline. Capture tooling must write the source SHA and
this environment record into each generated fixture.

## Rebuild procedure

Run only after confirming the source checkout is clean and points at the frozen
commit:

```bash
git -C /home/jcastillo/Proyectos/ddgs diff --quiet
test "$(git -C /home/jcastillo/Proyectos/ddgs rev-parse HEAD)" = \
  a12929a72429a39a0841c3d7caacb20ee17acd4d
uv venv --python 3.12 /tmp/goddgs-reference-a12929a
uv pip install --python /tmp/goddgs-reference-a12929a/bin/python \
  -e /home/jcastillo/Proyectos/ddgs
uv pip check --python /tmp/goddgs-reference-a12929a/bin/python
uv pip freeze --python /tmp/goddgs-reference-a12929a/bin/python
```

Do not commit the virtual environment, wheel cache, proxy credentials, cookies,
or live responses. If the environment is recreated later, compare its freeze
output with this record before it produces new fixtures. Exact package versions
are recorded, but distribution hashes were not captured; obtain and record
hashes before relying on a rebuilt environment for release-grade fixture
regeneration.

## Resolved package set

```text
anyio==4.14.2
brotli==1.2.0
certifi==2026.6.17
click==8.4.2
ddgs==9.14.4 (editable source at frozen checkout)
fake-useragent==2.2.0
h11==0.16.0
h2==4.3.0
hpack==4.2.0
httpcore==1.0.9
httpx==0.28.1
hyperframe==6.1.0
idna==3.18
lxml==6.1.1
primp==1.3.1
socksio==1.0.0
typing-extensions==4.16.0
```

`uv pip check` passed on the recorded environment.

## Pure reference probes completed

These probes made no external search-engine request. They confirm the oracle
loads frozen source and expose a few contract edges for future fixtures:

| Probe | Frozen output |
| --- | --- |
| Runtime registry | text: brave, duckduckgo, google, grokipedia, mojeek, startpage, wikipedia, yahoo, yandex; images: bing, duckduckgo; news: bing, duckduckgo, yahoo; videos: duckduckgo; books: annasarchive |
| `_normalize_text(' A <b>x</b> &amp; e\\u0301\\x00  \\n')` | `A x & é` |
| `_normalize_url('https://x/a%20b+c')` | `https://x/a+b+c` |
| `_normalize_date(True)` | `1970-01-01T00:00:01+00:00` |
| Image aggregation with blank `image` then same `url` | cache key is the first field `image`, so both entries collapse under `""`; first object remains |

These are initial pure checks only. They do not constitute parser, transport,
renderer, scheduler, or engine completion evidence.
