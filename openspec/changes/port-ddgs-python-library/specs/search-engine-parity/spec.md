## ADDED Requirements

### Requirement: Frozen active engine registry
Library SHALL use static Go registry equivalent to frozen Python runtime
discovery at `a12929a72429a39a0841c3d7caacb20ee17acd4d`. It SHALL register
active engines and provider labels exactly: text Brave, DuckDuckGo, Google,
Grokipedia, Mojeek, Startpage, Wikipedia, Yahoo, Yandex; images Bing and
DuckDuckGo; news Bing, DuckDuckGo, Yahoo; videos DuckDuckGo; books Anna's
Archive.

#### Scenario: Automatic text backend selection
- **WHEN** caller requests text search with source-equivalent `auto` backend
- **THEN** selection SHALL draw only from frozen active text registry and preserve source priority/randomization rules

#### Scenario: Disabled text Bing is requested
- **WHEN** caller requests text backend `bing`
- **THEN** library SHALL treat it unavailable and follow source invalid-backend/fallback behavior rather than activate disabled adapter

#### Scenario: Provider collision exists
- **WHEN** registry is inspected for DuckDuckGo and Yahoo text engines
- **THEN** both SHALL retain source provider label `bing` for orchestration de-duplication

### Requirement: Source-specific request sequence
Each engine SHALL reproduce frozen source request sequence, including method,
endpoint, query/body fields, relevant headers/cookies, bootstrap requests,
tokens, redirects, pagination, time limit, safe-search, region mapping, random
values, and source post-processing. Engine SHALL NOT be replaced by generic
request or undocumented fallback.

#### Scenario: DuckDuckGo media VQD sequence
- **WHEN** fixture invokes DuckDuckGo images, news, or videos
- **THEN** Go behavior SHALL obtain VQD from source-compatible bootstrap request before forming JSON endpoint request

#### Scenario: Startpage form-state sequence
- **WHEN** fixture invokes Startpage text search
- **THEN** Go behavior SHALL obtain source `sc` form state before issuing search POST request

#### Scenario: Wikipedia two-request enrichment
- **WHEN** fixture invokes Wikipedia text search with open-search hit
- **THEN** Go behavior SHALL make source-compatible extract request and apply source disambiguation filter

### Requirement: Parser and result-processing parity
Library SHALL parse saved engine responses using source-equivalent XPath or JSON
behavior and SHALL apply all source engine-specific transformations before
aggregation. Original source selectors and transformations SHALL be proven by
differential fixtures before modification.

#### Scenario: HTML engine fixture is parsed
- **WHEN** saved HTML fixture contains results for HTML source engine
- **THEN** Go parser output SHALL match frozen Python field values after source normalization

#### Scenario: Bing image embedded metadata is parsed
- **WHEN** Bing Images fixture contains JSON in image metadata attribute
- **THEN** Go result SHALL preserve source title, image, thumbnail, URL, dimensions, and source extraction behavior

#### Scenario: Yahoo News cleanup has malformed input
- **WHEN** fixture exercises source Yahoo News post-processing failure behavior
- **THEN** Go behavior SHALL match documented source result/error outcome and SHALL NOT invent replacement URL

### Requirement: Source normalization parity
Library SHALL reproduce source text, URL, and date normalization: regex HTML
tag stripping, entity unescaping, NFC, Unicode category-C removal, whitespace
collapse, percent decoding then spaces to `+`, and source-compatible UTC lexical
date output. Internal date normalization SHALL be error-capable: an integer
timestamp for which frozen Python raises SHALL not be converted into a Go-only
date string. JSON adapters SHALL preserve numeric lexical form until they can
distinguish a source integer from a source float.

#### Scenario: Text includes markup, entities, and controls
- **WHEN** fixture assigns normalizable text containing HTML, entities, Unicode decomposition, category-C characters, and repeated whitespace
- **THEN** Go output SHALL equal frozen Python normalized text

#### Scenario: URL includes encoded spaces and plus characters
- **WHEN** fixture assigns encoded URL
- **THEN** Go output SHALL follow Python `unquote(...).replace(" ", "+")` rather than generic query-unescape behavior

#### Scenario: Integer timestamp is normalized
- **WHEN** fixture assigns integer epoch date
- **THEN** Go output SHALL match frozen Python UTC ISO lexical output exactly

#### Scenario: Integer timestamp exceeds frozen source date range
- **WHEN** fixture assigns an integer timestamp for which frozen Python raises
  `ValueError` or `OSError`
