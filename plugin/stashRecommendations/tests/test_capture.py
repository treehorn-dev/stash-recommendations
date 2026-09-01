from __future__ import annotations

from datetime import datetime, timezone
import json
from pathlib import Path
import sqlite3

from rec_plugin.capture import handle_scene_update
from rec_plugin.outbox import Outbox
from rec_plugin.sync import SyncState
from recommendations import run


NOW = datetime(2026, 8, 30, 12, 0, tzinfo=timezone.utc)


def test_handle_scene_update_ignores_non_rating_changes(tmp_path: Path) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        {
            "id": "44",
            "rating100": 75,
            "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-44"}],
        }
    )

    count = handle_scene_update(
        {"id": 44, "inputFields": ["id", "title"]},
        stash,
        Outbox(database_path),
        SyncState(database_path),
    )

    assert count == 0
    assert stash.find_scene_calls == 0
    assert _pending_payloads(database_path) == []


def test_handle_scene_update_fans_out_rating_events_per_external_id(tmp_path: Path) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        {
            "id": "44",
            "title": "Scene 44",
            "rating100": 75,
            "stash_ids": [
                {"endpoint": "https://box.example/graphql", "stash_id": "scene-44"},
                {"endpoint": "https://box-2.example/graphql", "stash_id": "scene-44b"},
            ],
        }
    )

    count = handle_scene_update(
        {"id": 44, "inputFields": ["id", "rating100"]},
        stash,
        Outbox(database_path),
        SyncState(database_path),
    )

    payloads = _pending_payloads(database_path)

    assert count == 2
    assert stash.find_scene_calls == 1
    assert [payload["kind"] for payload in payloads] == ["scene.rating.set", "scene.rating.set"]
    assert [payload["rating"] for payload in payloads] == [0.75, 0.75]
    assert [payload["sequence"] for payload in payloads] == [1, 2]
    assert "title" not in payloads[0]


def test_handle_scene_update_queues_remove_events_when_rating_is_cleared(tmp_path: Path) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    stash = FakeStash(
        {
            "id": "44",
            "rating100": None,
            "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-44"}],
        }
    )

    count = handle_scene_update(
        {"id": 44, "inputFields": ["id", "rating100"]},
        stash,
        Outbox(database_path),
        SyncState(database_path),
    )

    payloads = _pending_payloads(database_path)

    assert count == 1
    assert [payload["kind"] for payload in payloads] == ["scene.rating.remove"]
    assert "rating" not in payloads[0]


def test_capture_rating_mode_materializes_rating_events(tmp_path: Path, monkeypatch: object) -> None:
    database_path = tmp_path / "recommendations.sqlite3"
    monkeypatch.setattr("recommendations.StashClient", lambda server_connection: FakeStash({
        "id": "44",
        "rating100": 75,
        "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-44"}],
    }))
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

    assert output["output"] == {"queued": 1, "kind": "rating"}
    assert len(_pending_payloads(database_path)) == 1


class FakeStash:
    def __init__(self, scene: dict[str, object]) -> None:
        self._scene = scene
        self.find_scene_calls = 0

    def find_scene(self, scene_id: int | str) -> dict[str, object] | None:
        assert str(scene_id) == "44"
        self.find_scene_calls += 1
        return dict(self._scene)


def _pending_payloads(path: Path) -> list[dict[str, object]]:
    with sqlite3.connect(path) as connection:
        rows = connection.execute(
            "SELECT payload_json FROM outbox WHERE item_type = 'preference_event' ORDER BY id ASC"
        ).fetchall()
    return [json.loads(row[0]) for row in rows]
