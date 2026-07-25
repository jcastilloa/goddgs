# Compuerta de fingerprint de transporte

Esta evidencia aplica a la línea fuente congelada
`a12929a72429a39a0841c3d7caacb20ee17acd4d`. No contiene una consulta de
buscador, cookies, URL con credenciales, IP de cliente ni respuesta cruda.

## Observación controlada — 2026-07-25

Se hicieron peticiones aisladas a un endpoint de diagnóstico TLS/HTTP2. La
ejecución es explícita y etiquetada:

```bash
GODDGS_FINGERPRINT_ENDPOINT=https://tls.peet.ws/api/all \
  go test -race -count=1 -v -tags=integration \
  -run 'LiveFingerprintObservation$' ./internal/transport
```

La prueba no tiene endpoint por defecto: sin `GODDGS_FINGERPRINT_ENDPOINT`
queda omitida. Solo registra versión HTTP y hashes sanitizados. En particular,
no conserva ni imprime la respuesta completa del servicio.

| Cliente observado | HTTP | JA3 | JA4 | Hash Akamai HTTP/2 | Resultado |
| --- | --- | --- | --- | --- | --- |
| Python `HttpClient` / `primp 1.3.1` | `h2` | `091b5b7b79dabfb28d2e6d498acabcfd` | `t13d1516h2_8daaf6152771_d8a2da3f94cd` | `52d84b11737d980aef856699f885ca86` | Referencia base observada. |
| Python `HttpClient2` / `httpx`+parche H2 | `h2` | `c8fe1b5352a80f2346fd55f755c75d54` | `t13d2311h1_60e844d7c027_c9e58cc61e36` | `9f8937ed5d7df159714246e27fa34a03` | Referencia temporal DDG observada. |
| Go `transport.Client` estándar, antes de 5.6 | `h2` | `03117a8ed39ef02427ebbc39f121275c` | `t13d1312h2_f57a46bbacb6_ab7e3b40a677` | `cbcbfae223bb97a0cc79109588321a5c` | Referencia de la divergencia de `net/http`. |
| Go `DuckDuckGoTextClient` | `h2` | `03117a8ed39ef02427ebbc39f121275c` | `t13d1312h2_f57a46bbacb6_ab7e3b40a677` | `cbcbfae223bb97a0cc79109588321a5c` | No equivale a `HttpClient2`. |
| Go `transport.Client`, Chrome 146/Windows | `h2` | `091b5b7b79dabfb28d2e6d498acabcfd` | `t13d1516h2_8daaf6152771_d8a2da3f94cd` | `52d84b11737d980aef856699f885ca86` | Igual a `primp` explícito en esta pareja. |
| Go `transport.Client`, Edge 148/Linux | `h2` | `211fd42460abf41337bf9d7ef016053f` | `t13d1516h2_8daaf6152771_d8a2da3f94cd` | `52d84b11737d980aef856699f885ca86` | Igual a `primp` explícito en esta pareja. |
| Go `transport.Client`, Opera 131/Android | `h2` | `ce07d5a8351d54b78bf83ebd79ac5cc7` | `t13d1516h2_8daaf6152771_d8a2da3f94cd` | `52d84b11737d980aef856699f885ca86` | Igual a `primp` explícito en esta pareja. |
| Go `transport.Client`, Safari 26.3/macOS | `h2` | `234b556820a0a1287f1cf50bf49aa910` | `t13d2014h2_a09f3c656075_7f0f34a4126d` | `c52879e43202aeb92740be6e8c86ea96` | Igual a `primp` explícito en esta pareja; se reobservó Python contra el mismo endpoint. |
| Go `transport.Client`, Firefox 148/iOS | `h2` | `6f7889b9fb1a62a9577e685c1fcfa919` | `t13d1717h2_5b57614c22b0_3cbfd9057e0d` | `6ea73faa8fc5aac76bded7bd238f6433` | Igual a `primp` explícito en esta pareja. |

La implementación no rota encabezados: elige al construir cada cliente uno de
los 115 bundles congelados (`23` navegadores/versiones × `5` SO). Cada bundle
incluye ClientHello, padding, ALPN, cabeceras y orden, SETTINGS/ventanas,
prioridad, stream inicial y pseudo-cabeceras H2. uTLS reconstruye el
ClientHello por conexión, por lo que random/key shares/GREASE vuelven a tener
entropía nueva sin compartir estado entre orígenes.

Las pruebas locales verifican los 115 pares: semántica JA3 sin GREASE,
prefacio/SETTINGS/ventanas/prioridad/stream/pseudo-cabeceras, orden de headers,
reutilización, cancelación y reintento. También prueban el bundle completo en
destino HTTPS directo, PEM, `verify=false`, HTTP CONNECT (los 115 pares),
HTTPS CONNECT y SOCKS5/SOCKS5H. El capturador Python verifica adicionalmente
los 115 perfiles fuente a través de HTTP CONNECT loopback.

Los cinco pares diagnósticos coinciden con su observación explícita de
`primp`. Los valores de entropía TLS siguen siendo dinámicos, por lo que este
resultado no implica igualdad byte a byte. `HttpClient2` de DuckDuckGo es otro
cliente fuente y sigue sin equivalencia de fingerprint.

## Matriz por motor activo

| Categoría | Motores fuente | Cliente fuente | Fixtures de transporte/adapter | Estado fingerprint |
| --- | --- | --- | --- | --- |
| Text | Brave, Google, Grokipedia, Mojeek, Startpage, Wikipedia, Yahoo, Yandex | `primp.HttpClient` | Fixtures verdes; 115 bundles y rutas HTTPS locales | Selección fuente completa; los cinco pares diagnósticos coinciden. |
| Text | DuckDuckGo | `HttpClient2` | Verde HTTP/2 local, redirección y ciclo de vida | No equivalente: cliente temporal TLS/H2 fuente no portado. |
| Images | Bing, DuckDuckGo | `primp.HttpClient` | Fixtures verdes; 115 bundles y rutas HTTPS locales | Selección fuente completa; los cinco pares diagnósticos coinciden. |
| News | Bing, DuckDuckGo, Yahoo | `primp.HttpClient` | Fixtures verdes; 115 bundles y rutas HTTPS locales | Selección fuente completa; los cinco pares diagnósticos coinciden. |
| Videos | DuckDuckGo | `primp.HttpClient` | Fixtures verdes; 115 bundles y rutas HTTPS locales | Selección fuente completa; los cinco pares diagnósticos coinciden. |
| Books | Anna's Archive | `primp.HttpClient` | Fixtures verdes; 115 bundles y rutas HTTPS locales | Selección fuente completa; los cinco pares diagnósticos coinciden. |

Texto Bing permanece deshabilitado en fuente y no se activa por esta matriz.

## Decisión

El artefacto, procedencia y comandos de regeneración están en
[`browser-profiles.md`](browser-profiles.md). No se aprueba equivalencia para
`HttpClient2`; toda extensión requiere revisar licencia/cgo/supply-chain,
aislamiento y observación controlada.
