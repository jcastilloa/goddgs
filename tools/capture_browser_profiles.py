#!/usr/bin/env python3
"""Capture frozen primp browser profile bundles against loopback TLS/H2 only.

The output is runtime compatibility data for ``internal/transport``.  It is
intentionally separate from ``reference_capture.py --check`` because TLS
ClientHello contains per-handshake entropy.  The capture never contacts a
search engine or public endpoint: every request targets an ephemeral TLS
listener bound to 127.0.0.1.
"""

from __future__ import annotations

import argparse
import base64
import json
import os
import socket
import ssl
import subprocess
import tempfile
import threading
import time
from collections.abc import Iterable
from dataclasses import dataclass
from hashlib import sha256
from pathlib import Path
from typing import Any

import primp
from hpack import Decoder, Encoder


SOURCE_COMMIT = "a12929a72429a39a0841c3d7caacb20ee17acd4d"
PROFILES = (
    *(f"chrome_{version}" for version in range(144, 149)),
    *(f"edge_{version}" for version in range(144, 149)),
    *(f"opera_{version}" for version in range(126, 132)),
    "safari_26",
    "safari_26.3",
    "safari_18.5",
    "firefox_140",
    "firefox_146",
    "firefox_147",
    "firefox_148",
)
# This is primp-python's IMPERSONATEOS_LIST. The Python binding chooses this
# value before applying impersonate="random" to the client builder.
OPERATING_SYSTEMS = ("android", "ios", "linux", "macos", "windows")
CLIENT_PREFACE = b"PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"


def read_exact(connection: ssl.SSLSocket, size: int) -> bytes:
    result = bytearray()
    while len(result) < size:
        chunk = connection.recv(size - len(result))
        if not chunk:
            raise ConnectionError(f"EOF while reading {size} bytes")
        result.extend(chunk)
    return bytes(result)


def read_frame(connection: ssl.SSLSocket) -> tuple[int, int, int, bytes]:
    header = read_exact(connection, 9)
    size = int.from_bytes(header[:3], "big")
    return header[3], header[4], int.from_bytes(header[5:], "big") & 0x7FFFFFFF, read_exact(connection, size)


def frame(frame_type: int, flags: int, stream_id: int, payload: bytes = b"") -> bytes:
    return (
        len(payload).to_bytes(3, "big")
        + bytes((frame_type, flags))
        + (stream_id & 0x7FFFFFFF).to_bytes(4, "big")
        + payload
    )


def parse_settings(payload: bytes) -> list[list[int]]:
    if len(payload) % 6:
        raise ValueError(f"invalid SETTINGS payload length {len(payload)}")
    return [[int.from_bytes(payload[index : index + 2], "big"), int.from_bytes(payload[index + 2 : index + 6], "big")] for index in range(0, len(payload), 6)]


def capture_tls_record(raw_connection: socket.socket) -> bytes:
    # Keep bytes in the socket.  ``SSLContext.wrap_socket`` must consume the
    # same ClientHello after this observation.
    header = raw_connection.recv(5, socket.MSG_PEEK)
    if len(header) != 5:
        raise ConnectionError("TLS ClientHello record header incomplete")
    size = int.from_bytes(header[3:], "big")
    expected = 5 + size
    while True:
        record = raw_connection.recv(expected, socket.MSG_PEEK)
        if len(record) == expected:
            return record
        if not record:
            raise ConnectionError("TLS ClientHello record body incomplete")


@dataclass
class CapturedRequest:
    hello: bytes
    settings: list[list[int]]
    connection_window: int
    stream_window: int | None
    stream_id: int
    priority: dict[str, Any] | None
    pseudo_order: list[str]
    headers: list[list[str]]


