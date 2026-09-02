from __future__ import annotations

from pathlib import Path

from rec_plugin.database import BUSY_TIMEOUT_MS, connect


def test_plugin_database_connections_enable_wal_and_busy_timeout(tmp_path: Path) -> None:
    connection = connect(tmp_path / "recommendations.sqlite3")
    try:
        assert connection.execute("PRAGMA journal_mode").fetchone()[0].lower() == "wal"
        assert connection.execute("PRAGMA busy_timeout").fetchone()[0] == BUSY_TIMEOUT_MS
    finally:
        connection.close()
