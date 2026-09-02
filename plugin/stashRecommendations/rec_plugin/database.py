from __future__ import annotations

from pathlib import Path
import sqlite3


BUSY_TIMEOUT_MS = 30_000


def connect(path: Path) -> sqlite3.Connection:
    """Open a plugin database connection that tolerates concurrent Stash tasks."""
    connection = sqlite3.connect(path, timeout=BUSY_TIMEOUT_MS / 1000)
    connection.execute("PRAGMA journal_mode = WAL")
    connection.execute(f"PRAGMA busy_timeout = {BUSY_TIMEOUT_MS}")
    return connection
