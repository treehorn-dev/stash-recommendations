from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from rec_plugin.outbox import Outbox
from rec_plugin.sync import SyncState, build_rating_events


def handle_scene_update(
    hook_context: Mapping[str, Any],
    stash: Any,
    outbox: Outbox,
    state: SyncState,
) -> int:
    input_fields = hook_context.get("inputFields") or []
    if "rating100" not in input_fields:
        return 0
    scene_id = hook_context.get("id")
    if scene_id is None:
        return 0
    scene = stash.find_scene(scene_id)
    if scene is None:
        return 0
    count = 0
    for event in build_rating_events(scene, state, origin="hook"):
        outbox.enqueue(event)
        count += 1
    return count
