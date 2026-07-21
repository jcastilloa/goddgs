#!/usr/bin/env python3
"""Capture deterministic, offline contracts from frozen DDGS Python source.

Run this only with the isolated interpreter recorded in
docs/reference-environment.md. The script deliberately has no live-engine
mode: engine traces use synthetic in-process HTTP clients only.
"""

from __future__ import annotations

import argparse
import gzip
import importlib
import importlib.metadata
import json
import math
import os
import platform
import re
import socket
import subprocess
import sys
import threading
import time
from binascii import hexlify
from collections.abc import Callable
from concurrent.futures import FIRST_EXCEPTION, Future, wait
from html import escape
from html.entities import html5
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from types import SimpleNamespace
from typing import Any
from urllib.parse import urlsplit

import ddgs
from lxml import html
from lxml.etree import HTMLParser as LHTMLParser
from ddgs.engines.annasarchive import AnnasArchive
from ddgs.engines.bing_images import BingImages
from ddgs.engines.bing_news import BingNews
from ddgs.engines.duckduckgo import Duckduckgo
from ddgs.engines.duckduckgo_videos import DuckduckgoVideos
from ddgs.engines.duckduckgo_images import DuckduckgoImages
from ddgs.engines.duckduckgo_news import DuckduckgoNews
from ddgs.engines import ENGINES
from ddgs.engines.bing import Bing
from ddgs.engines.brave import Brave
from ddgs.engines.google import Google
from ddgs.engines.grokipedia import Grokipedia
from ddgs.engines.mojeek import Mojeek
from ddgs.engines.startpage import Startpage
from ddgs.engines.wikipedia import Wikipedia
from ddgs.engines.yahoo import Yahoo
from ddgs.engines.yahoo_news import YahooNews
from ddgs.engines.yandex import Yandex
from ddgs.exceptions import DDGSException, TimeoutException
from ddgs.results import BooksResult, ImagesResult, NewsResult, ResultsAggregator, TextResult, VideosResult
from ddgs.similarity import SimpleFilterRanker
from ddgs.utils import _expand_proxy_tb_alias, _extract_vqd, _normalize_date, _normalize_text, _normalize_url

SOURCE_SHA = "a12929a72429a39a0841c3d7caacb20ee17acd4d"
REFERENCE_ENVIRONMENT = "docs/reference-environment.md"
SCHEMA_VERSION = 1

REFERENCE_DISTRIBUTIONS = {
    "anyio": "anyio",
    "brotli": "brotli",
    "certifi": "certifi",
    "click": "click",
    "ddgs": "ddgs",
    "fake-useragent": "fake-useragent",
    "h11": "h11",
    "h2": "h2",
    "hpack": "hpack",
    "httpcore": "httpcore",
    "httpx": "httpx",
    "hyperframe": "hyperframe",
    "idna": "idna",
    "lxml": "lxml",
    "primp": "primp",
    "socksio": "socksio",
    "typing-extensions": "typing_extensions",
}

Fixture = dict[str, Any]

PURE_REDACTION = {
    "sanitized": True,
    "rules": ["synthetic inputs", "no external request", "no credentials or cookies"],
}
ENGINE_REDACTION = {
    "sanitized": True,
    "rules": ["synthetic inputs and responses", "no external request", "no credentials or live session cookies"],
}
EXTRACT_REDACTION = ENGINE_REDACTION
PARSER_REDACTION = ENGINE_REDACTION
TRANSPORT_REDACTION = ENGINE_REDACTION
URL_RE = re.compile(r"(?:https?|socks5h?)://[^\s\"'<>]+")
LOCAL_HOSTS = {"127.0.0.1", "::1", "localhost"}
ALLOWED_LOOPBACK_URLS = {"socks5h://127.0.0.1:9150"}
SENSITIVE_COOKIE_PARTS = ("auth", "key", "pass", "secret", "session", "token")
SENSITIVE_HEADER_NAMES = {"authorization", "cookie", "proxy-authorization", "x-api-key"}
PRIVATE_PATH_MARKERS = ("/home/", "/tmp/", "file://")


def _resolved_packages() -> dict[str, str]:
    return {
        name: importlib.metadata.version(distribution)
        for name, distribution in sorted(REFERENCE_DISTRIBUTIONS.items())
    }


def _source() -> dict[str, Any]:
    return {
        "commit": SOURCE_SHA,
        "package_version": ddgs.__version__,
        "reference_environment": REFERENCE_ENVIRONMENT,
        "python": platform.python_version(),
        "resolved_packages": _resolved_packages(),
    }


def _fixture(
    fixture_id: str,
    operation: str,
    input_value: dict[str, Any],
    result: dict[str, Any],
    *,
    trace: list[dict[str, Any]] | None = None,
    clock: str = "not used",
    random: str = "not used",
    notes: list[str] | None = None,
) -> Fixture:
    controls: dict[str, Any] = {"clock": clock, "random": random}
    if notes:
        controls["notes"] = notes
    return {
        "schema_version": SCHEMA_VERSION,
        "fixture_id": fixture_id,
        "source": _source(),
        "contract": {"kind": "pure", "operation": operation},
        "input": input_value,
        "controls": controls,
        "trace": trace or [],
        "result": result,
        "redaction": {
            **PURE_REDACTION,
        },
    }


def _engine_fixture(
    fixture_id: str,
    category: str,
    engine: str,
    operation: str,
    input_value: dict[str, Any],
    result: dict[str, Any],
    *,
    trace: list[dict[str, Any]],
    clock: str = "not used",
    random: str = "not used",
    notes: list[str] | None = None,
) -> Fixture:
    fixture = _fixture(
        fixture_id,
        operation,
        input_value,
        result,
        trace=trace,
        clock=clock,
        random=random,
        notes=notes,
    )
    fixture["contract"] = {
        "kind": "engine",
        "category": category,
        "engine": engine,
        "operation": operation,
    }
    fixture["redaction"] = dict(ENGINE_REDACTION)
    return fixture


def _extract_fixture(
    fixture_id: str,
    operation: str,
    input_value: dict[str, Any],
    result: dict[str, Any],
    *,
    trace: list[dict[str, Any]],
    notes: list[str] | None = None,
) -> Fixture:
    fixture = _fixture(fixture_id, operation, input_value, result, trace=trace, notes=notes)
    fixture["contract"] = {"kind": "extract", "operation": operation}
    fixture["redaction"] = dict(EXTRACT_REDACTION)
    return fixture


def _parser_fixture(
    fixture_id: str,
    category: str,
    engine: str,
    operation: str,
    input_value: dict[str, Any],
    result: dict[str, Any],
    *,
    notes: list[str] | None = None,
) -> Fixture:
    """Record lxml/XPath behavior without a request or transport double."""
    fixture = _fixture(fixture_id, operation, input_value, result, notes=notes)
    fixture["contract"] = {
        "kind": "parser",
        "category": category,
        "engine": engine,
        "operation": operation,
    }
    fixture["redaction"] = dict(PARSER_REDACTION)
    return fixture


def _transport_fixture(
    fixture_id: str,
    operation: str,
    input_value: dict[str, Any],
    result: dict[str, Any],
    *,
    trace: list[dict[str, Any]],
    notes: list[str] | None = None,
) -> Fixture:
    """Record source HttpClient behavior without an external destination."""
    fixture = _fixture(fixture_id, operation, input_value, result, trace=trace, notes=notes)
    fixture["contract"] = {"kind": "transport", "operation": operation}
    fixture["redaction"] = dict(TRANSPORT_REDACTION)
    return fixture


def _ok(output: Any, *, field_order: list[list[str]] | None = None) -> dict[str, Any]:
    result: dict[str, Any] = {"status": "ok", "output": output}
    if field_order is not None:
        result["field_order"] = field_order
    return result


def _error(action: Callable[[], Any]) -> dict[str, Any]:
    try:
        action()
    except Exception as exc:  # frozen source deliberately exposes varied failures
        return _error_from_exception(exc)
    raise AssertionError("reference operation unexpectedly succeeded")


def _error_from_exception(exc: Exception) -> dict[str, Any]:
    return {
        "status": "error",
        "error": {
            "type": type(exc).__name__,
            "message": str(exc),
            "cause_type": type(exc.__cause__).__name__ if exc.__cause__ else None,
        },
    }


def _validate_fixture(fixture: Fixture) -> None:
    required = {
        "schema_version",
        "fixture_id",
        "source",
        "contract",
        "input",
        "controls",
        "trace",
        "result",
        "redaction",
    }
    if set(fixture) != required:
        raise ValueError(f"{fixture.get('fixture_id')}: invalid root keys")
    if fixture["schema_version"] != SCHEMA_VERSION:
        raise ValueError("unexpected schema version")
    if fixture["source"]["commit"] != SOURCE_SHA:
        raise ValueError("fixture source SHA drift")
    if fixture["source"].get("resolved_packages") != _resolved_packages():
        raise ValueError("fixture resolved package provenance drift")
    contract = fixture["contract"]
    kind = contract.get("kind")
    if kind == "pure":
        if set(contract) != {"kind", "operation"} or not isinstance(contract.get("operation"), str):
            raise ValueError("pure fixture misses operation or has extra contract fields")
    elif kind == "engine":
        if set(contract) != {"kind", "category", "engine", "operation"}:
            raise ValueError("engine fixture has invalid contract fields")
        if not all(isinstance(contract.get(key), str) for key in ("category", "engine", "operation")):
            raise ValueError("engine fixture misses category, engine, or operation")
    elif kind == "extract":
        if set(contract) != {"kind", "operation"} or not isinstance(contract.get("operation"), str):
            raise ValueError("extract fixture misses operation or has extra contract fields")
    elif kind == "parser":
        if set(contract) != {"kind", "category", "engine", "operation"}:
            raise ValueError("parser fixture has invalid contract fields")
        if not all(isinstance(contract.get(key), str) for key in ("category", "engine", "operation")):
            raise ValueError("parser fixture misses category, engine, or operation")
    elif kind == "transport":
        if set(contract) != {"kind", "operation"} or not isinstance(contract.get("operation"), str):
            raise ValueError("transport fixture misses operation or has extra contract fields")
    else:
        raise ValueError("offline capture emitted unsupported fixture kind")
    if fixture["result"]["status"] not in {"ok", "error"}:
        raise ValueError("invalid result status")
    result = fixture["result"]
    if result["status"] == "ok" and "output" not in result:
        raise ValueError("success fixture misses output")
    if result["status"] == "error":
        error = result.get("error")
        if not isinstance(error, dict):
            raise ValueError("error fixture misses error shape")
        if not isinstance(error.get("type"), str) or not isinstance(error.get("message"), str):
            raise ValueError("error fixture has invalid error shape")
        if error.get("cause_type") is not None and not isinstance(error["cause_type"], str):
            raise ValueError("error fixture has invalid cause type")
    previous_sequence = 0
    for entry in fixture["trace"]:
        sequence = entry.get("sequence")
        if not isinstance(sequence, int) or sequence <= previous_sequence:
            raise ValueError("trace sequence must be increasing positive integers")
        previous_sequence = sequence
        if entry.get("kind") == "request" and not all(isinstance(entry.get(key), str) for key in ("method", "url")):
            raise ValueError("request trace misses method or URL")
        if entry.get("kind") == "response" and not isinstance(entry.get("status"), int):
            raise ValueError("response trace misses status")
        if "text" in entry and not isinstance(entry["text"], str):
            raise ValueError("trace text must be a string")
        if "content_hex" in entry:
            content_hex = entry["content_hex"]
            if not isinstance(content_hex, str):
                raise ValueError("trace content hex must be a string")
            try:
                bytes.fromhex(content_hex)
            except ValueError as exc:
                raise ValueError("trace content hex is invalid") from exc
    expected_redaction = PURE_REDACTION if kind == "pure" else ENGINE_REDACTION
    if fixture["redaction"] != expected_redaction:
        raise ValueError("fixture redaction policy drift")
    if kind in {"engine", "extract", "transport"} and not any(entry.get("kind") == "request" for entry in fixture["trace"]):
        raise ValueError(f"{kind} fixture has no request trace")
    json.dumps(fixture, ensure_ascii=False, allow_nan=False)


def _fixture_strings(value: Any) -> list[str]:
    if isinstance(value, str):
        return [value]
    if isinstance(value, dict):
        return [text for item in value.values() for text in _fixture_strings(item)]
    if isinstance(value, list):
        return [text for item in value for text in _fixture_strings(item)]
    return []


def _validate_sanitized_fixture_content(fixture: Fixture) -> None:
    """Reject credentials, accidental loopback URLs, and local paths in generated data."""
    fixture_id = fixture["fixture_id"]
    for text in _fixture_strings(fixture):
        if any(marker in text for marker in PRIVATE_PATH_MARKERS):
            raise ValueError(f"{fixture_id}: local path leaked into fixture")
        for raw_url in URL_RE.findall(text):
            parsed = urlsplit(raw_url)
            if parsed.username or parsed.password:
                raise ValueError(f"{fixture_id}: URL userinfo leaked into fixture")
            if parsed.hostname in LOCAL_HOSTS and raw_url not in ALLOWED_LOOPBACK_URLS:
                raise ValueError(f"{fixture_id}: loopback URL leaked into fixture")

    for entry in fixture["trace"]:
        for header_name in entry.get("headers", {}):
            if header_name.lower() in SENSITIVE_HEADER_NAMES:
                raise ValueError(f"{fixture_id}: sensitive header leaked into fixture")
        for cookie_name in entry.get("cookies", {}):
            if any(part in cookie_name.lower() for part in SENSITIVE_COOKIE_PARTS):
                raise ValueError(f"{fixture_id}: sensitive cookie leaked into fixture")


def _patched_core_engines(
    engines: dict[str, dict[str, type[Any]]],
    shuffle: Callable[[list[str]], None],
) -> tuple[Any, Any, Any]:
    core = importlib.import_module("ddgs.ddgs")
    old_engines, old_shuffle, old_threads = core.ENGINES, core.shuffle, core.DDGS.threads
    core.ENGINES, core.shuffle, core.DDGS.threads = engines, shuffle, None
    return core, old_engines, old_shuffle, old_threads


def _restore_core_engines(core: Any, old_engines: Any, old_shuffle: Any, old_threads: Any) -> None:
    core.ENGINES, core.shuffle, core.DDGS.threads = old_engines, old_shuffle, old_threads


def _engine_class(name: str, provider: str, priority: float, search: Callable[..., list[Any]]) -> type[Any]:
    class FakeEngine:
        def __init__(self, **_kwargs: Any) -> None:
            pass

        def search(self, query: str, **kwargs: Any) -> list[Any]:
            return search(query, **kwargs)

    FakeEngine.name = name
    FakeEngine.provider = provider
    FakeEngine.priority = priority
    return FakeEngine


class _SyntheticResponse:
    """Minimal deterministic response used by engine-only capture adapters."""

    def __init__(self, *, text: str = "", content: bytes | None = None, status_code: int = 200) -> None:
        self.status_code = status_code
        self.text = text
        self.content = content if content is not None else text.encode()


class _SyntheticEngineFailure(Exception):
    """Carries a frozen-source exception together with all prior fake I/O."""

    def __init__(self, cause: Exception, trace: list[dict[str, Any]]) -> None:
        super().__init__(str(cause))
        self.cause = cause
        self.trace = trace


def _trace_headers(headers: Any) -> dict[str, str]:
    """Keep static engine headers while removing module-lifetime random UAs."""
    traced = dict(headers)
    user_agent = traced.get("User-Agent")
    if user_agent:
        marker = "google" if user_agent.endswith("NSTNWV") else "user-agent"
        traced["User-Agent"] = f"<source-module-lifetime-random-{marker}>"
    return traced