class LoopbackProfileServer:
    def __init__(self, certificate: Path, private_key: Path) -> None:
        self.listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.listener.bind(("127.0.0.1", 0))
        self.listener.listen(1)
        self.listener.settimeout(8)
        self.port = self.listener.getsockname()[1]
        self.context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        self.context.load_cert_chain(certificate, private_key)
        self.context.set_alpn_protocols(["h2"])
        self.result: CapturedRequest | Exception | None = None
        self.done = threading.Event()
        self.thread = threading.Thread(target=self._serve, daemon=True)

    def start(self) -> None:
        self.thread.start()

    def close(self) -> None:
        self.thread.join(8)
        self.listener.close()
        if not self.done.is_set():
            raise RuntimeError("profile loopback server did not finish")
        if isinstance(self.result, Exception):
            raise self.result
        if self.result is None:
            raise RuntimeError("profile loopback server produced no capture")

    def _serve(self) -> None:
        raw_connection: socket.socket | None = None
        tls_connection: ssl.SSLSocket | None = None
        observed_frames: list[tuple[int, int, int, int]] = []
        try:
            raw_connection, _ = self.listener.accept()
            # Capturing the raw ClientHello must happen before TLS wraps socket.
            hello = capture_tls_record(raw_connection)
            tls_connection = self.context.wrap_socket(raw_connection, server_side=True)
            if tls_connection.selected_alpn_protocol() != "h2":
                raise RuntimeError(f"expected h2 ALPN, got {tls_connection.selected_alpn_protocol()!r}")
            if read_exact(tls_connection, len(CLIENT_PREFACE)) != CLIENT_PREFACE:
                raise RuntimeError("unexpected HTTP/2 client preface")

            settings: list[list[int]] = []
            connection_window = 0
            stream_window: int | None = None
            stream_id = 0
            priority: dict[str, Any] | None = None
            headers: list[tuple[str, str]] | None = None
            decoder = Decoder()
            while headers is None:
                frame_type, flags, received_stream_id, payload = read_frame(tls_connection)
                observed_frames.append((frame_type, flags, received_stream_id, len(payload)))
                if frame_type == 4 and not flags & 0x1:
                    settings = parse_settings(payload)
                    continue
                if frame_type == 8:
                    increment = int.from_bytes(payload, "big") & 0x7FFFFFFF
                    if received_stream_id == 0:
                        connection_window = increment
                    else:
                        stream_window = increment
                    continue
                if frame_type != 1:
                    continue
                stream_id = received_stream_id
                index = 0
                if flags & 0x20:
                    dependency = int.from_bytes(payload[:4], "big")
                    priority = {
                        "stream_dependency": dependency & 0x7FFFFFFF,
                        "exclusive": bool(dependency & 0x80000000),
                        "weight": payload[4] + 1,
                    }
                    index = 5
                if flags & 0x8:
                    padding = payload[index]
                    index += 1
                    payload = payload[: len(payload) - padding]
                headers = [(name, value) for name, value in decoder.decode(payload[index:])]

            # Firefox advertises an extra receive window for each new stream.
            # Capture frames emitted immediately after HEADERS before supplying
            # a response, otherwise a request-only probe would hide it.
            tls_connection.settimeout(0.15)
            while True:
                try:
                    frame_type, _, received_stream_id, payload = read_frame(tls_connection)
                except (TimeoutError, ssl.SSLWantReadError):
                    break
                observed_frames.append((frame_type, 0, received_stream_id, len(payload)))
                if frame_type == 8:
                    increment = int.from_bytes(payload, "big") & 0x7FFFFFFF
                    if received_stream_id == 0:
                        connection_window = increment
                    elif received_stream_id == stream_id:
                        stream_window = increment

            pseudo_order = [name[1:] for name, _ in headers if name.startswith(":")]
            self.result = CapturedRequest(
                hello=hello,
                settings=settings,
                connection_window=connection_window,
                stream_window=stream_window,
                stream_id=stream_id,
                priority=priority,
                pseudo_order=pseudo_order,
                headers=[[name, value] for name, value in headers if not name.startswith(":")],
            )
            response_headers = Encoder().encode(((":status", "200"), ("content-length", "2")))
            tls_connection.sendall(frame(4, 0, 0) + frame(1, 0x4, stream_id, response_headers) + frame(0, 0x1, stream_id, b"ok"))
            # h2 clients acknowledge SETTINGS asynchronously.  Closing while
            # that frame is still queued can turn a complete response into a
            # TCP reset.  This is only a loopback capture server, so drain a
            # bounded interval before orderly TLS close.
            tls_connection.settimeout(0.1)
            deadline = time.monotonic() + 0.3
            while time.monotonic() < deadline:
                try:
                    tls_connection.recv(16_384)
                except (TimeoutError, ssl.SSLWantReadError):
                    break
                except OSError:
                    break
        except Exception as error:  # propagated in parent thread
            self.result = RuntimeError(f"profile loopback capture failed after frames {observed_frames!r}: {error}")
        finally:
            if tls_connection is not None:
                tls_connection.close()
            elif raw_connection is not None:
                raw_connection.close()
            self.done.set()


