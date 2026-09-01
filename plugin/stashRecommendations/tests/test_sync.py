from __future__ import annotations

from datetime import datetime, timezone
import json
from pathlib import Path
import sqlite3

from rec_plugin.outbox import Outbox
from rec_plugin.source_client import SourceClient
from rec_plugin.sync import SyncState, build_history_event_id, queue_engagement_sync, queue_metadata_sync, queue_rating_sync
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

    result = queue_metadata_sync(stash, outbox, source, confirmed=True)
    payloads = _pending_payloads(database_path, "source_snapshot")

    assert result == {"queued": 1, "kind": "metadata-sync"}
    assert len(payloads) == 1
    assert payloads[0]["content_key"] == {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"}
    assert payloads[0]["source_updated_at"] == "2026-08-31T00:00:00Z"


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


def _pending_payloads(path: Path, item_type: str) -> list[dict[str, object]]:
    with sqlite3.connect(path) as connection:
        rows = connection.execute(
            "SELECT payload_json FROM outbox WHERE item_type = ? ORDER BY id ASC",
            (item_type,),
        ).fetchall()
    return [json.loads(row[0]) for row in rows]