def _sequenced_trace(events: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [{"sequence": index, **event} for index, event in enumerate(events, start=1)]


def _capture_synthetic_engine(
    responses: list[_SyntheticResponse],
    action: Callable[[list[dict[str, Any]]], Any],
    *,
    client_module: str = "ddgs.base",
    client_name: str = "HttpClient",
) -> tuple[Any, list[dict[str, Any]]]:
    """Run one engine against an in-process HTTP double and record visible calls."""
    client_owner = importlib.import_module(client_module)
    old_http_client = getattr(client_owner, client_name)
    events: list[dict[str, Any]] = []
    pending_responses = list(responses)

    class SyntheticHTTP:
        def __init__(self, *_args: Any, **kwargs: Any) -> None:
            self.client = self
            if headers := kwargs.get("headers"):
                events.append({"kind": "note", "note": "constructor headers", "headers": _trace_headers(headers)})

        def headers_update(self, headers: Any) -> None:
            if headers:
                events.append({"kind": "note", "note": "headers_update", "headers": _trace_headers(headers)})

        def set_cookies(self, domain: str, cookies: Any) -> None:
            events.append({"kind": "cookie", "url": domain, "cookies": dict(cookies)})

        def request(self, method: str, url: str, **kwargs: Any) -> _SyntheticResponse:
            entry: dict[str, Any] = {"kind": "request", "method": method, "url": url}
            if "params" in kwargs:
                entry["query"] = dict(kwargs["params"])
            if "data" in kwargs:
                entry["form"] = dict(kwargs["data"])
            events.append(entry)
            if not pending_responses:
                raise AssertionError(f"unexpected synthetic engine request: {method} {url}")
            response = pending_responses.pop(0)
            events.append(
                {
                    "kind": "response",
                    "status": response.status_code,
                    "text": response.text,
                    "content_hex": hexlify(response.content).decode(),
                }
            )
            return response

    setattr(client_owner, client_name, SyntheticHTTP)
    try:
        try:
            output = action(events)
        except AssertionError:
            raise
        except Exception as exc:
            raise _SyntheticEngineFailure(exc, _sequenced_trace(events)) from exc
        if pending_responses:
            raise AssertionError(f"unused synthetic engine responses: {len(pending_responses)}")
    finally:
        setattr(client_owner, client_name, old_http_client)
    return output, _sequenced_trace(events)


def _capture_synthetic_engine_outcome(
    responses: list[_SyntheticResponse],
    action: Callable[[list[dict[str, Any]]], Any],
    *,
    client_module: str = "ddgs.base",
    client_name: str = "HttpClient",
) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    try:
        output, trace = _capture_synthetic_engine(
            responses,
            action,
            client_module=client_module,
            client_name=client_name,
        )
    except _SyntheticEngineFailure as captured:
        return _error_from_exception(captured.cause), captured.trace
    return _ok(output), trace


def _normalizer_fixtures() -> list[Fixture]:
    raw_text = " A <b>x</b> &amp; e\u0301\x00  \n"
    url = "https://example.test/a%20b+c"
    html5_entity_corpus = "|".join(f"&{name}" for name in sorted(html5))
    cases = [
        _fixture("pure.normalize-text-markup-controls", "normalize_text", {"value": raw_text}, _ok(_normalize_text(raw_text))),
        _fixture(
            "pure.normalize-text-newline-tag-boundary",
            "normalize_text",
            {"value": " <b>one</b>\n<i>two</i> "},
            _ok(_normalize_text(" <b>one</b>\n<i>two</i> ")),
        ),
        _fixture(
            "pure.normalize-text-unicode-categories",
            "normalize_text",
            {"value": "x\u200by\x00\t z"},
            _ok(_normalize_text("x\u200by\x00\t z")),
        ),
        _fixture("pure.normalize-text-nfc", "normalize_text", {"value": "A\u030a"}, _ok(_normalize_text("A\u030a"))),
        _fixture(
            "pure.normalize-text-unclosed-tag",
            "normalize_text",
            {"value": "<unterminated"},
            _ok(_normalize_text("<unterminated")),
        ),
        _fixture(
            "pure.normalize-text-html5-entities",
            "normalize_text",
            {"value": "&nbsp;A&#0;B&#x80;C&notit;"},
            _ok(_normalize_text("&nbsp;A&#0;B&#x80;C&notit;")),
        ),
        _fixture(
            "pure.normalize-text-python-only-html5-entities",
            "normalize_text",
            {"value": "&nGt; &nLt; &amp;nGt;"},
            _ok(_normalize_text("&nGt; &nLt; &amp;nGt;")),
        ),
        _fixture(
            "pure.normalize-text-all-html5-entities",
            "normalize_text",
            {"value": html5_entity_corpus},
            _ok(_normalize_text(html5_entity_corpus)),
        ),
        _fixture(
            "pure.normalize-text-multiline-tags",
            "normalize_text",
            {"value": "<a>one\n</a><b>two</b>"},
            _ok(_normalize_text("<a>one\n</a><b>two</b>")),
        ),
        _fixture("pure.normalize-text-empty", "normalize_text", {"value": ""}, _ok(_normalize_text(""))),
        _fixture("pure.normalize-url-space-plus", "normalize_url", {"value": url}, _ok(_normalize_url(url))),
        _fixture(
            "pure.normalize-url-invalid-percent-escape",
            "normalize_url",
            {"value": "https://example.test/%zz"},
            _ok(_normalize_url("https://example.test/%zz")),
        ),
        _fixture(
            "pure.normalize-url-invalid-utf8",
            "normalize_url",
            {"value": "https://example.test/%FF"},
            _ok(_normalize_url("https://example.test/%FF")),
        ),
        _fixture(
            "pure.normalize-url-plus-preserved",
            "normalize_url",
            {"value": "https://example.test/%2B+%20"},
            _ok(_normalize_url("https://example.test/%2B+%20")),
        ),
        _fixture(
            "pure.normalize-url-invalid-utf8-sequence",
            "normalize_url",
            {"value": "https://example.test/%FF%FF%E2%82"},
            _ok(_normalize_url("https://example.test/%FF%FF%E2%82")),
        ),
        _fixture(
            "pure.normalize-url-mixed-utf8-and-percent",
            "normalize_url",
            {"value": "https://example.test/é%FF/%C3é/%00"},
            _ok(_normalize_url("https://example.test/é%FF/%C3é/%00")),
        ),
        _fixture(
            "pure.normalize-url-incomplete-percent",
            "normalize_url",
            {"value": "https://example.test/%/%A"},
            _ok(_normalize_url("https://example.test/%/%A")),
        ),
        _fixture(
            "pure.normalize-url-lowercase-hex",
            "normalize_url",
            {"value": "https://example.test/%aa"},
            _ok(_normalize_url("https://example.test/%aa")),
        ),
        _fixture("pure.normalize-url-empty", "normalize_url", {"value": ""}, _ok(_normalize_url(""))),
        _fixture("pure.normalize-date-int", "normalize_date", {"value": 1}, _ok(_normalize_date(1))),
        _fixture("pure.normalize-date-negative-int", "normalize_date", {"value": -1}, _ok(_normalize_date(-1))),
        _fixture("pure.normalize-date-bool", "normalize_date", {"value": True}, _ok(_normalize_date(True))),
        _fixture("pure.normalize-date-false", "normalize_date", {"value": False}, _ok(_normalize_date(False))),
        _fixture(
            "pure.normalize-date-float-passthrough",
            "normalize_date",
            {"value": 1.5},
            _ok(_normalize_date(1.5)),
        ),
        _fixture(
            "pure.normalize-date-year-zero-error",
            "normalize_date",
            {"value": -62135596801},
            _error(lambda: _normalize_date(-62135596801)),
        ),
        _fixture(
            "pure.normalize-date-year-ten-thousand-error",
            "normalize_date",
            {"value": 253402300800},
            _error(lambda: _normalize_date(253402300800)),
        ),
        _fixture(
            "pure.normalize-date-tm-year-wrap-error",
            "normalize_date",
            {"value": 67768036191676799},
            _error(lambda: _normalize_date(67768036191676799)),
        ),
        _fixture(
            "pure.normalize-date-tm-year-overflow-error",
            "normalize_date",
            {"value": 67768036191676800},
            _error(lambda: _normalize_date(67768036191676800)),
        ),
        _fixture(
            "pure.normalize-date-negative-tm-year-overflow-error",
            "normalize_date",
            {"value": -67768040609740801},
            _error(lambda: _normalize_date(-67768040609740801)),
        ),
        _fixture(
            "pure.normalize-date-string-passthrough",
            "normalize_date",
            {"value": "2024-01-01"},
            _ok(_normalize_date("2024-01-01")),
        ),
        _fixture("pure.proxy-nil", "expand_proxy", {"value": None}, _ok(_expand_proxy_tb_alias(None))),
        _fixture("pure.proxy-tb-alias", "expand_proxy", {"value": "tb"}, _ok(_expand_proxy_tb_alias("tb"))),
        _fixture("pure.proxy-non-alias", "expand_proxy", {"value": "TB "}, _ok(_expand_proxy_tb_alias("TB "))),
        _fixture("pure.proxy-empty", "expand_proxy", {"value": ""}, _ok(_expand_proxy_tb_alias(""))),
    ]
    for fixture_id, marker, expected in [
        ("pure.vqd-double-quote", b'prefix vqd="double-token" suffix', "double-token"),
        ("pure.vqd-bare", b"prefix vqd=bare-token&suffix", "bare-token"),
        ("pure.vqd-single-quote", b"prefix vqd='single-token' suffix", "single-token"),
    ]:
        cases.append(
            _fixture(
                fixture_id,
                "extract_vqd",
                {"bytes": marker.decode("ascii"), "bytes_hex": hexlify(marker).decode("ascii"), "query": "probe"},
                _ok(_extract_vqd(marker, "probe")),
            )
        )
        if expected != cases[-1]["result"]["output"]:
            raise AssertionError("unexpected VQD oracle result")
    cases.append(
        _fixture(
            "pure.vqd-missing",
            "extract_vqd",
            {"bytes": "no token", "bytes_hex": "6e6f20746f6b656e", "query": "probe"},
            _error(lambda: _extract_vqd(b"no token", "probe")),
        )
    )
    cases.append(
        _fixture(
            "pure.vqd-invalid-utf8",
            "extract_vqd",
            {"bytes": "vqd=\\xff&", "bytes_hex": "7671643dff26", "query": "probe"},
            _error(lambda: _extract_vqd(b"vqd=\xff&", "probe")),
        )
    )
    for fixture_id, query in [
        ("pure.vqd-error-query-repr", "a'b\n\x1b"),
        ("pure.vqd-error-query-double-quote", 'a"b'),
        ("pure.vqd-error-query-both-quotes", 'a\'"b'),
        ("pure.vqd-error-query-controls", "\\\a\b\t\n\v\f\r\x00\x1f\x7f"),
        ("pure.vqd-error-query-unicode", "éĀ𐀀"),
        ("pure.vqd-error-query-unicode-separators", "\u00a0\u2028\u3000"),
    ]:
        cases.append(
            _fixture(
                fixture_id,
                "extract_vqd",
                {"bytes": "no token", "bytes_hex": "6e6f20746f6b656e", "query": query},
                _error(lambda query=query: _extract_vqd(b"no token", query)),
            )
        )
    return cases


def _with_proxy_environment(value: str | None, action: Callable[[], Any]) -> Any:
    previous = os.environ.get("DDGS_PROXY")
    try:
        if value is None:
            os.environ.pop("DDGS_PROXY", None)
        else:
            os.environ["DDGS_PROXY"] = value
        return action()
    finally:
        if previous is None:
            os.environ.pop("DDGS_PROXY", None)
        else:
            os.environ["DDGS_PROXY"] = previous


def _client_state(client: Any) -> dict[str, Any]:
    return {
        "proxy": client._proxy,
        "timeout": client._timeout,
        "verify": client._verify,
    }


def _client_configuration_fixtures() -> list[Fixture]:
    environment_proxy = "socks5://environment.test:1080"
    pem_path = "/fixtures/ca.pem"
    return [
        _fixture(
            "pure.client-default-configuration",
            "client_constructor",
            {"arguments": {}, "environment_proxy": None},
            _ok(_with_proxy_environment(None, lambda: _client_state(ddgs.DDGS()))),
        ),
        _fixture(
            "pure.client-environment-proxy",
            "client_constructor",
            {"arguments": {}, "environment_proxy": environment_proxy},
            _ok(_with_proxy_environment(environment_proxy, lambda: _client_state(ddgs.DDGS()))),
        ),
        _fixture(
            "pure.client-empty-environment-proxy",
            "client_constructor",
            {"arguments": {}, "environment_proxy": ""},
            _ok(_with_proxy_environment("", lambda: _client_state(ddgs.DDGS()))),
        ),
        _fixture(
            "pure.client-empty-proxy-falls-through-environment",
            "client_constructor",
            {"arguments": {"proxy": ""}, "environment_proxy": environment_proxy},
            _ok(_with_proxy_environment(environment_proxy, lambda: _client_state(ddgs.DDGS(proxy="")))),
        ),
        _fixture(
            "pure.client-tb-proxy-overrides-environment",
            "client_constructor",
            {"arguments": {"proxy": "tb"}, "environment_proxy": environment_proxy},
            _ok(_with_proxy_environment(environment_proxy, lambda: _client_state(ddgs.DDGS(proxy="tb")))),
        ),
        _fixture(
            "pure.client-explicit-proxy-overrides-environment",
            "client_constructor",
            {"arguments": {"proxy": "http://explicit.test:8080"}, "environment_proxy": environment_proxy},
            _ok(
                _with_proxy_environment(
                    environment_proxy,
                    lambda: _client_state(ddgs.DDGS(proxy="http://explicit.test:8080")),
                )
            ),
        ),
        _fixture(
            "pure.client-timeout-none",
            "client_constructor",
            {"arguments": {"timeout": None}, "environment_proxy": None},
            _ok(_with_proxy_environment(None, lambda: _client_state(ddgs.DDGS(timeout=None)))),
        ),
        _fixture(
            "pure.client-timeout-zero",
            "client_constructor",
            {"arguments": {"timeout": 0}, "environment_proxy": None},
            _ok(_with_proxy_environment(None, lambda: _client_state(ddgs.DDGS(timeout=0)))),
        ),
        _fixture(
            "pure.client-verify-false",
            "client_constructor",
            {"arguments": {"verify": False}, "environment_proxy": None},
            _ok(_with_proxy_environment(None, lambda: _client_state(ddgs.DDGS(verify=False)))),
        ),
        _fixture(
            "pure.client-verify-pem",
            "client_constructor",
            {"arguments": {"verify": pem_path}, "environment_proxy": None},
            _ok(_with_proxy_environment(None, lambda: _client_state(ddgs.DDGS(verify=pem_path)))),
        ),
    ]


def _aggregation_fixtures() -> list[Fixture]:
    shared = "https://same.example/item"
    first = TextResult(title="first", href=shared, body="short")
    second = TextResult(title="second", href=shared, body="a longer body")
    aggregator: ResultsAggregator[Any] = ResultsAggregator({"href", "image", "url", "embed_url"})
    aggregator.extend([first, second])
    longer_body = aggregator.extract_dicts()

    unicode_first = TextResult(title="first", href=shared, body="a")
    unicode_second = TextResult(title="second", href=shared, body="é")
    unicode_body: ResultsAggregator[Any] = ResultsAggregator({"href"})
    unicode_body.extend([unicode_first, unicode_second])
    unicode_body_output = unicode_body.extract_dicts()

    empty_body_first = TextResult(title="first", href=shared, body=[])
    empty_body_second = TextResult(title="second", href=shared, body=[])
    empty_body: ResultsAggregator[Any] = ResultsAggregator({"href"})
    empty_body.extend([empty_body_first, empty_body_second])
    empty_body_output = empty_body.extract_dicts()

    none_body_first = TextResult(title="first", href=shared, body=None)
    none_body_second = TextResult(title="second", href=shared, body=None)
    none_body: ResultsAggregator[Any] = ResultsAggregator({"href"})
    none_body_error = _error(lambda: none_body.extend([none_body_first, none_body_second]))

    scalar_body_fixtures = []
    for name, body in (("false", False), ("zero", 0), ("zero-float", 0.0)):
        body_first = TextResult(title="first", href=shared, body=body)
        body_second = TextResult(title="second", href=shared, body=body)
        body_aggregator: ResultsAggregator[Any] = ResultsAggregator({"href"})
        body_error = _error(lambda: body_aggregator.extend([body_first, body_second]))
        scalar_body_fixtures.append(
            _fixture(
                f"pure.aggregate-scalar-body-{name}-error",
                "results_aggregator",
                {
                    "cache_fields": ["href"],
                    "items": [dict(body_first.__dict__), dict(body_second.__dict__)],
                    "field_order": [list(body_first.__dict__), list(body_second.__dict__)],
                },
                body_error,
            )
        )

    image_first = ImagesResult(title="first", image="", url="https://first.example")
    image_second = ImagesResult(title="second", image="", url="https://second.example")
    blank_image: ResultsAggregator[Any] = ResultsAggregator({"href", "image", "url", "embed_url"})
    blank_image.extend([image_first, image_second])
    blank_image_output = blank_image.extract_dicts()

    tie_first = TextResult(title="first", href="https://first.example", body="")
    tie_second = TextResult(title="second", href="https://second.example", body="")
    tie: ResultsAggregator[Any] = ResultsAggregator({"href"})
    tie.extend([tie_first, tie_second])
    tie_output = tie.extract_dicts()

    count_second = TextResult(title="second", href="https://second.example", body="")
    count_first = TextResult(title="first", href="https://first.example", body="")
    count_first_repeat = TextResult(title="first repeat", href="https://first.example", body="")
    count_descending: ResultsAggregator[Any] = ResultsAggregator({"href"})
    count_descending.extend([count_second, count_first, count_first_repeat])
    count_descending_output = count_descending.extract_dicts()
    count_descending_length = len(count_descending)

    url_first = SimpleNamespace(url="https://first.example", image="")
    url_second = SimpleNamespace(url="https://second.example", image="")
    first_declared_cache_field: ResultsAggregator[Any] = ResultsAggregator({"image", "url"})
    first_declared_cache_field.extend([url_first, url_second])
    first_declared_cache_field_output = first_declared_cache_field.extract_dicts()

    video_input = {
        "title": "  Video\u200b title  ",
        "content": None,
        "description": 17,
        "duration": 123,
        "embed_html": "<iframe src='https://embed.example'></iframe>",
        "embed_url": "https://embed.example/path",
        "image_token": {"opaque": "token"},
        "images": {"large": "https://image.example/large", "width": 1280, "flag": True},
        "provider": ["provider", 2],
        "published": False,
        "publisher": None,
        "statistics": {"viewCount": 29059, "ratio": 1.25, "empty": None},
        "uploader": ["alice", None],
    }
    video_engine = DuckduckgoVideos.__new__(DuckduckgoVideos)
    video_output = [
        dict(item.__dict__)
        for item in DuckduckgoVideos.extract_results(video_engine, json.dumps({"results": [video_input]}))
    ]

    category_inputs = [
        {"title": "  Text\u200b title  ", "href": "https://text.example/a%20b", "body": " body\ntext "},
        {
            "title": " Image ",
            "image": "https://image.example/a%20b",
            "thumbnail": "https://thumbnail.example/a%20b",
            "url": "https://page.example/a%20b",
            "height": 720,
            "width": "1280",
            "source": None,
        },
        {
            "date": 1,
            "title": " News ",
            "body": " body\ttext ",
            "url": "https://news.example/a%20b",
            "image": "https://news-image.example/a%20b",
            "source": 2,
        },
        {
            "title": " Video ",
            "content": None,
            "description": 17,
            "duration": 123,
            "embed_html": "<iframe></iframe>",
            "embed_url": "https://embed.example/a%20b",
            "image_token": {"opaque": "token"},
            "images": {"width": 1280},
            "provider": ["provider"],
            "published": False,
            "publisher": None,
            "statistics": {"viewCount": 29059},
            "uploader": ["alice"],
        },
        {
            "title": " Book ",
            "author": " Author\nName ",
            "publisher": " Publisher ",
            "info": " Info\ttext ",
            "url": "https://book.example/a%20b",
            "thumbnail": "https://book-image.example/a%20b",
        },
    ]
    category_types = [TextResult, ImagesResult, NewsResult, VideosResult, BooksResult]
    category_output = [dict(result_type(**item).__dict__) for result_type, item in zip(category_types, category_inputs)]
    category_default_output = [dict(result_type().__dict__) for result_type in category_types]
    dynamic_text = TextResult()
    dynamic_text.__setattr__("title", "  Updated\u200b title  ")
    dynamic_text.__setattr__("engine_extra", {"rank": 2, "nested": [None, True]})
    dynamic_text_output = dict(dynamic_text.__dict__)
    falsy_category_inputs = [
        {"title": False, "href": 0, "body": None},
        {"date": True, "title": "", "body": [], "url": None, "image": False, "source": 0},
    ]
    falsy_category_types = [TextResult, NewsResult]
    falsy_category_output = [
        dict(result_type(**item).__dict__) for result_type, item in zip(falsy_category_types, falsy_category_inputs)
    ]

    falsy_image_fixtures = []
    for name, image in (("none", None), ("false", False), ("zero", 0), ("empty-list", []), ("empty-dict", {})):
        falsy_image_first = ImagesResult(title="first", image=image, url="https://first.example")
        falsy_image_second = ImagesResult(title="second", image=image, url="https://second.example")
        falsy_image: ResultsAggregator[Any] = ResultsAggregator({"image", "url"})
        falsy_image.extend([falsy_image_first, falsy_image_second])
        falsy_image_output = falsy_image.extract_dicts()
        falsy_image_fixtures.append(
            _fixture(
                f"pure.aggregate-falsy-image-{name}",
                "results_aggregator",
                {
                    "cache_fields": ["image", "url"],
                    "items": [dict(falsy_image_first.__dict__), dict(falsy_image_second.__dict__)],
                    "field_order": [list(falsy_image_first.__dict__), list(falsy_image_second.__dict__)],
                },
                _ok(falsy_image_output, field_order=[list(item) for item in falsy_image_output]),
            )
        )

    return [
        _fixture(
            "pure.aggregate-longer-body",
            "results_aggregator",
            {
                "cache_fields": ["embed_url", "href", "image", "url"],
                "items": [dict(first.__dict__), dict(second.__dict__)],
                "field_order": [list(first.__dict__), list(second.__dict__)],
            },
            _ok(longer_body, field_order=[list(item) for item in longer_body]),
        ),
        _fixture(
            "pure.aggregate-blank-image-key",
            "results_aggregator",
            {
                "cache_fields": ["embed_url", "href", "image", "url"],
                "items": [dict(image_first.__dict__), dict(image_second.__dict__)],
                "field_order": [list(image_first.__dict__), list(image_second.__dict__)],
            },
            _ok(blank_image_output, field_order=[list(item) for item in blank_image_output]),
        ),
        _fixture(
            "pure.aggregate-unicode-body-length",
            "results_aggregator",
            {
                "cache_fields": ["href"],
                "items": [dict(unicode_first.__dict__), dict(unicode_second.__dict__)],
                "field_order": [list(unicode_first.__dict__), list(unicode_second.__dict__)],
            },
            _ok(unicode_body_output, field_order=[list(item) for item in unicode_body_output]),
        ),
        _fixture(
            "pure.aggregate-empty-list-body",
            "results_aggregator",
            {
                "cache_fields": ["href"],
                "items": [dict(empty_body_first.__dict__), dict(empty_body_second.__dict__)],
                "field_order": [list(empty_body_first.__dict__), list(empty_body_second.__dict__)],
            },
            _ok(empty_body_output, field_order=[list(item) for item in empty_body_output]),
        ),
        _fixture(
            "pure.aggregate-none-body-error",
            "results_aggregator",
            {
                "cache_fields": ["href"],
                "items": [dict(none_body_first.__dict__), dict(none_body_second.__dict__)],
                "field_order": [list(none_body_first.__dict__), list(none_body_second.__dict__)],
            },
            none_body_error,
        ),
        _fixture(
            "pure.aggregate-count-tie-first-encounter",
            "results_aggregator",
            {
                "cache_fields": ["href"],
                "items": [
                    dict(tie_first.__dict__),
                    dict(tie_second.__dict__),
                ],
                "field_order": [list(tie_first.__dict__), list(tie_second.__dict__)],
            },
            _ok(tie_output, field_order=[list(item) for item in tie_output]),
        ),
        _fixture(
            "pure.aggregate-count-descending",
            "results_aggregator",
            {
                "cache_fields": ["href"],
                "items": [
                    dict(count_second.__dict__),
                    dict(count_first.__dict__),
                    dict(count_first_repeat.__dict__),
                ],
                "field_order": [
                    list(count_second.__dict__),
                    list(count_first.__dict__),
                    list(count_first_repeat.__dict__),
                ],
            },
            _ok(
                {"items": count_descending_output, "length": count_descending_length},
                field_order=[list(item) for item in count_descending_output],
            ),
        ),
        _fixture(
            "pure.aggregate-first-declared-cache-field",
            "results_aggregator",
            {
                "cache_fields": ["image", "url"],
                "items": [dict(url_first.__dict__), dict(url_second.__dict__)],
                "field_order": [list(url_first.__dict__), list(url_second.__dict__)],
            },
            _ok(
                first_declared_cache_field_output,
                field_order=[list(item) for item in first_declared_cache_field_output],
            ),
        ),
        _fixture(
            "pure.video-heterogeneous-values",
            "duckduckgo_videos_extract_results",
            {"response": {"results": [video_input]}},
            _ok(video_output, field_order=[list(item) for item in video_output]),
        ),
        _fixture(
            "pure.result-category-field-shapes",
            "result_construction",
            {
                "categories": ["text", "images", "news", "videos", "books"],
                "items": category_inputs,
                "field_order": [list(item) for item in category_inputs],
            },
            _ok(category_output, field_order=[list(item) for item in category_output]),
        ),
        _fixture(
            "pure.result-category-default-shapes",
            "result_construction",
            {"categories": ["text", "images", "news", "videos", "books"], "items": [{} for _ in category_types]},
            _ok(category_default_output, field_order=[list(item) for item in category_default_output]),
        ),
        _fixture(
            "pure.result-dynamic-field-order",
            "result_construction",
            {
                "category": "text",
                "updates": [
                    {"name": "title", "value": "  Updated\u200b title  "},
                    {"name": "engine_extra", "value": {"rank": 2, "nested": [None, True]}},
                ],
            },
            _ok(dynamic_text_output, field_order=[list(dynamic_text_output)]),
        ),
        _fixture(
            "pure.result-falsy-named-fields",
            "result_construction",
            {
                "categories": ["text", "news"],
                "items": falsy_category_inputs,
                "field_order": [list(item) for item in falsy_category_inputs],
            },
            _ok(falsy_category_output, field_order=[list(item) for item in falsy_category_output]),
        ),
        *falsy_image_fixtures,
        *scalar_body_fixtures,
    ]


def _ranker_fixtures() -> list[Fixture]:
    buckets = [
        {"title": "Wiki alpha", "href": "https://en.wikipedia.org/wiki/Alpha", "body": ""},
        {"title": "alpha both", "href": "https://both.example", "body": "alpha body"},
        {"title": "alpha title", "href": "https://title.example", "body": "other"},
        {"title": "other", "href": "https://body.example", "body": "alpha body"},
        {"title": "other", "href": "https://neither.example", "body": "other"},
        {"title": "Category: Wikimedia alpha", "href": "https://skip.example", "body": "alpha"},
    ]
    description_fallback = [
        {"title": "description", "href": "https://description.example", "description": "alpha value"},
        {"title": "body-empty", "href": "https://body-empty.example", "body": "", "description": "alpha ignored"},
    ]
    substring = [
        {"title": "prefixALPHAsuffix", "href": "https://substring.example", "body": ""},
        {"title": "other", "href": "https://body-substring.example", "body": "ALPHAvalue"},
        {"title": "other", "href": "https://none.example", "body": "other"},
    ]
    unicode_docs = [
        {"title": "café", "href": "https://cafe.example", "body": ""},
        {"title": "東京語", "href": "https://jp.example", "body": ""},
        {"title": "other", "href": "https://other.example", "body": "other"},
    ]
    href_list_membership = [
        {"title": "list href", "href": ["wikipedia.org"], "body": "ignored"},
    ]
    href_none = [
        {"title": "none href", "href": None, "body": "ignored"},
    ]
    href_dict_membership = [
        {"title": "dict href", "href": {"wikipedia.org": True}, "body": "ignored"},
    ]
    title_list_category = [
        {"title": ["Category:", "Wikimedia"], "href": "https://example.test", "body": "ignored"},
    ]
    title_list_lower = [
        {"title": [], "href": "https://example.test", "body": "ignored"},
    ]
    title_dict_category = [
        {"title": {"Category:": True, "Wikimedia": True}, "href": "https://example.test", "body": "ignored"},
    ]
    body_none = [
        {"title": "body none", "href": "https://example.test", "body": None},
    ]
    body_bool = [
        {"title": "body bool", "href": "https://example.test", "body": False},
    ]
    body_integer = [
        {"title": "body integer", "href": "https://example.test", "body": 1},
    ]
    body_dict = [
        {"title": "body dict", "href": "https://example.test", "body": {}},
    ]
    ranker = SimpleFilterRanker()
    return [
        _fixture(
            "pure.ranker-wikipedia-buckets-category",
            "simple_filter_ranker",
            {"query": "alpha", "documents": buckets},
            _ok(ranker.rank(buckets, "alpha")),
        ),
        _fixture(
            "pure.ranker-description-fallback-only-when-body-absent",
            "simple_filter_ranker",
            {"query": "alpha", "documents": description_fallback},
            _ok(ranker.rank(description_fallback, "alpha")),
        ),
        _fixture(
            "pure.ranker-wrong-body-type-error",
            "simple_filter_ranker",
            {
                "query": "alpha",
                "documents": [
                    {"title": "body-empty-list", "href": "https://body-list.example", "body": [], "description": "alpha ignored"},
                ],
            },
            _error(
                lambda: ranker.rank(
                    [
                        {"title": "body-empty-list", "href": "https://body-list.example", "body": [], "description": "alpha ignored"},
                    ],
                    "alpha",
                )
            ),
        ),
        _fixture(
            "pure.ranker-substring-and-case-fold",
            "simple_filter_ranker",
            {"query": "alpha", "documents": substring},
            _ok(ranker.rank(substring, "alpha")),
        ),
        _fixture(
            "pure.ranker-unicode-word-tokens",
            "simple_filter_ranker",
            {
                "queries": ["café 東京語", "ab abc", "İstanbul", "ab _"],
                "documents": unicode_docs,
            },
            _ok(
                {
                    "tokens": {
                        query: sorted(ranker._extract_tokens(query))
                        for query in ("café 東京語", "ab abc", "İstanbul", "ab _")
                    },
                    "ranked": ranker.rank(unicode_docs, "café 東京語"),
                }
            ),
        ),
        _fixture(
            "pure.ranker-empty-documents",
            "simple_filter_ranker",
            {"query": "alpha", "documents": []},
            _ok(ranker.rank([], "alpha")),
        ),
        _fixture(
            "pure.ranker-category-case-sensitive",
            "simple_filter_ranker",
            {
                "query": "alpha",
                "documents": [
                    {"title": "Category: Wikimedia exact", "href": "https://skip.example", "body": "alpha"},
                    {"title": "category: Wikimedia lower", "href": "https://keep.example", "body": "alpha"},
                    {"title": "Category: wikimedia lower", "href": "https://keep2.example", "body": "alpha"},
                    {"title": "Wikimedia only", "href": "https://keep3.example", "body": "alpha"},
                ],
            },
            _ok(
                ranker.rank(
                    [
                        {"title": "Category: Wikimedia exact", "href": "https://skip.example", "body": "alpha"},
                        {"title": "category: Wikimedia lower", "href": "https://keep.example", "body": "alpha"},
                        {"title": "Category: wikimedia lower", "href": "https://keep2.example", "body": "alpha"},
                        {"title": "Wikimedia only", "href": "https://keep3.example", "body": "alpha"},
                    ],
                    "alpha",
                )
            ),
        ),
        _fixture(
            "pure.ranker-wikipedia-case-sensitive",
            "simple_filter_ranker",
            {
                "query": "alpha",
                "documents": [
                    {"title": "upper", "href": "https://WIKIPEDIA.ORG/wiki/A", "body": ""},
                    {"title": "lower", "href": "https://en.wikipedia.org/wiki/A", "body": ""},
                    {"title": "both", "href": "https://both.example", "body": "alpha"},
                ],
            },
            _ok(
                ranker.rank(
                    [
                        {"title": "upper", "href": "https://WIKIPEDIA.ORG/wiki/A", "body": ""},
                        {"title": "lower", "href": "https://en.wikipedia.org/wiki/A", "body": ""},
                        {"title": "both", "href": "https://both.example", "body": "alpha"},
                    ],
                    "alpha",
                )
            ),
        ),
        _fixture(
            "pure.ranker-href-list-membership",
            "simple_filter_ranker",
            {"query": "alpha", "documents": href_list_membership},
            _ok(ranker.rank(href_list_membership, "alpha")),
        ),
        _fixture(
            "pure.ranker-href-none-membership-error",
            "simple_filter_ranker",
            {"query": "alpha", "documents": href_none},
            _error(lambda: ranker.rank(href_none, "alpha")),
        ),
        _fixture(
            "pure.ranker-href-dict-membership",
            "simple_filter_ranker",
            {"query": "alpha", "documents": href_dict_membership},
            _ok(ranker.rank(href_dict_membership, "alpha")),
        ),
        _fixture(
            "pure.ranker-title-list-category-membership",
            "simple_filter_ranker",
            {"query": "alpha", "documents": title_list_category},
            _ok(ranker.rank(title_list_category, "alpha")),
        ),
        _fixture(
            "pure.ranker-title-list-lower-error",
            "simple_filter_ranker",
            {"query": "alpha", "documents": title_list_lower},
            _error(lambda: ranker.rank(title_list_lower, "alpha")),
        ),
        _fixture(
            "pure.ranker-title-dict-category-membership",
            "simple_filter_ranker",
            {"query": "alpha", "documents": title_dict_category},
            _ok(ranker.rank(title_dict_category, "alpha")),
        ),
        _fixture(
            "pure.ranker-body-none-lower-error",
            "simple_filter_ranker",
            {"query": "alpha", "documents": body_none},
            _error(lambda: ranker.rank(body_none, "alpha")),
        ),
        _fixture(
            "pure.ranker-body-bool-lower-error",
            "simple_filter_ranker",
            {"query": "alpha", "documents": body_bool},
            _error(lambda: ranker.rank(body_bool, "alpha")),
        ),
        _fixture(
            "pure.ranker-body-integer-lower-error",
            "simple_filter_ranker",
            {"query": "alpha", "documents": body_integer},
            _error(lambda: ranker.rank(body_integer, "alpha")),
        ),
        _fixture(
            "pure.ranker-body-dict-lower-error",
            "simple_filter_ranker",
            {"query": "alpha", "documents": body_dict},
            _error(lambda: ranker.rank(body_dict, "alpha")),
        ),
        _fixture(
            "pure.ranker-none-document-get-error",
            "simple_filter_ranker",
            {"query": "alpha", "documents": [None]},
            _error(lambda: ranker.rank([None], "alpha")),
        ),
    ]


def _engine_metadata(engine_class: type[Any]) -> dict[str, Any]:
    return {
        "name": engine_class.name,
        "category": engine_class.category,
        "provider": engine_class.provider,
        "priority": engine_class.priority,
        "disabled": engine_class.disabled,
    }


def _engine_registry_fixture() -> Fixture:
    active = [
        {
            "category": category,
            "engines": [_engine_metadata(engine_class) for engine_class in engines.values()],
        }
        for category, engines in ENGINES.items()
    ]
    return _fixture(
        "pure.engine-registry-active-and-disabled",
        "engine_registry",
        {"discovery": "ddgs.engines.ENGINES"},
        _ok({"active": active, "disabled": [_engine_metadata(Bing)]}),
    )


def _backend_fixtures() -> list[Fixture]:
    noop = lambda _query, **_kwargs: []
    engines = {
        "text": {
            "first": _engine_class("first", "first", 1, noop),
            "wikipedia": _engine_class("wikipedia", "wikipedia", 2, noop),
            "grokipedia": _engine_class("grokipedia", "grokipedia", 1.9, noop),
            "second": _engine_class("second", "second", 1, noop),
        },
        "images": {
            "first": _engine_class("first", "first", 1, noop),
            "second": _engine_class("second", "second", 1, noop),
        },
    }
    fixture_registry = [
        {
            "category": category,
            "engines": [
                {
                    "name": name,
                    "provider": engine_class.provider,
                    "priority": engine_class.priority,
                    "disabled": False,
                }
                for name, engine_class in category_engines.items()
            ],
        }
        for category, category_engines in engines.items()
    ]

    shuffle_calls: list[list[str]] = []

    def reverse(items: list[str]) -> None:
        shuffle_calls.append(list(items))
        items.reverse()

    core, old_engines, old_shuffle, old_threads = _patched_core_engines(engines, reverse)
    try:
        shuffle_inputs: dict[str, list[list[str]]] = {}

        def selected(label: str, category: str, backend: str) -> list[dict[str, Any]]:
            call_offset = len(shuffle_calls)
            selected_engines = [
                {"name": engine.name, "provider": engine.provider, "priority": engine.priority}
                for engine in core.DDGS()._get_engines(category, backend)
            ]
            shuffle_inputs[label] = shuffle_calls[call_offset:]
            return selected_engines

        output = {
            "auto": selected("auto", "text", "auto"),
            "all": selected("all", "text", "all"),
            "invalid_fallback": selected("invalid_fallback", "text", "missing"),
            "disabled_bing_fallback": selected("disabled_bing_fallback", "text", "bing"),
            "explicit_trimmed": selected("explicit_trimmed", "text", " second, first "),
            "explicit_with_invalid": selected("explicit_with_invalid", "text", "missing, first"),
            "explicit_duplicates": selected("explicit_duplicates", "text", "first, first, second"),
            "mixed_auto_invalid": selected("mixed_auto_invalid", "text", "auto, missing"),
            "empty_invalid_fallback": selected("empty_invalid_fallback", "text", ", missing, "),
            "uppercase_auto_fallback": selected("uppercase_auto_fallback", "text", "AUTO"),
            "control_whitespace_trimmed": selected("control_whitespace_trimmed", "text", "\x1csecond\x1c,\xa0first\xa0"),
            "zero_width_not_trimmed_fallback": selected("zero_width_not_trimmed_fallback", "text", "\u200bfirst\u200b"),
            "nontext_all": selected("nontext_all", "images", "all"),
            "shuffle_inputs": shuffle_inputs,
        }
        unknown_category_error = _error(lambda: core.DDGS()._get_engines("missing-category", "auto"))
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    return [
        _fixture(
            "pure.backend-auto-priority-stable-shuffle",
            "get_engines",
            {
                "category": "text",
                "cases": [
                    {"label": "auto", "category": "text", "backend": "auto"},
                    {"label": "all", "category": "text", "backend": "all"},
                    {"label": "invalid_fallback", "category": "text", "backend": "missing"},
                    {"label": "disabled_bing_fallback", "category": "text", "backend": "bing"},
                    {"label": "explicit_trimmed", "category": "text", "backend": " second, first "},
                    {"label": "explicit_with_invalid", "category": "text", "backend": "missing, first"},
                    {"label": "explicit_duplicates", "category": "text", "backend": "first, first, second"},
                    {"label": "mixed_auto_invalid", "category": "text", "backend": "auto, missing"},
                    {"label": "empty_invalid_fallback", "category": "text", "backend": ", missing, "},
                    {"label": "uppercase_auto_fallback", "category": "text", "backend": "AUTO"},
                    {
                        "label": "control_whitespace_trimmed",
                        "category": "text",
                        "backend": "\x1csecond\x1c,\xa0first\xa0",
                    },
                    {"label": "zero_width_not_trimmed_fallback", "category": "text", "backend": "\u200bfirst\u200b"},
                    {"label": "nontext_all", "category": "images", "backend": "all"},
                ],
                "registry": fixture_registry,
                "shuffle": "reverse",
            },
            _ok(output),
            random="shuffle patched to reverse; sort tie key is frozen function object",
            trace=[{"sequence": 1, "kind": "note", "note": "fake engines only; no HTTP client is used"}],
        )
        ,
        _fixture(
            "pure.backend-unknown-category-error",
            "get_engines",
            {"category": "missing-category", "backend": "auto"},
            unknown_category_error,
            random="not reached",
            trace=[{"sequence": 1, "kind": "note", "note": "fake registry only; no HTTP client is used"}],
        ),
    ]


def _frozen_registry_backend_fixture() -> Fixture:
    noop = lambda _query, **_kwargs: []
    engines = {
        category: {
            name: _engine_class(name, engine_class.provider, engine_class.priority, noop)
            for name, engine_class in category_engines.items()
        }
        for category, category_engines in ENGINES.items()
    }
    shuffle_calls: list[list[str]] = []

    def reverse(items: list[str]) -> None:
        shuffle_calls.append(list(items))
        items.reverse()

    core, old_engines, old_shuffle, old_threads = _patched_core_engines(engines, reverse)
    try:
        shuffle_inputs: dict[str, list[list[str]]] = {}

        def selected(label: str, category: str, backend: str) -> list[dict[str, Any]]:
            call_offset = len(shuffle_calls)
            selected_engines = [
                {"name": engine.name, "provider": engine.provider, "priority": engine.priority}
                for engine in core.DDGS()._get_engines(category, backend)
            ]
            shuffle_inputs[label] = shuffle_calls[call_offset:]
            return selected_engines

        output = {
            "text_auto": selected("text_auto", "text", "auto"),
            "text_all": selected("text_all", "text", "all"),
            "text_disabled_bing_fallback": selected("text_disabled_bing_fallback", "text", "bing"),
            "images_auto": selected("images_auto", "images", "auto"),
            "news_all": selected("news_all", "news", "all"),
            "videos_auto": selected("videos_auto", "videos", "auto"),
            "books_auto": selected("books_auto", "books", "auto"),
            "shuffle_inputs": shuffle_inputs,
        }
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    return _fixture(
        "pure.backend-frozen-registry-selection",
        "get_engines",
        {
            "cases": [
                {"label": "text_auto", "category": "text", "backend": "auto"},
                {"label": "text_all", "category": "text", "backend": "all"},
                {"label": "text_disabled_bing_fallback", "category": "text", "backend": "bing"},
                {"label": "images_auto", "category": "images", "backend": "auto"},
                {"label": "news_all", "category": "news", "backend": "all"},
                {"label": "videos_auto", "category": "videos", "backend": "auto"},
                {"label": "books_auto", "category": "books", "backend": "auto"},
            ],
            "registry_fixture": "pure.engine-registry-active-and-disabled",
            "shuffle": "reverse",
        },
        _ok(output),
        random="shuffle patched to reverse; sort tie key is frozen function object",
        trace=[{"sequence": 1, "kind": "note", "note": "source registry metadata copied into fake classes; no HTTP client is used"}],
    )


def _search_invocation_fixtures() -> list[Fixture]:
    def capture(arguments: dict[str, Any]) -> dict[str, Any]:
        calls: list[dict[str, Any]] = []
        selections: list[dict[str, str]] = []

        def search(query: str, **kwargs: Any) -> list[Any]:
            calls.append({"query": query, "kwargs": kwargs})
            return [TextResult(title="probe", href="https://probe.example", body="")]

        core = importlib.import_module("ddgs.ddgs")
        old_get_engines = core.DDGS._get_engines

        def get_engines(_client: Any, category: str, backend: str) -> list[Any]:
            selections.append({"category": category, "backend": backend})
            return [_engine_class("probe", "probe", 1, search)()]

        core.DDGS._get_engines = get_engines
        try:
            results = core.DDGS()._search_sync("probe", "query", **arguments)
        finally:
            core.DDGS._get_engines = old_get_engines
        return {
            "calls": calls,
            "hrefs": [item["href"] for item in results],
            "selections": selections,
        }

    return [
        _fixture(
            "pure.search-call-defaults",
            "search_invocation",
            {"category": "probe", "query": "query", "arguments": {}},
            _ok(capture({})),
            trace=[{"sequence": 1, "kind": "note", "note": "fake engine records _search_sync call only"}],
        ),
        _fixture(
            "pure.search-call-explicit-zero-empty-values",
            "search_invocation",
            {
                "category": "probe",
                "query": "query",
                "arguments": {
                    "region": "zz-zz",
                    "safesearch": "",
                    "timelimit": "",
                    "max_results": 0,
                    "page": 0,
                    "backend": "other, custom",
                },
            },
            _ok(
                capture(
                    {
                        "region": "zz-zz",
                        "safesearch": "",
                        "timelimit": "",
                        "max_results": 0,
                        "page": 0,
                        "backend": "other, custom",
                    }
                )
            ),
            trace=[{"sequence": 1, "kind": "note", "note": "fake engine records unvalidated source values"}],
        ),
        _fixture(
            "pure.search-call-explicit-none-max-results",
            "search_invocation",
            {
                "category": "probe",
                "query": "query",
                "arguments": {"max_results": None},
            },
            _ok(capture({"max_results": None})),
            trace=[{"sequence": 1, "kind": "note", "note": "fake engine records explicit unlimited source value"}],
        ),
        _fixture(
            "pure.search-call-negative-page",
            "search_invocation",
            {"category": "probe", "query": "query", "arguments": {"page": -7}},
            _ok(capture({"page": -7})),
            trace=[{"sequence": 1, "kind": "note", "note": "fake engine records unvalidated negative page"}],
        ),
    ]


def _scheduler_fixtures() -> list[Fixture]:
    core = importlib.import_module("ddgs.ddgs")
    fixtures: list[Fixture] = []

    completed_future: Future[str] = Future()
    completed_future.set_result("done")
    completed, pending = wait({completed_future}, timeout=0, return_when=FIRST_EXCEPTION)
    fixtures.append(
        _fixture(
            "pure.scheduler-zero-timeout-keeps-already-completed-future",
            "search_scheduler_wait",
            {"wait_timeout_seconds": 0, "future_state": "success"},
            _ok({"done": len(completed), "pending": len(pending)}),
            trace=[
                {
                    "sequence": 1,
                    "kind": "note",
                    "note": "concurrent.futures.wait observes already-completed futures before zero timeout",
                }
            ],
        )
    )

    started_first, started_second, started_other = threading.Event(), threading.Event(), threading.Event()

    def first(_query: str, **_kwargs: Any) -> list[Any]:
        started_first.set()
        if not started_second.wait(1):
            raise RuntimeError("second shared-provider engine did not start")
        return [TextResult(title="first", href="https://first.example", body="one")]

    def second(_query: str, **_kwargs: Any) -> list[Any]:
        started_second.set()
        return [TextResult(title="second", href="https://second.example", body="two")]

    def other(_query: str, **_kwargs: Any) -> list[Any]:
        started_other.set()
        return [TextResult(title="other", href="https://other.example", body="three")]

    engines = {
        "probe": {
            "first": _engine_class("first", "shared", 1, first),
            "second": _engine_class("second", "shared", 1, second),
            "other": _engine_class("other", "other", 1, other),
        }
    }
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(engines, lambda _items: None)
    try:
        provider_output = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=10, backend="all")
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-provider-not-reserved-on-submit",
            "search_scheduler",
            {"category": "probe", "max_results": 10, "backend": "all"},
            _ok({
                "hrefs": [item["href"] for item in provider_output],
                "started": {"first": started_first.is_set(), "second": started_second.is_set(), "other": started_other.is_set()},
            }),
            random="shuffle patched to identity",
            trace=[{"sequence": 1, "kind": "note", "note": "first and second share provider; both fake searches run"}],
        )
    )

    seen_empty, seen_success, seen_skipped = threading.Event(), threading.Event(), threading.Event()

    def empty_provider(_query: str, **_kwargs: Any) -> list[Any]:
        seen_empty.set()
        return []

    def successful_provider(_query: str, **_kwargs: Any) -> list[Any]:
        seen_success.set()
        return [TextResult(title="success", href="https://success.example", body="")]

    def skipped_provider(_query: str, **_kwargs: Any) -> list[Any]:
        seen_skipped.set()
        return [TextResult(title="skipped", href="https://skipped.example", body="")]

    same_provider_engines = {
        "probe": {
            "empty": _engine_class("empty", "shared", 1, empty_provider),
            "success": _engine_class("success", "shared", 1, successful_provider),
            "skipped": _engine_class("skipped", "shared", 1, skipped_provider),
        }
    }
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(same_provider_engines, lambda _items: None)
    try:
        same_provider_output = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=10, backend="all")
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-provider-seen-after-nonempty",
            "search_scheduler",
            {"category": "probe", "max_results": 10, "backend": "all"},
            _ok(
                {
                    "hrefs": [item["href"] for item in same_provider_output],
                    "started": {
                        "empty": seen_empty.is_set(),
                        "success": seen_success.is_set(),
                        "skipped": seen_skipped.is_set(),
                    },
                }
            ),
            random="shuffle patched to identity",
            trace=[
                {
                    "sequence": 1,
                    "kind": "note",
                    "note": "empty result does not mark provider; later nonempty result does",
                }
            ],
        )
    )

    error_started, recovery_started = threading.Event(), threading.Event()

    def failed_provider(_query: str, **_kwargs: Any) -> list[Any]:
        error_started.set()
        raise RuntimeError("first provider failure")

    def recovered_provider(_query: str, **_kwargs: Any) -> list[Any]:
        recovery_started.set()
        return [TextResult(title="recovered", href="https://recovered.example", body="")]

    recovery_engines = {
        "probe": {
            "failed": _engine_class("failed", "shared", 1, failed_provider),
            "recovered": _engine_class("recovered", "shared", 1, recovered_provider),
        }
    }
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(recovery_engines, lambda _items: None)
    try:
        recovery_output = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=10, backend="all")
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-provider-error-does-not-mark-seen",
            "search_scheduler",
            {"category": "probe", "max_results": 10, "backend": "all"},
            _ok(
                {
                    "hrefs": [item["href"] for item in recovery_output],
                    "started": {"failed": error_started.is_set(), "recovered": recovery_started.is_set()},
                }
            ),
            random="shuffle patched to identity",
            trace=[
                {
                    "sequence": 1,
                    "kind": "note",
                    "note": "engine error does not mark provider; later same-provider engine runs",
                }
            ],
        )
    )

    def two_results(_query: str, **_kwargs: Any) -> list[Any]:
        return [
            TextResult(title="one", href="https://one.example", body=""),
            TextResult(title="two", href="https://two.example", body=""),
        ]

    def three_results(_query: str, **_kwargs: Any) -> list[Any]:
        return [
            TextResult(title="one", href="https://one.example", body=""),
            TextResult(title="two", href="https://two.example", body=""),
            TextResult(title="three", href="https://three.example", body=""),
        ]

    one_engine = {"probe": {"one": _engine_class("one", "one", 1, two_results)}}
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(one_engine, lambda _items: None)
    try:
        zero_output = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=0, backend="all")
        negative_max = _error(lambda: core.DDGS(timeout=1)._search_sync("probe", "query", max_results=-20, backend="all"))
        core.DDGS.threads = -1
        negative_threads = _error(lambda: core.DDGS(timeout=1)._search_sync("probe", "query", max_results=10, backend="all"))
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.extend([
        _fixture(
            "pure.scheduler-zero-max-unlimited",
            "search_scheduler",
            {"max_results": 0},
            _ok([item["href"] for item in zero_output]),
            trace=[{"sequence": 1, "kind": "note", "note": "fake engine returns two results"}],
        ),
        _fixture("pure.scheduler-negative-max-workers", "search_scheduler", {"max_results": -20}, negative_max),
        _fixture("pure.scheduler-negative-threads-workers", "search_scheduler", {"max_results": 10, "threads": -1}, negative_threads),
    ])

    slice_engine = {"probe": {"many": _engine_class("many", "many", 1, three_results)}}
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(slice_engine, lambda _items: None)
    try:
        positive_limit = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=2, backend="all")
        unlimited_none = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=None, backend="all")
        negative_one = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=-1, backend="all")
        negative_nine = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=-9, backend="all")
        negative_ten = _error(lambda: core.DDGS(timeout=1)._search_sync("probe", "query", max_results=-10, backend="all"))
        core.DDGS.threads = 0
        zero_threads = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=2, backend="all")
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.extend([
        _fixture(
            "pure.scheduler-positive-max-final-slice",
            "search_scheduler",
            {"max_results": 2},
            _ok([item["href"] for item in positive_limit]),
            trace=[{"sequence": 1, "kind": "note", "note": "fake engine returns three results before final slice"}],
        ),
        _fixture(
            "pure.scheduler-none-max-unlimited",
            "search_scheduler",
            {"max_results": None},
            _ok([item["href"] for item in unlimited_none]),
            trace=[{"sequence": 1, "kind": "note", "note": "fake engine returns three results"}],
        ),
        _fixture(
            "pure.scheduler-negative-one-final-slice",
            "search_scheduler",
            {"max_results": -1},
            _ok([item["href"] for item in negative_one]),
            trace=[{"sequence": 1, "kind": "note", "note": "negative value leaves one positive worker then uses Python negative slicing"}],
        ),
        _fixture(
            "pure.scheduler-negative-nine-final-slice",
            "search_scheduler",
            {"max_results": -9},
            _ok([item["href"] for item in negative_nine]),
            trace=[{"sequence": 1, "kind": "note", "note": "negative value leaves one positive worker then uses Python negative slicing"}],
        ),
        _fixture("pure.scheduler-negative-ten-max-workers", "search_scheduler", {"max_results": -10}, negative_ten),
        _fixture(
            "pure.scheduler-zero-threads-ignored",
            "search_scheduler",
            {"max_results": 2, "threads": 0},
            _ok([item["href"] for item in zero_threads]),
            trace=[{"sequence": 1, "kind": "note", "note": "falsy class threads does not lower worker count"}],
        ),
    ])

    def formula_result(name: str) -> Callable[..., list[Any]]:
        def search(_query: str, **_kwargs: Any) -> list[Any]:
            return [TextResult(title=name, href=f"https://{name}.example", body="")]

        return search

    formula_engines = {
        "probe": {
            name: _engine_class(name, name, 1, formula_result(name))
            for name in ("one", "two", "three")
        }
    }

    def capture_worker_case(max_results: int | None, threads: int | None) -> dict[str, Any]:
        core, old_engines, old_shuffle, old_threads = _patched_core_engines(formula_engines, lambda _items: None)
        original_executor = core.ThreadPoolExecutor
        observed_workers: list[int] = []

        class RecordingExecutor(original_executor):
            def __init__(self, max_workers: int | None = None, *args: Any, **kwargs: Any) -> None:
                observed_workers.append(max_workers if max_workers is not None else 1)
                super().__init__(max_workers=max_workers, *args, **kwargs)

        core.ThreadPoolExecutor = RecordingExecutor
        core.DDGS.threads = threads
        try:
            output = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=max_results, backend="all")
            return {
                "max_workers": observed_workers,
                "hrefs": [item["href"] for item in output],
            }
        finally:
            core.ThreadPoolExecutor = original_executor
            _restore_core_engines(core, old_engines, old_shuffle, old_threads)

    worker_cases = [
        {"name": "none", "max_results": None, "threads": None},
        {"name": "zero", "max_results": 0, "threads": None},
        {"name": "one", "max_results": 1, "threads": None},
        {"name": "ten", "max_results": 10, "threads": None},
        {"name": "eleven", "max_results": 11, "threads": None},
        {"name": "twenty", "max_results": 20, "threads": None},
        {"name": "thread-one", "max_results": 10, "threads": 1},
        {"name": "thread-large", "max_results": 10, "threads": 10},
        {"name": "thread-zero", "max_results": 10, "threads": 0},
    ]
    worker_output = {
        case["name"]: capture_worker_case(case["max_results"], case["threads"])
        for case in worker_cases
    }
    fixtures.append(
        _fixture(
            "pure.scheduler-worker-formula",
            "search_scheduler",
            {"cases": worker_cases},
            _ok(worker_output),
            random="shuffle patched to identity",
            trace=[{"sequence": 1, "kind": "note", "note": "ThreadPoolExecutor constructor records source worker count"}],
        )
    )

    float_boundary_values = [
        9_007_199_254_740_991,
        9_007_199_254_740_992,
        9_007_199_254_740_993,
        9_223_372_036_854_775_806,
        9_223_372_036_854_775_807,
    ]
    fixtures.append(
        _fixture(
            "pure.scheduler-worker-formula-float-boundary",
            "search_scheduler",
            {"max_results": float_boundary_values},
            _ok({str(value): math.ceil(value / 10) + 1 for value in float_boundary_values}),
            trace=[
                {
                    "sequence": 1,
                    "kind": "note",
                    "note": "frozen source evaluates ceil(max_results / 10) through Python float",
                }
            ],
        )
    )

    pool_lock = threading.Lock()
    pool_release = threading.Event()
    pool_second_started = threading.Event()
    pool_active = 0
    pool_max_active = 0
    pool_started: dict[str, bool] = {"one": False, "two": False, "three": False}

    def pool_bound_result(name: str) -> Callable[..., list[Any]]:
        def search(_query: str, **_kwargs: Any) -> list[Any]:
            nonlocal pool_active, pool_max_active
            with pool_lock:
                pool_started[name] = True
                pool_active += 1
                pool_max_active = max(pool_max_active, pool_active)
            if name == "two":
                pool_second_started.set()
                threading.Timer(0.05, pool_release.set).start()
            elif name == "one" and not pool_second_started.wait(1):
                raise RuntimeError("second pool worker did not start")
            if not pool_release.wait(1):
                raise RuntimeError("pool release timer failed")
            with pool_lock:
                pool_active -= 1
            return [TextResult(title=name, href=f"https://pool-{name}.example", body="")]

        return search

    pool_engines = {
        "probe": {
            name: _engine_class(name, name, 1, pool_bound_result(name))
            for name in ("one", "two", "three")
        }
    }
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(pool_engines, lambda _items: None)
    try:
        pool_output = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=10, backend="all")
    finally:
        pool_release.set()
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-worker-pool-bound",
            "search_scheduler",
            {"category": "probe", "max_results": 10, "backend": "all"},
            _ok(
                {
                    "hrefs": [item["href"] for item in pool_output],
                    "max_active": pool_max_active,
                    "started": pool_started,
                }
            ),
            random="shuffle patched to identity",
            trace=[
                {
                    "sequence": 1,
                    "kind": "note",
                    "note": "three unique providers, source worker formula gives two concurrent workers",
                }
            ],
        )
    )

    release_slow, slow_started = threading.Event(), threading.Event()

    def slow(_query: str, **_kwargs: Any) -> list[Any]:
        slow_started.set()
        if not release_slow.wait(1):
            raise RuntimeError("release timer failed")
        return [TextResult(title="late", href="https://late.example", body="late")]

    def failing(_query: str, **_kwargs: Any) -> list[Any]:
        if not slow_started.wait(1):
            raise RuntimeError("slow engine did not start")
        threading.Timer(0.05, release_slow.set).start()
        raise RuntimeError("boom")

    partial_engines = {
        "probe": {
            "failing": _engine_class("failing", "failing", 1, failing),
            "slow": _engine_class("slow", "slow", 1, slow),
        }
    }
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(partial_engines, lambda _items: None)
    try:
        partial = _error(lambda: core.DDGS(timeout=1)._search_sync("probe", "query", max_results=10, backend="all"))
    finally:
        release_slow.set()
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-first-exception-late-result-not-aggregated",
            "search_scheduler",
            {"max_results": 10, "engines": ["failing", "slow"]},
            partial,
            trace=[{"sequence": 1, "kind": "note", "note": "slow result completes after FIRST_EXCEPTION wakeup"}],
        )
    )

    success_started, failure_started, success_finished = threading.Event(), threading.Event(), threading.Event()

    def completed_success(_query: str, **_kwargs: Any) -> list[Any]:
        success_started.set()
        if not failure_started.wait(1):
            raise RuntimeError("failing engine did not start")
        success_finished.set()
        return [TextResult(title="kept", href="https://kept.example", body="")]

    def completed_failure(_query: str, **_kwargs: Any) -> list[Any]:
        failure_started.set()
        if not success_finished.wait(1):
            raise RuntimeError("successful engine did not complete")
        raise RuntimeError("boom after success")

    completed_engines = {
        "probe": {
            "success": _engine_class("success", "success", 1, completed_success),
            "failure": _engine_class("failure", "failure", 1, completed_failure),
        }
    }
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(completed_engines, lambda _items: None)
    try:
        completed_output = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=10, backend="all")
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-completed-success-survives-first-exception",
            "search_scheduler",
            {"category": "probe", "max_results": 10, "backend": "all"},
            _ok([item["href"] for item in completed_output]),
            random="shuffle patched to identity",
            trace=[
                {
                    "sequence": 1,
                    "kind": "note",
                    "note": "success is done before error wakeup and remains in output",
                }
            ],
        )
    )

    quick_started, slow_timeout_started, release_timeout_slow = threading.Event(), threading.Event(), threading.Event()

    def quick_timeout_result(_query: str, **_kwargs: Any) -> list[Any]:
        quick_started.set()
        if not slow_timeout_started.wait(1):
            raise RuntimeError("slow engine did not start")
        return [TextResult(title="quick", href="https://quick.example", body="")]

    def slow_timeout_result(_query: str, **_kwargs: Any) -> list[Any]:
        slow_timeout_started.set()
        threading.Timer(0.08, release_timeout_slow.set).start()
        if not release_timeout_slow.wait(1):
            raise RuntimeError("slow release timer failed")
        return [TextResult(title="late", href="https://late-timeout.example", body="")]

    timeout_partial_engines = {
        "probe": {
            "quick": _engine_class("quick", "quick", 1, quick_timeout_result),
            "slow": _engine_class("slow", "slow", 1, slow_timeout_result),
        }
    }
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(timeout_partial_engines, lambda _items: None)
    try:
        timeout_partial_output = core.DDGS(timeout=0.01)._search_sync("probe", "query", max_results=10, backend="all")
    finally:
        release_timeout_slow.set()
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-wait-timeout-late-result-not-aggregated",
            "search_scheduler",
            {"category": "probe", "max_results": 10, "wait_timeout_seconds": 0.01},
            _ok([item["href"] for item in timeout_partial_output]),
            random="shuffle patched to identity",
            trace=[
                {
                    "sequence": 1,
                    "kind": "note",
                    "note": "quick completion is retained; pending slow completion is joined but omitted after wait timeout",
                }
            ],
        )
    )

    release_timeout_only = threading.Event()

    def timeout_only_slow(_query: str, **_kwargs: Any) -> list[Any]:
        threading.Timer(0.08, release_timeout_only.set).start()
        if not release_timeout_only.wait(1):
            raise RuntimeError("timeout-only slow release timer failed")
        return [TextResult(title="late", href="https://timeout-only.example", body="")]

    timeout_only_engines = {"probe": {"slow": _engine_class("slow", "slow", 1, timeout_only_slow)}}
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(timeout_only_engines, lambda _items: None)
    try:
        timeout_only = _error(lambda: core.DDGS(timeout=0.01)._search_sync("probe", "query", max_results=10, backend="all"))
    finally:
        release_timeout_only.set()
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-wait-timeout-no-result-generic-error",
            "search_scheduler",
            {"category": "probe", "max_results": 10, "wait_timeout_seconds": 0.01},
            timeout_only,
            random="shuffle patched to identity",
            trace=[
                {
                    "sequence": 1,
                    "kind": "note",
                    "note": "scheduler wait timeout alone leaves no last engine error",
                }
            ],
        )
    )

    first_limit_started, second_limit_started, third_limit_started = threading.Event(), threading.Event(), threading.Event()

    def first_limit(_query: str, **_kwargs: Any) -> list[Any]:
        first_limit_started.set()
        return [TextResult(title="first", href="https://limit-first.example", body="")]

    def second_limit(_query: str, **_kwargs: Any) -> list[Any]:
        second_limit_started.set()
        return [TextResult(title="second", href="https://limit-second.example", body="")]

    def third_limit(_query: str, **_kwargs: Any) -> list[Any]:
        third_limit_started.set()
        return [TextResult(title="third", href="https://limit-third.example", body="")]

    limit_engines = {
        "probe": {
            "first": _engine_class("first", "first", 1, first_limit),
            "second": _engine_class("second", "second", 1, second_limit),
            "third": _engine_class("third", "third", 1, third_limit),
        }
    }
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(limit_engines, lambda _items: None)
    try:
        limit_output = core.DDGS(timeout=1)._search_sync("probe", "query", max_results=1, backend="all")
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-max-reached-stops-later-dispatch",
            "search_scheduler",
            {"category": "probe", "max_results": 1, "backend": "all"},
            _ok(
                {
                    "hrefs": [item["href"] for item in limit_output],
                    "started": {
                        "first": first_limit_started.is_set(),
                        "second": second_limit_started.is_set(),
                        "third": third_limit_started.is_set(),
                    },
                }
            ),
            random="shuffle patched to identity",
            trace=[
                {
                    "sequence": 1,
                    "kind": "note",
                    "note": "batch reaches distinct-result limit before third engine is submitted",
                }
            ],
        )
    )

    def rank_before_slice(_query: str, **_kwargs: Any) -> list[Any]:
        return [
            TextResult(title="irrelevant", href="https://first.example", body=""),
            TextResult(title="needle result", href="https://second.example", body=""),
        ]

    ranking_engines = {"probe": {"rank": _engine_class("rank", "rank", 1, rank_before_slice)}}
    core, old_engines, old_shuffle, old_threads = _patched_core_engines(ranking_engines, lambda _items: None)
    try:
        rank_before_slice_output = core.DDGS(timeout=1)._search_sync(
            "probe", "needle", max_results=1, backend="all"
        )
    finally:
        _restore_core_engines(core, old_engines, old_shuffle, old_threads)
    fixtures.append(
        _fixture(
            "pure.scheduler-rank-before-final-slice",
            "search_scheduler",
            {"category": "probe", "query": "needle", "max_results": 1, "backend": "all"},
            _ok([item["href"] for item in rank_before_slice_output]),
            random="shuffle patched to identity",
            trace=[{"sequence": 1, "kind": "note", "note": "ranked matching second result precedes final max-results slice"}],
        )
    )
    return fixtures


