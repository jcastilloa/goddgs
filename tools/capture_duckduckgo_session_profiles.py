#!/usr/bin/env python3
"""Capture frozen DDG HttpClient2 TLS/H2 templates against loopback only.

The frozen source creates its TLS context once in ``HttpClient2.__init__``.
It applies the HTTP/2 monkey patch around each request, but that code runs
only when httpcore creates an HTTP/2 connection.  This tool captures those
two lifetimes without contacting a search engine or a public endpoint.

It replaces the source ``SystemRandom`` only for the duration of each local
probe.  The replacement makes policy/template captures reproducible; Go still
uses crypto-safe entropy for every production client and new H2 connection.
"""

from __future__ import annotations

import argparse
import base64
import importlib.metadata
import json
import subprocess
import tempfile
from collections.abc import Sequence
from pathlib import Path
from typing import Any

from capture_browser_profiles import CapturedRequest, LoopbackProfileServer, generate_certificate
from ddgs import http_client2


SOURCE_COMMIT = "a12929a72429a39a0841c3d7caacb20ee17acd4d"
EXPECTED_VERSIONS = {
    "ddgs": "9.14.4",
    "h2": "4.3.0",
    "hpack": "4.2.0",
    "httpcore": "1.0.9",
    "httpx": "0.28.1",
    "hyperframe": "6.1.0",
}
POLICIES = (
    ("verify_default", 0, True),
    ("verify_max_tls12", 1, True),
    ("verify_min_tls13", 2, True),
    ("verify_no_ticket", 3, True),
    ("verify_off", None, False),
)


class FixedRandom:
    """Deterministic stand-in for the source's SystemRandom.

    The source obtains its policy with ``choice`` and shuffles only the
    configurable TLS 1.2 cipher suffix.  Returning the input ordering lets
    the generated file document a stable template instead of private entropy.
    HTTP/2 values use their inclusive lower bounds.
    """

    def __init__(self, policy_index: int) -> None:
        self.policy_index = policy_index

    def sample(self, population: Sequence[Any], sample_size: int) -> list[Any]:
        return list(population)[:sample_size]

    def choice(self, population: Sequence[Any]) -> Any:
        return population[self.policy_index]

    def randint(self, lower: int, _upper: int) -> int:
        return lower


class VerificationOffRandom:
    """Proves the disabled-verification constructor skips TLS randomization."""

    def sample(self, _population: Sequence[Any], _sample_size: int) -> list[Any]:
        raise AssertionError("verify=False must not sample source TLS ciphers")

    def choice(self, _population: Sequence[Any]) -> Any:
        raise AssertionError("verify=False must not choose a source TLS policy")

    def randint(self, lower: int, _upper: int) -> int:
        return lower


