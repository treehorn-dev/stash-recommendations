from __future__ import annotations

from pathlib import Path
import os
import time

import pytest

from tests.e2e.test_rating_to_recommendation import (
    FakeStash,
    _drain_outbox,
    _free_port,
    _issue_service_api_key,
    _mock_scenes,
    _run_plugin_mode,
    _schema_dsn,
    _service_process,
    _stored_snapshot_payloads,
    _temporary_schema,
)
from tests.e2e.mock_stash_box import MockStashBox


@pytest.mark.filterwarnings("ignore::ResourceWarning")
def test_readme_privacy_smoke_checks_snapshot_json_column(tmp_path: Path) -> None:
    repo_root = Path(__file__).resolve().parents[2]
    postgres_dsn = os.environ.get(
        "POSTGRES_TEST_DSN",
        "postgres://stash_recommendations:stash_recommendations@127.0.0.1:5432/stash_recommendations?sslmode=disable",
    )
    schema = f"task11_readme_{time.time_ns()}"
    with _temporary_schema(postgres_dsn, schema):
        schema_dsn = _schema_dsn(postgres_dsn, schema)
        service_api_key = _issue_service_api_key(repo_root, schema_dsn)
        port = _free_port()

        with MockStashBox("127.0.0.1", _free_port(), "source-key", _mock_scenes()) as stash_box:
            stash = FakeStash(
                service_url=f"http://127.0.0.1:{port}",
                service_api_key=service_api_key,
                stash_box=stash_box,
                plugin_dir=tmp_path / "plugin-data",
            )
            with _service_process(repo_root, schema_dsn, port, build_model_on_start=False):
                preview = _run_plugin_mode(repo_root, stash, {"mode": "sync-metadata"})
                result = _run_plugin_mode(repo_root, stash, {"mode": "sync-metadata", "confirmed": True})
                drained = _drain_outbox(repo_root, stash)

        stored_payloads = _stored_snapshot_payloads(schema_dsn)

    assert preview == {"requires_confirmation": True, "count": 2, "kind": "metadata-sync"}
    assert result == {"queued": 2, "kind": "metadata-sync"}
    assert drained["delivery"] == {"delivered": 2, "retried": 0, "quarantined": 0, "paused": False}
    assert "Scene A" in stored_payloads
    assert "source-key" not in stored_payloads