def _error_and_extract_fixtures() -> list[Fixture]:
    core = importlib.import_module("ddgs.ddgs")
    empty_query = _error(lambda: core.DDGS()._search_sync("text", "", keywords=""))

    def timeout_search(_query: str, **_kwargs: Any) -> list[Any]:
        raise RuntimeError("operation timed out exactly")

    def uppercase_timeout_search(_query: str, **_kwargs: Any) -> list[Any]:
        raise RuntimeError("operation TIMED OUT exactly")

    engines = {"probe": {"timeout": _engine_class("timeout", "timeout", 1, timeout_search)}}
    patched, old_engines, old_shuffle, old_threads = _patched_core_engines(engines, lambda _items: None)
    try:
        timeout_result = _error(lambda: patched.DDGS(timeout=1)._search_sync("probe", "query", backend="all"))
    finally:
        _restore_core_engines(patched, old_engines, old_shuffle, old_threads)

    uppercase_engines = {"probe": {"timeout": _engine_class("timeout", "timeout", 1, uppercase_timeout_search)}}
    patched, old_engines, old_shuffle, old_threads = _patched_core_engines(uppercase_engines, lambda _items: None)
    try:
        uppercase_timeout_result = _error(lambda: patched.DDGS(timeout=1)._search_sync("probe", "query", backend="all"))
    finally:
        _restore_core_engines(patched, old_engines, old_shuffle, old_threads)

    accesses: list[str] = []

    class Response:
        status_code = 200
        text = "raw-text"
        content = b"raw-bytes"

        @property
        def text_markdown(self) -> str:
            accesses.append("markdown")
            return "markdown"

        @property
        def text_plain(self) -> str:
            accesses.append("plain")
            return "plain"

        @property
        def text_rich(self) -> str:
            accesses.append("rich")
            return "rich"

    class Client:
        def __init__(self, **_kwargs: Any) -> None:
            pass

        def get(self, _url: str) -> Response:
            return Response()

    old_client = core.HttpClient
    extract_output: dict[str, dict[str, Any]] = {}
    core.HttpClient = Client
    try:
        for fmt in ("content", "text", "text_plain", "text_rich", "unknown"):
            accesses.clear()
            content = core.DDGS().extract("https://example.test", fmt=fmt)["content"]
            extract_output[fmt] = {
                "content": content.decode("ascii") if isinstance(content, bytes) else content,
                "content_kind": "bytes" if isinstance(content, bytes) else "string",
                "renderer_accesses": list(accesses),
            }
    finally:
        core.HttpClient = old_client

    return [
        _fixture("pure.error-empty-query", "search_error", {"query": "", "keywords": ""}, empty_query),
        _fixture(
            "pure.error-timeout-string-heuristic",
            "search_error",
            {"engine_error": "operation timed out exactly"},
            timeout_result,
            trace=[{"sequence": 1, "kind": "note", "note": "fake engine only"}],
        ),
        _fixture(
            "pure.error-uppercase-timeout-string-generic",
            "search_error",
            {"engine_error": "operation TIMED OUT exactly"},
            uppercase_timeout_result,
            trace=[{"sequence": 1, "kind": "note", "note": "source substring match is lowercase and case-sensitive"}],
        ),
        _fixture(
            "pure.extract-lazy-format-selection",
            "extract_format_selection",
            {"formats": ["content", "text", "text_plain", "text_rich", "unknown"]},
            _ok(extract_output),
            trace=[{"sequence": 1, "kind": "note", "note": "fake HTTP client only; selected renderer access is observable"}],
        ),
    ]


