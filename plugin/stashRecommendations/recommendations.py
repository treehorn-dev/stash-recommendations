from __future__ import annotations

import json
from pathlib import Path
import sys
from typing import Any

from rec_plugin.outbox import Outbox
from rec_plugin.settings import Settings
from rec_plugin.stash_client import StashClient


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
    outbox = Outbox(_plugin_dir(server_connection) / "recommendations.sqlite3")
    stash = StashClient(server_connection)

    if mode == "capture-rating":
        outbox.enqueue_hook("capture-rating", dict(args.get("hookContext", {})))
        output["output"] = {"queued": 1, "kind": "hook"}
        return
    if mode == "status":
        plugin_config = stash.plugin_config(PLUGIN_ID)
        settings = Settings.from_plugin_config(plugin_config)
        output["output"] = {
            "settings": settings.to_status(),
            "outbox": outbox.status(),
        }
        return
    if mode in {"sync-ratings", "sync-engagement", "sync-metadata", "deliver-outbox"}:
        output["output"] = {"queued": 0, "kind": "task", "mode": mode}
        return
    raise ValueError(f"unsupported mode: {mode}")


def _plugin_dir(server_connection: dict[str, Any]) -> Path:
    return Path(server_connection.get("PluginDir") or Path(__file__).resolve().parent)


if __name__ == "__main__":  # pragma: no cover - raw plugin boundary
    main()
