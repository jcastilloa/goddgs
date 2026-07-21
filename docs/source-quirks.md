# Frozen source quirks — do not silently fix

This register records observable behavior of frozen Python source
`a12929a72429a39a0841c3d7caacb20ee17acd4d`. Some entries look accidental. For
this port, they are contracts until an explicit OpenSpec baseline change says
otherwise. Each needs a differential fixture before Go implementation is
complete.

## Baseline and public surface

| ID | Frozen behavior | Port rule |
| --- | --- | --- |
| Q-01 | Package `__version__` is `9.14.4`, while source HEAD is two commits after tag `v9.14.4`. | Pin SHA, not version string alone. |
| Q-02 | README lists text `bing`; runtime discovery omits it because `Bing.disabled = True`. | Text `bing` stays unavailable; explicit selection follows invalid-backend fallback. |
| Q-03 | `DDGS.__enter__` returns self and `__exit__` is a no-op. | Do not infer source transport cleanup from Python context-manager use. Go still closes its resources safely. |
| Q-04 | `RatelimitException` is declared but frozen core/engine code never raises it. | Do not invent HTTP-status-to-rate-limit conversion. |
| Q-05 | Empty explicit proxy is falsy: `_expand_proxy_tb_alias(proxy) or DDGS_PROXY`. | Empty proxy falls through to environment; only nonempty explicit proxy overrides it. |

## Engine selection and scheduler

| ID | Frozen behavior | Port rule |
| --- | --- | --- |
| Q-10 | Backend is comma-split/trimmed. Python list is deprecated but accepted. `auto` and `all` share selection behavior. | Preserve normal string parsing/fallback; decide Go list representation separately. |
| Q-11 | Engine keys are shuffled. Text prepends Wikipedia/Grokipedia. Sort key is `(e.priority, random)`: it returns function object `random`, it does **not** call `random()`. Python stable sort preserves shuffled equal-priority order. | Do not "fix" tie sorting by sampling a fresh random value. |
| Q-12 | Provider enters `seen_providers` only after completed future returns nonempty result. It is not reserved at submit time. | Do not strengthen this into strict one-in-flight-provider scheduling. |
| Q-13 | Batch flush condition is `len(futures) >= max_workers or i >= max_workers`, where `i` is enumerated engine position, not submitted-future count. | Preserve outcome behavior with provider-duplicate/skip fixtures. |
| Q-14 | `wait(..., FIRST_EXCEPTION)` processes only done futures. Pending futures remain; executor shutdown waits them, but results can remain unaggregated after loop/break. | Do not replace with generic cancel-on-first-error or gather-all behavior. |
| Q-15 | Workers: `min(unique_providers, ceil(max_results/10)+1)` if max-results truthy; otherwise unique providers. Truthy `DDGS.threads` only lowers count. | Preserve zero/unlimited and thread-limit semantics; test edge values before adding validation. |
| Q-16 | No-result becomes `TimeoutException` only when lowercase `"timed out"` appears in stringified last error; otherwise `DDGSException`. | Do not infer source timeout from arbitrary context or HTTP status. |
| Q-17 | Neither `max_results` nor `DDGS.threads` is range-validated. Negative truthy values can make the computed `ThreadPoolExecutor(max_workers=...)` non-positive and expose its `ValueError`; `0` is treated as falsy/unlimited for `max_results` and ignored for `threads`. | Capture exact reference exceptions in the frozen Python environment. Do not add friendly validation or silently clamp values. |
| Q-18 | `concurrent.futures.wait` first observes futures already completed before applying a zero timeout. | A zero scheduler wait must retain a completion already available at batch entry. |

## Parser, result, and error behavior

| ID | Frozen behavior | Port rule |
| --- | --- | --- |
| Q-20 | Generic engine request returns text only for status 200. Other status becomes `None`. VQD bootstrap reads raw response content. `extract()` handles non-200 separately. | Keep these three status paths distinct. |
| Q-21 | Generic HTML extraction applies lxml XPath, joins XPath text with spaces, collapses whitespace, then result setters may normalize again. | Do not substitute generic DOM-to-text pipeline. |
| Q-22 | Aggregator key scans result object field insertion order, not cache-field set order. `Counter.most_common()` retains first encounter order for equal counts. | Preserve declared/dynamic field order and count-tie behavior. |
| Q-23 | Normalizers run only for named fields and only if value is truthy. Images/videos can retain integers, maps, or null-like JSON values. | Do not force raw results into strings or normalize unrelated fields. |
| Q-24 | `_normalize_date` converts Python `int` to UTC `isoformat`; bool is an `int` subclass and float is not converted. Frozen CPython/Linux raises `ValueError` outside years 1–9999 and `OSError [Errno 75]` when C `tm_year` overflows. | Keep date normalization error-capable; preserve captured source class/message. JSON adapters must retain numeric lexical form so an integer is not silently changed to `float64`. |
| Q-25 | Text uses non-DOTALL `<.*?>`, Python unescape/NFC/category-C deletion/`split()`. URL uses `unquote` then literal-space-to-`+`. Python HTML5 recognizes `nGt;`/`nLt;`; Go stdlib omits both. Python and frozen Go 1.26 use Unicode 15.0.0, while later Go toolchains can drift. | Match source with adversarial fixtures; supplement only those two entities before unescape; do not use generic query unescape; reject unreviewed newer Unicode toolchains. |
| Q-26 | Relative news dates use current UTC; Bing naïve dates pass through local-time interpretation before UTC conversion. | Inject clock and capture environment/time-zone behavior. |
| Q-27 | Dataclass result fields are inserted in declaration order; later `__setattr__` updates retain their position and a new dynamic field appends. Aggregation inspects that evolving order. | Keep an ordered internal field sequence. Never use Go map iteration for source cache-key selection. |
| Q-28 | Duplicate aggregation calls Python `len()` on raw `body`. Empty list/dict have length; `None`, bool, and numeric bodies reach the aggregator when falsy and raise source `TypeError` on the second duplicate. | Preserve captured replacement/error behavior; do not coerce body values to strings or zero length. |
| Q-29 | `SimpleFilterRanker` applies raw `in` checks to `title` and `href` before calling `.lower()` on title/body. JSON lists and dicts can satisfy membership, while `None` is non-iterable and non-string title/body values raise Python-specific `AttributeError`. | Preserve category → Wikipedia → lower evaluation order and the captured raw type/error behavior. Do not coerce ranked documents to strings before filtering. |

