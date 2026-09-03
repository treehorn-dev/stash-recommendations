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
            _add_column_if_missing(connection, "metadata_jobs", "attempt_count", "INTEGER NOT NULL DEFAULT 0")
            _add_column_if_missing(connection, "metadata_jobs", "last_error", "TEXT")
            _add_column_if_missing(connection, "metadata_jobs", "last_attempt_run", "TEXT")
            # A Stash task can be interrupted externally. Its claimed work must be retryable.
            connection.execute("UPDATE metadata_jobs SET state = 'pending' WHERE state = 'in_progress'")

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

    def claim(self, limit: int | None = None, *, attempt_run: str | None = None) -> list[tuple[str, str]]:
        if limit is not None and limit <= 0:
            return []
        with connect(self._path) as connection:
            parameters: list[object] = []
            where = "state = 'pending'"
            if attempt_run is not None:
                where += " AND (last_attempt_run IS NULL OR last_attempt_run <> ?)"
                parameters.append(attempt_run)
            if limit is None:
                rows = connection.execute(
                    f"SELECT endpoint, stash_id FROM metadata_jobs WHERE {where} ORDER BY endpoint, stash_id",
                    parameters,
                ).fetchall()
            else:
                parameters.append(limit)
                rows = connection.execute(
                    f"SELECT endpoint, stash_id FROM metadata_jobs WHERE {where} ORDER BY endpoint, stash_id LIMIT ?", parameters
                ).fetchall()
            connection.executemany(
                "UPDATE metadata_jobs SET state = 'in_progress', attempt_count = attempt_count + 1, last_attempt_run = ? WHERE endpoint = ? AND stash_id = ?",
                [(attempt_run, endpoint, stash_id) for endpoint, stash_id in rows],
            )
        return [(str(row[0]), str(row[1])) for row in rows]

    def status(self) -> dict[str, int]:
        with connect(self._path) as connection:
            counts = dict(connection.execute("SELECT state, count(*) FROM metadata_jobs GROUP BY state"))
        return {state: int(counts.get(state, 0)) for state in ("pending", "in_progress", "completed", "failed")}

    def complete(self, endpoint: str, stash_id: str) -> None:
        key = ContentKey.normalize(endpoint, stash_id)
        with connect(self._path) as connection:
            connection.execute(
                "UPDATE metadata_jobs SET state = 'completed', last_error = NULL WHERE endpoint = ? AND stash_id = ?",
                (key.endpoint, key.stash_id),
            )

    def fail(self, endpoint: str, stash_id: str, error: str) -> None:
        key = ContentKey.normalize(endpoint, stash_id)
        with connect(self._path) as connection:
            connection.execute(
                "UPDATE metadata_jobs SET state = 'pending', last_error = ? WHERE endpoint = ? AND stash_id = ?",
                (error, key.endpoint, key.stash_id),
            )
        with self._path.with_name("recommendations.metadata.log").open("a", encoding="utf-8") as log_file:
            log_file.write(f"endpoint={key.endpoint} stash_id={key.stash_id} error={error}\n")

    def diagnostics(self, limit: int = 20) -> list[dict[str, object]]:
        with connect(self._path) as connection:
            rows = connection.execute(
                """
                SELECT endpoint, stash_id, attempt_count, last_error
                FROM metadata_jobs
                WHERE last_error IS NOT NULL
                ORDER BY attempt_count DESC, endpoint, stash_id
                LIMIT ?
                """,
                (limit,),
            ).fetchall()
        return [
            {"endpoint": str(endpoint), "stash_id": str(stash_id), "attempts": int(attempts), "last_error": str(error)}
            for endpoint, stash_id, attempts, error in rows
        ]


def _add_column_if_missing(connection: object, table: str, column: str, definition: str) -> None:
    columns = {str(row[1]) for row in connection.execute(f"PRAGMA table_info({table})")}
    if column not in columns:
        connection.execute(f"ALTER TABLE {table} ADD COLUMN {column} {definition}")
