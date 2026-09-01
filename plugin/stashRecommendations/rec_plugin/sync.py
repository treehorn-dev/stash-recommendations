from __future__ import annotations

from datetime import datetime, timezone
from dataclasses import dataclass
from pathlib import Path
import sqlite3
from typing import Any, Iterable, Iterator
from uuid import NAMESPACE_URL, uuid4, uuid5

from rec_plugin.contracts import ContentKey, PreferenceEvent
from rec_plugin.outbox import Outbox
from rec_plugin.snapshots import to_source_snapshot


class SyncState:
    def __init__(self, path: Path, *, client_id_factory: callable | None = None) -> None:
        self._path = path
        self._path.parent.mkdir(parents=True, exist_ok=True)
        self._client_id_factory = client_id_factory or (lambda: str(uuid4()))
        self._initialize()

    @property
    def client_id(self) -> str:
        with self._connect() as connection:
            row = connection.execute("SELECT client_id FROM sync_identity WHERE singleton = 1").fetchone()
            if row is not None:
                return str(row[0])
            client_id = str(self._client_id_factory())
            connection.execute(
                "INSERT INTO sync_identity(singleton, client_id, next_sequence) VALUES(1, ?, 1)",
                (client_id,),
            )
            return client_id

    def next_sequence(self) -> int:
        with self._connect() as connection:
            row = connection.execute("SELECT client_id, next_sequence FROM sync_identity WHERE singleton = 1").fetchone()
            if row is None:
                client_id = str(self._client_id_factory())
                connection.execute(
                    "INSERT INTO sync_identity(singleton, client_id, next_sequence) VALUES(1, ?, 2)",
                    (client_id,),
                )
                return 1
            sequence = int(row[1])
            connection.execute(
                "UPDATE sync_identity SET next_sequence = ? WHERE singleton = 1",
                (sequence + 1,),
            )
            return sequence

    def has_history_event(self, event_id: str) -> bool:
        with self._connect() as connection:
            row = connection.execute(
                "SELECT 1 FROM synced_history_events WHERE event_id = ?",
                (event_id,),
            ).fetchone()
        return row is not None

    def remember_history_event(self, event_id: str) -> None:
        with self._connect() as connection:
            connection.execute(
                "INSERT OR IGNORE INTO synced_history_events(event_id, remembered_at) VALUES(?, ?)",
                (event_id, _isoformat(_utcnow())),
            )

    def _initialize(self) -> None:
        with self._connect() as connection:
            connection.executescript(
                """
                CREATE TABLE IF NOT EXISTS sync_identity (
                    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
                    client_id TEXT NOT NULL,
                    next_sequence INTEGER NOT NULL
                );
                CREATE TABLE IF NOT EXISTS synced_history_events (
                    event_id TEXT PRIMARY KEY,
                    remembered_at TEXT NOT NULL
                );
                """
            )

    def _connect(self) -> sqlite3.Connection:
        return sqlite3.connect(self._path)


def queue_rating_sync(stash: Any, outbox: Outbox, state: SyncState, *, confirmed: bool) -> dict[str, Any]:
    scenes = list(stash.iter_rated_scenes())
    event_count = sum(len(list(_scene_content_keys(scene))) for scene in scenes)
    if not confirmed:
        return {"requires_confirmation": True, "count": event_count, "kind": "rating-sync"}
    events = []
    occurred_at = _utcnow()
    for scene in scenes:
        events.extend(build_rating_events(scene, state, origin="sync-ratings", occurred_at=occurred_at))
    for event in events:
        outbox.enqueue(event)
    return {"queued": len(events), "kind": "rating-sync"}


def queue_engagement_sync(stash: Any, outbox: Outbox, state: SyncState, *, confirmed: bool) -> dict[str, Any]:
    candidates = list(_new_history_candidates(stash.iter_engagement_history(), state))
    if not confirmed:
        return {"requires_confirmation": True, "count": len(candidates), "kind": "engagement-sync"}
    events = list(_materialize_history_events(candidates, state))
    for event in events:
        outbox.enqueue(event)
        state.remember_history_event(event.event_id)
    return {"queued": len(events), "kind": "engagement-sync"}


def queue_metadata_sync(stash: Any, outbox: Outbox, source: Any, *, confirmed: bool) -> dict[str, Any]:
    keys = list(_configured_content_keys(stash, source))
    if not confirmed:
        return {"requires_confirmation": True, "count": len(keys), "kind": "metadata-sync"}
    queued = 0
    for key in keys:
        scene = source.fetch_scene(key.endpoint, key.stash_id)
        if scene is None:
            continue
        outbox.enqueue(to_source_snapshot(key.endpoint, _utcnow(), scene))
        queued += 1
    return {"queued": queued, "kind": "metadata-sync"}


