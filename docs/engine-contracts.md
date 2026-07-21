# Contratos congelados de motores

Inventario de comportamiento de `ddgs` Python en
`a12929a72429a39a0841c3d7caacb20ee17acd4d`. Complementa
[engine-matrix.md](engine-matrix.md) y [source-quirks.md](source-quirks.md).
No es implementación ni prueba de paridad: cada fila requiere fixture
diferencial Python-vs-Go antes de marcar su motor como terminado.

No se hicieron peticiones live para este inventario. Endpoints, payloads,
selectores y transformaciones salen del código congelado.

## Pipeline común

1. `build_payload` puede cambiar estado, cookies o lanzar bootstrap antes de
   la petición de búsqueda.
2. El motor genérico hace GET con parámetros o POST con form data.
3. `BaseSearchEngine.request` devuelve texto sólo con HTTP 200; otro estado o
   texto vacío equivale a resultado ausente. Bootstrap directo usa respuesta
   cruda y no necesariamente ese filtro.
4. HTML genérico usa recuperación lxml y XPath; une texto XPath, colapsa
   espacios y después normaliza campos de resultado.
5. Postproceso ocurre tras parsear. No añadir recuperación genérica que la
   fuente no tenga.

Transport fingerprint, TLS, HTTP/2, parser XPath y renderer de `extract` son
gates separados: esta tabla no prueba ninguno.

## Texto

| Motor / provider | Secuencia y payload | Parseo y postproceso observable |
| --- | --- | --- |
| Brave / `brave` | GET `https://search.brave.com/search`; `q`, `source=web`; cookies por región lower-case (`<country>=<country>`, `useLocation=0`) y safesearch `strict/off`; el `dict` fuente reemplaza una clave de país repetida sin moverla; `tf`: `d/w/m/y -> pd/pw/pm/py`; `offset=page-1`. | `//div[@data-type='web']`; título de title/sitename final, href de enlace con div title, snippet/content como body. |
| DuckDuckGo / `bing` | POST `https://html.duckduckgo.com/html/`; form en orden `q`, `b=""`, `l=region`, después `s=10+(page-2)*15` sólo si `page>1`, y `df=timelimit` sólo si es truthy. Ignora safesearch. Usa `HttpClient2`. | `//div[contains(@class,'body')]`; h2, href `./a`, body `./a`; elimina href que empieza `https://duckduckgo.com/y.js?`. Un 200 con `response.text == ""` devuelve `None`; DOM vacío o malformado no vacío devuelve `[]`. |
| Google / `google` | GET `https://www.google.com/search`; UA Android/Chrome se decide una vez por módulo; cookie bare-domain `CONSENT=YES+`; safe case-insensitive `on/moderate/off -> 2/1/0`, `start=(page-1)*10`; región conserva case salvo país upper para `hl`, `lr`, `cr`; `tbs=qdr:<time>` sólo truthy. | `//div[@data-hveid][.//h3]`; normaliza el href, desenvuelve `/url?q=`, conserva sólo título no vacío y href que empieza `http`. |
| Grokipedia / `grokipedia` | GET `https://grokipedia.com/api/typeahead`; sólo `query` y `limit=1`. | JSON `results`; usa sólo primero, strip de `_` en title, corta snippet tras primer doble salto, href usa f-string de `slug`. `get` sólo aplica default cuando falta la clave: `null`/tipo incorrecto propaga el error fuente. El f-string conserva orden/repr Python de JSON anidado, incluidos `NaN`/`Infinity`. |
| Mojeek / `mojeek` | GET `https://www.mojeek.com/search`; cookies `arc`/`lb` desde región lower-case; `q`; sólo safe exacto/minúsculo `on` añade `safe=1`; página sólo si `page>1`, `s=(page-1)*10+1`; ignora timelimit. | `//ul[contains(@class,'results')]/li`; h2, h2/a y `p.s`. |
| Startpage / `google` | GET bootstrap `https://www.startpage.com/` para primer input `sc`, luego POST `https://www.startpage.com/sp/search`; Referer explícito; campos fijos `cat`, `t`, `lui`, `language`, `abp`, `abd`, `abe`, `segment`; `qsr`, `qadf`, `sc`, página/time. | `//div[contains(@class,'result')][./a]`; h2, href directo, p. El `_sc` se guarda pero no se reutiliza. |
| Wikipedia / `wikipedia`, prioridad 2 | `region.lower().split("-")` debe tener exactamente dos partes; GET open-search fuzzy por idioma (query con `quote`, `/` seguro), luego GET extracts del primer título. | JSON: primer título/href; body de `next(iter(query.pages.values()))`, por tanto primer miembro en orden de entrada JSON, no llave léxica. Un segundo no-200 deja body vacío. Descarta sólo substring exacto/minúsculo `may refer to:`. |
| Yahoo / `bing` | GET URL nueva por búsqueda con `_ylt=token_urlsafe(18)` y `_ylu=token_urlsafe(35)`; `p`; página `b=(page-1)*7+1`; `btf`. | `relsrch`; elimina Bing aclick; `/RU=` se corta en `/RK=` o `/RS=` y pasa `unquote_plus`; dato malformado puede fallar. |
| Yandex / `yandex` | GET `https://yandex.com/search/site/`; `text`, `web=1`, `searchid` aleatorio inclusivo 1000000..9999999; página `p=page-1`. | `serp-item`; h3, h3/a, div text. |

