from __future__ import annotations

from datetime import datetime, timedelta, timezone
from pathlib import Path
import socket
from urllib import error

from rec_plugin.contracts import ContentKey, PreferenceEvent, SourceSnapshot
from rec_plugin.delivery import DeliveryWorker, ServiceResponse
from rec_plugin import outbox as outbox_module
from rec_plugin.outbox import Outbox
from rec_plugin.service_client import ServiceClient
from rec_plugin.settings import Settings
from recommendations import run


NOW = datetime(2026, 8, 30, 12, 0, tzinfo=timezone.utc)
CONTENT = ContentKey.normalize("https://box.example/graphql", "scene-1")


def test_401_pauses_delivery_and_persists_auth_status(tmp_path: Path, monkeypatch: object) -> None:
    freeze_outbox_now(monkeypatch)
    outbox = seeded_outbox(tmp_path)
    client = FakeServiceClient([ServiceResponse(status_code=401)])

    result = DeliveryWorker(outbox, client, pause_key="auth-a").deliver_ready(NOW)

    assert result.paused is True
    assert result.delivered == 0
    assert result.retried == 0
    assert result.quarantined == 0
    assert outbox.status()["paused"] == {
        "active": True,
        "reason": "service authentication failed",
    }
    assert outbox.status()["pending"]["rating"] == 1


def test_401_pause_blocks_later_delivery_runs_until_pause_key_changes(tmp_path: Path, monkeypatch: object) -> None:
    freeze_outbox_now(monkeypatch)
    outbox = seeded_outbox(tmp_path)
    client = FakeServiceClient([ServiceResponse(status_code=401), ServiceResponse(status_code=202)])

    first = DeliveryWorker(outbox, client, pause_key="auth-a").deliver_ready(NOW)
    second = DeliveryWorker(outbox, client, pause_key="auth-a").deliver_ready(NOW + timedelta(minutes=2))

    assert first.paused is True
    assert second.paused is True
    assert len(client.preference_events) == 1
    assert outbox.status()["paused"] == {
        "active": True,
        "reason": "service authentication failed",
    }
    assert outbox.status()["pending"]["rating"] == 1


def test_422_quarantines_invalid_events_and_snapshots_deliver_independently(tmp_path: Path, monkeypatch: object) -> None:
    freeze_outbox_now(monkeypatch)
    outbox = seeded_outbox(tmp_path)
    outbox.enqueue(snapshot())
    client = FakeServiceClient(
        [
            ServiceResponse(status_code=422, error="schema invalid"),
            ServiceResponse(status_code=202),
        ]
    )

    result = DeliveryWorker(outbox, client).deliver_ready(NOW)
    status = outbox.status()

    assert result.paused is False
    assert result.delivered == 1
    assert result.quarantined == 1
    assert status["quarantined"]["rating"] == 1
    assert status["delivered"]["snapshot"] == 1
    assert status["last_error"] == "schema invalid"


def test_429_uses_retry_after_header_for_next_attempt(tmp_path: Path, monkeypatch: object) -> None:
    freeze_outbox_now(monkeypatch)
    outbox = seeded_outbox(tmp_path)
    client = FakeServiceClient([ServiceResponse(status_code=429, retry_after_seconds=90)])

    result = DeliveryWorker(outbox, client).deliver_ready(NOW)

    assert result.retried == 1
    assert outbox.next_ready(NOW + timedelta(seconds=89)) is None
    assert outbox.next_ready(NOW + timedelta(seconds=90)) is not None


def test_network_and_5xx_errors_retry_without_quarantine(tmp_path: Path, monkeypatch: object) -> None:
    freeze_outbox_now(monkeypatch)
    outbox = seeded_outbox(tmp_path)
    client = FakeServiceClient([OSError("connection reset"), ServiceResponse(status_code=503, error="server busy")])

    first = DeliveryWorker(outbox, client).deliver_ready(NOW)
    second = DeliveryWorker(outbox, client).deliver_ready(NOW + timedelta(minutes=2))
    status = outbox.status()

    assert first.retried == 1
    assert second.retried == 1
    assert status["pending"]["rating"] == 1
    assert status["quarantined"]["rating"] == 0
    assert status["last_error"] == "server busy"


def test_delivery_status_records_redacted_latest_attempt(tmp_path: Path, monkeypatch: object) -> None:
    freeze_outbox_now(monkeypatch)
    outbox = seeded_outbox(tmp_path)
    client = FakeServiceClient([ServiceResponse(status_code=401, error="unauthorized")])

    DeliveryWorker(outbox, client, pause_key="auth-a").deliver_ready(NOW)

    assert outbox.status()["last_delivery_attempt"] == {
        "attempted_at": "2026-08-30T12:00:00Z",
        "item_type": "preference_event",
        "metric": "rating",
        "outcome": "paused",
        "status_code": 401,
        "error": "unauthorized",
    }


