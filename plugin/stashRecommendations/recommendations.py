from __future__ import annotations

import json
from pathlib import Path
import sys
from typing import Any

from rec_plugin.contracts import ContentKey
from rec_plugin.capture import handle_scene_update
from rec_plugin.delivery import DeliveryWorker
from rec_plugin.outbox import Outbox
from rec_plugin.metadata_jobs import MetadataJobs
from rec_plugin.service_client import ServiceClient
from rec_plugin.source_client import SourceClient
from rec_plugin.settings import Settings
from rec_plugin.stash_client import StashClient
from rec_plugin.status import build_status_output
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
    _task_log(f"task mode={mode}")
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
        outbox.sync_delivery_pause(settings.delivery_pause_key())
        output["output"] = _status_output(settings, outbox)
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
        plugin_config = stash.plugin_config(PLUGIN_ID)
        settings = Settings.from_plugin_config(plugin_config)
        outbox.sync_delivery_pause(settings.delivery_pause_key())
        _task_log(f"delivery start pending={outbox.status()['pending']}")
        if not settings.service_url or not settings.api_key:
            _task_log("delivery skipped because service URL or API key is not configured")
            output["output"] = _status_output(settings, outbox)
            return
        delivery = DeliveryWorker(
            outbox,
            ServiceClient(settings),
            pause_key=settings.delivery_pause_key(),
        ).deliver_ready(_utcnow())
        payload = _status_output(settings, outbox)
        payload["delivery"] = {
            "delivered": delivery.delivered,
            "retried": delivery.retried,
            "quarantined": delivery.quarantined,
            "paused": delivery.paused,
        }
        _task_log(
            "delivery finished "
            f"delivered={delivery.delivered} retried={delivery.retried} "
            f"quarantined={delivery.quarantined} paused={delivery.paused}"
        )
        output["output"] = payload
        return
    if mode == "fetch-related":
        plugin_config = stash.plugin_config(PLUGIN_ID)
        settings = Settings.from_plugin_config(plugin_config)
        output["output"] = _fetch_related(
            ServiceClient(settings),
            list(args.get("content_keys", [])),
            int(args.get("limit", 20)),
        )
        return
    if mode == "fetch-for-you":
        plugin_config = stash.plugin_config(PLUGIN_ID)
        settings = Settings.from_plugin_config(plugin_config)
        if not settings.service_url or not settings.api_key:
            output["output"] = {"model_version": "", "items": []}
            return
        output["output"] = ServiceClient(settings).fetch_for_you(
            int(args.get("limit", 20)),
            offset=int(args.get("offset", 0)),
            filters=args.get("filters") if isinstance(args.get("filters"), dict) else None,
        )
        return
    raise ValueError(f"unsupported mode: {mode}")


def _plugin_dir(server_connection: dict[str, Any]) -> Path:
    return Path(server_connection.get("PluginDir") or Path(__file__).resolve().parent)


def _status_output(settings: Settings, outbox: Outbox) -> dict[str, Any]:
    jobs = MetadataJobs(outbox.path)
    payload = build_status_output(settings, outbox.status())
    payload["metadata"] = {
        "jobs": jobs.status(),
        "diagnostics": jobs.diagnostics(),
    }
    return payload


def _utcnow():
    from datetime import datetime, timezone

    return datetime.now(timezone.utc)


def _task_log(message: str) -> None:
    print(f"[Stash Recommendations] {message}", file=sys.stderr, flush=True)


def _fetch_related(service: ServiceClient, content_keys: list[dict[str, Any]], limit: int) -> dict[str, Any]:
    if not content_keys:
        return {"model_version": "", "items": []}
    merged: dict[tuple[str, str], dict[str, Any]] = {}
    model_version = ""
    for item in content_keys:
        key = ContentKey.normalize(str(item.get("endpoint", "")), str(item.get("stash_id", "")))
        response = service.fetch_related([{"endpoint": key.endpoint, "stash_id": key.stash_id}], limit)
        if not model_version:
            model_version = str(response.get("model_version", ""))
        for candidate in response.get("items", []):
            content_key = dict(candidate.get("content_key", {}))
            marker = (
                str(content_key.get("endpoint", "")),
                str(content_key.get("stash_id", "")),
            )
            if marker not in merged or float(candidate.get("score", 0.0)) > float(merged[marker].get("score", 0.0)):
                merged[marker] = dict(candidate)
    items = sorted(merged.values(), key=lambda item: float(item.get("score", 0.0)), reverse=True)[:limit]
    return {"model_version": model_version, "items": items}


if __name__ == "__main__":  # pragma: no cover - raw plugin boundary
    main()
