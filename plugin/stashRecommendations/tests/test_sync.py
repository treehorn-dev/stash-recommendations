from __future__ import annotations

from datetime import datetime, timezone
import inspect
import json
from pathlib import Path
import socket
import sqlite3
from typing import Any
from urllib import error

from rec_plugin.outbox import Outbox
from rec_plugin.metadata_jobs import MetadataJobs
from rec_plugin.source_client import HTTP_TIMEOUT_SECONDS, SourceClient
from rec_plugin.sync import (
    SyncState,
    build_history_event_id,
    queue_engagement_sync,
    queue_metadata_sync,
    queue_rating_sync,
)
from recommendations import run


CLIENT_ID = "550e8400-e29b-41d4-a716-446655440001"
PLAYED_AT = datetime(2026, 8, 30, 12, 0, tzinfo=timezone.utc)
O_AT = datetime(2026, 8, 30, 12, 30, tzinfo=timezone.utc)


def test_queue_rating_sync_requires_confirmation_before_enqueueing(tmp_path: Path) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        rated_scenes=[
            {
                "id": "1",
                "rating100": 80,
                "stash_ids": [
                    {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"},
                    {"endpoint": "https://box-2.example/graphql", "stash_id": "scene-1b"},
                ],
            },
            {
                "id": "2",
                "rating100": 40,
                "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-2"}],
            },
        ]
    )
    state = SyncState(database_path, client_id_factory=lambda: CLIENT_ID)
    outbox = Outbox(database_path)

    preview = queue_rating_sync(stash, outbox, state, confirmed=False)

    assert preview == {"requires_confirmation": True, "count": 3, "kind": "rating-sync"}
    assert _pending_payloads(database_path, "preference_event") == []

    result = queue_rating_sync(stash, outbox, state, confirmed=True)
    payloads = _pending_payloads(database_path, "preference_event")

    assert result == {"queued": 3, "kind": "rating-sync"}
    assert [payload["rating"] for payload in payloads] == [0.8, 0.8, 0.4]
    assert [payload["sequence"] for payload in payloads] == [1, 2, 3]


def test_sync_state_reserves_sequences_in_one_transaction(tmp_path: Path) -> None:
    state = SyncState(tmp_path / "recommendations.sqlite3", client_id_factory=lambda: CLIENT_ID)

    client_id, sequences = state.reserve_sequences(3)

    assert client_id == CLIENT_ID
    assert sequences == [1, 2, 3]
    assert state.next_sequence() == 4


def test_queue_engagement_sync_imports_only_new_history_with_stable_identity(tmp_path: Path) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        engagement_history=[
            {
                "id": "44",
                "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-44"}],
                "play_history": [PLAYED_AT],
                "o_history": [O_AT],
            }
        ]
    )
    state = SyncState(database_path, client_id_factory=lambda: CLIENT_ID)
    outbox = Outbox(database_path)

    preview = queue_engagement_sync(stash, outbox, state, confirmed=False)

    assert preview == {"requires_confirmation": True, "count": 2, "kind": "engagement-sync"}

    result = queue_engagement_sync(stash, outbox, state, confirmed=True)
    payloads = _pending_payloads(database_path, "preference_event")

    assert result == {"queued": 2, "kind": "engagement-sync"}
    assert [payload["kind"] for payload in payloads] == ["scene.played", "scene.o"]
    assert [payload["sequence"] for payload in payloads] == [1, 2]
    assert queue_engagement_sync(stash, outbox, state, confirmed=True) == {"queued": 0, "kind": "engagement-sync"}
    assert build_history_event_id(
        CLIENT_ID,
        "scene.played",
        "https://box.example/graphql",
        "scene-44",
        PLAYED_AT,
    ) == payloads[0]["event_id"]