def _capture_loopback_extract(
    body: bytes,
    status: int,
    fmt: str,
) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    """Run frozen extract against synthetic loopback HTML and sanitize its URL."""
    core = importlib.import_module("ddgs.ddgs")
    fixture_url = "https://extract.fixture/page"
    events: list[dict[str, Any]] = []
    received_requests: list[tuple[str, str]] = []

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:  # noqa: N802
            received_requests.append((self.command, self.path))
            self.send_response(status)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, _format: str, *_args: Any) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    actual_url = f"http://127.0.0.1:{server.server_port}/page"
    original_client = core.HttpClient
    original_proxy_env = os.environ.pop("DDGS_PROXY", None)

    class CapturedResponse:
        def __init__(self, response: Any) -> None:
            self._response = response

        @property
        def status_code(self) -> int:
            return self._response.status_code

        @property
        def content(self) -> bytes:
            events.append({"kind": "note", "note": "response.content selected"})
            return self._response.content

        @property
        def text(self) -> str:
            events.append({"kind": "note", "note": "response.text selected"})
            return self._response.text

        @property
        def text_markdown(self) -> str:
            events.append({"kind": "note", "note": "response.text_markdown selected"})
            return self._response.text_markdown

        @property
        def text_plain(self) -> str:
            events.append({"kind": "note", "note": "response.text_plain selected"})
            return self._response.text_plain

        @property
        def text_rich(self) -> str:
            events.append({"kind": "note", "note": "response.text_rich selected"})
            return self._response.text_rich

    class CapturingClient:
        def __init__(self, *args: Any, **kwargs: Any) -> None:
            events.append(
                {
                    "kind": "note",
                    "note": "HttpClient constructor",
                    "value": {
                        "proxy": kwargs.get("proxy"),
                        "timeout": kwargs.get("timeout"),
                        "verify": kwargs.get("verify"),
                    },
                }
            )
            self._client = original_client(*args, **kwargs)

        def get(self, url: str, *args: Any, **kwargs: Any) -> CapturedResponse:
            events.append({"kind": "request", "method": "GET", "url": url})
            response = self._client.get(url, *args, **kwargs)
            events.append(
                {
                    "kind": "response",
                    "status": response.status_code,
                    "text": response.text,
                    "content_hex": hexlify(response.content).decode(),
                }
            )
            return CapturedResponse(response)

    core.HttpClient = CapturingClient
    try:
        try:
            extracted = core.DDGS(proxy=None, timeout=5, verify=True).extract(actual_url, fmt=fmt)
        except Exception as exc:  # frozen source error is a fixture result
            outcome = _error_from_exception(exc)
            outcome["error"]["message"] = outcome["error"]["message"].replace(actual_url, fixture_url)
        else:
            if extracted["url"] != actual_url:
                raise AssertionError("frozen extract did not preserve the input URL")
            content = extracted["content"]
            output: dict[str, Any] = {"url": fixture_url}
            if isinstance(content, bytes):
                output["content_kind"] = "bytes"
                output["content_hex"] = hexlify(content).decode()
            else:
                output["content_kind"] = "string"
                output["content"] = content
            outcome = _ok(output)
    finally:
        core.HttpClient = original_client
        if original_proxy_env is None:
            os.environ.pop("DDGS_PROXY", None)
        else:
            os.environ["DDGS_PROXY"] = original_proxy_env
        server.shutdown()
        thread.join()
        server.server_close()

    if received_requests != [("GET", "/page")]:
        raise AssertionError(f"unexpected loopback extract requests: {received_requests!r}")
    sanitized_trace: list[dict[str, Any]] = []
    for event in _sequenced_trace(events):
        sanitized_event = dict(event)
        if "url" in sanitized_event:
            sanitized_event["url"] = sanitized_event["url"].replace(actual_url, fixture_url)
        sanitized_trace.append(sanitized_event)
    return outcome, sanitized_trace