- **THEN** Go normalization SHALL return no normalized value and an error with
  the captured source class/message rather than format a year outside the
  source range

#### Scenario: Frozen Unicode/HTML edge is normalized
- **WHEN** fixture contains Python-only HTML5 entities `nGt;` or `nLt;`, or
  category-C code points under Unicode 15.0.0
- **THEN** Go output SHALL match frozen Python and SHALL not drift solely
  because a newer Go toolchain changes Unicode tables

### Requirement: Source orchestration, aggregation, and ranking
Library SHALL preserve backend parsing, engine-cache scope, provider
de-duplication, worker-count formula, occurrence aggregation, duplicate
replacement, ranker buckets, category-page filtering, no-result classification,
and `max_results` semantics.

#### Scenario: Provider duplicate is scheduled before first result returns
- **WHEN** controlled engine fixtures share provider label and first submitted engine has not yet returned nonempty result
- **THEN** scheduler behavior SHALL match frozen source submit/seen-provider timing rather than reserve provider at submission

#### Scenario: Equal engine priorities are selected
- **WHEN** controlled backend selection includes equal-priority engines
- **THEN** equal-priority order SHALL preserve frozen shuffled-order behavior rather than draw new random tie value

#### Scenario: Duplicate result occurs from multiple engines
- **WHEN** fixtures produce items with same source cache key
- **THEN** Go aggregation SHALL count occurrences, retain source-selected item, and return source frequency order

#### Scenario: Duplicate has longer body
- **WHEN** duplicate items share cache key and later item has longer `body`
- **THEN** aggregate output SHALL use longer-body item while retaining occurrence count

#### Scenario: Ranking contains Wikipedia and category page
- **WHEN** aggregate fixtures include Wikipedia hit, title/body token variants, and title containing both `Category:` and `Wikimedia`
- **THEN** Go ranking SHALL place Wikipedia first, exclude category page, and preserve frozen bucket order

#### Scenario: Ranking receives heterogeneous raw fields
- **WHEN** a ranking fixture provides list, dictionary, null, or scalar values in raw `href`, `title`, or `body` fields
- **THEN** Go ranking SHALL preserve the frozen source's category-membership, Wikipedia-membership, and `.lower()` evaluation order, including captured result or error shape, without pre-coercing fields to strings

#### Scenario: Max result is zero or unlimited
- **WHEN** caller explicitly uses source-equivalent falsy/unlimited max-results value
- **THEN** scheduler and final slicing SHALL follow frozen source unlimited behavior rather than treat zero as no results

#### Scenario: Caller cancels scheduled search
- **WHEN** caller context is canceled while a Go scheduler operation owns running engine work
- **THEN** scheduler SHALL stop dispatch, cancel only caller-owned work, join its operation goroutines, and return a context-classifiable error without converting it to a source timeout or canceling siblings merely because one engine failed

#### Scenario: Scheduled inputs are independently owned
- **WHEN** an internal scheduler operation begins with optional request values or an engine metadata slice supplied by its caller
- **THEN** it SHALL snapshot those inputs before dispatch; later caller mutation SHALL NOT change worker count, timeout, final slicing, engine selection, or engine request values

#### Scenario: Bing Images receives public max-results value
- **WHEN** caller invokes normal public image search with Bing and a max-results value
- **THEN** request payload SHALL match frozen source data flow, including engine default-count behavior unless direct engine-only contract supplies engine kwargs

#### Scenario: Bing Images receives documented timelimit shorthand
- **WHEN** caller invokes Bing Images with source public shorthand such as `d`
- **THEN** Go behavior SHALL match frozen source exception/result behavior rather than silently map it to different time unit

### Requirement: Transport compatibility gate
Library SHALL not declare engine complete until required HTTP, cookie, proxy,
TLS, HTTP/2, compression, redirect, UA, and fingerprint behavior is proven by
fixture contract and applicable controlled integration evidence. Default Go HTTP
behavior SHALL NOT be assumed source-equivalent.

#### Scenario: Engine requires browser-like transport
- **WHEN** engine path depends on frozen `primp` impersonation or temporary DuckDuckGo HTTP/2 behavior
- **THEN** completion evidence SHALL identify and test approved Go transport capability for that path

#### Scenario: Transport feature cannot be proven
- **WHEN** required source transport behavior cannot be reproduced with reviewed dependencies
- **THEN** engine SHALL remain explicit unresolved compatibility blocker and SHALL NOT be represented fully ported