def test_deliver_outbox_logs_start_and_summary(tmp_path: Path, monkeypatch: object, capsys: object) -> None:
    freeze_outbox_now(monkeypatch)
    seeded_outbox(tmp_path)
    monkeypatch.setattr("recommendations.StashClient", lambda server_connection: FakeConfiguredStash(tmp_path))
    monkeypatch.setattr(
        "recommendations.ServiceClient",
        lambda settings: FakeServiceClient([ServiceResponse(status_code=202)]),
    )

    output: dict[str, object] = {}
    run(
        {
            "server_connection": {"PluginDir": str(tmp_path)},
            "args": {"mode": "deliver-outbox"},
        },
        output,
    )

    assert output["output"]["delivery"] == {
        "delivered": 1,
        "retried": 0,
        "quarantined": 0,
        "paused": False,
    }
    captured = capsys.readouterr().err
    assert "task mode=deliver-outbox" in captured
    assert "delivery start pending={'rating': 1, 'play': 0, 'o': 0, 'snapshot': 0, 'hook': 0}" in captured
    assert "delivery finished delivered=1 retried=0 quarantined=0 paused=False" in captured


def test_service_client_timeout_reaches_retry_without_quarantine(tmp_path: Path, monkeypatch: object) -> None:
    freeze_outbox_now(monkeypatch)
    outbox = seeded_outbox(tmp_path)

    def fake_urlopen(http_request: object, timeout: float) -> object:
        del http_request, timeout
        raise error.URLError(socket.timeout("timed out"))

    monkeypatch.setattr("rec_plugin.service_client.request.urlopen", fake_urlopen)
    client = ServiceClient(
        Settings(service_url="https://stashrec.example", api_key="secret-api-key", show_remote_results=False)
    )

    result = DeliveryWorker(outbox, client).deliver_ready(NOW)
    status = outbox.status()

    assert result.retried == 1
    assert result.delivered == 0
    assert result.quarantined == 0
    assert status["pending"]["rating"] == 1
    assert status["quarantined"]["rating"] == 0
    assert status["last_error"] == "timed out"


