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
| Go `transport.Client` | `h2` | `03117a8ed39ef02427ebbc39f121275c` | `t13d1312h2_f57a46bbacb6_ab7e3b40a677` | `cbcbfae223bb97a0cc79109588321a5c` | No equivale a `primp`. |
| Go `DuckDuckGoTextClient` | `h2` | `03117a8ed39ef02427ebbc39f121275c` | `t13d1312h2_f57a46bbacb6_ab7e3b40a677` | `cbcbfae223bb97a0cc79109588321a5c` | No equivale a `HttpClient2`. |

El resultado es deliberadamente una **no-aprobación**: HTTP/2 negociado no
prueba ClientHello ni SETTINGS HTTP/2 equivalentes. Los valores fuente pueden
variar por su aleatorización; una coincidencia aislada tampoco aprobaría la
paridad.

## Matriz por motor activo

| Categoría | Motores fuente | Cliente fuente | Fixtures de transporte/adapter | Estado fingerprint |
| --- | --- | --- | --- | --- |
| Text | Brave, Google, Grokipedia, Mojeek, Startpage, Wikipedia, Yahoo, Yandex | `primp.HttpClient` | Verdes offline | No demostrado; no afirmar paridad 1:1. |
| Text | DuckDuckGo | `HttpClient2` | Verde HTTP/2 local, redirección y ciclo de vida | No demostrado; no afirmar paridad 1:1. |
| Images | Bing, DuckDuckGo | `primp.HttpClient` | Verdes offline | No demostrado; no afirmar paridad 1:1. |
| News | Bing, DuckDuckGo, Yahoo | `primp.HttpClient` | Verdes offline | No demostrado; no afirmar paridad 1:1. |
| Videos | DuckDuckGo | `primp.HttpClient` | Verdes offline | No demostrado; no afirmar paridad 1:1. |
| Books | Anna's Archive | `primp.HttpClient` | Verdes offline | No demostrado; no afirmar paridad 1:1. |

Texto Bing permanece deshabilitado en fuente y no se activa por esta matriz.

## Decisión

La tarea 5.5 queda cerrada como **compuerta de evidencia**, no como aprobación
de equivalencia. Todo motor de la matriz se mantiene explícitamente incompleto
en la dimensión browser TLS/HTTP2. Adoptar un transporte nuevo exige un nuevo
cambio OpenSpec, revisión de licencia/cgo/supply chain, pruebas de aislamiento
por petición y nuevas observaciones por cliente/engine afectado.
