from __future__ import annotations

import json
from typing import Any

import pytest

from rec_plugin.source_client import HTTP_TIMEOUT_SECONDS, SourceClient


def test_source_client_fetches_only_configured_endpoint() -> None:
    calls: list[tuple[str, str, str, dict[str, object]]] = []

    def transport(url: str, api_key: str, query: str, variables: dict[str, object]) -> dict[str, object]:
        calls.append((url, api_key, query, variables))
        return {"data": {"findScene": {"id": "scene-44", "updated": "2026-08-30T12:00:00Z"}}}

    client = SourceClient(
        [{"endpoint": "https://box.example/graphql", "api_key": "source-key", "max_requests_per_minute": 30}],
        transport=transport,
    )

    assert client.credentials_for("https://other.example/graphql") is None
    assert client.fetch_scene("https://other.example/graphql", "scene-44") is None

    scene = client.fetch_scene("https://box.example/graphql", "scene-44")

    assert scene is not None
    assert scene["id"] == "scene-44"
    assert calls == [
        ("https://box.example/graphql", "source-key", calls[0][2], {"id": "scene-44"})
    ]
    assert "findScene" in calls[0][2]
    assert "groups" in calls[0][2]


def test_source_client_rate_limits_repeated_requests_per_endpoint() -> None:
    sleeps: list[float] = []

    class Clock:
        def __init__(self) -> None:
            self.value = 0.0

        def now(self) -> float:
            return self.value

        def sleep(self, seconds: float) -> None:
            sleeps.append(seconds)
            self.value += seconds

    clock = Clock()

    def transport(url: str, api_key: str, query: str, variables: dict[str, object]) -> dict[str, object]:
        del url, api_key, query, variables
        return {"data": {"findScene": {"id": "scene-44", "updated": "2026-08-30T12:00:00Z"}}}

    client = SourceClient(
        [{"endpoint": "https://box.example/graphql", "api_key": "source-key", "max_requests_per_minute": 30}],
        transport=transport,
        monotonic=clock.now,
        sleep=clock.sleep,
    )

    client.fetch_scene("https://box.example/graphql", "scene-44")
    clock.value = 1.0

    client.fetch_scene("https://box.example/graphql", "scene-45")

    assert sleeps == [1.0]


def test_source_client_fetches_schema_compatible_group_relationships() -> None:
    captured_queries: list[str] = []

    def transport(url: str, api_key: str, query: str, variables: dict[str, object]) -> dict[str, object]:
        del url, api_key, variables
        captured_queries.append(query)
        return {
            "data": {
                "findScene": {
                    "id": "scene-44",
                    "updated": "2026-08-30T12:00:00Z",
                    "groups": [{"group": {"id": "group-1", "name": "Compilation"}}],
                }
            }
        }

    client = SourceClient(
        [{"endpoint": "https://box.example/graphql", "api_key": "source-key", "max_requests_per_minute": 30}],
        transport=transport,
    )

    scene = client.fetch_scene("https://box.example/graphql", "scene-44")

    assert scene is not None
    assert scene["groups"] == [{"group": {"id": "group-1", "name": "Compilation"}}]
    assert "groups {" in captured_queries[0]
    assert "group {" in captured_queries[0]


def test_source_client_default_transport_uses_explicit_timeout(monkeypatch: pytest.MonkeyPatch) -> None:
    calls: list[dict[str, Any]] = []

    def fake_urlopen(http_request: Any, timeout: float) -> object:
        payload = json.loads(http_request.data.decode("utf-8"))
        calls.append(
            {
                "url": http_request.full_url,
                "timeout": timeout,
                "api_key": http_request.headers["Apikey"],
                "variables": payload["variables"],
            }
        )
        return FakeHTTPResponse({"data": {"findScene": {"id": "scene-44", "updated": "2026-08-30T12:00:00Z"}}})

    monkeypatch.setattr("rec_plugin.source_client.request.urlopen", fake_urlopen)
    client = SourceClient([{"endpoint": "https://box.example/graphql", "api_key": "source-key", "max_requests_per_minute": 30}])

    scene = client.fetch_scene("https://box.example/graphql", "scene-44")

    assert scene is not None
    assert scene["id"] == "scene-44"
    assert calls == [
        {
            "url": "https://box.example/graphql",
            "timeout": HTTP_TIMEOUT_SECONDS,
            "api_key": "source-key",
            "variables": {"id": "scene-44"},
        }
    ]


class FakeHTTPResponse:
    def __init__(self, body: dict[str, object]) -> None:
        self._payload = json.dumps(body).encode("utf-8")

    def __enter__(self) -> "FakeHTTPResponse":
        return self

    def __exit__(self, exc_type: object, exc: object, tb: object) -> None:
        del exc_type, exc, tb
        return None

    def read(self) -> bytes:
        return self._payload