def test_fetch_related_mode_merges_unique_results_across_content_keys(tmp_path: Path, monkeypatch: object) -> None:
    client = FakeReadServiceClient(
        {
            ("https://box.example/graphql", "scene-1"): {
                "model_version": "model-1",
                "items": [
                    recommendation_item("scene-a", 0.8),
                    recommendation_item("scene-b", 0.6),
                ],
            },
            ("https://box-2.example/graphql", "scene-2"): {
                "model_version": "model-1",
                "items": [
                    recommendation_item("scene-b", 0.6),
                    recommendation_item("scene-c", 0.9),
                ],
            },
        }
    )
    monkeypatch.setattr("recommendations.StashClient", lambda server_connection: FakeConfiguredStash(tmp_path))
    monkeypatch.setattr("recommendations.ServiceClient", lambda settings: client)

    output: dict[str, object] = {}
    run(
        {
            "server_connection": {"PluginDir": str(tmp_path)},
            "args": {
                "mode": "fetch-related",
                "content_keys": [
                    {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"},
                    {"endpoint": "https://box-2.example/graphql", "stash_id": "scene-2"},
                ],
                "limit": 2,
            },
        },
        output,
    )

    assert client.related_calls == [
        ([{"endpoint": "https://box.example/graphql", "stash_id": "scene-1"}], 2),
        ([{"endpoint": "https://box-2.example/graphql", "stash_id": "scene-2"}], 2),
    ]
    assert output["output"] == {
        "items": [
            recommendation_item("scene-c", 0.9),
            recommendation_item("scene-a", 0.8),
        ],
        "model_version": "model-1",
    }


def test_fetch_for_you_mode_proxies_authenticated_read_without_api_key_markup(
    tmp_path: Path, monkeypatch: object
) -> None:
    client = FakeReadServiceClient({}, for_you={"model_version": "model-2", "items": [recommendation_item("scene-z", 0.5)]})
    monkeypatch.setattr("recommendations.StashClient", lambda server_connection: FakeConfiguredStash(tmp_path))
    monkeypatch.setattr("recommendations.ServiceClient", lambda settings: client)

    output: dict[str, object] = {}
    run(
        {
            "server_connection": {"PluginDir": str(tmp_path)},
            "args": {"mode": "fetch-for-you", "limit": 8},
        },
        output,
    )

    encoded = str(output["output"])

    assert client.for_you_calls == [(8, 0, None)]
    assert output["output"] == {"model_version": "model-2", "items": [recommendation_item("scene-z", 0.5)]}
    assert "secret-api-key" not in encoded


def test_fetch_for_you_mode_proxies_numeric_filters(tmp_path: Path, monkeypatch: object) -> None:
    client = FakeReadServiceClient({}, for_you={"model_version": "model-2", "items": []})
    monkeypatch.setattr("recommendations.StashClient", lambda server_connection: FakeConfiguredStash(tmp_path))
    monkeypatch.setattr("recommendations.ServiceClient", lambda settings: client)

    output: dict[str, object] = {}
    run(
        {
            "server_connection": {"PluginDir": str(tmp_path)},
            "args": {
                "mode": "fetch-for-you",
                "filters": {"rating": {"operator": "gte", "value": 4}},
            },
        },
        output,
    )

    assert client.for_you_calls == [(20, 0, {"rating": {"operator": "gte", "value": 4}})]


def test_status_clears_paused_auth_when_settings_change(tmp_path: Path, monkeypatch: object) -> None:
    freeze_outbox_now(monkeypatch)
    outbox = seeded_outbox(tmp_path)
    outbox.pause_delivery("service authentication failed", pause_key="auth-a")
    monkeypatch.setattr(
        "recommendations.StashClient",
        lambda server_connection: FakeConfiguredStash(tmp_path, api_key="new-secret-api-key"),
    )

    output: dict[str, object] = {}
    run(
        {
            "server_connection": {"PluginDir": str(tmp_path)},
            "args": {"mode": "status"},
        },
        output,
    )

    assert output["output"]["outbox"]["paused"] == {
        "active": False,
        "reason": None,
    }
    assert output["output"]["metadata"] == {
        "jobs": {"pending": 0, "in_progress": 0, "completed": 0, "failed": 0},
        "diagnostics": [],
    }


def seeded_outbox(tmp_path: Path) -> Outbox:
    outbox = Outbox(tmp_path / "recommendations.sqlite3")
    outbox.enqueue(
        PreferenceEvent(
            schema_version=1,
            event_id="550e8400-e29b-41d4-a716-446655440110",
            client_id="550e8400-e29b-41d4-a716-446655440001",
            sequence=1,
            occurred_at=NOW,
            content_key=CONTENT,
            kind="scene.rating.set",
            origin="hook",
            rating=0.8,
        )
    )
    return outbox


def snapshot() -> SourceSnapshot:
    return SourceSnapshot(
        schema_version=1,
        content_key=CONTENT,
        captured_at=NOW,
        source_updated_at=NOW,
        scenes=[{"id": "scene-1", "title": "Example"}],
        performers=[],
    )


class FakeServiceClient:
    def __init__(self, responses: list[ServiceResponse | Exception]) -> None:
        self._responses = list(responses)
        self.preference_events: list[dict[str, object]] = []
        self.snapshots: list[dict[str, object]] = []

    def deliver_preference_event(self, payload: dict[str, object]) -> ServiceResponse:
        self.preference_events.append(payload)
        return self._next_response()

    def deliver_snapshot(self, payload: dict[str, object]) -> ServiceResponse:
        self.snapshots.append(payload)
        return self._next_response()

    def _next_response(self) -> ServiceResponse:
        response = self._responses.pop(0)
        if isinstance(response, Exception):
            raise response
        return response


def freeze_outbox_now(monkeypatch: object) -> None:
    monkeypatch.setattr(outbox_module, "_utcnow", lambda: NOW)


class FakeConfiguredStash:
    def __init__(self, tmp_path: Path, *, api_key: str = "secret-api-key", service_url: str = "https://stashrec.example") -> None:
        self._tmp_path = tmp_path
        self._api_key = api_key
        self._service_url = service_url

    def plugin_config(self, plugin_id: str) -> dict[str, object]:
        assert plugin_id == "stashRecommendations"
        return {
            "service_url": self._service_url,
            "api_key": self._api_key,
            "show_remote_results": False,
        }


class FakeReadServiceClient:
    def __init__(self, related: dict[tuple[str, str], dict[str, object]], *, for_you: dict[str, object] | None = None) -> None:
        self._related = related
        self._for_you = for_you or {"model_version": "", "items": []}
        self.related_calls: list[tuple[list[dict[str, str]], int]] = []
        self.for_you_calls: list[tuple[int, int, dict[str, object] | None]] = []

    def fetch_related(self, content_keys: list[dict[str, str]], limit: int) -> dict[str, object]:
        self.related_calls.append((content_keys, limit))
        key = content_keys[0]["endpoint"], content_keys[0]["stash_id"]
        return dict(self._related[key])

    def fetch_for_you(
        self, limit: int, *, offset: int = 0, filters: dict[str, object] | None = None
    ) -> dict[str, object]:
        self.for_you_calls.append((limit, offset, filters))
        return dict(self._for_you)


def recommendation_item(stash_id: str, score: float) -> dict[str, object]:
    return {
        "content_key": {"endpoint": "https://box.example/graphql", "stash_id": stash_id},
        "score": score,
        "reasons": ["session_cooccurrence"],
        "model_version": "model-1",
        "canonical_url": f"https://box.example/scenes/{stash_id}",
    }
