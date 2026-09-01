from __future__ import annotations

from contextlib import contextmanager
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
from typing import Any, Iterator
from unittest.mock import patch
from urllib import error, parse, request

import pytest

from rec_plugin.source_client import SourceClient
from rec_plugin.settings import Settings
from recommendations import run
from tests.e2e.mock_stash_box import MockStashBox


@pytest.mark.filterwarnings("ignore::ResourceWarning")
def test_hook_to_snapshot_to_recommendation(tmp_path: Path) -> None:
    repo_root = Path(__file__).resolve().parents[2]
    postgres_dsn = os.environ.get(
        "POSTGRES_TEST_DSN",
        "postgres://stash_recommendations:stash_recommendations@127.0.0.1:5432/stash_recommendations?sslmode=disable",
    )
    schema = f"task11_e2e_{time.time_ns()}"
    with _temporary_schema(postgres_dsn, schema):
        schema_dsn = _schema_dsn(postgres_dsn, schema)
        service_api_key = _issue_service_api_key(repo_root, schema_dsn)

        port = _free_port()
        service_url = f"http://127.0.0.1:{port}"
        with MockStashBox("127.0.0.1", _free_port(), "source-key", _mock_scenes()) as stash_box:
            stash = FakeStash(
                service_url=service_url,
                service_api_key=service_api_key,
                stash_box=stash_box,
                plugin_dir=tmp_path / "plugin-data",
            )
            first_logs: list[str] = []
            with _service_process(repo_root, schema_dsn, port, build_model_on_start=False, captured_logs=first_logs):
                _run_plugin_mode(repo_root, stash, {"mode": "capture-rating", "hookContext": {"id": 44, "inputFields": ["id", "rating100"]}})
                _run_plugin_mode(repo_root, stash, {"mode": "sync-metadata", "confirmed": True})
                delivery = _drain_outbox(repo_root, stash)

            model_version = str(_go_admin(repo_root, schema_dsn, "build-model")["model_version"])
            second_logs: list[str] = []
            with _service_process(repo_root, schema_dsn, port, build_model_on_start=False, captured_logs=second_logs):
                related = _fetch_related(service_url, service_api_key, stash_box.scene_key("scene-a"))

            assert delivery["delivery"] == {"delivered": 3, "retried": 0, "quarantined": 0, "paused": False}
            assert related["model_version"] == model_version, "\n".join(second_logs) or str(related)
            assert related["items"], related
            assert related["items"][0]["content_key"] == stash_box.scene_key("scene-b")
            assert all(captured.headers.get("apikey") == "source-key" for captured in stash_box.requests)
            assert "source-key" not in _stored_snapshot_payloads(schema_dsn)


class FakeStash:
    def __init__(self, *, service_url: str, service_api_key: str, stash_box: MockStashBox, plugin_dir: Path) -> None:
        self._service_url = service_url
        self._service_api_key = service_api_key
        self._stash_box = stash_box
        self._plugin_dir = plugin_dir

    def find_scene(self, scene_id: int | str) -> dict[str, Any] | None:
        if str(scene_id) != "44":
            return None
        return {
            "id": "44",
            "rating100": 80,
            "stash_ids": [self._stash_box.scene_key("scene-a")],
        }

    def iter_rated_scenes(self) -> list[dict[str, Any]]:
        return [
            {"id": "44", "rating100": 80, "stash_ids": [self._stash_box.scene_key("scene-a")]},
            {"id": "45", "rating100": 60, "stash_ids": [self._stash_box.scene_key("scene-b")]},
        ]

    def iter_engagement_history(self) -> list[dict[str, Any]]:
        return []

    def configured_stash_boxes(self) -> list[dict[str, object]]:
        return self._stash_box.credentials_config()

    def plugin_config(self, plugin_id: str) -> dict[str, object]:
        assert plugin_id == "stashRecommendations"
        return {
            "service_url": self._service_url.replace("http://", "https://"),
            "api_key": self._service_api_key,
            "show_remote_results": False,
        }

    def source_transport(self, url: str, api_key: str, query: str, variables: dict[str, object]) -> dict[str, object]:
        target = self._stash_box.transport_url if url == self._stash_box.endpoint else url
        payload = json.dumps({"query": query, "variables": variables}).encode("utf-8")
        http_request = request.Request(
            target,
            data=payload,
            headers={"Content-Type": "application/json", "ApiKey": api_key},
            method="POST",
        )
        with request.urlopen(http_request) as response:
            return json.loads(response.read().decode("utf-8"))