Detalles obligatorios de texto:

- User-Agent de Google y DuckDuckGo se decide al crear módulo/clase Python,
  no por request.
- Startpage siempre hace bootstrap; Wikipedia puede hacer segunda request.
- Yahoo y Yandex generan valores aleatorios por búsqueda.
- Valores de región/safesearch/timelimit no soportados pueden producir
  `KeyError`/`ValueError`; no validarlos ni corregirlos silenciosamente.

## Imágenes

| Motor / provider | Secuencia y payload | Parseo y quirk |
| --- | --- | --- |
| Bing / `bing` | GET `https://www.bing.com/images/async`; `q`, `async=1`, `first`, `count`; count directo es `max(int(max_results),35)`. Llamada pública normal no reenvía su `max_results`, por tanto count 35. Time acepta literalmente `day/week/month/year`, no `d/w/m/y`. | XPath de tarjetas con `a[@m]`; JSON `m` usa `t/murl/turl/purl`; dimensions desde texto con multiplicación; sólo añade item con metadata. |
| DuckDuckGo / `bing` | GET bootstrap `https://duckduckgo.com?q=...` para VQD crudo, luego GET `https://duckduckgo.com/i.js`; `o=json`, `q`, `l`, `vqd`, `p`, `ct=AT`; filtro `f` con slots ordenados time/size/color/type/layout/license; página `s=(page-1)*100`. | JSON `results` mapea title/image/thumbnail/url/height/width/source sin forzar tipos. Headers explícitos de navegación/cors son contrato. |

## Noticias

| Motor / provider | Secuencia y payload | Parseo y postproceso observable |
| --- | --- | --- |
| Bing / `bing` | GET `https://www.bing.com/news/infinitescrollajax`; `q`, `InfiniteScroll=1`, `first=page*10+1`, `SFX=page`, `cc`, `setlang`; `qft` para d/w/m/y. | `newsitem`; fecha aria-label, title/data-title, snippet, url, image, author. Fecha local/UTC específica; image se prefija Bing y corta en `&`. |
| DuckDuckGo / `bing` | Bootstrap VQD, luego GET `https://duckduckgo.com/news.js`; `l`, `o=json`, `noamp=1`, `q`, `vqd`, safe `1/-1/-2`; `df`; página `s=(page-1)*30`. | JSON mapea date/title/excerpt->body/url/image/source. |
| Yahoo / `yahoo` | GET `https://news.search.yahoo.com/search`; `p`; página `b=(page-1)*10+1`; `btf`. | XPath `web`/`li`; un único `try` para todo postproceso: fecha UTC relativa, URL `/RU=`, image `-/`, source ` via Yahoo`. Un item malo deja resto parcialmente sin limpiar. |

## Vídeos y libros

| Motor / provider | Secuencia y payload | Parseo y quirk |
| --- | --- | --- |
| DuckDuckGo vídeos / `bing` | Bootstrap VQD, luego GET `https://duckduckgo.com/v.js`; `l`, `o=json`, `q`, `vqd`, safe `1/-1/-2`, siempre `f` con slots `publishedAfter`, `videoDefinition`, `videoDuration`, `videoLicense`; página `s=(page-1)*60`. | JSON `results` conserva content, embed, imágenes, statistics y tipos anidados/dinámicos. |
| Anna's Archive / `annasarchive` | GET en TLD `gd/gl/pk` elegido una vez al cargar clase/módulo; `q`, `page` string. | Quita delimitadores HTML comment antes de XPath; prefija base URL a **todo** URL incluso absoluto. |

## Motor presente pero deshabilitado

Bing texto tiene clase Python completa pero `disabled=True`. Registro dinámico
lo excluye. Registro Go debe retener este estado; backend explícito `bing` es
inválido y cae por la ruta de warning/fallback `auto`, no activa Bing.

## Fixture mínimo por adaptador

1. Inputs, incluida diferencia omitido/valor explícito.
2. Reloj y aleatoriedad controlados en su ámbito correcto.
3. Secuencia ordenada de bootstrap/search/enrichment: método, URL, query/form,
   headers relevantes y cambios de cookies.
4. Respuestas/status saneados y salida raw normalizada o error clase/mensaje.
5. Orden de campos, XPath/JSON, postproceso y colisión de provider.

Esta documentación no sustituye parser lxml, prueba de fingerprint, renderer
ni fixtures diferenciales.
