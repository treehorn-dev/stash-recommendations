from __future__ import annotations

from pathlib import Path

from rec_plugin.contracts import ContentKey
from rec_plugin.database import connect


class MetadataJobs:
    def __init__(self, path: Path) -> None:
        self._path = path
        with connect(path) as connection:
            connection.execute(
                "CREATE TABLE IF NOT EXISTS metadata_jobs (endpoint TEXT NOT NULL, stash_id TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'pending', PRIMARY KEY(endpoint, stash_id))"
            )

    def enqueue(self, endpoint: str, stash_id: str) -> None:
        key = ContentKey.normalize(endpoint, stash_id)
        with connect(self._path) as connection:
            connection.execute(
                """
                INSERT INTO metadata_jobs(endpoint, stash_id, state)
                VALUES(?, ?, 'pending')
                ON CONFLICT(endpoint, stash_id) DO NOTHING
                """,
                (key.endpoint, key.stash_id),
            )

    def claim(self, limit: int) -> list[tuple[str, str]]:
        if limit <= 0:
            return []
        with connect(self._path) as connection:
            rows = connection.execute(
                "SELECT endpoint, stash_id FROM metadata_jobs WHERE state = 'pending' ORDER BY endpoint, stash_id LIMIT ?", (limit,)
            ).fetchall()
            connection.executemany("UPDATE metadata_jobs SET state = 'in_progress' WHERE endpoint = ? AND stash_id = ?", rows)
        return [(str(row[0]), str(row[1])) for row in rows]

    def status(self) -> dict[str, int]:
        with connect(self._path) as connection:
            counts = dict(connection.execute("SELECT state, count(*) FROM metadata_jobs GROUP BY state"))
        return {state: int(counts.get(state, 0)) for state in ("pending", "in_progress", "completed", "failed")}

    def complete(self, endpoint: str, stash_id: str) -> None:
        key = ContentKey.normalize(endpoint, stash_id)
        with connect(self._path) as connection:
            connection.execute("UPDATE metadata_jobs SET state = 'completed' WHERE endpoint = ? AND stash_id = ?", (key.endpoint, key.stash_id))

    def fail(self, endpoint: str, stash_id: str) -> None:
        key = ContentKey.normalize(endpoint, stash_id)
        with connect(self._path) as connection:
            connection.execute("UPDATE metadata_jobs SET state = 'pending' WHERE endpoint = ? AND stash_id = ?", (key.endpoint, key.stash_id))
