from __future__ import annotations

from dataclasses import dataclass
import hashlib
from typing import Any
from urllib.parse import urlparse, urlunparse


@dataclass(frozen=True)
class Settings:
    service_url: str
    api_key: str
    show_remote_results: bool

    @classmethod
    def from_plugin_config(cls, config: dict[str, Any] | None) -> "Settings":
        data = config or {}
        service_url = _normalize_service_url(str(data.get("service_url", "")).strip()) if data.get("service_url") else ""
        api_key = str(data.get("api_key", "")).strip()
        show_remote_results = data.get("show_remote_results", False)
        if type(show_remote_results) is not bool:
            raise ValueError("show_remote_results must be a boolean")
        return cls(
            service_url=service_url,
            api_key=api_key,
            show_remote_results=show_remote_results,
        )

    def to_status(self) -> dict[str, Any]:
        return {
            "service_url": self.service_url,
            "api_key_configured": bool(self.api_key),
            "show_remote_results": self.show_remote_results,
        }

    def delivery_pause_key(self) -> str:
        material = f"{self.service_url}\0{self.api_key}".encode("utf-8")
        return hashlib.sha256(material).hexdigest()


def _normalize_service_url(value: str) -> str:
    parsed = urlparse(value)
    if parsed.scheme.lower() != "https" or not parsed.netloc or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError("service_url must be a public HTTPS URL")
    path = parsed.path[:-1] if parsed.path.endswith("/") and parsed.path != "/" else parsed.path
    return urlunparse(("https", parsed.netloc.lower(), path, "", "", ""))