## Engine-specific behavior

| ID | Frozen behavior | Port rule |
| --- | --- | --- |
| Q-30 | Google and DuckDuckGo text user agents are chosen at Python module/class creation, not per request. | Model source lifetime correctly; do not casually re-randomize every request. |
| Q-31 | Anna's Archive TLD is randomly chosen in class declaration at module import, then shared by instances in Python process. | Model source-lifetime selection, not mandatory per-request rotation. |
| Q-32 | Yahoo random `_ylt`/`_ylu` path and Yandex random `searchid` are built per search. | Keep request-time random seams deterministic in tests. |
| Q-33 | Startpage retrieves home-page `sc` for every payload build; it stores `_sc` but does not reuse it. | Preserve bootstrap sequence and missing-token behavior. |
| Q-34 | Wikipedia mutates cached instance `search_url` and `lang` per search, then makes second request during extraction. | Go avoids races while preserving per-call request/result behavior. |
| Q-35 | Bing Images reads `max_results` only from engine kwargs. Public `DDGS().images(..., max_results=N)` binds it in `_search_sync` and does not forward it, so normal path uses default count `35`. | Preserve/test data-flow quirk; do not silently pass public max-results into Bing Images payload. |
| Q-36 | Bing Images timelimit mapping accepts `day`, `week`, `month`, `year`; public docs/options use `d`, `w`, `m`, `y`. Documented shorthand produces mapping `KeyError`. | Treat observed exception as source behavior until baseline change fixes it. |
| Q-37 | DuckDuckGo media gets VQD by scanning three raw-byte marker forms. Missing marker raises DDGS error. | Capture all markers and missing-token failure. |
| Q-38 | Yahoo News wraps whole post-process loop in broad `try`; one malformed item can stop later transformations and return partial cleanup. | Preserve fixture-observed partial transformation/error swallowing. |
| Q-39 | Engine mapping lookups can raise `KeyError`/`ValueError` for unsupported region, safesearch, or timelimit. | Do not invent validation/default normalization. |
| Q-39a | Bing Images splits dimensions after replacing `×` with `x`; a normal-looking synthetic value such as `640 × 480 px` yields three pieces and raises `ValueError: too many values to unpack (expected 2)`. | Preserve observed failure until source-baseline change; do not trim a `px` suffix proactively. |
| Q-39b | DuckDuckGo text's generic engine path returns `None` for a 200 response whose `text` is exactly empty, while a nonempty empty or malformed HTML document parses to `[]`. Its POST form preserves insertion order `q,b,l[,s][,df]`; safesearch is ignored. | Preserve nil-versus-empty result, payload ordering, page/timelimit conditions, and `y.js` filtering; do not collapse these paths. |

## Transport behavior

| ID | Frozen behavior | Port rule |
| --- | --- | --- |
| Q-40 | Default client uses `primp.Client` with random browser/OS impersonation. | Fingerprint must be proven per affected engine; `net/http` is not presumed equal. |
| Q-41 | DuckDuckGo text enables HTTP/2, disables redirect follow, randomizes TLS/H2 settings, and globally monkey-patches `httpcore` H2 init around request. | `internal/transport.DuckDuckGoTextClient` proves request-local standard H2/no-redirect/header behavior and never recreates the global patch. Randomized TLS/H2 fingerprint remains unproven until task 5.5. |
| Q-42 | `verify` bool and PEM path use different source-client branches. | Fixture default/false/PEM before Go transport selection. |
| Q-43 | Frozen `extract()` accesses only chosen rendered property; unknown format selects Markdown. | Do not eagerly render all formats; preserve raw/fallback behavior. |
| Q-44 | With resolved frozen `primp` 1.3.1, `extract(fmt="content")` preserves raw non-UTF-8 bytes while `extract(fmt="text")` exposes the source response decoding with replacement characters. | Preserve raw bytes without a text round trip; compare decoded raw text against extraction fixtures when selecting Go transport/renderer. |
| Q-45 | Google calls `set_cookies("google.com", {"CONSENT": "YES+"})` without a scheme, while other engines pass full HTTPS URLs. | Canonicalize a bare domain to the cookie jar's HTTPS URL; do not silently drop Google consent state. |

## Upstream test gap

Frozen upstream tests are live HTTP smoke tests with a two-second pause. They
mostly assert nonempty lists or output type and do not lock payloads, selectors,
edge cases, scheduler timing, normalizers, fingerprint, or quirks above. Go
fixture corpus supplies this missing executable specification; passing
upstream-style live smoke tests is not proof of parity.