def _run_plugin_mode(repo_root: Path, stash: FakeStash, args: dict[str, object]) -> dict[str, Any]:
    del repo_root
    output: dict[str, Any] = {}
    with patch("recommendations.StashClient", lambda server_connection: stash):
        with patch("recommendations.SourceClient", lambda configured_sources: SourceClient(configured_sources, transport=stash.source_transport)):
            with patch.object(
                Settings,
                "from_plugin_config",
                classmethod(lambda cls, config: Settings(service_url=stash._service_url, api_key=stash._service_api_key, show_remote_results=False)),
            ):
                run(
                    {
                        "server_connection": {"PluginDir": str(stash_db_dir(stash))},
                        "args": args,
                    },
                    output,
                )
    return dict(output["output"])


def _drain_outbox(repo_root: Path, stash: FakeStash) -> dict[str, Any]:
    for _ in range(5):
        output = _run_plugin_mode(repo_root, stash, {"mode": "deliver-outbox"})
        if sum(output["outbox"]["pending"].values()) == 0:
            return output
    raise AssertionError("outbox did not drain")


def stash_db_dir(stash: FakeStash) -> Path:
    return stash._plugin_dir


def _mock_scenes() -> dict[str, dict[str, Any]]:
    timestamp = "2026-08-30T12:00:00Z"
    performer = {
        "id": "performer-1",
        "name": "Performer 1",
        "aliases": [],
        "gender": "female",
        "country": "US",
        "ethnicity": "white",
        "eye_color": "blue",
        "hair_color": "blonde",
        "career_start_year": 2020,
        "career_end_year": 2026,
        "images": [{"url": "https://images.example/performer-1.jpg"}],
        "urls": ["https://box.example/performers/performer-1"],
    }
    return {
        "scene-a": {
            "id": "scene-a",
            "title": "Scene A",
            "release_date": "2026-08-01",
            "updated": timestamp,
            "images": [{"url": "https://images.example/scene-a.jpg"}],
            "performers": [{"performer": performer}],
            "tags": [],
            "groups": [],
        },
        "scene-b": {
            "id": "scene-b",
            "title": "Scene B",
            "release_date": "2026-08-02",
            "updated": timestamp,
            "images": [{"url": "https://images.example/scene-b.jpg"}],
            "performers": [{"performer": performer}],
            "tags": [],
            "groups": [],
        },
    }


def _issue_service_api_key(repo_root: Path, schema_dsn: str) -> str:
    return str(_go_admin(repo_root, schema_dsn, "issue-key")["api_key"])


@contextmanager
def _service_process(
    repo_root: Path,
    schema_dsn: str,
    port: int,
    *,
    build_model_on_start: bool,
    captured_logs: list[str] | None = None,
) -> Iterator[None]:
    log_file = tempfile.NamedTemporaryFile(mode="w+", encoding="utf-8", delete=False)
    process = subprocess.Popen(
        ["go", "run", "./server/cmd/recommendations"],
        cwd=repo_root,
        env={
            **os.environ,
            "DATABASE_URL": schema_dsn,
            "HTTP_ADDR": f"127.0.0.1:{port}",
            "BUILD_MODEL_ON_START": str(build_model_on_start).lower(),
        },
        stdout=log_file,
        stderr=subprocess.STDOUT,
        text=True,
    )
    try:
        _wait_for_health(f"http://127.0.0.1:{port}/healthz", process)
        yield
    finally:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
        log_file.flush()
        log_file.close()
        if captured_logs is not None:
            captured_logs.append(Path(log_file.name).read_text(encoding="utf-8"))
        Path(log_file.name).unlink(missing_ok=True)


@contextmanager
def _temporary_schema(postgres_dsn: str, schema: str) -> Iterator[None]:
    repo_root = Path(__file__).resolve().parents[2]
    _go_admin(repo_root, postgres_dsn, "create-schema", schema)
    try:
        yield
    finally:
        _go_admin(repo_root, postgres_dsn, "drop-schema", schema)