def test_queue_engagement_sync_skips_pending_deterministic_events_without_history_marker(tmp_path: Path) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        engagement_history=[
            {
                "id": "44",
                "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-44"}],
                "play_history": [PLAYED_AT],
                "o_history": [],
            }
        ]
    )
    state = SyncState(database_path, client_id_factory=lambda: CLIENT_ID)
    outbox = Outbox(database_path)

    assert queue_engagement_sync(stash, outbox, state, confirmed=True) == {"queued": 1, "kind": "engagement-sync"}
    with sqlite3.connect(database_path) as connection:
        connection.execute("DELETE FROM synced_history_events")

    result = queue_engagement_sync(stash, outbox, state, confirmed=True)

    assert result == {"queued": 0, "kind": "engagement-sync"}
    assert len(_pending_payloads(database_path, "preference_event")) == 1


def test_queue_engagement_sync_batches_sequence_reservation_history_markers_and_outbox_writes(tmp_path: Path) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        engagement_history=[
            {
                "id": "44",
                "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-44"}],
                "play_history": [PLAYED_AT],
                "o_history": [O_AT],
            }
        ]
    )
    state = RecordingSyncState(database_path, client_id_factory=lambda: CLIENT_ID)
    outbox = RecordingOutbox(database_path)

    result = queue_engagement_sync(stash, outbox, state, confirmed=True)
    payloads = _pending_payloads(database_path, "preference_event")

    assert result == {"queued": 2, "kind": "engagement-sync"}
    assert state.reserve_counts == [2]
    assert state.next_sequence_calls == 0
    assert state.remember_many_event_ids == [[payload["event_id"] for payload in payloads]]
    assert state.remembered_event_ids == []
    assert outbox.enqueue_many_counts == [2]
    assert outbox.enqueue_event_ids == []
    assert [payload["sequence"] for payload in payloads] == [1, 2]
    assert sorted(_remembered_history_event_ids(database_path)) == sorted(payload["event_id"] for payload in payloads)


def test_build_history_event_id_varies_by_client_id() -> None:
    first = build_history_event_id(
        "550e8400-e29b-41d4-a716-446655440001",
        "scene.played",
        "https://box.example/graphql",
        "scene-44",
        PLAYED_AT,
    )
    second = build_history_event_id(
        "550e8400-e29b-41d4-a716-446655440099",
        "scene.played",
        "https://box.example/graphql",
        "scene-44",
        PLAYED_AT,
    )

    assert first != second


def test_metadata_sync_claims_every_pending_scene_by_default(tmp_path: Path) -> None:
    jobs = MetadataJobs(tmp_path / "recommendations.sqlite3")
    jobs.enqueue("https://box.example/graphql", "scene-2")
    jobs.enqueue("https://box.example/graphql", "scene-1")

    assert jobs.claim() == [
        ("https://box.example/graphql", "scene-1"),
        ("https://box.example/graphql", "scene-2"),
    ]
    assert inspect.signature(queue_metadata_sync).parameters["batch_size"].default is None