def _capture_http_client_constructor(
    config: dict[str, Any],
    *,
    method: str = "GET",
    failure: str | None = None,
) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    """Capture HttpClient construction and one synthetic request without I/O."""
    http_client = importlib.import_module("ddgs.http_client")
    events: list[dict[str, Any]] = []
    constructor_calls: list[dict[str, Any]] = []

    class SyntheticResponse:
        status_code = 201
        content = b"transport fixture bytes"
        text = "transport fixture text"

    class SyntheticClient:
        def __init__(self, **kwargs: Any) -> None:
            constructor_calls.append(kwargs)

        def request(self, request_method: str, url: str, **kwargs: Any) -> SyntheticResponse:
            event: dict[str, Any] = {"kind": "request", "method": request_method, "url": url}
            if "params" in kwargs:
                event["query"] = dict(kwargs["params"])
            if "cookies" in kwargs:
                event["cookies"] = dict(kwargs["cookies"])
            events.append(event)
            if failure == "timeout":
                raise http_client.primp.TimeoutError("fixture timed out")
            if failure == "generic":
                raise RuntimeError("fixture failure")
            return SyntheticResponse()

    original_client = http_client.primp.Client
    http_client.primp.Client = SyntheticClient
    try:
        client = http_client.HttpClient(**config)
        if len(constructor_calls) != 1:
            raise AssertionError(f"expected one source constructor call, got {constructor_calls!r}")
        constructor = constructor_calls[0]
        events.insert(
            0,
            {
                "kind": "note",
                "note": "primp.Client constructor",
                "value": {
                    "proxy": constructor["proxy"],
                    "timeout": constructor["timeout"],
                    "impersonate": constructor["impersonate"],
                    "impersonate_os": constructor["impersonate_os"],
                    "verify": constructor["verify"],
                    "ca_cert_file": constructor["ca_cert_file"],
                },
            },
        )
        try:
            response = client.request(
                method,
                "https://transport.fixture/request",
                params={"q": "needle"},
                cookies={"fixture_cookie": "value"},
            )
        except Exception as exc:
            outcome = _error_from_exception(exc)
        else:
            events.append(
                {
                    "kind": "response",
                    "status": response.status_code,
                    "text": response.text,
                    "content_hex": hexlify(response.content).decode(),
                }
            )
            outcome = _ok(
                {
                    "status": response.status_code,
                    "text": response.text,
                    "content_hex": hexlify(response.content).decode(),
                }
            )
    finally:
        http_client.primp.Client = original_client
    return outcome, _sequenced_trace(events)


