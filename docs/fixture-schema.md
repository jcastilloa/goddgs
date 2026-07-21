# Esquema de fixtures diferenciales

El contrato canónico está en
[`testdata/contracts/schema.json`](../testdata/contracts/schema.json). Cada
fixture describe una observación del Python congelado, no una expectativa
inventada para Go.

## Reglas

- Un fixture corresponde a un caso observable y tiene `fixture_id` estable.
- `source.commit` siempre es SHA de 40 caracteres. No se acepta solamente
  versión `9.14.4` porque el HEAD objetivo está después del tag.
- `source.reference_environment` apunta a
  `docs/reference-environment.md`; cualquier recaptura debe comparar las
  versiones resueltas antes de sustituir expectativas. Cada fixture también
  guarda `source.resolved_packages` para que la procedencia quede junto al
  resultado, no sólo en documentación.
- `contract.kind` vale `pure`, `engine`, `extract`, `parser` o `transport`.
  Los casos de motor y parser deben indicar categoría, motor y operación.
- Los ficheros se separan por clase: `testdata/contracts/pure/`,
  `testdata/contracts/engine/`, `testdata/contracts/extract/`,
  `testdata/contracts/parser/` y `testdata/contracts/transport/`. El
  capturador conserva `--output` para los puros y expone un destino explícito
  para cada clase restante.
- `trace` conserva secuencia, no sólo petición final: bootstrap VQD, cookies,
  Startpage `sc`, Wikipedia extract y redirects son observables.
- Una entrada `response` sintética conserva `status`, `text` y `content_hex`.
  Esto entrega al futuro test Go el mismo input saneado que consumió el parser
  Python; no se guardan respuestas live.
- `input` conserva presencia/ausencia de opciones; no colapsar omitido,
  `null`, cadena vacía, cero y valor explícito.
- `controls` explica reloj, zona horaria y aleatoriedad. Si la fuente escoge
  valor al cargar módulo/clase, el fixture debe decirlo.
- `result.output` admite JSON heterogéneo. `result.field_order` guarda el
  orden de campos por resultado cuando afecte deduplicación o comparación.
- Errores se comparan por clase y mensaje; `cause_type` es opcional y puede
  ser `null` cuando la excepción Python no tiene causa.
- Las respuestas, URLs, cookies, headers y errores se saneán antes de
  versionar. Nunca guardar credenciales, proxy con usuario/clave, sesiones,
  datos privados ni HTML de terceros sin revisión.

## Captura sintética de motores

Las fixtures bajo `engine/` se ejecutan contra un doble en proceso de
`ddgs.base.HttpClient`. Sus requests, respuestas, cookies y valores aleatorios
son sintéticos; el doble falla si la fuente intenta una request no prevista.
Por tanto prueban la secuencia y forma que el motor solicita, pero **no**
demuestran fingerprint TLS/HTTP2, cierre de cuerpos, parser lxml ni
conectividad. Esos contratos pertenecen a las gates de transporte y parser.

Los valores decididos al cargar el módulo —por ejemplo el User-Agent de Google—
se sustituyen por un marcador estable. Valores aleatorios por búsqueda se
parchean dentro del capturador y se registran como eventos `random`.

El capturador valida una matriz mínima contra registro activo de fuente
congelada. Cada par categoría/motor debe tener búsqueda exitosa con `page`,
`region`, `safesearch` y `timelimit` explícitos, un `200` vacío, un `200`
malformado y un `503` que devuelve `None`. Si registro cambia, recaptura falla
hasta revisar matriz; motor nuevo no se omite en silencio.

## Captura sintética de extracción

Las fixtures bajo `extract/` levantan sólo un servidor HTTP efímero en loopback
con HTML o bytes sintéticos. La URL real de loopback se reemplaza por
`https://extract.fixture/page` antes de escribir. La captura conserva
constructor `HttpClient`, GET, status, bytes/text sintéticos y la única
propiedad de respuesta consultada. Por eso congela la selección de formato y
la salida del `primp` resuelto, pero no aprueba todavía un renderer o transporte
Go.

## Captura sintética de transporte

Las fixtures bajo `transport/` observan el constructor fuente con una doble
local de `primp.Client` y comportamiento HTTP con un servidor loopback efímero
de payload sintético. Cubren configuración observable, cookies, redirects,
compresión, estados no-200 y timeout; una doble SOCKS local también congela la
diferencia entre resolución `socks5` local y `socks5h` remota. La URL loopback
se reescribe antes de versionar. No contienen proxies externos ni prueban
fingerprint de navegador, TLS/HTTP2 de `primp`, ni la elección de cliente Go:
esas siguen siendo gates de transporte.

Antes de cada escritura, el capturador rechaza URL con userinfo, loopback no
permitido, rutas locales, headers de autenticación y nombres de cookie que
parecen secreto/sesión/token. Las cookies de payload de motor restantes son
valores estáticos sintéticos revisados, no cookies de sesión.

## Captura sintética de parser

Las fixtures bajo `parser/` ejecutan el `lxml` congelado con el mismo
`HTMLParser(remove_blank_text=True, remove_comments=True, remove_pis=True,
collect_ids=False)` que `BaseSearchEngine`. Conservan HTML sintético, XPath
fuente, valores XPath crudos, valor unido con el `"".join(...).split()` de
fuente y `elements_order`. Esa lista conserva el orden de `elements_xpath` de
Python; no se puede recuperar de un objeto JSON/mapa Go. Incluyen selectores de
documento, uniones, atributos, recuperación HTML malformada y el
preprocesado de Anna's Archive. No guardan HTML de terceros ni autorizan un
selector reescrito para acomodar un parser Go.

Las mismas fixtures incluyen operaciones `json_loads` de motores JSON:
Grokipedia, metadata `m` de Bing Images y respuestas DuckDuckGo de imágenes,
noticias y vídeo. Congelan claves ausentes frente a `null`, mapas/listas
anidados, valores mixtos y errores de JSON truncado o con segundo valor. Go
debe conservar números como `json.Number`; el orden de objetos JSON no se usa
para resultado porque los adaptadores llevan orden declarado de campos fuente.

## Ejemplo mínimo puro

```json
{
  "schema_version": 1,
  "fixture_id": "pure.normalize-url-space-plus",
  "source": {
    "commit": "a12929a72429a39a0841c3d7caacb20ee17acd4d",
    "package_version": "9.14.4",
    "reference_environment": "docs/reference-environment.md"
  },
  "contract": { "kind": "pure", "operation": "normalize_url" },
  "input": { "value": "https://example.test/a%20b+c" },
  "controls": { "clock": "not used", "random": "not used" },
  "trace": [],
  "result": { "status": "ok", "output": "https://example.test/a+b+c" },
  "redaction": { "sanitized": true, "rules": ["synthetic URL"] }
}
```

## Flujo

1. Ejecutar el capturador con el intérprete del entorno de referencia.
2. Revisar diffs y aplicar saneado explícito.
3. Validar estructura contra el esquema. El capturador valida las invariantes
   de toda salida que genera, incluidas formas de resultado/error, secuencia
   de trace, procedencia y redacción; un validador JSON Schema externo puede
   añadirse después sólo si aporta cobertura distinta. El entorno de referencia
   actual no instala `jsonschema`, y el flujo no lo exige.
4. Escribir el test Go RED contra el fixture antes de código Go.
5. No editar un expected para ocultar una divergencia: documentar primero el
   cambio de baseline mediante OpenSpec.
