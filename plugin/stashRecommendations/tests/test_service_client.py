from __future__ import annotations

import json
import socket
from typing import Any
from urllib import error

import pytest

from rec_plugin.service_client import HTTP_TIMEOUT_SECONDS, ServiceClient
from rec_plugin.settings import Settings


def test_service_client_uses_explicit_timeout_for_post_and_get(monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[dict[str, Any]] = []
    responses = [
        FakeHTTPResponse(202, {"accepted": True}),
        FakeHTTPResponse(200, {"model_version": "model-1", "items": []}),
    ]

    def fake_urlopen(http_request: Any, timeout: float) -> FakeHTTPResponse:
        calls.append(
            {
                "method": http_request.get_method(),
                "url": http_request.full_url,
                "timeout": timeout,
                "authorization": http_request.headers["Authorization"],
            }
        )
        return responses.pop(0)

    monkeypatch.setattr("rec_plugin.service_client.request.urlopen", fake_urlopen)
    client = ServiceClient(Settings(service_url="https://stashrec.example", api_key="secret-api-key", show_remote_results=False))

    delivered = client.deliver_preference_event({"event_id": "550e8400-e29b-41d4-a716-446655440110"})
    related = client.fetch_related([{"endpoint": "https://box.example/graphql", "stash_id": "scene-1"}], 5)

    assert delivered.status_code == 202
    assert related == {"model_version": "model-1", "items": []}
    assert calls == [
        {
            "method": "POST",
            "url": "https://stashrec.example/v1/events/interactions",
            "timeout": HTTP_TIMEOUT_SECONDS,
            "authorization": "Bearer secret-api-key",
        },
        {
            "method": "GET",
            "url": "https://stashrec.example/v1/recommendations/related?endpoint=https%3A%2F%2Fbox.example%2Fgraphql&stash_id=scene-1&limit=5",
            "timeout": HTTP_TIMEOUT_SECONDS,
            "authorization": "Bearer secret-api-key",
        },
    ]


def test_service_client_raises_oserror_when_urlopen_times_out(monkeypatch: pytest.MonkeyPatch) -> None:
    def fake_urlopen(http_request: Any, timeout: float) -> FakeHTTPResponse:
        del http_request, timeout
        raise error.URLError(socket.timeout("timed out"))

    monkeypatch.setattr("rec_plugin.service_client.request.urlopen", fake_urlopen)
    client = ServiceClient(Settings(service_url="https://stashrec.example", api_key="secret-api-key", show_remote_results=False))

    with pytest.raises(OSError, match="timed out"):
        client.deliver_snapshot({"schema_version": 1})


def test_service_client_accepts_whitespace_only_success_response(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(
        "rec_plugin.service_client.request.urlopen",
        lambda http_request, timeout: FakeHTTPResponse(202, b"\n"),
    )
    client = ServiceClient(Settings(service_url="https://stashrec.example", api_key="secret-api-key", show_remote_results=False))

    response = client.deliver_snapshot({"schema_version": 1})

    assert response.status_code == 202
    assert response.body is None


class FakeHTTPResponse:
    def __init__(self, status: int, body: dict[str, object] | bytes, headers: dict[str, str] | None = None) -> None:
        self.status = status
        self._payload = body if isinstance(body, bytes) else json.dumps(body).encode("utf-8")
        self.headers = headers or {}

    def __enter__(self) -> "FakeHTTPResponse":
        return self

    def __exit__(self, exc_type: object, exc: object, tb: object) -> None:
        del exc_type, exc, tb
        return None

    def read(self) -> bytes:
        return self._payload
