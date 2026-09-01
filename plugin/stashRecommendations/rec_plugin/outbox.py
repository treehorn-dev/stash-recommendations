from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import sqlite3
from typing import Any

from rec_plugin.contracts import PreferenceEvent, SourceSnapshot


STATE_PENDING = "pending"
STATE_QUARANTINED = "quarantined"


@dataclass
class OutboxItem:
    item_type: str
    metric: str
    payload: dict[str, Any]
    event_id: str | None = None


class Outbox:
    def __init__(self, path: Path) -> None:
        self._path = path
        self._path.parent.mkdir(parents=True, exist_ok=True)
        self._initialize()

    def enqueue(self, item: PreferenceEvent | SourceSnapshot) -> None:
        if isinstance(item, PreferenceEvent):
            self._insert_row(
                event_id=item.event_id,
                item_type="preference_event",
                metric=_metric_for_event(item.kind),
                payload=item.to_dict(),
            )
            return
        if isinstance(item, SourceSnapshot):
            self._insert_row(
                event_id=None,
                item_type="source_snapshot",
                metric="snapshot",
                payload=item.to_dict(),
            )
            return
        raise TypeError(f"unsupported outbox item: {type(item)!r}")

    def enqueue_hook(self, hook_name: str, hook_context: dict[str, Any]) -> None:
        self._insert_row(
            event_id=None,
            item_type="hook",
            metric="hook",
            payload={"name": hook_name, "hook_context": hook_context},
        )

    def next_ready(self, now: datetime) -> OutboxItem | None:
        row = self._fetchone(
            """
            SELECT event_id, item_type, metric, payload_json
            FROM outbox
            WHERE state = ? AND next_attempt_at <= ?
            ORDER BY id ASC
            LIMIT 1
            """,
            (STATE_PENDING, _isoformat(now)),
        )
        if row is None:
            return None
        return OutboxItem(
            event_id=row[0],
            item_type=row[1],
            metric=row[2],
            payload=json.loads(row[3]),
        )

    def ack(self, event_id: str) -> None:
        with self._connect() as connection:
            row = connection.execute(
                "SELECT metric FROM outbox WHERE event_id = ?",
                (event_id,),
            ).fetchone()
            if row is None:
                return
            connection.execute(
                """
                INSERT INTO delivery_counters(metric, delivered_count)
                VALUES(?, 1)
                ON CONFLICT(metric) DO UPDATE SET delivered_count = delivered_count + 1
                """,
                (row[0],),
            )
            connection.execute("DELETE FROM outbox WHERE event_id = ?", (event_id,))

    def record_retry(self, event_id: str, now: datetime) -> None:
        with self._connect() as connection:
            row = connection.execute(
                "SELECT attempt_count FROM outbox WHERE event_id = ?",
                (event_id,),
            ).fetchone()
            if row is None:
                return
            attempt_count = int(row[0]) + 1
            delay_seconds = min(3600, 120 * (2 ** (attempt_count - 1)))
            connection.execute(
                """
                UPDATE outbox
                SET attempt_count = ?, next_attempt_at = ?, updated_at = ?
                WHERE event_id = ?
                """,
                (
                    attempt_count,
                    _isoformat(now + timedelta(seconds=delay_seconds)),
                    _isoformat(now),
                    event_id,
                ),
            )

    def quarantine(self, event_id: str, error: str) -> None:
        with self._connect() as connection:
            connection.execute(
                """
                UPDATE outbox
                SET state = ?, last_error = ?, updated_at = ?
                WHERE event_id = ?
                """,
                (STATE_QUARANTINED, error, _isoformat(_utcnow()), event_id),
            )

    def status(self) -> dict[str, Any]:
        pending = self._counts_for_state(STATE_PENDING)
        quarantined = self._counts_for_state(STATE_QUARANTINED)
        delivered = _empty_counts()
        with self._connect() as connection:
            for metric, count in connection.execute("SELECT metric, delivered_count FROM delivery_counters"):
                delivered[metric] = int(count)
            row = connection.execute(
                """
                SELECT last_error
                FROM outbox
                WHERE last_error IS NOT NULL
                ORDER BY updated_at DESC, id DESC
                LIMIT 1
                """
            ).fetchone()
        return {
            "pending": pending,
            "quarantined": quarantined,
            "delivered": delivered,
            "last_error": row[0] if row else None,
        }

    def _initialize(self) -> None:
        with self._connect() as connection:
            connection.executescript(
                """
                CREATE TABLE IF NOT EXISTS outbox (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    event_id TEXT,
                    item_type TEXT NOT NULL,
                    metric TEXT NOT NULL,
                    payload_json TEXT NOT NULL,
                    attempt_count INTEGER NOT NULL DEFAULT 0,
                    next_attempt_at TEXT NOT NULL,
                    state TEXT NOT NULL DEFAULT 'pending',
                    last_error TEXT,
                    updated_at TEXT NOT NULL
                );
                CREATE UNIQUE INDEX IF NOT EXISTS outbox_event_id_unique
                ON outbox(event_id)
                WHERE event_id IS NOT NULL;
                CREATE TABLE IF NOT EXISTS delivery_counters (
                    metric TEXT PRIMARY KEY,
                    delivered_count INTEGER NOT NULL DEFAULT 0
                );
                """
            )

    def _insert_row(self, *, event_id: str | None, item_type: str, metric: str, payload: dict[str, Any]) -> None:
        now = _utcnow()
        with self._connect() as connection:
            connection.execute(
                """
                INSERT INTO outbox(event_id, item_type, metric, payload_json, attempt_count, next_attempt_at, state, updated_at)
                VALUES(?, ?, ?, ?, 0, ?, ?, ?)
                """,
                (
                    event_id,
                    item_type,
                    metric,
                    json.dumps(payload, sort_keys=True),
                    _isoformat(now),
                    STATE_PENDING,
                    _isoformat(now),
                ),
            )

    def _counts_for_state(self, state: str) -> dict[str, int]:
        counts = _empty_counts()
        with self._connect() as connection:
            for metric, count in connection.execute(
                "SELECT metric, COUNT(*) FROM outbox WHERE state = ? GROUP BY metric",
                (state,),
            ):
                counts[metric] = int(count)
        return counts

    def _fetchone(self, query: str, parameters: tuple[object, ...]) -> tuple[Any, ...] | None:
        with self._connect() as connection:
            return connection.execute(query, parameters).fetchone()

    def _connect(self) -> sqlite3.Connection:
        connection = sqlite3.connect(self._path)
        connection.row_factory = sqlite3.Row
        return connection


def _metric_for_event(kind: str) -> str:
    if kind == "scene.rating.set" or kind == "scene.rating.remove":
        return "rating"
    if kind == "scene.played":
        return "play"
    if kind == "scene.o":
        return "o"
    raise ValueError(f"unsupported event kind: {kind}")


def _empty_counts() -> dict[str, int]:
    return {"rating": 0, "play": 0, "o": 0, "snapshot": 0, "hook": 0}


def _isoformat(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat().replace("+00:00", "Z")


def _utcnow() -> datetime:
    return datetime.now(timezone.utc)