class LoopbackHTTPConnectProxy:
    """One-request HTTP CONNECT tunnel used only for local source evidence."""

    def __init__(self) -> None:
        self.listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self.listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self.listener.bind(("127.0.0.1", 0))
        self.listener.listen(1)
        self.listener.settimeout(8)
        self.port = self.listener.getsockname()[1]
        self.result: str | Exception | None = None
        self.done = threading.Event()
        self.thread = threading.Thread(target=self._serve, daemon=True)

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self.port}"

    def start(self) -> None:
        self.thread.start()

    def close(self) -> str:
        self.thread.join(8)
        self.listener.close()
        if not self.done.is_set():
            raise RuntimeError("profile loopback CONNECT proxy did not finish")
        if isinstance(self.result, Exception):
            raise self.result
        if not isinstance(self.result, str):
            raise RuntimeError("profile loopback CONNECT proxy produced no target")
        return self.result

    def _serve(self) -> None:
        client: socket.socket | None = None
        target: socket.socket | None = None
        try:
            client, _ = self.listener.accept()
            client.settimeout(8)
            request = bytearray()
            while b"\r\n\r\n" not in request:
                chunk = client.recv(4096)
                if not chunk:
                    raise ConnectionError("EOF before CONNECT request headers")
                request.extend(chunk)
                if len(request) > 16_384:
                    raise ValueError("CONNECT request headers exceed loopback limit")
            first_line = bytes(request).split(b"\r\n", 1)[0].decode("ascii")
            method, target_address, version = first_line.split(" ", 2)
            if method != "CONNECT" or version != "HTTP/1.1":
                raise ValueError(f"unexpected CONNECT request {first_line!r}")
            host, port = target_address.rsplit(":", 1)
            target = socket.create_connection((host, int(port)), timeout=8)
            target.settimeout(None)
            client.settimeout(None)
            client.sendall(b"HTTP/1.1 200 Connection Established\r\n\r\n")
            self.result = target_address

            def forward(source: socket.socket, destination: socket.socket) -> None:
                try:
                    while True:
                        chunk = source.recv(16_384)
                        if not chunk:
                            break
                        destination.sendall(chunk)
                except OSError:
                    # The reverse direction may close the socket after the
                    # loopback target completes its response. That is normal
                    # tunnel teardown, not source-capture evidence.
                    pass
                finally:
                    try:
                        destination.shutdown(socket.SHUT_WR)
                    except OSError:
                        pass

            forward_thread = threading.Thread(target=forward, args=(client, target), daemon=True)
            forward_thread.start()
            forward(target, client)
            # `primp.Client` keeps pooled connections alive after materializing
            # the response. This one-shot local evidence proxy owns neither
            # pool, so close its relay immediately after the target ends rather
            # than waiting for the source client's keep-alive timeout.
            try:
                client.shutdown(socket.SHUT_RDWR)
            except OSError:
                pass
            forward_thread.join(0.5)
        except Exception as error:  # propagated in parent thread
            self.result = RuntimeError(f"profile loopback CONNECT proxy failed: {error}")
        finally:
            if target is not None:
                target.close()
            if client is not None:
                client.close()
            self.done.set()


