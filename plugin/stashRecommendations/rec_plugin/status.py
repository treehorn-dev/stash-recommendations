from __future__ import annotations

from typing import Any

from rec_plugin.settings import Settings


def build_status_output(settings: Settings, outbox_status: dict[str, Any]) -> dict[str, Any]:
    return {
        "configured": bool(settings.service_url and settings.api_key),
        "settings": settings.to_status(),
        "outbox": outbox_status,
    }