def assert_reference_environment(source_checkout: Path) -> None:
    source_checkout = source_checkout.resolve()
    revision = subprocess.run(
        ["git", "-C", str(source_checkout), "rev-parse", "HEAD"],
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()
    if revision != SOURCE_COMMIT:
        raise RuntimeError(f"source revision = {revision}, want {SOURCE_COMMIT}")
    subprocess.run(["git", "-C", str(source_checkout), "diff", "--quiet"], check=True)

    module_path = Path(http_client2.__file__).resolve()
    try:
        module_path.relative_to(source_checkout)
    except ValueError as error:
        raise RuntimeError(
            f"ddgs is imported from {module_path}, not editable source {source_checkout}; "
            "install the checkout into this exact environment first"
        ) from error
    for package, expected in EXPECTED_VERSIONS.items():
        actual = importlib.metadata.version(package)
        if actual != expected:
            raise RuntimeError(f"{package} = {actual}, want {expected}")


def capture_request(
    client: http_client2.HttpClient2,
    certificate: Path,
    private_key: Path,
) -> CapturedRequest:
    server = LoopbackProfileServer(certificate, private_key)
    server.start()
    request_error: Exception | None = None
    try:
        response = client.request("GET", f"https://localhost:{server.port}/ddg-profile")
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
    return server.result


def capture_policy(
    name: str,
    policy_index: int | None,
    verification_enabled: bool,
    certificate: Path,
    private_key: Path,
    root_certificate: Path,
) -> dict[str, Any]:
    source_random: Any
    if verification_enabled:
        assert policy_index is not None
        source_random = FixedRandom(policy_index)
        verify: bool | str = str(root_certificate)
    else:
        source_random = VerificationOffRandom()
        verify = False

    original_random = http_client2.random
    http_client2.random = source_random
    try:
        client = http_client2.HttpClient2(
            headers={"User-Agent": "goddgs-ddg-source-capture"},
            verify=verify,
            timeout=5,
        )
        first = capture_request(client, certificate, private_key)
        second = capture_request(client, certificate, private_key)
    finally:
        try:
            client.client.close()
        except UnboundLocalError:
            pass
        http_client2.random = original_random

    if first.settings != second.settings:
        raise AssertionError(f"{name}: deterministic H2 template differs across new connections")
    if first.connection_window != second.connection_window:
        raise AssertionError(f"{name}: deterministic connection window differs across new connections")
    return {
        "name": name,
        "verification_enabled": verification_enabled,
        "client_hello_base64": base64.b64encode(sanitize_client_hello_template(first.hello)).decode("ascii"),
        "http2": {
            "settings": first.settings,
            "connection_window_increment": first.connection_window,
            "stream_window_increment": first.stream_window,
            "initial_stream_id": first.stream_id,
            "priority": first.priority,
            "pseudo_order": first.pseudo_order,
            "headers": first.headers,
        },
    }


def sanitize_client_hello_template(raw: bytes) -> bytes:
    """Remove per-handshake entropy while retaining a valid uTLS template."""

    template = bytearray(raw)
    if len(template) < 43 or template[0] != 22 or template[5] != 1:
        raise ValueError("captured TLS record is not a ClientHello")
    if int.from_bytes(template[3:5], "big") + 5 != len(template):
        raise ValueError("captured ClientHello record length is invalid")
    handshake = memoryview(template)[5:]
    if int.from_bytes(handshake[1:4], "big") + 4 != len(handshake):
        raise ValueError("captured ClientHello handshake length is invalid")
    offset = 4 + 2
    handshake[offset : offset + 32] = bytes(32)
    offset += 32
    session_id_length = handshake[offset]
    offset += 1
    handshake[offset : offset + session_id_length] = bytes(session_id_length)
    offset += session_id_length
    cipher_length = int.from_bytes(handshake[offset : offset + 2], "big")
    offset += 2 + cipher_length
    offset += 1 + handshake[offset]
    extension_length = int.from_bytes(handshake[offset : offset + 2], "big")
    offset += 2
    end = offset + extension_length
    while offset < end:
        extension_type = int.from_bytes(handshake[offset : offset + 2], "big")
        content_length = int.from_bytes(handshake[offset + 2 : offset + 4], "big")
        offset += 4
        content_end = offset + content_length
        if content_end > end:
            raise ValueError("captured ClientHello extension is truncated")
        if extension_type == 51:
            key_share_length = int.from_bytes(handshake[offset : offset + 2], "big")
            key_share_end = offset + 2 + key_share_length
            if key_share_end != content_end:
                raise ValueError("captured ClientHello key-share extension is invalid")
            key_offset = offset + 2
            while key_offset < key_share_end:
                key_offset += 2
                key_length = int.from_bytes(handshake[key_offset : key_offset + 2], "big")
                key_offset += 2
                handshake[key_offset : key_offset + key_length] = bytes(key_length)
                key_offset += key_length
        offset = content_end
    if offset != end:
        raise ValueError("captured ClientHello extension length is invalid")
    return bytes(template)


def capture_profiles() -> dict[str, Any]:
    with tempfile.TemporaryDirectory(prefix="goddgs-ddg-profile-capture-") as temporary:
        certificate, private_key, root_certificate = generate_certificate(Path(temporary))
        profiles = [
            capture_policy(name, policy_index, verification_enabled, certificate, private_key, root_certificate)
            for name, policy_index, verification_enabled in POLICIES
        ]
    return {
        "schema_version": 1,
        "source": {
            "commit": SOURCE_COMMIT,
            "package": "ddgs.HttpClient2",
            "package_version": EXPECTED_VERSIONS["ddgs"],
            "python_packages": EXPECTED_VERSIONS,
            "capture": "deterministic source-random templates through loopback TLS/H2 only; no external request, cookies, credentials, or proxy",
        },
        "profiles": profiles,
    }


def semantic_profile(profile: dict[str, Any]) -> dict[str, Any]:
    """Exclude ClientHello entropy while retaining all template structure."""

    return {
        "name": profile["name"],
        "verification_enabled": profile["verification_enabled"],
        "client_hello": client_hello_shape(base64.b64decode(profile["client_hello_base64"])),
        "http2": profile["http2"],
    }


def client_hello_shape(raw: bytes) -> dict[str, Any]:
    """Return static ClientHello fields, excluding random/key-share bytes."""

    if len(raw) < 9 or raw[0] != 22 or raw[5] != 1:
        raise ValueError("captured TLS record is not a ClientHello")
    record_length = int.from_bytes(raw[3:5], "big")
    if record_length + 5 != len(raw):
        raise ValueError("captured ClientHello record length is invalid")
    handshake = raw[5:]
    if int.from_bytes(handshake[1:4], "big") + 4 != len(handshake):
        raise ValueError("captured ClientHello handshake length is invalid")
    offset = 4
    legacy_version = handshake[offset : offset + 2].hex()
    offset += 2 + 32
    offset += 1 + handshake[offset]
    cipher_length = int.from_bytes(handshake[offset : offset + 2], "big")
    offset += 2
    ciphers = [handshake[index : index + 2].hex() for index in range(offset, offset + cipher_length, 2)]
    offset += cipher_length
    offset += 1 + handshake[offset]
    extension_length = int.from_bytes(handshake[offset : offset + 2], "big")
    offset += 2
    end = offset + extension_length
    extensions: list[str] = []
    supported_versions: list[str] | None = None
    while offset < end:
        extension_type = int.from_bytes(handshake[offset : offset + 2], "big")
        content_length = int.from_bytes(handshake[offset + 2 : offset + 4], "big")
        offset += 4
        content = handshake[offset : offset + content_length]
        offset += content_length
        extensions.append(str(extension_type))
        if extension_type == 43:
            supported_versions = [content[index : index + 2].hex() for index in range(1, len(content), 2)]
    if offset != end:
        raise ValueError("captured ClientHello extension length is invalid")
    return {
        "legacy_version": legacy_version,
        "ciphers": ciphers,
        "extensions": extensions,
        "supported_versions": supported_versions,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=Path("internal/transport/duckduckgo_session_profiles_source.json"))
    parser.add_argument("--source-checkout", type=Path, default=Path("/home/jcastillo/Proyectos/ddgs"))
    parser.add_argument("--verify", action="store_true", help="recapture source templates and compare with the stored asset")
    args = parser.parse_args()
    assert_reference_environment(args.source_checkout)
    observed = capture_profiles()
    if args.verify:
        expected = json.loads(args.output.read_text(encoding="utf-8"))
        expected_profiles = [semantic_profile(profile) for profile in expected.get("profiles", [])]
        observed_profiles = [semantic_profile(profile) for profile in observed["profiles"]]
        if observed_profiles != expected_profiles:
            raise AssertionError("DDG source profile asset differs from fresh deterministic loopback capture")
        print(f"verified {len(observed_profiles)} DDG session/connection profile templates")
        return 0
    encoded = json.dumps(observed, indent=2, sort_keys=True) + "\n"
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(encoded, encoding="utf-8")
    print(f"wrote {len(observed['profiles'])} DDG session/connection profile templates to {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