def capture_one(
    profile: str,
    operating_system: str,
    certificate: Path,
    private_key: Path,
    root_certificate: Path,
) -> dict[str, Any]:
    server = LoopbackProfileServer(certificate, private_key)
    server.start()
    client = primp.Client(
        impersonate=profile,
        impersonate_os=operating_system,
        verify=True,
        ca_cert_file=str(root_certificate),
        timeout=5,
    )
    request_error: Exception | None = None
    try:
        response = client.get(f"https://localhost:{server.port}/profile")
        if response.status_code != 200 or response.content != b"ok":
            raise AssertionError(f"unexpected profile probe response {response.status_code!r}/{response.content!r}")
    except Exception as error:
        request_error = error
    finally:
        try:
            server.close()
        except Exception as server_error:
            if request_error is not None:
                raise RuntimeError(f"{request_error}; {server_error}") from server_error
            raise
    if request_error is not None:
        raise request_error
    assert isinstance(server.result, CapturedRequest)
    capture = server.result
    return {
        "profile": profile,
        "operating_system": operating_system,
        "client_hello_base64": base64.b64encode(capture.hello).decode("ascii"),
        "client_hello_sha256": sha256(capture.hello).hexdigest(),
        "http2": {
            "settings": capture.settings,
            "connection_window_increment": capture.connection_window,
            "stream_window_increment": capture.stream_window,
            "initial_stream_id": capture.stream_id,
            "priority": capture.priority,
            "pseudo_order": capture.pseudo_order,
            "headers": capture.headers,
        },
    }


def capture_one_via_http_connect(
    profile: str,
    operating_system: str,
    certificate: Path,
    private_key: Path,
    root_certificate: Path,
) -> dict[str, Any]:
    """Capture a target-side source profile after an HTTP CONNECT tunnel.

    The proxy and target are both loopback-only. This does not make a public
    request and does not persist raw entropy-bearing ClientHello bytes.
    """

    server = LoopbackProfileServer(certificate, private_key)
    proxy = LoopbackHTTPConnectProxy()
    server.start()
    proxy.start()
    request_error: Exception | None = None
    try:
        client = primp.Client(
            proxy=proxy.url,
            impersonate=profile,
            impersonate_os=operating_system,
            verify=True,
            ca_cert_file=str(root_certificate),
            timeout=5,
        )
        response = client.get(f"https://localhost:{server.port}/profile")
        if response.status_code != 200 or response.content != b"ok":
            raise AssertionError(f"unexpected tunnel profile response {response.status_code!r}/{response.content!r}")
    except Exception as error:
        request_error = error
    finally:
        try:
            # The target sends the complete response before this point. Close
            # it first so its target→proxy relay gets EOF; waiting for the
            # proxy first would deadlock against a pooled source connection.
            server.close()
            target = proxy.close()
            if target != f"localhost:{server.port}":
                raise AssertionError(f"CONNECT target {target!r} does not match loopback profile server")
        except Exception as tunnel_error:
            if request_error is not None:
                raise RuntimeError(f"{request_error}; {tunnel_error}") from tunnel_error
            raise
    if request_error is not None:
        raise request_error
    assert isinstance(server.result, CapturedRequest)
    capture = server.result
    return {
        "profile": profile,
        "operating_system": operating_system,
        "http2": {
            "settings": capture.settings,
            "connection_window_increment": capture.connection_window,
            "stream_window_increment": capture.stream_window,
            "initial_stream_id": capture.stream_id,
            "priority": capture.priority,
            "pseudo_order": capture.pseudo_order,
            "headers": capture.headers,
        },
    }