def build_rating_events(
    scene: dict[str, Any],
    state: SyncState,
    *,
    origin: str,
    occurred_at: datetime | None = None,
) -> list[PreferenceEvent]:
    occurred = (occurred_at or _utcnow()).astimezone(timezone.utc)
    rating_value = scene.get("rating100")
    if isinstance(rating_value, int) and not isinstance(rating_value, bool) and rating_value > 0:
        kind = "scene.rating.set"
        rating = rating_value / 100.0
    else:
        kind = "scene.rating.remove"
        rating = None
    client_id = state.client_id
    events: list[PreferenceEvent] = []
    for key in _scene_content_keys(scene):
        event = PreferenceEvent(
            schema_version=1,
            event_id=str(uuid4()),
            client_id=client_id,
            sequence=state.next_sequence(),
            occurred_at=occurred,
            content_key=key,
            kind=kind,
            origin=origin,
            rating=rating,
        )
        events.append(event)
    return events


def build_history_event_id(
    client_id: str,
    kind: str,
    endpoint: str,
    stash_id: str,
    occurred_at: datetime,
) -> str:
    key = ContentKey.normalize(endpoint, stash_id)
    return str(
        uuid5(
            NAMESPACE_URL,
            "|".join([client_id, kind, key.endpoint, key.stash_id, _isoformat(occurred_at)]),
        )
    )


@dataclass(frozen=True)
class HistoryCandidate:
    event_id: str
    content_key: ContentKey
    kind: str
    occurred_at: datetime


def _new_history_candidates(history_rows: Iterable[dict[str, Any]], state: SyncState) -> Iterator[HistoryCandidate]:
    candidates: list[HistoryCandidate] = []
    seen_ids: set[str] = set()
    client_id = state.client_id
    for row in history_rows:
        for candidate in _history_candidates_for_row(row, client_id):
            if candidate.event_id in seen_ids or state.has_history_event(candidate.event_id):
                continue
            seen_ids.add(candidate.event_id)
            candidates.append(candidate)
    order = {"scene.played": 0, "scene.o": 1}
    candidates.sort(
        key=lambda candidate: (
            candidate.occurred_at,
            order[candidate.kind],
            candidate.content_key.endpoint,
            candidate.content_key.stash_id,
        )
    )
    for candidate in candidates:
        yield candidate


def _materialize_history_events(candidates: Iterable[HistoryCandidate], state: SyncState) -> Iterator[PreferenceEvent]:
    client_id = state.client_id
    for candidate in candidates:
        yield PreferenceEvent(
            schema_version=1,
            event_id=candidate.event_id,
            client_id=client_id,
            sequence=state.next_sequence(),
            occurred_at=candidate.occurred_at,
            content_key=candidate.content_key,
            kind=candidate.kind,
            origin="sync-engagement",
        )


def _history_candidates_for_row(row: dict[str, Any], client_id: str) -> Iterator[HistoryCandidate]:
    for kind, field in (("scene.played", "play_history"), ("scene.o", "o_history")):
        for occurred_at in row.get(field, []) or []:
            if not isinstance(occurred_at, datetime):
                continue
            for key in _scene_content_keys(row):
                yield HistoryCandidate(
                    event_id=build_history_event_id(client_id, kind, key.endpoint, key.stash_id, occurred_at),
                    content_key=key,
                    kind=kind,
                    occurred_at=occurred_at.astimezone(timezone.utc),
                )


def _configured_content_keys(stash: Any, source: Any) -> Iterator[ContentKey]:
    seen: set[tuple[str, str]] = set()
    for row in list(stash.iter_rated_scenes()) + list(stash.iter_engagement_history()):
        for key in _scene_content_keys(row):
            if source.credentials_for(key.endpoint) is None:
                continue
            marker = (key.endpoint, key.stash_id)
            if marker in seen:
                continue
            seen.add(marker)
            yield key


def _scene_content_keys(scene: dict[str, Any]) -> Iterator[ContentKey]:
    for entry in scene.get("stash_ids", []) or []:
        if isinstance(entry, ContentKey):
            yield entry
            continue
        endpoint = entry.get("endpoint")
        stash_id = entry.get("stash_id")
        if isinstance(endpoint, str) and isinstance(stash_id, str):
            yield ContentKey.normalize(endpoint, stash_id)


def _utcnow() -> datetime:
    return datetime.now(timezone.utc)


def _isoformat(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")
