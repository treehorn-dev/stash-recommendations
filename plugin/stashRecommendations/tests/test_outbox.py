from __future__ import annotations

from datetime import datetime, timedelta, timezone
import json
from pathlib import Path

from rec_plugin.contracts import ContentKey, PreferenceEvent, SourceSnapshot
from rec_plugin import outbox as outbox_module
from rec_plugin.outbox import Outbox
EVENT_ID = "550e8400-e29b-41d4-a716-446655440000"
CLIENT_ID = "550e8400-e29b-41d4-a716-446655440001"
CONTENT = ContentKey.normalize("https://box.example/graphql", "scene-1")
NOW = datetime(2026, 8, 30, 12, 0, tzinfo=timezone.utc)


def test_outbox_preserves_event_identity_across_retry(
    tmp_path: Path, monkeypatch: object
) -> None:
    _freeze_outbox_now(monkeypatch)
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
    ready = outbox.next_ready(NOW)
    assert ready is not None
    outbox.record_retry(ready.row_id, NOW)

    assert outbox.next_ready(NOW) is None
    assert outbox.next_ready(NOW + timedelta(minutes=2)).event_id == EVENT_ID


def test_outbox_snapshot_rows_can_be_acked_and_retried_by_row_identity(
    tmp_path: Path, monkeypatch: object
) -> None:
    _freeze_outbox_now(monkeypatch)
    outbox = Outbox(tmp_path / "recommendations.sqlite3")
    outbox.enqueue(_snapshot())

    ready = outbox.next_ready(NOW)

    assert ready is not None
    assert ready.item_type == "source_snapshot"
    assert ready.event_id is None
    assert ready.row_id is not None

    outbox.record_retry(ready.row_id, NOW)

    assert outbox.next_ready(NOW) is None

    retried = outbox.next_ready(NOW + timedelta(minutes=2))

    assert retried is not None
    assert retried.row_id == ready.row_id

    outbox.ack(retried.row_id)

    assert outbox.status()["delivered"]["snapshot"] == 1


def test_outbox_hook_rows_can_be_quarantined_by_row_identity(
    tmp_path: Path, monkeypatch: object
) -> None:
    _freeze_outbox_now(monkeypatch)
    outbox = Outbox(tmp_path / "recommendations.sqlite3")
    outbox.enqueue_hook("capture-rating", {"id": 44})

    ready = outbox.next_ready(NOW)

    assert ready is not None
    assert ready.item_type == "hook"
    assert ready.event_id is None
    assert ready.row_id is not None

    outbox.quarantine(ready.row_id, "invalid payload")

    assert outbox.status()["quarantined"]["hook"] == 1


def test_outbox_status_counts_rating_play_and_o_separately_without_payload_leakage(
    tmp_path: Path, monkeypatch: object
) -> None:
    _freeze_outbox_now(monkeypatch)
    outbox = Outbox(tmp_path / "recommendations.sqlite3")
    outbox.enqueue(_event("scene.rating.set", "550e8400-e29b-41d4-a716-446655440010", 0.9))
    outbox.enqueue(_event("scene.played", "550e8400-e29b-41d4-a716-446655440011"))
    outbox.enqueue(_event("scene.o", "550e8400-e29b-41d4-a716-446655440012"))

    rating = outbox.next_ready(NOW)
    assert rating is not None
    outbox.record_retry(rating.row_id, NOW)

    play = outbox.next_ready(NOW)
    assert play is not None
    outbox.quarantine(play.row_id, "invalid payload")

    o_event = outbox.next_ready(NOW)
    assert o_event is not None
    outbox.ack(o_event.row_id)

    status = outbox.status()
    encoded = json.dumps(status)

    assert status["pending"] == {"rating": 1, "play": 0, "o": 0, "snapshot": 0, "hook": 0}
    assert status["quarantined"] == {"rating": 0, "play": 1, "o": 0, "snapshot": 0, "hook": 0}
    assert status["delivered"] == {"rating": 0, "play": 0, "o": 1, "snapshot": 0, "hook": 0}
    assert status["last_error"] == "invalid payload"
    assert "scene.rating.set" not in encoded
    assert "secret" not in encoded


def test_outbox_retry_persists_last_error_in_status(tmp_path: Path, monkeypatch: object) -> None:
    _freeze_outbox_now(monkeypatch)
    outbox = Outbox(tmp_path / "recommendations.sqlite3")
    outbox.enqueue(_event("scene.played", "550e8400-e29b-41d4-a716-446655440013"))

    ready = outbox.next_ready(NOW)

    assert ready is not None

    outbox.record_retry(ready.row_id, NOW, "stash service unavailable")

    status = outbox.status()

    assert status["pending"] == {"rating": 0, "play": 1, "o": 0, "snapshot": 0, "hook": 0}
    assert status["last_error"] == "stash service unavailable"


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


def _snapshot() -> SourceSnapshot:
    return SourceSnapshot(
        schema_version=1,
        content_key=CONTENT,
        captured_at=NOW,
        source_updated_at=NOW,
        scenes=[{"id": "scene-1"}],
        performers=[],
    )


def _freeze_outbox_now(monkeypatch: object) -> None:
    monkeypatch.setattr(outbox_module, "_utcnow", lambda: NOW)