def _schema_dsn(dsn: str, schema: str) -> str:
    parsed = parse.urlparse(dsn)
    query = parse.parse_qs(parsed.query, keep_blank_values=True)
    query["search_path"] = [schema]
    return parse.urlunparse(parsed._replace(query=parse.urlencode(query, doseq=True)))


def _fetch_related(service_url: str, api_key: str, content_key: dict[str, str]) -> dict[str, Any]:
    query = parse.urlencode({"endpoint": content_key["endpoint"], "stash_id": content_key["stash_id"], "limit": "5"})
    response = _http_json(
        request.Request(
            f"{service_url}/v1/recommendations/related?{query}",
            headers={"Authorization": f"Bearer {api_key}"},
            method="GET",
        )
    )
    return dict(response)


def _stored_snapshot_payloads(schema_dsn: str) -> str:
    return str(_go_admin(Path(__file__).resolve().parents[2], schema_dsn, "read-snapshots").get("payloads", ""))


def _wait_for_health(url: str, process: subprocess.Popen[str]) -> None:
    deadline = time.time() + 30
    while time.time() < deadline:
        if process.poll() is not None:
            output = process.stdout.read() if process.stdout is not None else ""
            raise AssertionError(f"service exited before healthz became ready:\n{output}")
        try:
            response = _http_json(request.Request(url, method="GET"))
            if response.get("status") == "ok":
                return
        except OSError:
            time.sleep(0.2)
    raise AssertionError("service did not become healthy before timeout")


def _http_json(http_request: request.Request) -> dict[str, Any]:
    try:
        with request.urlopen(http_request) as response:
            return json.loads(response.read().decode("utf-8"))
    except error.HTTPError as exc:  # pragma: no cover - assertion path
        raise AssertionError(exc.read().decode("utf-8")) from exc


def _free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _go_admin(repo_root: Path, dsn: str, command: str, *args: str) -> dict[str, Any]:
    helper_dir = Path(tempfile.mkdtemp(prefix="task11-admin-", dir=repo_root / "server"))
    try:
        main_file = helper_dir / "main.go"
        main_file.write_text(
            """
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/treehorn/stash-recommendations/server/internal/model"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("command is required")
	}
	ctx := context.Background()
	repository, err := store.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer repository.Close(ctx)
	if err := repository.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	command := os.Args[1]
	switch command {
	case "create-schema":
		if len(os.Args) != 3 {
			log.Fatal("schema name is required")
		}
		_, err = repository.Pool().Exec(ctx, "CREATE SCHEMA "+os.Args[2])
		if err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "ok"})
	case "drop-schema":
		if len(os.Args) != 3 {
			log.Fatal("schema name is required")
		}
		_, err = repository.Pool().Exec(ctx, "DROP SCHEMA IF EXISTS "+os.Args[2]+" CASCADE")
		if err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "ok"})
	case "issue-key":
		account, err := repository.CreateAccount(ctx)
		if err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{
			"account_id": account.ID,
			"api_key": account.PlaintextKey,
		})
	case "build-model":
		version, err := model.NewBuilder(model.NewRepository(repository.Pool()), model.DefaultOWeight).BuildAndActivate(ctx)
		if err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"model_version": version})
	case "read-snapshots":
		rows, err := repository.Pool().Query(ctx, "SELECT snapshot::text FROM source_snapshots ORDER BY endpoint, stash_id")
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()
		payloads := ""
		first := true
		for rows.Next() {
			var payload string
			if err := rows.Scan(&payload); err != nil {
				log.Fatal(err)
			}
			if !first {
				payloads += "\\n"
			}
			first = false
			payloads += payload
		}
		if err := rows.Err(); err != nil {
			log.Fatal(err)
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"payloads": payloads})
	default:
		log.Fatal(fmt.Sprintf("unsupported command: %s", command))
	}
}
""".strip()
        )
        result = subprocess.run(
            ["go", "run", f"./{helper_dir.relative_to(repo_root)}", command, *args],
            cwd=repo_root,
            env={**os.environ, "DATABASE_URL": dsn},
            check=True,
            capture_output=True,
            text=True,
        )
        return json.loads(result.stdout)
    finally:
        shutil.rmtree(helper_dir, ignore_errors=True)