def test_queue_metadata_sync_deduplicates_keys_and_queues_only_configured_sources(tmp_path: Path) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        rated_scenes=[
            {
                "id": "1",
                "rating100": 80,
                "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-1"}],
            }
        ],
        engagement_history=[
            {
                "id": "1",
                "stash_ids": [
                    {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"},
                    {"endpoint": "https://other.example/graphql", "stash_id": "scene-1-remote"},
                ],
                "play_history": [PLAYED_AT],
                "o_history": [],
            }
        ],
    )
    source = SourceClient(
        [{"endpoint": "https://box.example/graphql", "api_key": "source-key", "max_requests_per_minute": 30}],
        transport=lambda url, api_key, query, variables: {
            "data": {
                "findScene": {
                    "id": variables["id"],
                    "title": "Example Scene",
                    "release_date": "2026-08-30",
                    "urls": ["https://example.test/scenes/scene-1"],
                    "updated": "2026-08-31T00:00:00Z",
                    "images": [{"url": "https://images.example/scene.jpg"}],
                    "performers": [],
                    "tags": [],
                }
            }
        },
    )
    outbox = Outbox(database_path)

    preview = queue_metadata_sync(stash, outbox, source, confirmed=False)

    assert preview == {"requires_confirmation": True, "count": 1, "kind": "metadata-sync"}
    assert _pending_payloads(database_path, "source_snapshot") == []

    result = queue_metadata_sync(stash, outbox, source, confirmed=True, batch_size=1)
    payloads = _pending_payloads(database_path, "source_snapshot")

    assert result == {
        "queued": 1,
        "processed": 1,
        "job_status": {"pending": 0, "in_progress": 0, "completed": 1, "failed": 0},
        "kind": "metadata-sync",
    }
    assert len(payloads) == 1
    assert payloads[0]["content_key"] == {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"}
    assert payloads[0]["source_updated_at"] == "2026-08-31T00:00:00Z"


def test_queue_metadata_sync_processes_all_jobs_in_recoverable_claim_batches(tmp_path: Path) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        rated_scenes=[
            {
                "id": "1",
                "rating100": 80,
                "stash_ids": [{"endpoint": "HTTPS://BOX.EXAMPLE/graphql/", "stash_id": "scene-2"}],
            },
            {
                "id": "2",
                "rating100": 40,
                "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-1"}],
            },
        ]
    )
    source = SourceClient(
        [{"endpoint": "https://box.example/graphql", "api_key": "source-key", "max_requests_per_minute": 30}],
        transport=lambda url, api_key, query, variables: {
            "data": {
                "findScene": {
                    "id": variables["id"],
                    "title": f"Example {variables['id']}",
                    "release_date": "2026-08-30",
                    "urls": [f"https://example.test/scenes/{variables['id']}"],
                    "updated": "2026-08-31T00:00:00Z",
                    "images": [{"url": "https://images.example/scene.jpg"}],
                    "performers": [],
                    "tags": [],
                }
            }
        },
    )
    outbox = Outbox(database_path)

    result = queue_metadata_sync(stash, outbox, source, confirmed=True, batch_size=1)
    payloads = _pending_payloads(database_path, "source_snapshot")

    assert result == {
        "queued": 2,
        "processed": 2,
        "job_status": {"pending": 0, "in_progress": 0, "completed": 2, "failed": 0},
        "kind": "metadata-sync",
    }
    assert len(payloads) == 2
    assert [payload["content_key"]["stash_id"] for payload in payloads] == ["scene-1", "scene-2"]

    second = queue_metadata_sync(stash, outbox, source, confirmed=True, batch_size=1)
    payloads = _pending_payloads(database_path, "source_snapshot")

    assert second == {
        "queued": 0,
        "processed": 0,
        "job_status": {"pending": 0, "in_progress": 0, "completed": 2, "failed": 0},
        "kind": "metadata-sync",
    }
    assert [payload["content_key"]["stash_id"] for payload in payloads] == ["scene-1", "scene-2"]


def test_queue_metadata_sync_returns_failed_jobs_to_pending_after_processing_error(
    tmp_path: Path, monkeypatch: object
) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        rated_scenes=[
            {
                "id": "1",
                "rating100": 80,
                "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-1"}],
            },
            {
                "id": "2",
                "rating100": 40,
                "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-2"}],
            },
        ]
    )
    timeouts: list[float] = []

    def fake_urlopen(http_request: Any, timeout: float) -> object:
        payload = json.loads(http_request.data.decode("utf-8"))
        stash_id = str(payload["variables"]["id"])
        timeouts.append(timeout)
        if stash_id == "scene-1":
            raise error.URLError(socket.timeout("timed out"))
        return FakeHTTPResponse(
            {
                "data": {
                    "findScene": {
                        "id": stash_id,
                        "title": f"Example {stash_id}",
                        "release_date": "2026-08-30",
                        "urls": [f"https://example.test/scenes/{stash_id}"],
                        "updated": "2026-08-31T00:00:00Z",
                        "images": [{"url": "https://images.example/scene.jpg"}],
                        "performers": [],
                        "tags": [],
                    }
                }
            }
        )

    monkeypatch.setattr("rec_plugin.source_client.request.urlopen", fake_urlopen)
    source = SourceClient(
        [{"endpoint": "https://box.example/graphql", "api_key": "source-key", "max_requests_per_minute": 30}]
    )
    outbox = Outbox(database_path)

    result = queue_metadata_sync(stash, outbox, source, confirmed=True, batch_size=2)
    payloads = _pending_payloads(database_path, "source_snapshot")

    assert result == {
        "queued": 1,
        "failed": 1,
        "processed": 2,
        "job_status": {"pending": 1, "in_progress": 0, "completed": 1, "failed": 0},
        "kind": "metadata-sync",
        "errors": [
            {
                "content_key": {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"},
                "error": "timed out",
            }
        ],
        "diagnostics": [
            {
                "attempts": 1,
                "endpoint": "https://box.example/graphql",
                "last_error": "timed out",
                "stash_id": "scene-1",
            }
        ],
    }
    assert len(payloads) == 1
    assert payloads[0]["content_key"] == {"endpoint": "https://box.example/graphql", "stash_id": "scene-2"}
    assert timeouts == [HTTP_TIMEOUT_SECONDS, HTTP_TIMEOUT_SECONDS]


def test_sync_engagement_mode_requires_confirmation_before_queueing(tmp_path: Path, monkeypatch: object) -> None:
    monkeypatch.setattr(
        "recommendations.StashClient",
        lambda server_connection: FakeStash(
            engagement_history=[
                {
                    "id": "44",
                    "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-44"}],
                    "play_history": [PLAYED_AT],
                    "o_history": [O_AT],
                }
            ]
        ),
    )
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
                "mode": "sync-engagement",
            },
        },
        output,
    )

    assert output["output"] == {"requires_confirmation": True, "count": 2, "kind": "engagement-sync"}
    assert _pending_payloads(tmp_path / "recommendations.sqlite3", "preference_event") == []


