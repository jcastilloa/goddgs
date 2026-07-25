# Perfiles de sesión DuckDuckGo

`ddgs.http_client2.HttpClient2` es distinto de `primp.Client`. La fuente
congelada construye un `ssl.SSLContext` solamente en el constructor si
`verify` es verdadero o una ruta PEM; en ese punto baraja la parte variable de
los cifrados y selecciona exactamente una política: normal, máximo TLS 1.2,
mínimo TLS 1.3 o sin ticket. Por tanto esa política pertenece al cliente que
la instancia `Duckduckgo` cacheada conserva durante la sesión `DDGS`.

Cada llamada entra temporalmente en `Patch`, pero el cuerpo de su reemplazo
`HTTP2Connection._send_connection_init` se ejecuta únicamente al abrir una
conexión H2. Para cada conexión nueva, la fuente sortea siete valores:

1. `INITIAL_WINDOW_SIZE` `[100, 200]`
2. `HEADER_TABLE_SIZE` `[4000, 5000]`
3. `MAX_FRAME_SIZE` `[16384, 65535]`
4. `MAX_CONCURRENT_STREAMS` `[100, 200]`
5. `MAX_HEADER_LIST_SIZE` `[65500, 66500]`
6. `ENABLE_CONNECT_PROTOCOL` `[0, 1]`
7. `ENABLE_PUSH` `[0, 1]`

El frame que observa el destino conserva el orden de `h2`: tabla, push,
ventana inicial, frame máximo, connect, concurrencia, lista de cabeceras;
después se emiten los incrementos de ventana de conexión y del primer stream
observados (`2**24`). Una conexión reutilizada no vuelve a emitir esa
inicialización. Con `verify=false` no se llama a `_get_random_ssl_context`,
pero la inicialización H2 sigue existiendo.

La plantilla guarda la parte fija de cada política. En producción se barajan
una vez por cliente los cifrados legacy que OpenSSL mantiene en el ClientHello;
el prefijo de política y `TLS_EMPTY_RENEGOTIATION_INFO_SCSV` quedan fijos. La
plantilla se reconstruye mediante uTLS por conexión, que genera de nuevo
random, key share y SNI; esos bytes no se comparten ni se prometen idénticos a
Python.

El artefacto
[`duckduckgo_session_profiles_source.json`](../internal/transport/duckduckgo_session_profiles_source.json)
se genera con:

```bash
/tmp/goddgs-ddg-probe-exact-a12929a/bin/python \
  tools/capture_duckduckgo_session_profiles.py
```

El script exige el checkout local `a12929a72429a39a0841c3d7caacb20ee17acd4d`
sin cambios, la instalación editable de ese checkout y CPython/dependencias
fijadas. Solo abre listeners TLS/H2 efímeros en loopback. El modo `--verify`
recaptura y compara semántica estática de TLS (sin random/key-share), H2 y
cabeceras; no realiza ninguna consulta de buscador.
