from __future__ import annotations

from dataclasses import dataclass
import hashlib
import ipaddress
from typing import Any
from urllib.parse import urlparse, urlunparse


PRIVATE_HTTP_NETWORKS = (
    ipaddress.ip_network("10.0.0.0/8"),
    ipaddress.ip_network("172.16.0.0/12"),
    ipaddress.ip_network("192.168.0.0/16"),
    ipaddress.ip_network("100.64.0.0/10"),
    ipaddress.ip_network("fd7a:115c:a1e0::/48"),
)


@dataclass(frozen=True)
class Settings:
    service_url: str
    api_key: str
    show_remote_results: bool
    allow_private_http: bool = False

    @classmethod
    def from_plugin_config(cls, config: dict[str, Any] | None) -> "Settings":
        data = config or {}
        allow_private_http = data.get("allow_private_http", False)
        if type(allow_private_http) is not bool:
            raise ValueError("allow_private_http must be a boolean")
        service_url = (
            _normalize_service_url(str(data.get("service_url", "")).strip(), allow_private_http)
            if data.get("service_url")
            else ""
        )
        api_key = str(data.get("api_key", "")).strip()
        show_remote_results = data.get("show_remote_results", False)
        if type(show_remote_results) is not bool:
            raise ValueError("show_remote_results must be a boolean")
        return cls(
            service_url=service_url,
            api_key=api_key,
            show_remote_results=show_remote_results,
            allow_private_http=allow_private_http,
        )

    def to_status(self) -> dict[str, Any]:
        return {
            "service_url": self.service_url,
            "api_key_configured": bool(self.api_key),
            "show_remote_results": self.show_remote_results,
            "allow_private_http": self.allow_private_http,
        }

    def delivery_pause_key(self) -> str:
        material = f"{self.service_url}\0{self.api_key}".encode("utf-8")
        return hashlib.sha256(material).hexdigest()


def _normalize_service_url(value: str, allow_private_http: bool) -> str:
    parsed = urlparse(value)
    scheme = parsed.scheme.lower()
    if not parsed.netloc or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError("service_url must be a public HTTPS URL")
    if scheme == "http" and allow_private_http:
        if not _is_private_http_host(parsed.hostname):
            raise ValueError("service_url must use a private or Tailnet IP address when HTTP is enabled")
    elif scheme != "https":
        raise ValueError("service_url must be a public HTTPS URL")
    path = parsed.path[:-1] if parsed.path.endswith("/") and parsed.path != "/" else parsed.path
    return urlunparse((scheme, parsed.netloc.lower(), path, "", "", ""))


def _is_private_http_host(host: str | None) -> bool:
    if not host:
        return False
    try:
        address = ipaddress.ip_address(host)
    except ValueError:
        return False
    return address.is_loopback or any(address in network for network in PRIVATE_HTTP_NETWORKS)