class FakeStash:
    def __init__(
        self,
        *,
        rated_scenes: list[dict[str, object]] | None = None,
        engagement_history: list[dict[str, object]] | None = None,
    ) -> None:
        self._rated_scenes = rated_scenes or []
        self._engagement_history = engagement_history or []

    def iter_rated_scenes(self) -> list[dict[str, object]]:
        return [dict(scene) for scene in self._rated_scenes]

    def iter_engagement_history(self) -> list[dict[str, object]]:
        return [dict(scene) for scene in self._engagement_history]


class RecordingSyncState(SyncState):
    def __init__(self, path: Path, *, client_id_factory: callable | None = None) -> None:
        super().__init__(path, client_id_factory=client_id_factory)
        self.reserve_counts: list[int] = []
        self.next_sequence_calls = 0
        self.remembered_event_ids: list[str] = []
        self.remember_many_event_ids: list[list[str]] = []

    def next_sequence(self) -> int:
        self.next_sequence_calls += 1
        return super().next_sequence()

    def reserve_sequences(self, count: int) -> tuple[str, list[int]]:
        self.reserve_counts.append(count)
        return super().reserve_sequences(count)

    def remember_history_event(self, event_id: str) -> None:
        self.remembered_event_ids.append(event_id)
        super().remember_history_event(event_id)

    def remember_history_events(self, event_ids: list[str]) -> None:
        event_id_list = list(event_ids)
        self.remember_many_event_ids.append(event_id_list)
        super().remember_history_events(event_id_list)


class RecordingOutbox(Outbox):
    def __init__(self, path: Path) -> None:
        super().__init__(path)
        self.enqueue_event_ids: list[str] = []
        self.enqueue_many_counts: list[int] = []

    def enqueue(self, item: object) -> None:
        self.enqueue_event_ids.append(item.event_id)
        super().enqueue(item)

    def enqueue_many(self, items: list[object]) -> None:
        self.enqueue_many_counts.append(len(items))
        super().enqueue_many(items)


def _pending_payloads(path: Path, item_type: str) -> list[dict[str, object]]:
    with sqlite3.connect(path) as connection:
        rows = connection.execute(
            "SELECT payload_json FROM outbox WHERE item_type = ? ORDER BY id ASC",
            (item_type,),
        ).fetchall()
    return [json.loads(row[0]) for row in rows]


def _remembered_history_event_ids(path: Path) -> list[str]:
    with sqlite3.connect(path) as connection:
        rows = connection.execute(
            "SELECT event_id FROM synced_history_events ORDER BY event_id ASC"
        ).fetchall()
    return [str(row[0]) for row in rows]


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
