from __future__ import annotations

from datetime import datetime, timedelta, timezone
import json
from pathlib import Path

from rec_plugin.contracts import ContentKey, PreferenceEvent
from rec_plugin.outbox import Outbox
from recommendations import run


EVENT_ID = "550e8400-e29b-41d4-a716-446655440000"
CLIENT_ID = "550e8400-e29b-41d4-a716-446655440001"
CONTENT = ContentKey.normalize("https://box.example/graphql", "scene-1")
NOW = datetime(2026, 8, 30, 12, 0, tzinfo=timezone.utc)


def test_outbox_preserves_event_identity_across_retry(tmp_path: Path) -> None:
    outbox = Outbox(tmp_path / "recommendations.sqlite3")
    event = PreferenceEvent(
        schema_version=1,
        event_id=EVENT_ID,
        client_id=CLIENT_ID,
        sequence=1,
        occurred_at=NOW,
        content_key=CONTENT,
        kind="scene.rating.set",
        origin="hook",
        rating=0.8,
    )
    outbox.enqueue(event)
    outbox.record_retry(EVENT_ID, NOW)

    assert outbox.next_ready(NOW) is None
    assert outbox.next_ready(NOW + timedelta(minutes=2)).event_id == EVENT_ID


def test_outbox_status_counts_rating_play_and_o_separately_without_payload_leakage(tmp_path: Path) -> None:
    outbox = Outbox(tmp_path / "recommendations.sqlite3")
    outbox.enqueue(_event("scene.rating.set", "550e8400-e29b-41d4-a716-446655440010", 0.9))
    outbox.enqueue(_event("scene.played", "550e8400-e29b-41d4-a716-446655440011"))
    outbox.enqueue(_event("scene.o", "550e8400-e29b-41d4-a716-446655440012"))

    outbox.quarantine("550e8400-e29b-41d4-a716-446655440011", "invalid payload")
    outbox.ack("550e8400-e29b-41d4-a716-446655440012")

    status = outbox.status()
    encoded = json.dumps(status)

    assert status["pending"] == {"rating": 1, "play": 0, "o": 0, "snapshot": 0, "hook": 0}
    assert status["quarantined"] == {"rating": 0, "play": 1, "o": 0, "snapshot": 0, "hook": 0}
    assert status["delivered"] == {"rating": 0, "play": 0, "o": 1, "snapshot": 0, "hook": 0}
    assert status["last_error"] == "invalid payload"
    assert "scene.rating.set" not in encoded
    assert "secret" not in encoded


def test_capture_rating_task_only_queues_hook_work(tmp_path: Path) -> None:
    output: dict[str, object] = {}
    run(
        {
            "server_connection": {
                "Scheme": "http",
                "Host": "127.0.0.1",
                "Port": 9999,
                "PluginDir": str(tmp_path),
            },
            "args": {
                "mode": "capture-rating",
                "hookContext": {"id": 44, "inputFields": ["id", "rating100"]},
            },
        },
        output,
    )

    status = Outbox(tmp_path / "recommendations.sqlite3").status()

    assert output["output"] == {"queued": 1, "kind": "hook"}
    assert status["pending"] == {"rating": 0, "play": 0, "o": 0, "snapshot": 0, "hook": 1}


def _event(kind: str, event_id: str, rating: float | None = None) -> PreferenceEvent:
    return PreferenceEvent(
        schema_version=1,
        event_id=event_id,
        client_id=CLIENT_ID,
        sequence=1,
        occurred_at=NOW,
        content_key=CONTENT,
        kind=kind,
        origin="sync",
        rating=rating,
    )
