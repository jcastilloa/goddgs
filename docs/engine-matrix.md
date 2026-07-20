# Frozen engine matrix

Source: `/home/jcastillo/Proyectos/ddgs` at
`a12929a72429a39a0841c3d7caacb20ee17acd4d`. Planning matrix only; no engine is
ported merely because it appears here.

## Active engines

| Category | Engine | Provider | Flow | Parsing / post-processing | Key parity risk |
| --- | --- | --- | --- | --- | --- |
| text | Brave | `brave` | GET `search.brave.com/search`; region/safesearch cookies | HTML XPath | cookie scope + markup drift |
| text | DuckDuckGo | `bing` | POST `html.duckduckgo.com/html/`; special `HttpClient2` | HTML XPath; reject `y.js` redirect | temporary random HTTP/2/TLS client + UA |
| text | Google | `google` | GET search; consent cookie; random Android Google UA | HTML XPath; `/url?q=` unwrap/filter | fingerprint + brittle markup |
| text | Grokipedia | `grokipedia` | GET typeahead JSON | first result; title/snippet rewrite | heterogeneous/missing JSON fields |
| text | Mojeek | `mojeek` | GET with region cookies | HTML XPath | cookie scope + markup drift |
| text | Startpage | `google` | GET home for `sc`, then POST search | HTML XPath | multi-request state and provider collision |
| text | Wikipedia | `wikipedia` | GET opensearch JSON, then GET extract JSON | one fuzzy result; reject disambiguation phrase | mutable language/request state |
| text | Yahoo | `bing` | GET random `_ylt`/`_ylu` path | HTML XPath; `/RU=` redirect decode | random path and provider collision |
| text | Yandex | `yandex` | GET random `searchid` | HTML XPath | random request state + markup drift |
| images | Bing | `bing` | GET async endpoint | HTML XPath + JSON stored in `m` attribute | dimensions/embedded JSON |
| images | DuckDuckGo | `bing` | GET homepage VQD, then GET `i.js` JSON | JSON `results`; image filters | token extraction + provider collision |
| news | Bing | `bing` | GET infinite scroll endpoint | HTML XPath; locale/relative date and image rewrite | clock-dependent date lexical form |
| news | DuckDuckGo | `bing` | GET homepage VQD, then GET `news.js` JSON | JSON `results` | token extraction + provider collision |
| news | Yahoo | `yahoo` | GET news search | HTML XPath; URL/image/source/date cleanup | fragile cleanup error behavior |
| videos | DuckDuckGo | `bing` | GET homepage VQD, then GET `v.js` JSON | heterogeneous JSON `results` | nested maps and scalar type preservation |
| books | Anna's Archive | `annasarchive` | GET randomized `.gd`/`.gl`/`.pk` hostname | HTML XPath after comment removal; relative URL prefix | random TLD and legal/availability volatility |

## Source-present but disabled

| Category | Engine | Runtime registry status | Required Go behavior |
| --- | --- | --- | --- |
| text | Bing | `disabled = True`; omitted by Python discovery | Do not include in active/static registry or `auto`; explicit `bing` follows source invalid-backend warning/fallback behavior. |

## Request concerns

| Concern | Affected engines | Evidence before completion |
| --- | --- | --- |
| Browser impersonation / TLS fingerprint | default `primp` engines, especially Google/Brave/Startpage | captured request metadata plus controlled live check using approved transport |
| Custom HTTP/2 settings | DuckDuckGo text | concurrent-safe request-local implementation; race test; no global patch |
| Bootstrap token | DDG images/news/videos | two-request fixture and exact VQD failure behavior |
| Bootstrap form state | Startpage | GET/`sc`/POST fixture, including missing `sc` |
| Cookies | Brave, Google, Mojeek, Bing | host/path-scoped request fixture |
| Random values | Google UA, Yahoo tokens, Yandex ID, Anna domain, sort ties | deterministic injected source in tests; secure source at runtime |
| XPath recovery | HTML engines | source selector/result corpus |
| Nested JSON types | DDG videos, Bing image metadata | raw `map[string]any` fixture; no coercion |
| Clock-derived dates | Bing News, Yahoo News | injected clock and exact lexical output |