def _capture_loopback_transport(case: str) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    """Run frozen HttpClient against synthetic loopback endpoints only."""
    http_client = importlib.import_module("ddgs.http_client")
    fixture_base = "https://transport.fixture"
    events: list[dict[str, Any]] = []
    cookie_value = ""

    class Handler(BaseHTTPRequestHandler):
        def do_GET(self) -> None:  # noqa: N802
            nonlocal cookie_value
            events.append({"kind": "request", "method": "GET", "url": actual_base + self.path})
            if self.path == "/set-cookie":
                body = b"cookie set"
                self._send(200, body, {"Set-Cookie": "fixture_cookie=source; Path=/"})
                return
            if self.path == "/cookie-check":
                cookie_value = self.headers.get("Cookie", "")
                events.append(
                    {
                        "kind": "cookie",
                        "url": actual_base + self.path,
                        "cookies": {"fixture_cookie": cookie_value.removeprefix("fixture_cookie=")},
                    }
                )
                self._send(200, cookie_value.encode())
                return
            if self.path == "/redirect":
                self._send(302, b"redirect", {"Location": "/target"})
                return
            if self.path == "/target":
                self._send(200, b"redirect target")
                return
            if self.path == "/gzip":
                self._send(200, gzip.compress(b"compressed fixture", mtime=0), {"Content-Encoding": "gzip"})
                return
            if self.path == "/status":
                self._send(503, b"unavailable")
                return
            if self.path == "/slow":
                time.sleep(0.15)
                self._send(200, b"slow response")
                return
            self._send(404, b"not found")

        def _send(self, status: int, body: bytes, headers: dict[str, str] | None = None) -> None:
            response_headers = headers or {}
            self.send_response(status)
            self.send_header("Content-Length", str(len(body)))
            for name, value in response_headers.items():
                self.send_header(name, value)
            self.end_headers()
            events.append(
                {
                    "kind": "response",
                    "status": status,
                    "headers": response_headers,
                    "content_hex": hexlify(body).decode(),
                }
            )
            try:
                self.wfile.write(body)
            except BrokenPipeError:
                pass

        def log_message(self, _format: str, *_args: Any) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    actual_base = f"http://127.0.0.1:{server.server_port}"
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    saved_environment = {name: os.environ.pop(name, None) for name in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy")}

    try:
        timeout = 0.01 if case == "timeout" else 5
        client = http_client.HttpClient(proxy=None, timeout=timeout, verify=True)
        try:
            if case == "cookies-redirect":
                set_response = client.get(actual_base + "/set-cookie")
                cookie_response = client.get(actual_base + "/cookie-check")
                redirect_response = client.get(actual_base + "/redirect")
                outcome = _ok(
                    {
                        "set": {"status": set_response.status_code, "text": set_response.text},
                        "cookie": {"status": cookie_response.status_code, "text": cookie_response.text},
                        "redirect": {"status": redirect_response.status_code, "text": redirect_response.text},
                    }
                )
            else:
                path = {"gzip": "/gzip", "status": "/status", "timeout": "/slow"}[case]
                response = client.get(actual_base + path)
                outcome = _ok(
                    {
                        "status": response.status_code,
                        "text": response.text,
                        "content_hex": hexlify(response.content).decode(),
                    }
                )
        except Exception as exc:
            outcome = _error_from_exception(exc)
            outcome["error"]["message"] = outcome["error"]["message"].replace(actual_base, fixture_base)
    finally:
        for name, value in saved_environment.items():
            if value is None:
                os.environ.pop(name, None)
            else:
                os.environ[name] = value
        server.shutdown()
        thread.join()
        server.server_close()

    trace: list[dict[str, Any]] = []
    for event in _sequenced_trace(events):
        sanitized = dict(event)
        if "url" in sanitized:
            sanitized["url"] = sanitized["url"].replace(actual_base, fixture_base)
        trace.append(sanitized)
    return outcome, trace


def _capture_socks_loopback_transport(scheme: str) -> tuple[dict[str, Any], list[dict[str, Any]]]:
    """Capture source SOCKS5 versus SOCKS5H name-resolution behavior locally."""
    http_client = importlib.import_module("ddgs.http_client")
    fixture_url = "https://transport.fixture/probe"
    actual_target = "http://localhost:43210/probe"
    events: list[dict[str, Any]] = []
    server_result: dict[str, Any] = {}
    server_done = threading.Event()

    def read_exact(connection: socket.socket, size: int) -> bytes:
        value = b""
        while len(value) < size:
            chunk = connection.recv(size - len(value))
            if not chunk:
                raise EOFError(f"wanted {size} SOCKS bytes, got {len(value)}")
            value += chunk
        return value

    listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    listener.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    listener.bind(("127.0.0.1", 0))
    listener.listen(1)
    listener.settimeout(4)
    actual_proxy = f"{scheme}://127.0.0.1:{listener.getsockname()[1]}"

    def serve() -> None:
        try:
            connection, _address = listener.accept()
            with connection:
                connection.settimeout(3)
                version, method_count = read_exact(connection, 2)
                methods = read_exact(connection, method_count)
                if version != 5 or methods != b"\x00":
                    raise AssertionError(f"unexpected SOCKS greeting {(version, methods)!r}")
                connection.sendall(b"\x05\x00")

                version, command, reserved, address_type = read_exact(connection, 4)
                if address_type == 1:
                    target_host = socket.inet_ntoa(read_exact(connection, 4))
                    address_type_name = "ipv4"
                elif address_type == 3:
                    target_host = read_exact(connection, read_exact(connection, 1)[0]).decode("ascii")
                    address_type_name = "domain"
                elif address_type == 4:
                    target_host = socket.inet_ntop(socket.AF_INET6, read_exact(connection, 16))
                    address_type_name = "ipv6"
                else:
                    raise AssertionError(f"unexpected SOCKS address type {address_type}")
                target_port = int.from_bytes(read_exact(connection, 2), "big")
                server_result["connect"] = {
                    "version": version,
                    "command": command,
                    "reserved": reserved,
                    "address_type": address_type_name,
                    "host": target_host,
                    "port": target_port,
                }
                connection.sendall(b"\x05\x00\x00\x01\x00\x00\x00\x00\x00\x00")

                request_line = connection.recv(4096).split(b"\r\n", 1)[0].decode("ascii", "replace")
                server_result["request_line"] = request_line
                body = b"socks fixture"
                connection.sendall(
                    b"HTTP/1.1 200 OK\r\nContent-Length: "
                    + str(len(body)).encode("ascii")
                    + b"\r\nConnection: close\r\n\r\n"
                    + body
                )
        except Exception as exc:  # source transport behavior is fixture data
            server_result["error"] = f"{type(exc).__name__}: {exc!r}"
        finally:
            server_done.set()

    thread = threading.Thread(target=serve, daemon=True)
    thread.start()
    saved_environment = {
        name: os.environ.pop(name, None)
        for name in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy")
    }
    try:
        try:
            response = http_client.HttpClient(proxy=actual_proxy, timeout=2).get(actual_target)
        except Exception as exc:
            outcome = _error_from_exception(exc)
            outcome["error"]["message"] = outcome["error"]["message"].replace(actual_target, fixture_url)
        else:
            outcome = _ok(
                {
                    "status": response.status_code,
                    "text": response.text,
                    "content_hex": hexlify(response.content).decode(),
                    "connect": server_result.get("connect"),
                    "request_line": server_result.get("request_line"),
                }
            )
    finally:
        for name, value in saved_environment.items():
            if value is None:
                os.environ.pop(name, None)
            else:
                os.environ[name] = value
        server_done.wait(4)
        thread.join(0.1)
        listener.close()

    if "error" in server_result:
        raise AssertionError(f"SOCKS loopback server failed: {server_result['error']}")
    if not server_done.is_set():
        raise AssertionError("SOCKS loopback server did not finish")
    if outcome["status"] != "ok":
        raise AssertionError(f"source SOCKS request failed: {outcome!r}")

    events.extend(
        [
            {"kind": "request", "method": "GET", "url": fixture_url},
            {
                "kind": "note",
                "note": "SOCKS CONNECT target observed by synthetic proxy",
                "value": {"scheme": scheme, **server_result["connect"]},
            },
            {
                "kind": "response",
                "status": outcome["output"]["status"],
                "content_hex": outcome["output"]["content_hex"],
            },
        ]
    )
    return outcome, _sequenced_trace(events)


def _transport_fixtures() -> list[Fixture]:
    """Capture HttpClient construction and loopback-only HTTP behavior."""
    fixtures: list[Fixture] = []
    constructor_cases = [
        ("transport.constructor-default-get", {}, "GET", None),
        (
            "transport.constructor-http-proxy-verify-off-post",
            {"proxy": "http://proxy.fixture:8080", "timeout": 7, "verify": False},
            "POST",
            None,
        ),
        (
            "transport.constructor-https-proxy-pem",
            {"proxy": "https://proxy.fixture:8443", "timeout": None, "verify": "fixture-root.pem"},
            "GET",
            None,
        ),
        (
            "transport.constructor-socks5-proxy",
            {"proxy": "socks5://proxy.fixture:1080", "timeout": 5, "verify": True},
            "GET",
            None,
        ),
        (
            "transport.constructor-socks5h-proxy",
            {"proxy": "socks5h://proxy.fixture:1080", "timeout": 5, "verify": True},
            "GET",
            None,
        ),
        ("transport.timeout-error-classification", {}, "GET", "timeout"),
        ("transport.generic-error-classification", {}, "GET", "generic"),
    ]
    for fixture_id, config, method, failure in constructor_cases:
        outcome, trace = _capture_http_client_constructor(config, method=method, failure=failure)
        fixtures.append(
            _transport_fixture(
                fixture_id,
                "http_client_request",
                {"constructor": config, "method": method, "failure": failure},
                outcome,
                trace=trace,
                notes=["local primp.Client double; no socket or proxy connection"],
            )
        )

    for case, fixture_id in (
        ("cookies-redirect", "transport.loopback-cookies-and-redirects"),
        ("gzip", "transport.loopback-gzip-decompression"),
        ("status", "transport.loopback-non-200-preserved"),
        ("timeout", "transport.loopback-timeout-classification"),
    ):
        outcome, trace = _capture_loopback_transport(case)
        fixtures.append(
            _transport_fixture(
                fixture_id,
                "http_client_loopback",
                {"case": case, "base_url": "https://transport.fixture"},
                outcome,
                trace=trace,
                notes=["ephemeral loopback server with synthetic payload; URL rewritten before output"],
            )
        )

    for scheme in ("socks5", "socks5h"):
        outcome, trace = _capture_socks_loopback_transport(scheme)
        fixtures.append(
            _transport_fixture(
                f"transport.loopback-{scheme}-resolution",
                "http_client_socks_loopback",
                {"proxy_scheme": scheme, "target_url": "https://transport.fixture/probe"},
                outcome,
                trace=trace,
                notes=["ephemeral local SOCKS5 server; target and proxy URLs are rewritten before output"],
            )
        )
    return fixtures


def _extract_fixtures() -> list[Fixture]:
    """Capture source extraction selection and rendering from loopback-only HTML."""
    html_body = (
        b'<!doctype html><html><body><h1>Fixture heading</h1><p>Hello '
        b'<a href="https://target.example/path">link</a>.</p><ul><li>One</li><li>Two</li></ul></body></html>'
    )
    cases = [
        ("content", "extract.html-content-bytes"),
        ("text", "extract.html-text-raw"),
        ("text_markdown", "extract.html-text-markdown"),
        ("text_plain", "extract.html-text-plain"),
        ("text_rich", "extract.html-text-rich"),
        ("unknown-format", "extract.html-unknown-format-markdown-fallback"),
    ]
    fixtures: list[Fixture] = []
    for fmt, fixture_id in cases:
        outcome, trace = _capture_loopback_extract(html_body, 200, fmt)
        fixtures.append(
            _extract_fixture(
                fixture_id,
                "extract",
                {"url": "https://extract.fixture/page", "fmt": fmt},
                outcome,
                trace=trace,
                notes=["loopback-only synthetic HTML; rendered values come from frozen primp response properties"],
            )
        )

    binary_body = b"\xff\x00fixture\x80"
    for fmt, fixture_id in (
        ("content", "extract.binary-content-preserves-bytes"),
        ("text", "extract.binary-text-source-decoding"),
    ):
        outcome, trace = _capture_loopback_extract(binary_body, 200, fmt)
        fixtures.append(
            _extract_fixture(
                fixture_id,
                "extract",
                {"url": "https://extract.fixture/page", "fmt": fmt, "response_content_hex": hexlify(binary_body).decode()},
                outcome,
                trace=trace,
                notes=["loopback-only synthetic non-UTF-8 body; records source raw-byte versus decoded-text behavior"],
            )
        )

    outcome, trace = _capture_loopback_extract(b"synthetic unavailable", 503, "text_markdown")
    fixtures.append(
        _extract_fixture(
            "extract.non-200-ddgs-error",
            "extract",
            {"url": "https://extract.fixture/page", "fmt": "text_markdown", "status": 503},
            outcome,
            trace=trace,
            notes=["loopback-only synthetic HTTP 503; source rejects it before selecting a renderer property"],
        )
    )
    return fixtures


def _engine_result_dicts(results: list[Any] | None) -> list[dict[str, Any]]:
    return [dict(result.__dict__) for result in results or []]


def _engine_search_output(results: list[Any] | None) -> list[dict[str, Any]] | None:
    return None if results is None else _engine_result_dicts(results)


def _synthetic_search_fixture(
    fixture_id: str,
    category: str,
    engine_name: str,
    input_value: dict[str, Any],
    engine_class: type[Any],
    responses: list[_SyntheticResponse],
    *,
    client_module: str = "ddgs.base",
    client_name: str = "HttpClient",
    notes: list[str] | None = None,
) -> Fixture:
    outcome, trace = _capture_synthetic_engine_outcome(
        responses,
        lambda _events: _engine_search_output(engine_class().search(**input_value)),
        client_module=client_module,
        client_name=client_name,
    )
    if outcome["status"] == "ok" and isinstance(outcome["output"], list):
        outcome = _ok(outcome["output"], field_order=[list(item) for item in outcome["output"]])
    return _engine_fixture(
        fixture_id,
        category,
        engine_name,
        "search",
        input_value,
        outcome,
        trace=trace,
        notes=notes or ["all HTTP responses are synthetic"],
    )


def _synthetic_engine_fixture(
    fixture_id: str,
    category: str,
    engine_name: str,
    operation: str,
    input_value: dict[str, Any],
    responses: list[_SyntheticResponse],
    action: Callable[[list[dict[str, Any]]], Any],
    *,
    client_module: str = "ddgs.base",
    client_name: str = "HttpClient",
    clock: str = "not used",
    random: str = "not used",
    notes: list[str] | None = None,
) -> Fixture:
    outcome, trace = _capture_synthetic_engine_outcome(
        responses,
        action,
        client_module=client_module,
        client_name=client_name,
    )
    if outcome["status"] == "ok" and isinstance(outcome["output"], list):
        if all(isinstance(item, dict) for item in outcome["output"]):
            outcome = _ok(outcome["output"], field_order=[list(item) for item in outcome["output"]])
    return _engine_fixture(
        fixture_id,
        category,
        engine_name,
        operation,
        input_value,
        outcome,
        trace=trace,
        clock=clock,
        random=random,
        notes=notes or ["all HTTP responses are synthetic"],
    )


def _stable_engine_search(engine_class: type[Any], input_value: dict[str, Any], events: list[dict[str, Any]]) -> Any:
    """Invoke one source engine while controlling its request-time random state."""
    restores: list[Callable[[], None]] = []
    if engine_class is Yahoo:
        yahoo_module = importlib.import_module("ddgs.engines.yahoo")
        old_token_urlsafe = yahoo_module.token_urlsafe

        def token_urlsafe(nbytes: int) -> str:
            value = f"synthetic-token-{nbytes}"
            events.append({"kind": "random", "value": {"function": "token_urlsafe", "nbytes": nbytes, "value": value}})
            return value

        yahoo_module.token_urlsafe = token_urlsafe
        restores.append(lambda: setattr(yahoo_module, "token_urlsafe", old_token_urlsafe))
    elif engine_class is Yandex:
        yandex_module = importlib.import_module("ddgs.engines.yandex")
        old_random = yandex_module.random

        class SyntheticRandom:
            def randint(self, lower: int, upper: int) -> int:
                value = 4242424
                events.append(
                    {"kind": "random", "value": {"function": "randint", "lower": lower, "upper": upper, "value": value}}
                )
                return value

        yandex_module.random = SyntheticRandom()
        restores.append(lambda: setattr(yandex_module, "random", old_random))
    elif engine_class is AnnasArchive:
        old_search_url = AnnasArchive.search_url
        AnnasArchive.search_url = "https://annas-archive.fixture/search"
        restores.append(lambda: setattr(AnnasArchive, "search_url", old_search_url))
    try:
        return _engine_search_output(engine_class().search(**input_value))
    finally:
        for restore in reversed(restores):
            restore()


def _engine_visible_fixtures() -> list[Fixture]:
    """Capture source engine request behavior against deterministic fake HTTP."""
    fixtures: list[Fixture] = []

    media_cases: list[tuple[str, type[Any], dict[str, Any]]] = [
        (
            "images",
            DuckduckgoImages,
            {
                "query": "fixture images",
                "region": "us-en",
                "safesearch": "off",
                "timelimit": "w",
                "page": 2,
                "size": "Large",
                "color": "Red",
                "type_image": "photo",
                "layout": "Wide",
                "license_image": "Public",
            },
        ),
        (
            "news",
            DuckduckgoNews,
            {
                "query": "fixture news",
                "region": "us-en",
                "safesearch": "off",
                "timelimit": "d",
                "page": 2,
            },
        ),
        (
            "videos",
            DuckduckgoVideos,
            {
                "query": "fixture videos",
                "region": "us-en",
                "safesearch": "off",
                "timelimit": "w",
                "page": 2,
                "resolution": "high",
                "duration": "medium",
                "license_videos": "creativeCommon",
            },
        ),
    ]
    for category, engine_class, input_value in media_cases:
        output, trace = _capture_synthetic_engine(
            [
                _SyntheticResponse(content=b'<html><input vqd="fixture-vqd"></html>'),
                _SyntheticResponse(text=json.dumps({"results": []})),
            ],
            lambda _events, engine_class=engine_class, input_value=input_value: _engine_result_dicts(
                engine_class().search(**input_value)
            ),
        )
        fixtures.append(
            _engine_fixture(
                f"engine.{category}.duckduckgo-vqd-bootstrap",
                category,
                "duckduckgo",
                "search",
                input_value,
                _ok(output),
                trace=trace,
                notes=["all HTTP responses are synthetic; VQD comes from synthetic raw bytes"],
            )
        )

    def startpage_action(_events: list[dict[str, Any]]) -> dict[str, Any]:
        engine = Startpage()
        first = engine.search(
            "fixture startpage",
            region="us-en",
            safesearch="off",
            timelimit="w",
            page=2,
        )
        second = engine.search(
            "fixture startpage",
            region="us-en",
            safesearch="off",
            timelimit="m",
            page=3,
        )
        return {"results": [_engine_result_dicts(first), _engine_result_dicts(second)], "stored_sc": engine._sc}

    startpage_input = {
        "calls": [
            {"query": "fixture startpage", "region": "us-en", "safesearch": "off", "timelimit": "w", "page": 2},
            {"query": "fixture startpage", "region": "us-en", "safesearch": "off", "timelimit": "m", "page": 3},
        ]
    }
    startpage_output, startpage_trace = _capture_synthetic_engine(
        [
            _SyntheticResponse(text='<form id="search"><input name="sc" value="synthetic-sc-one"></form>'),
            _SyntheticResponse(text="<html><body></body></html>"),
            _SyntheticResponse(text='<form id="search"><input name="sc" value="synthetic-sc-two"></form>'),
            _SyntheticResponse(text="<html><body></body></html>"),
        ],
        startpage_action,
    )
    fixtures.append(
        _engine_fixture(
            "engine.text.startpage-sc-bootstrap-per-payload",
            "text",
            "startpage",
            "search_twice",
            startpage_input,
            _ok(startpage_output),
            trace=startpage_trace,
            notes=["the second payload performs a new bootstrap instead of reusing the first sc value"],
        )
    )

    wikipedia_input = {
        "query": "fixture wiki query",
        "region": "de-de",
        "safesearch": "off",
        "timelimit": "y",
        "page": 3,
    }
    wikipedia_output, wikipedia_trace = _capture_synthetic_engine(
        [
            _SyntheticResponse(
                text=json.dumps(
                    [
                        "fixture wiki query",
                        ["Caf\u00e9 Fixture"],
                        [""],
                        ["https://de.wikipedia.org/wiki/Caf%C3%A9_Fixture"],
                    ]
                )
            ),
            _SyntheticResponse(text=json.dumps({"query": {"pages": {"123": {"extract": "<p>Fixture <b>body</b></p>"}}}})),
        ],
        lambda _events: _engine_result_dicts(Wikipedia().search(**wikipedia_input)),
    )
    fixtures.append(
        _engine_fixture(
            "engine.text.wikipedia-opensearch-and-extract",
            "text",
            "wikipedia",
            "search",
            wikipedia_input,
            _ok(wikipedia_output, field_order=[list(item) for item in wikipedia_output]),
            trace=wikipedia_trace,
            notes=["synthetic OpenSearch response triggers the source extract enrichment request"],
        )
    )

    brave_input = {
        "query": "fixture brave",
        "region": "DE-de",
        "safesearch": "off",
        "timelimit": "w",
        "page": 3,
    }
    brave_output, brave_trace = _capture_synthetic_engine(
        [_SyntheticResponse(text="<html><body></body></html>")],
        lambda _events: _engine_result_dicts(Brave().search(**brave_input)),
    )
    fixtures.append(
        _engine_fixture(
            "engine.text.brave-region-cookie-and-filters",
            "text",
            "brave",
            "search",
            brave_input,
            _ok(brave_output),
            trace=brave_trace,
            notes=["cookie values are synthetic deterministic source payload values, not captured session state"],
        )
    )

    google_input = {
        "query": "fixture google redirect",
        "region": "us-en",
        "safesearch": "off",
        "timelimit": "m",
        "page": 2,
    }
    google_html = """
    <html><body><div data-hveid="fixture">
      <a href="/url?q=https%3A%2F%2Ftarget.example%2Fpath&amp;sa=U"><h3>Fixture title</h3></a>
      <div><div>Fixture body</div></div>
    </div></body></html>
    """
    google_output, google_trace = _capture_synthetic_engine(
        [_SyntheticResponse(text=google_html)],
        lambda _events: _engine_result_dicts(Google().search(**google_input)),
    )
    fixtures.append(
        _engine_fixture(
            "engine.text.google-consent-and-redirect-filter",
            "text",
            "google",
            "search",
            google_input,
            _ok(google_output, field_order=[list(item) for item in google_output]),
            trace=google_trace,
            notes=["Google module-lifetime randomized User-Agent is redacted to a stable marker"],
        )
    )

    yahoo_module = importlib.import_module("ddgs.engines.yahoo")
    old_token_urlsafe = yahoo_module.token_urlsafe
    yahoo_input = {"query": "fixture yahoo", "region": "us-en", "safesearch": "moderate", "timelimit": "y", "page": 3}
    yahoo_html = """
    <html><body><div class="relsrch">
      <div class="Title"><a href="https://r.search.yahoo.com/_ylt=x/RU=https%3A%2F%2Ftarget.example%2Fpath%3Fa%3D1%2Btwo/RK=fixture"><h3>Yahoo fixture</h3></a></div>
      <div class="Text">Yahoo body</div>
    </div></body></html>
    """

    def yahoo_action(events: list[dict[str, Any]]) -> dict[str, Any]:
        def token_urlsafe(nbytes: int) -> str:
            value = f"synthetic-token-{nbytes}"
            events.append({"kind": "random", "value": {"function": "token_urlsafe", "nbytes": nbytes, "value": value}})
            return value

        yahoo_module.token_urlsafe = token_urlsafe
        try:
            engine = Yahoo()
            return {"results": _engine_result_dicts(engine.search(**yahoo_input)), "search_url": engine.search_url}
        finally:
            yahoo_module.token_urlsafe = old_token_urlsafe

    yahoo_output, yahoo_trace = _capture_synthetic_engine([_SyntheticResponse(text=yahoo_html)], yahoo_action)
    fixtures.append(
        _engine_fixture(
            "engine.text.yahoo-random-path-and-redirect-unwrapping",
            "text",
            "yahoo",
            "search",
            yahoo_input,
            _ok(yahoo_output, field_order=[list(item) for item in yahoo_output["results"]]),
            trace=yahoo_trace,
            random="token_urlsafe patched to deterministic synthetic values per source search",
        )
    )

    yandex_module = importlib.import_module("ddgs.engines.yandex")
    old_random = yandex_module.random
    yandex_input = {"query": "fixture yandex", "region": "us-en", "safesearch": "moderate", "page": 3}

    def yandex_action(events: list[dict[str, Any]]) -> list[dict[str, Any]]:
        class SyntheticRandom:
            def randint(self, lower: int, upper: int) -> int:
                value = 4242424
                events.append(
                    {"kind": "random", "value": {"function": "randint", "lower": lower, "upper": upper, "value": value}}
                )
                return value

        yandex_module.random = SyntheticRandom()
        try:
            return _engine_result_dicts(Yandex().search(**yandex_input))
        finally:
            yandex_module.random = old_random

    yandex_output, yandex_trace = _capture_synthetic_engine(
        [_SyntheticResponse(text="<html><body></body></html>")], yandex_action
    )
    fixtures.append(
        _engine_fixture(
            "engine.text.yandex-random-searchid",
            "text",
            "yandex",
            "search",
            yandex_input,
            _ok(yandex_output),
            trace=yandex_trace,
            random="randint patched to deterministic synthetic source-compatible range value",
        )
    )
    return fixtures


def _json_engine_matrix_fixtures() -> list[Fixture]:
    """Cover JSON/VQD engines before the independent HTML/XPath gate."""
    fixtures: list[Fixture] = []

    groki_input = {
        "query": "fixture grokipedia",
        "region": "de-de",
        "safesearch": "off",
        "timelimit": "y",
        "page": 3,
    }
    fixtures.extend(
        [
            _synthetic_search_fixture(
                "engine.text.grokipedia-happy-json",
                "text",
                "grokipedia",
                groki_input,
                Grokipedia,
                [
                    _SyntheticResponse(
                        text=json.dumps(
                            {
                                "results": [
                                    {
                                        "title": "__Fixture Grokipedia__",
                                        "snippet": "Heading\\n\\nFixture <b>body</b>",
                                        "slug": "fixture-page",
                                    }
                                ]
                            }
                        )
                    )
                ],
            ),
            _synthetic_search_fixture(
                "engine.text.grokipedia-empty-json",
                "text",
                "grokipedia",
                groki_input,
                Grokipedia,
                [_SyntheticResponse(text=json.dumps({"results": []}))],
            ),
            _synthetic_search_fixture(
                "engine.text.grokipedia-missing-slug-error",
                "text",
                "grokipedia",
                groki_input,
                Grokipedia,
                [_SyntheticResponse(text=json.dumps({"results": [{"title": "fixture"}]}))],
                notes=["synthetic malformed result omits the source-required slug key"],
            ),
        ]
    )

    wikipedia_input = {
        "query": "fixture wiki empty",
        "region": "de-de",
        "safesearch": "off",
        "timelimit": "y",
        "page": 3,
    }
    fixtures.extend(
        [
            _synthetic_search_fixture(
                "engine.text.wikipedia-empty-opensearch",
                "text",
                "wikipedia",
                wikipedia_input,
                Wikipedia,
                [_SyntheticResponse(text=json.dumps(["fixture wiki empty", [], [], []]))],
            ),
            _synthetic_search_fixture(
                "engine.text.wikipedia-disambiguation-filter",
                "text",
                "wikipedia",
                wikipedia_input,
                Wikipedia,
                [
                    _SyntheticResponse(
                        text=json.dumps(
                            [
                                "fixture wiki empty",
                                ["Fixture disambiguation"],
                                [""],
                                ["https://de.wikipedia.org/wiki/Fixture_disambiguation"],
                            ]
                        )
                    ),
                    _SyntheticResponse(
                        text=json.dumps(
                            {"query": {"pages": {"456": {"extract": "Fixture may refer to: several topics"}}}}
                        )
                    ),
                ],
                notes=["source filters only the exact lowercase substring may refer to:"],
            ),
        ]
    )

    image_input = {
        "query": "fixture image result",
        "region": "us-en",
        "safesearch": "on",
        "timelimit": "d",
        "page": 2,
        "size": "Medium",
        "color": "Blue",
        "type_image": "photo",
        "layout": "Square",
        "license_image": "Share",
    }
    image_result = {
        "title": " <b>Fixture image</b> ",
        "image": "https://image.example/a%20b",
        "thumbnail": "https://image.example/thumb%20one",
        "url": "https://page.example/a%20b",
        "height": 480,
        "width": 640,
        "source": "Fixture source",
    }
    fixtures.extend(
        [
            _synthetic_search_fixture(
                "engine.images.duckduckgo-happy-json",
                "images",
                "duckduckgo",
                image_input,
                DuckduckgoImages,
                [
                    _SyntheticResponse(content=b"vqd=fixture-vqd&"),
                    _SyntheticResponse(text=json.dumps({"results": [image_result]})),
                ],
            ),
            _synthetic_search_fixture(
                "engine.images.duckduckgo-missing-vqd-error",
                "images",
                "duckduckgo",
                image_input,
                DuckduckgoImages,
                [_SyntheticResponse(content=b"<html>synthetic without token</html>")],
                notes=["bootstrap status remains 200 but raw bytes have no accepted VQD marker"],
            ),
            _synthetic_search_fixture(
                "engine.images.duckduckgo-malformed-json-error",
                "images",
                "duckduckgo",
                image_input,
                DuckduckgoImages,
                [_SyntheticResponse(content=b"vqd='fixture-vqd'"), _SyntheticResponse(text="{")],
            ),
        ]
    )

    news_input = {
        "query": "fixture news result",
        "region": "us-en",
        "safesearch": "moderate",
        "timelimit": "w",
        "page": 2,
    }
    news_result = {
        "date": 1,
        "title": " <b>Fixture news</b> ",
        "excerpt": "Fixture <i>body</i>",
        "url": "https://news.example/a%20b",
        "image": "https://news.example/image%20one",
        "source": 7,
    }
    fixtures.extend(
        [
            _synthetic_search_fixture(
                "engine.news.duckduckgo-happy-json",
                "news",
                "duckduckgo",
                news_input,
                DuckduckgoNews,
                [
                    _SyntheticResponse(content=b'vqd="fixture-vqd"'),
                    _SyntheticResponse(text=json.dumps({"results": [news_result]})),
                ],
            ),
            _synthetic_search_fixture(
                "engine.news.duckduckgo-empty-json",
                "news",
                "duckduckgo",
                news_input,
                DuckduckgoNews,
                [_SyntheticResponse(content=b"vqd=fixture-vqd&"), _SyntheticResponse(text=json.dumps({"results": []}))],
            ),
        ]
    )

    video_input = {
        "query": "fixture video result",
        "region": "us-en",
        "safesearch": "on",
        "timelimit": "m",
        "page": 2,
        "resolution": "high",
        "duration": "long",
        "license_videos": "youtube",
    }
    video_result = {
        "title": "Fixture video",
        "content": None,
        "description": "<b>Raw description</b>",
        "duration": 12,
        "embed_html": "<iframe></iframe>",
        "embed_url": "https://video.example/embed%20one",
        "image_token": "token",
        "images": {"large": "https://video.example/large"},
        "provider": "provider",
        "published": 42,
        "publisher": None,
        "statistics": {"views": 9},
        "uploader": ["fixture", None],
    }
    fixtures.extend(
        [
            _synthetic_search_fixture(
                "engine.videos.duckduckgo-happy-heterogeneous-json",
                "videos",
                "duckduckgo",
                video_input,
                DuckduckgoVideos,
                [_SyntheticResponse(content=b"vqd=fixture-vqd&"), _SyntheticResponse(text=json.dumps({"results": [video_result]}))],
            ),
            _synthetic_search_fixture(
                "engine.videos.duckduckgo-empty-json",
                "videos",
                "duckduckgo",
                video_input,
                DuckduckgoVideos,
                [_SyntheticResponse(content=b"vqd=fixture-vqd&"), _SyntheticResponse(text=json.dumps({"results": []}))],
            ),
        ]
    )
    return fixtures


def _html_engine_matrix_fixtures() -> list[Fixture]:
    """Capture synthetic happy paths for every HTML-backed active engine."""
    fixtures: list[Fixture] = []

    brave_input = {
        "query": "fixture brave html",
        "region": "DE-de",
        "safesearch": "on",
        "timelimit": "m",
        "page": 2,
    }
    fixtures.append(
        _synthetic_search_fixture(
            "engine.text.brave-happy-html",
            "text",
            "brave",
            brave_input,
            Brave,
            [
                _SyntheticResponse(
                    text="""
                    <html><body><div data-type="web">
                      <a href="https://brave.example/a%20b"><div class="title">Fixture Brave</div></a>
                      <div class="snippet"><div class="content">Fixture <b>body</b></div></div>
                    </div></body></html>
                    """
                )
            ],
        )
    )

    ddg_input = {
        "query": "fixture ddg text",
        "region": "us-en",
        "safesearch": "off",
        "timelimit": "w",
        "page": 3,
    }
    fixtures.append(
        _synthetic_search_fixture(
            "engine.text.duckduckgo-happy-html-and-yjs-filter",
            "text",
            "duckduckgo",
            ddg_input,
            Duckduckgo,
            [
                _SyntheticResponse(
                    text="""
                    <html><body>
                      <div class="body"><a href="https://duckduckgo.com/y.js?fixture"><h2>Filtered ad</h2>ad body</a></div>
                      <div class="body"><a href="https://ddg-target.example/a%20b"><h2>Fixture DDG</h2>fixture body</a></div>
                    </body></html>
                    """
                )
            ],
            client_module="ddgs.engines.duckduckgo",
            client_name="HttpClient2",
            notes=["module-lifetime fake-useragent is redacted; HTTP/2/fingerprint parity is a separate transport gate"],
        )
    )

    mojeek_input = {
        "query": "fixture mojeek",
        "region": "DE-de",
        "safesearch": "on",
        "timelimit": "y",
        "page": 3,
    }
    fixtures.append(
        _synthetic_search_fixture(
            "engine.text.mojeek-happy-html",
            "text",
            "mojeek",
            mojeek_input,
            Mojeek,
            [
                _SyntheticResponse(
                    text="""
                    <html><body><ul class="results"><li>
                      <h2><a href="https://mojeek.example/a%20b">Fixture Mojeek</a></h2>
                      <p class="s">Fixture <i>body</i></p>
                    </li></ul></body></html>
                    """
                )
            ],
        )
    )

    startpage_input = {
        "query": "fixture startpage html",
        "region": "us-en",
        "safesearch": "moderate",
        "timelimit": "d",
        "page": 2,
    }
    fixtures.append(
        _synthetic_search_fixture(
            "engine.text.startpage-happy-html",
            "text",
            "startpage",
            startpage_input,
            Startpage,
            [
                _SyntheticResponse(text='<form id="search"><input name="sc" value="synthetic-sc"></form>'),
                _SyntheticResponse(
                    text="""
                    <html><body><div class="result"><a href="https://startpage.example/a%20b"><h2>Fixture Startpage</h2></a>
                    <p>Fixture <b>body</b></p></div></body></html>
                    """
                ),
            ],
        )
    )

    yandex_input = {
        "query": "fixture yandex html",
        "region": "us-en",
        "safesearch": "moderate",
        "timelimit": "d",
        "page": 2,
    }
    yandex_module = importlib.import_module("ddgs.engines.yandex")
    old_yandex_random = yandex_module.random

    def yandex_html_action(events: list[dict[str, Any]]) -> list[dict[str, Any]]:
        class SyntheticRandom:
            def randint(self, lower: int, upper: int) -> int:
                events.append(
                    {
                        "kind": "random",
                        "value": {"function": "randint", "lower": lower, "upper": upper, "value": 3131313},
                    }
                )
                return 3131313

        yandex_module.random = SyntheticRandom()
        try:
            return _engine_search_output(Yandex().search(**yandex_input)) or []
        finally:
            yandex_module.random = old_yandex_random

    fixtures.append(
        _synthetic_engine_fixture(
            "engine.text.yandex-happy-html",
            "text",
            "yandex",
            "search",
            yandex_input,
            [
                _SyntheticResponse(
                    text="""
                    <html><body><li class="serp-item"><h3><a href="https://yandex.example/a%20b">Fixture Yandex</a></h3>
                    <div class="text">Fixture <b>body</b></div></li></body></html>
                    """
                )
            ],
            yandex_html_action,
            random="randint patched to deterministic synthetic source-compatible range value",
        )
    )

    bing_images_input = {
        "query": "fixture bing images",
        "region": "de-de",
        "safesearch": "off",
        "timelimit": "week",
        "page": 2,
        "max_results": "40",
    }
    bing_images_metadata = json.dumps(
        {
            "t": "Fixture Bing Image",
            "murl": "https://bing-image.example/a%20b",
            "turl": "https://bing-image.example/thumb%20one",
            "purl": "https://bing-page.example/a%20b",
        }
    )
    fixtures.append(
        _synthetic_search_fixture(
            "engine.images.bing-happy-html-metadata",
            "images",
            "bing",
            bing_images_input,
            BingImages,
            [
                _SyntheticResponse(
                    text=(
                        '<html><body><div><div class="imgpt"><a class="iusc" m="'
                        + escape(bing_images_metadata, quote=True)
                        + '"></a></div><div class="infopt"></div><div class="img_info"><span class="nowrap">640 × 480</span></div>'
                        '<div class="lnkw"><a>Fixture source</a></div></div></body></html>'
                    )
                )
            ],
        )
    )
    fixtures.append(
        _synthetic_search_fixture(
            "engine.images.bing-dimension-px-error",
            "images",
            "bing",
            bing_images_input,
            BingImages,
            [
                _SyntheticResponse(
                    text=(
                        '<html><body><div><div class="imgpt"><a class="iusc" m="'
                        + escape(bing_images_metadata, quote=True)
                        + '"></a></div><div class="infopt"></div><div class="img_info"><span class="nowrap">640 × 480 px</span></div>'
                        '<div class="lnkw"><a>Fixture source</a></div></div></body></html>'
                    )
                )
            ],
            notes=["source dimension parser splits every x; a trailing px creates three pieces and raises ValueError"],
        )
    )

    bing_news_input = {
        "query": "fixture bing news",
        "region": "DE-de",
        "safesearch": "off",
        "timelimit": "d",
        "page": 2,
    }
    fixtures.append(
        _synthetic_search_fixture(
            "engine.news.bing-happy-html",
            "news",
            "bing",
            bing_news_input,
            BingNews,
            [
                _SyntheticResponse(
                    text="""
                    <html><body><div class="newsitem" data-title="Fixture Bing News" url="https://bing-news.example/a%20b" data-author="Fixture source">
                      <span aria-label="fixture date"></span><div class="snippet">Fixture <b>body</b></div>
                      <a class="image" src="/image?fixture&amp;tracking=1"></a>
                    </div></body></html>
                    """
                )
            ],
        )
    )

    yahoo_news_input = {
        "query": "fixture yahoo news",
        "region": "us-en",
        "safesearch": "off",
        "timelimit": "m",
        "page": 2,
    }
    fixtures.append(
        _synthetic_search_fixture(
            "engine.news.yahoo-happy-html-postprocess",
            "news",
            "yahoo",
            yahoo_news_input,
            YahooNews,
            [
                _SyntheticResponse(
                    text="""
                    <html><body><div id="web"><li><a>
                      <span class="time">fixture date</span><h4><a href="https://r.search.yahoo.com/RU=https%3A%2F%2Fyahoo-news.example%2Fa%3Fx%3D1/RK=fixture">Fixture Yahoo News</a></h4>
                      <p>Fixture <b>body</b></p><img data-src="https://image.example/-/fixture.jpg" />
                      <span class="source">Fixture source ·  via Yahoo</span>
                    </a></li></div></body></html>
                    """
                )
            ],
            notes=["relative-date clock behavior is covered separately; this case uses a non-date lexical value"],
        )
    )

    annas_input = {
        "query": "fixture annas",
        "region": "us-en",
        "safesearch": "off",
        "timelimit": "y",
        "page": 2,
    }
    old_annas_url = AnnasArchive.search_url

    def annas_action(_events: list[dict[str, Any]]) -> list[dict[str, Any]]:
        AnnasArchive.search_url = "https://annas-archive.fixture/search"
        try:
            return _engine_search_output(AnnasArchive().search(**annas_input)) or []
        finally:
            AnnasArchive.search_url = old_annas_url

    fixtures.append(
        _synthetic_engine_fixture(
            "engine.books.annasarchive-happy-comment-and-relative-url",
            "books",
            "annasarchive",
            "search",
            annas_input,
            [
                _SyntheticResponse(
                    text="""
                    <!--<div class="record-list-outer"><div>
                      <a class="text-lg">Fixture Anna</a><a><span class="user">Fixture Author</span></a>
                      <a><span class="company">Fixture Publisher</span></a><div class="text-gray-800">Fixture info</div>
                      <a href="/md5/fixture"></a><img src="/cover.jpg" />
                    </div></div>-->
                    """
                )
            ],
            annas_action,
            notes=["source module-lifetime archive TLD is replaced only with a synthetic deterministic test domain"],
        )
    )
    return fixtures


def _engine_non_200_fixtures() -> list[Fixture]:
    """Capture the BaseSearchEngine status-200-only path for every active engine."""
    cases: list[tuple[str, str, type[Any], dict[str, Any], str, str]] = [
        (
            "text",
            "brave",
            Brave,
            {"query": "fixture status brave", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "text",
            "duckduckgo",
            Duckduckgo,
            {"query": "fixture status ddg", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.engines.duckduckgo",
            "HttpClient2",
        ),
        (
            "text",
            "google",
            Google,
            {"query": "fixture status google", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "text",
            "grokipedia",
            Grokipedia,
            {"query": "fixture status groki", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "text",
            "mojeek",
            Mojeek,
            {"query": "fixture status mojeek", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "text",
            "startpage",
            Startpage,
            {"query": "fixture status startpage", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "text",
            "wikipedia",
            Wikipedia,
            {"query": "fixture status wiki", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "text",
            "yahoo",
            Yahoo,
            {"query": "fixture status yahoo", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "text",
            "yandex",
            Yandex,
            {"query": "fixture status yandex", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "images",
            "bing",
            BingImages,
            {"query": "fixture status bing images", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "images",
            "duckduckgo",
            DuckduckgoImages,
            {"query": "fixture status ddg images", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "news",
            "bing",
            BingNews,
            {"query": "fixture status bing news", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "news",
            "duckduckgo",
            DuckduckgoNews,
            {"query": "fixture status ddg news", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "news",
            "yahoo",
            YahooNews,
            {"query": "fixture status yahoo news", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "videos",
            "duckduckgo",
            DuckduckgoVideos,
            {"query": "fixture status ddg videos", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
        (
            "books",
            "annasarchive",
            AnnasArchive,
            {"query": "fixture status annas", "region": "us-en", "safesearch": "moderate", "timelimit": None, "page": 1},
            "ddgs.base",
            "HttpClient",
        ),
    ]
    fixtures: list[Fixture] = []
    for category, engine_name, engine_class, input_value, client_module, client_name in cases:
        responses = [_SyntheticResponse(text="synthetic non-200", status_code=503)]
        if engine_class is Startpage:
            responses = [
                _SyntheticResponse(text='<form id="search"><input name="sc" value="synthetic-sc"></form>'),
                _SyntheticResponse(text="synthetic non-200", status_code=503),
            ]
        if engine_class in {DuckduckgoImages, DuckduckgoNews, DuckduckgoVideos}:
            responses = [
                _SyntheticResponse(content=b"vqd=synthetic-vqd&", status_code=503),
                _SyntheticResponse(text="synthetic non-200", status_code=503),
            ]
        action = lambda events, engine_class=engine_class, input_value=input_value: _stable_engine_search(
            engine_class, input_value, events
        )
        fixtures.append(
            _synthetic_engine_fixture(
                f"engine.{category}.{engine_name}-non-200-none",
                category,
                engine_name,
                "search",
                input_value,
                responses,
                action,
                client_module=client_module,
                client_name=client_name,
                notes=["BaseSearchEngine.request returns no text for synthetic HTTP 503; source search returns None"],
            )
        )
    return fixtures


def _engine_empty_fixtures() -> list[Fixture]:
    """Capture source distinction between successful empty parse and non-200 None."""
    cases: list[tuple[str, str, type[Any], str, str]] = [
        ("text", "brave", Brave, "ddgs.base", "HttpClient"),
        ("text", "duckduckgo", Duckduckgo, "ddgs.engines.duckduckgo", "HttpClient2"),
        ("text", "google", Google, "ddgs.base", "HttpClient"),
        ("text", "grokipedia", Grokipedia, "ddgs.base", "HttpClient"),
        ("text", "mojeek", Mojeek, "ddgs.base", "HttpClient"),
        ("text", "startpage", Startpage, "ddgs.base", "HttpClient"),
        ("text", "wikipedia", Wikipedia, "ddgs.base", "HttpClient"),
        ("text", "yahoo", Yahoo, "ddgs.base", "HttpClient"),
        ("text", "yandex", Yandex, "ddgs.base", "HttpClient"),
        ("images", "bing", BingImages, "ddgs.base", "HttpClient"),
        ("images", "duckduckgo", DuckduckgoImages, "ddgs.base", "HttpClient"),
        ("news", "bing", BingNews, "ddgs.base", "HttpClient"),
        ("news", "duckduckgo", DuckduckgoNews, "ddgs.base", "HttpClient"),
        ("news", "yahoo", YahooNews, "ddgs.base", "HttpClient"),
        ("videos", "duckduckgo", DuckduckgoVideos, "ddgs.base", "HttpClient"),
        ("books", "annasarchive", AnnasArchive, "ddgs.base", "HttpClient"),
    ]
    fixtures: list[Fixture] = []
    for category, engine_name, engine_class, client_module, client_name in cases:
        input_value = {
            "query": f"fixture empty {category} {engine_name}",
            "region": "us-en",
            "safesearch": "moderate",
            "timelimit": None,
            "page": 1,
        }
        responses = [_SyntheticResponse(text="<html><body></body></html>")]
        if engine_class is Startpage:
            responses = [
                _SyntheticResponse(text='<form id="search"><input name="sc" value="synthetic-sc"></form>'),
                _SyntheticResponse(text="<html><body></body></html>"),
            ]
        elif engine_class is Wikipedia:
            responses = [_SyntheticResponse(text=json.dumps([input_value["query"], [], [], []]))]
        elif engine_class is Grokipedia:
            responses = [_SyntheticResponse(text=json.dumps({"results": []}))]
        elif engine_class in {DuckduckgoImages, DuckduckgoNews, DuckduckgoVideos}:
            responses = [
                _SyntheticResponse(content=b"vqd=synthetic-vqd&"),
                _SyntheticResponse(text=json.dumps({"results": []})),
            ]
        fixtures.append(
            _synthetic_engine_fixture(
                f"engine.{category}.{engine_name}-empty-200",
                category,
                engine_name,
                "search",
                input_value,
                responses,
                lambda events, engine_class=engine_class, input_value=input_value: _stable_engine_search(
                    engine_class, input_value, events
                ),
                client_module=client_module,
                client_name=client_name,
                notes=["synthetic HTTP 200 parses to an empty list; this differs from the non-200 None contract"],
            )
        )
    return fixtures


def _engine_malformed_response_fixtures() -> list[Fixture]:
    """Capture each active engine's response to a syntactically broken HTTP-200 body."""
    cases: list[tuple[str, str, type[Any], str, str]] = [
        ("text", "brave", Brave, "ddgs.base", "HttpClient"),
        ("text", "duckduckgo", Duckduckgo, "ddgs.engines.duckduckgo", "HttpClient2"),
        ("text", "google", Google, "ddgs.base", "HttpClient"),
        ("text", "grokipedia", Grokipedia, "ddgs.base", "HttpClient"),
        ("text", "mojeek", Mojeek, "ddgs.base", "HttpClient"),
        ("text", "startpage", Startpage, "ddgs.base", "HttpClient"),
        ("text", "wikipedia", Wikipedia, "ddgs.base", "HttpClient"),
        ("text", "yahoo", Yahoo, "ddgs.base", "HttpClient"),
        ("text", "yandex", Yandex, "ddgs.base", "HttpClient"),
        ("images", "bing", BingImages, "ddgs.base", "HttpClient"),
        ("images", "duckduckgo", DuckduckgoImages, "ddgs.base", "HttpClient"),
        ("news", "bing", BingNews, "ddgs.base", "HttpClient"),
        ("news", "duckduckgo", DuckduckgoNews, "ddgs.base", "HttpClient"),
        ("news", "yahoo", YahooNews, "ddgs.base", "HttpClient"),
        ("videos", "duckduckgo", DuckduckgoVideos, "ddgs.base", "HttpClient"),
        ("books", "annasarchive", AnnasArchive, "ddgs.base", "HttpClient"),
    ]
    fixtures: list[Fixture] = []
    for category, engine_name, engine_class, client_module, client_name in cases:
        input_value = {
            "query": f"fixture malformed {category} {engine_name}",
            "region": "us-en",
            "safesearch": "moderate",
            "timelimit": None,
            "page": 1,
        }
        responses = [_SyntheticResponse(text="<")]
        if engine_class is Startpage:
            responses = [
                _SyntheticResponse(text='<form id="search"><input name="sc" value="synthetic-sc"></form>'),
                _SyntheticResponse(text="<"),
            ]
        elif engine_class in {Wikipedia, Grokipedia}:
            responses = [_SyntheticResponse(text="{")]
        elif engine_class in {DuckduckgoImages, DuckduckgoNews, DuckduckgoVideos}:
            responses = [
                _SyntheticResponse(content=b"vqd=synthetic-vqd&"),
                _SyntheticResponse(text="{"),
            ]
        fixtures.append(
            _synthetic_engine_fixture(
                f"engine.{category}.{engine_name}-malformed-response",
                category,
                engine_name,
                "search",
                input_value,
                responses,
                lambda events, engine_class=engine_class, input_value=input_value: _stable_engine_search(
                    engine_class, input_value, events
                ),
                client_module=client_module,
                client_name=client_name,
                notes=["synthetic HTTP 200 carries malformed source-format content; output/error is frozen source behavior"],
            )
        )
    return fixtures


def _has_request_trace(fixture: Fixture) -> bool:
    return any(entry.get("kind") == "request" for entry in fixture["trace"])


def _validate_active_engine_coverage(fixtures: list[Fixture]) -> None:
    """Require each frozen active engine to retain its task-2.5 fixture matrix."""
    active_engines = {(category, engine) for category, engines in ENGINES.items() for engine in engines}
    engine_fixtures = [fixture for fixture in fixtures if fixture["contract"]["kind"] == "engine"]
    for category, engine_name in sorted(active_engines):
        prefix = f"engine.{category}.{engine_name}-"
        cases = [fixture for fixture in engine_fixtures if fixture["fixture_id"].startswith(prefix)]

        def has_fixture(suffix: str, predicate: Callable[[Fixture], bool]) -> bool:
            return any(fixture["fixture_id"].endswith(suffix) and predicate(fixture) for fixture in cases)

        has_option_happy_path = any(
            fixture["result"]["status"] == "ok"
            and _has_request_trace(fixture)
            and all(key in fixture["input"] for key in ("region", "safesearch", "timelimit", "page"))
            and fixture["input"]["page"] > 1
            and bool(fixture["input"]["timelimit"])
            for fixture in cases
        )
        if not has_option_happy_path:
            raise ValueError(f"{category}/{engine_name}: missing successful page/region/safesearch/timelimit fixture")
        if not has_fixture(
            "-empty-200",
            lambda fixture: fixture["result"]["status"] == "ok"
            and fixture["result"].get("output") == []
            and _has_request_trace(fixture),
        ):
            raise ValueError(f"{category}/{engine_name}: missing successful empty-200 fixture")
        if not has_fixture(
            "-malformed-response",
            lambda fixture: _has_request_trace(fixture),
        ):
            raise ValueError(f"{category}/{engine_name}: missing malformed-response fixture")
        if not has_fixture(
            "-non-200-none",
            lambda fixture: fixture["result"] == _ok(None) and _has_request_trace(fixture),
        ):
            raise ValueError(f"{category}/{engine_name}: missing non-200-none fixture")


def _lxml_tree(html_text: str) -> Any:
    """Create the exact parser configuration used by BaseSearchEngine."""
    parser = LHTMLParser(remove_blank_text=True, remove_comments=True, remove_pis=True, collect_ids=False)
    return html.fromstring(html_text, parser=parser)


def _xpath_strings(node: Any, expression: str) -> list[str]:
    """Return the source field-query shape before BaseSearchEngine joins it."""
    values = node.xpath(expression)
    if any(not isinstance(value, str) for value in values):
        raise TypeError(f"fixture XPath must return strings: {expression}")
    return values


def _joined_xpath_values(values: list[str]) -> str:
    """Mirror BaseSearchEngine's exact field-value whitespace collapse."""
    return " ".join("".join(values).split())


def _parser_xpath_fixture(
    fixture_id: str,
    category: str,
    engine: str,
    html_text: str,
    items_xpath: str,
    elements_xpath: dict[str, str],
    *,
    pre_process_html: bool = False,
    notes: list[str] | None = None,
) -> Fixture:
    """Capture the lxml generic engine extraction contract for one HTML engine."""
    parser_input = html_text.replace("<!--", "").replace("-->", "") if pre_process_html else html_text
    tree = _lxml_tree(parser_input)
    items = tree.xpath(items_xpath)
    if any(not hasattr(item, "xpath") for item in items):
        raise TypeError(f"fixture item XPath did not return elements: {items_xpath}")

    output_items = []
    for index, item in enumerate(items):
        fields = {}
        for field, expression in elements_xpath.items():
            raw = _xpath_strings(item, expression)
            fields[field] = {"raw": raw, "joined": _joined_xpath_values(raw)}
        output_items.append(
            {
                "index": index,
                "marker": item.get("data-parser-id", ""),
                "fields": fields,
            }
        )

    return _parser_fixture(
        fixture_id,
        category,
        engine,
        "xpath_generic_extraction",
        {
            "html": html_text,
            "items_xpath": items_xpath,
            "elements_xpath": elements_xpath,
            "elements_order": list(elements_xpath),
            "pre_process_html": "remove_comment_delimiters" if pre_process_html else "none",
        },
        _ok({"item_count": len(items), "items": output_items}),
        notes=notes,
    )


def _parser_document_xpath_fixture(
    fixture_id: str,
    category: str,
    engine: str,
    html_text: str,
    expression: str,
    *,
    notes: list[str] | None = None,
) -> Fixture:
    """Capture an XPath called directly on a source document tree."""
    values = _xpath_strings(_lxml_tree(html_text), expression)
    return _parser_fixture(
        fixture_id,
        category,
        engine,
        "xpath_document_values",
        {"html": html_text, "xpath": expression},
        _ok(values),
        notes=notes,
    )


def _parser_xpath_fixtures() -> list[Fixture]:
    """Capture every frozen source XPath expression against synthetic HTML."""
    generic_cases: dict[tuple[str, str], tuple[type[Any], str, bool]] = {
        (
            "text",
            "bing",
        ): (
            Bing,
            """
            <html><body><li class="b_algo" data-parser-id="bing"><h2><a href="https://bing.example/a%20b">\n              Fixture <b>Bing</b>\n            </a></h2><p>Fixture <i>body</i></p></li></body></html>
            """,
            False,
        ),
        (
            "text",
            "brave",
        ): (
            Brave,
            """
            <html><body><div data-type="web" data-parser-id="brave">
              <a href="https://brave.example/a%20b"><div class="title">Anchor title</div></a>
              <div class="title"> First title </div>
              <div class="sitename-container"> Last <b> title </b> </div>
              <div class="snippet"><div class="content"> Fixture\n <b> body </b> </div></div>
            </div></body></html>
            """,
            False,
        ),
        (
            "text",
            "duckduckgo",
        ): (
            Duckduckgo,
            """
            <html><body><div class="body" data-parser-id="duckduckgo"><a href="https://ddg.example/a%20b">
              <h2> Fixture <b> DDG </b> </h2> Fixture <i> body </i>
            </a></div></body></html>
            """,
            False,
        ),
        (
            "text",
            "google",
        ): (
            Google,
            """
            <html><body><div data-hveid="fixture" data-parser-id="google"><a href="https://google.example/a%20b"><h3> Fixture <b> Google </b> </h3></a>
              <div><div> Older body </div><div> Final <i> body </i> </div></div>
            </div></body></html>
            """,
            False,
        ),
        (
            "text",
            "mojeek",
        ): (
            Mojeek,
            """
            <html><body><ul class="results"><li data-parser-id="mojeek"><h2><a href="https://mojeek.example/a%20b"> Fixture Mojeek </a></h2>
              <p class="s"> Fixture <b> body </b> </p></li></ul></body></html>
            """,
            False,
        ),
        (
            "text",
            "startpage",
        ): (
            Startpage,
            """
            <html><body><div class="result" data-parser-id="startpage"><a href="https://startpage.example/a%20b"><h2> Fixture Startpage </h2></a>
              <p> Fixture <b> body </b> </p></div></body></html>
            """,
            False,
        ),
        (
            "text",
            "yahoo",
        ): (
            Yahoo,
            """
            <html><body><div class="relsrch" data-parser-id="yahoo"><div class="Title"><a href="https://yahoo.example/a%20b"><h3> Fixture Yahoo </h3></a></div>
              <div class="Text"> Fixture <b> body </b> </div></div></body></html>
            """,
            False,
        ),
        (
            "text",
            "yandex",
        ): (
            Yandex,
            """
            <html><body><li class="serp-item" data-parser-id="yandex"><h3><a href="https://yandex.example/a%20b"> Fixture Yandex </a></h3>
              <div class="text"> Fixture <b> body </b> </div></li></body></html>
            """,
            False,
        ),
        (
            "news",
            "bing",
        ): (
            BingNews,
            """
            <html><body><div class="newsitem" data-parser-id="bing-news" data-title="Fixture Bing News" url="https://bing-news.example/a%20b" data-author="Fixture source">
              <span aria-label="fixture date"></span><div class="snippet"> Fixture <b> body </b> </div>
              <a class="image" src="/image?fixture&amp;tracking=1"></a>
            </div></body></html>
            """,
            False,
        ),
        (
            "news",
            "yahoo",
        ): (
            YahooNews,
            """
            <html><body><div id="web"><li data-parser-id="yahoo-news"><a>
              <span class="time"> Fixture date </span><h4><a href="https://yahoo-news.example/a%20b"> Fixture Yahoo News </a></h4>
              <p> Fixture <b> body </b> </p><img src="fallback.jpg" data-src="primary.jpg" />
              <span class="source"> Fixture source </span>
            </a></li></div></body></html>
            """,
            False,
        ),
        (
            "books",
            "annasarchive",
        ): (
            AnnasArchive,
            """
            <!--<div class="record-list-outer"><div data-parser-id="annasarchive">
              <a class="text-lg"> Fixture Anna </a><a><span class="user"> Fixture Author </span></a>
              <a><span class="company"> Fixture Publisher </span></a><div class="text-gray-800"> Fixture info </div>
              <a href="/md5/fixture"></a><img src="/cover.jpg" />
            </div></div>-->
            """,
            True,
        ),
    }
    source_xpath_engines = {
        (category, name): engine_class
        for category, engines in ENGINES.items()
        for name, engine_class in engines.items()
        if hasattr(engine_class, "items_xpath") and hasattr(engine_class, "elements_xpath")
    }
    source_xpath_engines[("text", "bing")] = Bing
    if set(generic_cases) != set(source_xpath_engines):
        missing = sorted(set(source_xpath_engines) - set(generic_cases))
        unexpected = sorted(set(generic_cases) - set(source_xpath_engines))
        raise ValueError(f"parser generic XPath coverage drift: missing={missing}, unexpected={unexpected}")

    fixtures = []
    for (category, engine), (engine_class, html_text, pre_process_html) in sorted(generic_cases.items()):
        fixtures.append(
            _parser_xpath_fixture(
                f"parser.{category}.{engine}-generic-xpath",
                category,
                engine,
                html_text,
                engine_class.items_xpath,
                dict(engine_class.elements_xpath),
                pre_process_html=pre_process_html,
                notes=["synthetic HTML; lxml parser configuration and source XPath expressions are frozen"],
            )
        )

    fixtures.append(
        _parser_xpath_fixture(
            "parser.images.bing-special-xpath",
            "images",
            "bing",
            """
            <html><body><div data-parser-id="bing-images"><div class="imgpt"><a class="iusc" m="{&quot;t&quot;:&quot;Fixture&quot;}"></a></div>
              <div class="infopt"></div><div class="img_info"><span class="nowrap">640 × 480</span></div><div class="lnkw"><a> Fixture source </a></div>
            </div></body></html>
            """,
            BingImages.items_xpath,
            {
                "metadata": ".//a[@class='iusc']/@m",
                "dimension": ".//div[contains(@class, 'img_info')][./span]/span[@class='nowrap']/text()",
                "source": ".//div[@class='lnkw']//a/text()",
            },
            notes=["Bing Images overrides generic result extraction but retains these source XPath expressions"],
        )
    )
    fixtures.append(
        _parser_document_xpath_fixture(
            "parser.text.startpage-sc-document-xpath",
            "text",
            "startpage",
            '<html><body><form id="search"><input name="sc" value="fixture-sc" /></form></body></html>',
            '//form[@id="search"]//input[@name="sc"]/@value',
            notes=["Startpage bootstrap XPath executes on the document, not an item node"],
        )
    )
    fixtures.append(
        _parser_xpath_fixture(
            "parser.text.startpage-malformed-html-recovery",
            "text",
            "startpage",
            '<div class="result" data-parser-id="malformed"><a href="https://malformed.example/a"><h2> Broken <b> title </a><p> Body',
            Startpage.items_xpath,
            dict(Startpage.elements_xpath),
            notes=["lxml HTMLParser recovers this synthetic unclosed HTML; Go candidate must be tested without selector rewrites"],
        )
    )
    return fixtures


def _parser_json_fixtures() -> list[Fixture]:
    """Capture json.loads values used by JSON-backed frozen engines."""
    cases = [
        (
            "parser.text.grokipedia-json-absent-null-nested",
            "text",
            "grokipedia",
            """
            {
              "results": [
                {
                  "title": "__Fixture Grokipedia__",
                  "snippet": null,
                  "nested": {"tags": ["fixture", null], "rank": 1}
                }
              ]
            }
            """,
            [
                "Grokipedia accesses this json.loads object with an optional null snippet and a required slug later.",
                "The fixture deliberately omits slug to preserve absent-key behavior before engine-specific access.",
            ],
        ),
        (
            "parser.images.bing-metadata-json-mixed",
            "images",
            "bing",
            """
            {
              "t": "Fixture Bing Image",
              "murl": null,
              "turl": "https://bing-image.fixture/thumb",
              "purl": "https://bing-page.fixture/page",
              "width": 640,
              "height": 480,
              "flags": [true, null],
              "nested": {"ratio": 1.5}
            }
            """,
            ["Bing Images applies json.loads to the HTML m attribute before selecting metadata fields."],
        ),
        (
            "parser.images.duckduckgo-json-mixed",
            "images",
            "duckduckgo",
            """
            {
              "results": [
                {
                  "title": "Fixture image",
                  "image": "https://image.fixture/a",
                  "thumbnail": null,
                  "height": 480,
                  "width": 640,
                  "source": {"name": "fixture"}
                }
              ]
            }
            """,
            ["DuckDuckGo Images copies JSON result values without string coercion."],
        ),
        (
            "parser.news.duckduckgo-json-absent-null-mixed",
            "news",
            "duckduckgo",
            """
            {
              "results": [
                {
                  "date": 1,
                  "title": "Fixture news",
                  "excerpt": null,
                  "source": 7,
                  "metadata": {"regions": ["us-en", null]}
                }
              ]
            }
            """,
            ["DuckDuckGo News reads optional image/url fields with dict.get, so their absence must survive decoding."],
        ),
        (
            "parser.videos.duckduckgo-json-heterogeneous",
            "videos",
            "duckduckgo",
            """
            {
              "results": [
                {
                  "title": "Fixture video",
                  "content": null,
                  "duration": 12,
                  "images": {"large": "https://video.fixture/large"},
                  "published": 42,
                  "publisher": null,
                  "statistics": {"views": 9},
                  "uploader": ["fixture", null]
                }
              ]
            }
            """,
            ["DuckDuckGo Videos retains nested and heterogeneous JSON values in result fields."],
        ),
    ]

    fixtures = []
    for fixture_id, category, engine, source_json, notes in cases:
        source_json = source_json.strip()
        fixtures.append(
            _parser_fixture(
                fixture_id,
                category,
                engine,
                "json_loads",
                {"json": source_json},
                _ok(json.loads(source_json)),
                notes=notes,
            )
        )

    trailing_json = '{"results": []} {"unexpected": true}'
    fixtures.append(
        _parser_fixture(
            "parser.text.grokipedia-json-trailing-data-error",
            "text",
            "grokipedia",
            "json_loads",
            {"json": trailing_json},
            _error(lambda: json.loads(trailing_json)),
            notes=["Python json.loads rejects a second JSON value after an otherwise valid object."],
        )
    )
    malformed_json = '{"results": ['
    fixtures.append(
        _parser_fixture(
            "parser.images.duckduckgo-json-malformed-error",
            "images",
            "duckduckgo",
            "json_loads",
            {"json": malformed_json},
            _error(lambda: json.loads(malformed_json)),
            notes=["Python json.loads rejects truncated DuckDuckGo-style JSON before an engine can inspect results."],
        )
    )
    return fixtures


def _fixture_path(
    fixture: Fixture,
    pure_output: Path,
    engine_output: Path,
    extract_output: Path,
    parser_output: Path,
    transport_output: Path,
) -> Path:
    kind = fixture["contract"]["kind"]
    output = {
        "pure": pure_output,
        "engine": engine_output,
        "extract": extract_output,
        "parser": parser_output,
        "transport": transport_output,
    }[kind]
    return output / f"{fixture['fixture_id']}.json"


def build_fixtures() -> list[Fixture]:
    fixtures = [
        *_normalizer_fixtures(),
        *_client_configuration_fixtures(),
        *_aggregation_fixtures(),
        *_ranker_fixtures(),
        _engine_registry_fixture(),
        *_backend_fixtures(),
        _frozen_registry_backend_fixture(),
        *_search_invocation_fixtures(),
        *_scheduler_fixtures(),
        *_error_and_extract_fixtures(),
        *_extract_fixtures(),
        *_transport_fixtures(),
        *_parser_xpath_fixtures(),
        *_parser_json_fixtures(),
        *_engine_visible_fixtures(),
        *_json_engine_matrix_fixtures(),
        *_html_engine_matrix_fixtures(),
        *_engine_non_200_fixtures(),
        *_engine_empty_fixtures(),
        *_engine_malformed_response_fixtures(),
    ]
    ids = [fixture["fixture_id"] for fixture in fixtures]
    if len(ids) != len(set(ids)):
        raise ValueError("duplicate fixture id")
    for fixture in fixtures:
        _validate_fixture(fixture)
        _validate_sanitized_fixture_content(fixture)
    _validate_active_engine_coverage(fixtures)
    return fixtures


def _check_source(source_checkout: Path) -> None:
    head = subprocess.run(
        ["git", "-C", str(source_checkout), "rev-parse", "HEAD"],
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()
    if head != SOURCE_SHA:
        raise RuntimeError(f"expected source {SOURCE_SHA}, got {head}")
    clean = subprocess.run(
        ["git", "-C", str(source_checkout), "diff", "--quiet"],
        check=False,
    ).returncode == 0
    if not clean:
        raise RuntimeError("frozen source worktree is dirty")


def _render(fixture: Fixture) -> str:
    return json.dumps(fixture, ensure_ascii=False, indent=2, sort_keys=True) + "\n"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source-checkout", type=Path, default=Path("/home/jcastillo/Proyectos/ddgs"))
    parser.add_argument("--output", type=Path, default=Path("testdata/contracts/pure"), help="pure fixture output")
    parser.add_argument(
        "--engine-output",
        type=Path,
        default=Path("testdata/contracts/engine"),
        help="synthetic engine fixture output",
    )
    parser.add_argument(
        "--extract-output",
        type=Path,
        default=Path("testdata/contracts/extract"),
        help="synthetic extraction fixture output",
    )
    parser.add_argument(
        "--parser-output",
        type=Path,
        default=Path("testdata/contracts/parser"),
        help="synthetic lxml/XPath fixture output",
    )
    parser.add_argument(
        "--transport-output",
        type=Path,
        default=Path("testdata/contracts/transport"),
        help="synthetic transport fixture output",
    )
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--write", action="store_true", help="write deterministic offline fixtures")
    mode.add_argument("--check", action="store_true", help="verify generated fixtures match files")
    args = parser.parse_args()

    _check_source(args.source_checkout)
    fixtures = build_fixtures()
    args.output.mkdir(parents=True, exist_ok=True)
    args.engine_output.mkdir(parents=True, exist_ok=True)
    args.extract_output.mkdir(parents=True, exist_ok=True)
    args.parser_output.mkdir(parents=True, exist_ok=True)
    args.transport_output.mkdir(parents=True, exist_ok=True)
    failures: list[str] = []
    for fixture in fixtures:
        path = _fixture_path(
            fixture,
            args.output,
            args.engine_output,
            args.extract_output,
            args.parser_output,
            args.transport_output,
        )
        rendered = _render(fixture)
        if args.write:
            path.write_text(rendered, encoding="utf-8")
        elif not path.is_file() or path.read_text(encoding="utf-8") != rendered:
            failures.append(str(path))
    if args.check and failures:
        print("fixtures differ or are missing:", *failures, sep="\n", file=sys.stderr)
        return 1
    counts = {
        kind: sum(fixture["contract"]["kind"] == kind for fixture in fixtures)
        for kind in ("pure", "engine", "extract", "parser", "transport")
    }
    print(
        f"{len(fixtures)} offline frozen-source fixtures "
        f"({counts['pure']} pure, {counts['engine']} engine, {counts['extract']} extract, "
        f"{counts['parser']} parser, {counts['transport']} transport) "
        f"{'written' if args.write else 'verified'}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