def verify_http_connect_profiles(
    asset: dict[str, Any],
    certificate: Path,
    private_key: Path,
    root_certificate: Path,
) -> None:
    """Prove all source profiles preserve their target H2 bundle via CONNECT."""

    entries = asset.get("profiles")
    if not isinstance(entries, list) or len(entries) != len(PROFILES) * len(OPERATING_SYSTEMS):
        raise ValueError("browser profile asset does not contain the expected 23-by-5 source outcomes")
    expected = {(entry["profile"], entry["operating_system"]): entry["http2"] for entry in entries}
    for profile in PROFILES:
        for operating_system in OPERATING_SYSTEMS:
            observed = capture_one_via_http_connect(profile, operating_system, certificate, private_key, root_certificate)
            key = (profile, operating_system)
            if observed["http2"] != expected[key]:
                raise AssertionError(f"HTTP CONNECT profile mismatch for {profile}/{operating_system}")


def generate_certificate(directory: Path) -> tuple[Path, Path, Path]:
    root_key = directory / "root-key.pem"
    root_certificate = directory / "root-cert.pem"
    certificate = directory / "cert.pem"
    private_key = directory / "key.pem"
    subprocess.run(
        [
            "openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
            "-keyout", str(root_key), "-out", str(root_certificate), "-subj", "/CN=goddgs profile capture CA",
            "-addext", "basicConstraints=critical,CA:TRUE",
            "-addext", "keyUsage=critical,keyCertSign,cRLSign",
        ],
        check=True,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    request = directory / "server.csr"
    extensions = directory / "server.ext"
    extensions.write_text(
        "basicConstraints=critical,CA:FALSE\n"
        "keyUsage=critical,digitalSignature,keyEncipherment\n"
        "extendedKeyUsage=serverAuth\n"
        "subjectAltName=DNS:localhost,IP:127.0.0.1\n",
        encoding="utf-8",
    )
    subprocess.run(
        [
            "openssl", "req", "-newkey", "rsa:2048", "-nodes",
            "-keyout", str(private_key), "-out", str(request), "-subj", "/CN=localhost",
        ],
        check=True,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    subprocess.run(
        [
            "openssl", "x509", "-req", "-days", "1", "-in", str(request),
            "-CA", str(root_certificate), "-CAkey", str(root_key), "-CAcreateserial",
            "-out", str(certificate), "-extfile", str(extensions),
        ],
        check=True,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    return certificate, private_key, root_certificate


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path("internal/transport/browser_profiles_source.json"))
    parser.add_argument(
        "--verify-http-connect",
        action="store_true",
        help="compare every source profile's loopback CONNECT target H2/header bundle with the generated asset",
    )
    args = parser.parse_args()
    if args.verify_http_connect:
        asset = json.loads(args.output.read_text(encoding="utf-8"))
        with tempfile.TemporaryDirectory(prefix="goddgs-profile-connect-verify-") as temporary:
            certificate, private_key, root_certificate = generate_certificate(Path(temporary))
            verify_http_connect_profiles(asset, certificate, private_key, root_certificate)
        print(f"verified {len(PROFILES) * len(OPERATING_SYSTEMS)} browser profile bundles through loopback HTTP CONNECT")
        return 0
    with tempfile.TemporaryDirectory(prefix="goddgs-profile-capture-") as temporary:
        certificate, private_key, root_certificate = generate_certificate(Path(temporary))
        entries = [
            capture_one(profile, operating_system, certificate, private_key, root_certificate)
            for profile in PROFILES
            for operating_system in OPERATING_SYSTEMS
        ]
    output = {
        "schema_version": 1,
        "source": {
            "commit": SOURCE_COMMIT,
            "package": "primp",
            "package_version": "1.3.1",
            "capture": "loopback TLS/H2 only; no external request, cookies, or credentials",
        },
        "profiles": entries,
    }
    encoded = json.dumps(output, indent=2, sort_keys=True) + "\n"
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(encoded, encoding="utf-8")
    print(f"wrote {len(entries)} browser profile bundles to {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
