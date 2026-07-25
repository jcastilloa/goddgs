# Perfiles de navegador congelados

`ddgs.HttpClient` construye `primp.Client` con `impersonate="random"` e
`impersonate_os="random"`. No rota solo `User-Agent`: elige una identidad
completa una vez por cliente y la conserva con sus cookies y conexiones.

## Espacio fuente

- 23 variantes: Chrome 144–148, Edge 144–148, Opera 126–131, Safari 26,
  26.3 y 18.5, Firefox 140, 146, 147 y 148.
- Cinco SO: Android, iOS, Linux, macOS y Windows. Es el orden exacto de
  `IMPERSONATEOS_LIST` del binding Python.
- Dos elecciones uniformes e independientes: primero SO (`5`), después
  variante (`23`), para **115 pares**. El perfil se fija al construir cada
  `internal/transport.Client`; nunca se rota por request.

Cada par incluye, como un único bundle inmutable:

- ClientHello TLS, ALPN, grupos/cifrados/extensiones y padding;
- cabeceras por defecto y su orden;
- SETTINGS, incrementos de ventana, prioridad, stream inicial y orden de
  pseudo-cabeceras HTTP/2.

No existe un selector de cabeceras separado: `User-Agent` forma parte del
bundle y no puede divergir del TLS/H2 elegido.

## Artefacto y regeneración

`internal/transport/browser_profiles_source.json` se genera exclusivamente
contra TLS/H2 loopback desde el entorno Python congelado.

| Campo | Valor |
| --- | --- |
| Baseline DDGS | `a12929a72429a39a0841c3d7caacb20ee17acd4d` |
| `primp` resuelto | `1.3.1` |
| Pares capturados | `115` |
| SHA-256 del artefacto | `931866c179b009efb1d2813591e6f9729241d435018fe92434199e25987f1a7e` |
| Generador | `tools/capture_browser_profiles.py` |

El loader Go verifica commit, checksum, cardinalidad y cada combinación antes
de habilitar selección. El artefacto no contiene cookies, credenciales,
respuestas externas ni URLs proxy.

Regenerar exige revisar el baseline y el diff del JSON:

```bash
/tmp/goddgs-reference-a12929a/bin/python \
  tools/capture_browser_profiles.py
```

La verificación adicional hace pasar los 115 perfiles fuente por HTTP CONNECT
loopback y compara el bundle que ve el destino con el artefacto directo:

```bash
/tmp/goddgs-reference-a12929a/bin/python \
  tools/capture_browser_profiles.py --verify-http-connect
```

## Rutas

En destino HTTPS, el mismo bundle se usa en conexión directa, PEM propio,
`verify=false`, HTTP CONNECT, HTTPS CONNECT y SOCKS5/SOCKS5H. En túneles, TLS
del navegador se abre *después* de CONNECT/SOCKS; TLS exterior de un proxy
HTTPS es canal de control del proxy y conserva su política de certificado.

HTTP plano sigue transporte estándar: no recibe una identidad de navegador a
medias sin TLS/H2 que la respalde.

## Límites honestos

El artefacto guarda plantillas públicas de ClientHello capturadas localmente:
no contienen claves privadas, cookies ni credenciales. uTLS las reconstruye
por conexión y genera entropía nueva para random, key shares y GREASE. Las
pruebas comparan estructura JA3 sin GREASE, no bytes crudos. El cliente temporal `HttpClient2` de
DuckDuckGo sigue siendo un transporte distinto: no forma parte de estos 115
perfiles.
