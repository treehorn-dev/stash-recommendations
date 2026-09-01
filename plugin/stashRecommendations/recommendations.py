from __future__ import annotations

import json
from pathlib import Path
import sys
from typing import Any

from rec_plugin.capture import handle_scene_update
from rec_plugin.outbox import Outbox
from rec_plugin.source_client import SourceClient
from rec_plugin.settings import Settings
from rec_plugin.stash_client import StashClient
from rec_plugin.sync import SyncState, queue_engagement_sync, queue_metadata_sync, queue_rating_sync


PLUGIN_ID = "stashRecommendations"


def main() -> None:
    output: dict[str, Any] = {}
    try:
        run(read_json_input(), output)
    except Exception as error:  # pragma: no cover - raw plugin boundary
        output["error"] = str(error)
    print(json.dumps(output))


def read_json_input() -> dict[str, Any]:
    return json.loads(sys.stdin.read())


def run(plugin_input: dict[str, Any], output: dict[str, Any]) -> None:
    args = dict(plugin_input.get("args", {}))
    server_connection = dict(plugin_input.get("server_connection", {}))
    mode = str(args.get("mode", "status"))
    database_path = _plugin_dir(server_connection) / "recommendations.sqlite3"
    outbox = Outbox(database_path)
    stash = StashClient(server_connection)
    state = SyncState(database_path)

    if mode == "capture-rating":
        output["output"] = {
            "queued": handle_scene_update(dict(args.get("hookContext", {})), stash, outbox, state),
            "kind": "rating",
        }
        return
    if mode == "status":
        plugin_config = stash.plugin_config(PLUGIN_ID)
        settings = Settings.from_plugin_config(plugin_config)
        output["output"] = {
            "settings": settings.to_status(),
            "outbox": outbox.status(),
        }
        return
    if mode == "sync-ratings":
        output["output"] = queue_rating_sync(stash, outbox, state, confirmed=bool(args.get("confirmed", False)))
        return
    if mode == "sync-engagement":
        output["output"] = queue_engagement_sync(stash, outbox, state, confirmed=bool(args.get("confirmed", False)))
        return
    if mode == "sync-metadata":
        output["output"] = queue_metadata_sync(
            stash,
            outbox,
            SourceClient(stash.configured_stash_boxes()),
            confirmed=bool(args.get("confirmed", False)),
        )
        return
    if mode == "deliver-outbox":
        output["output"] = {"queued": 0, "kind": "task", "mode": mode}
        return
    raise ValueError(f"unsupported mode: {mode}")


def _plugin_dir(server_connection: dict[str, Any]) -> Path:
    return Path(server_connection.get("PluginDir") or Path(__file__).resolve().parent)


if __name__ == "__main__":  # pragma: no cover - raw plugin boundary
    main()
