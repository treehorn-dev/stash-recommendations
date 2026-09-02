from __future__ import annotations

from dataclasses import dataclass
import json
import time
from typing import Any, Callable, Mapping
from urllib import error, request

from rec_plugin.contracts import ContentKey


Transport = Callable[[str, str, str, dict[str, object]], dict[str, object]]
HTTP_TIMEOUT_SECONDS = 15.0

SCENE_QUERY = """
query FindScene($id: ID!) {
  findScene(id: $id) {
    id
    title
    details
    date
    release_date
    production_date
    urls {
      url
    }
    duration
    director
    code
    studio {
      id
      name
    }
    tags {
      id
      name
    }
    images {
      url
    }
    performers {
      as
      performer {
        id
        name
        aliases
        gender
        urls {
          url
        }
        ethnicity
        country
        eye_color
        hair_color
        cup_size
        band_size
        waist_size
        hip_size
        career_start_year
        career_end_year
        images {
          url
        }
      }
    }
    updated
  }
}
"""


@dataclass(frozen=True)
class SourceCredentials:
    endpoint: str
    api_key: str
    max_requests_per_minute: int


class SourceClient:
    def __init__(
        self,
        configured_sources: list[dict[str, Any]] | Mapping[str, str],
        *,
        transport: Transport | None = None,
        monotonic: Callable[[], float] | None = None,
        sleep: Callable[[float], None] | None = None,
    ) -> None:
        self._transport = transport or _default_transport
        self._monotonic = monotonic or time.monotonic
        self._sleep = sleep or time.sleep
        self._next_allowed_at: dict[str, float] = {}
        self._sources = _normalize_sources(configured_sources)

    def credentials_for(self, endpoint: str) -> SourceCredentials | None:
        try:
            normalized = ContentKey.normalize(endpoint, "_").endpoint
        except ValueError:
            return None
        return self._sources.get(normalized)

    def fetch_scene(self, endpoint: str, stash_id: str) -> dict[str, Any] | None:
        credentials = self.credentials_for(endpoint)
        if credentials is None:
            return None
        self._wait_for_slot(credentials)
        response = self._transport(credentials.endpoint, credentials.api_key, SCENE_QUERY, {"id": stash_id})
        if "errors" in response:
            raise ValueError(response["errors"][0]["message"])
        scene = dict(response.get("data", {})).get("findScene")
        if scene is None:
            return None
        return dict(scene)

    def _wait_for_slot(self, credentials: SourceCredentials) -> None:
        if credentials.max_requests_per_minute <= 0:
            return
        now = self._monotonic()
        next_allowed_at = self._next_allowed_at.get(credentials.endpoint, 0.0)
        if next_allowed_at > now:
            self._sleep(next_allowed_at - now)
            now = next_allowed_at
        self._next_allowed_at[credentials.endpoint] = now + (60.0 / credentials.max_requests_per_minute)


def _normalize_sources(configured_sources: list[dict[str, Any]] | Mapping[str, str]) -> dict[str, SourceCredentials]:
    if isinstance(configured_sources, Mapping):
        iterable = [
            {"endpoint": endpoint, "api_key": api_key, "max_requests_per_minute": 60}
            for endpoint, api_key in configured_sources.items()
        ]
    else:
        iterable = configured_sources
    sources: dict[str, SourceCredentials] = {}
    for source in iterable:
        api_key = str(source.get("api_key", "")).strip()
        if not api_key:
            continue
        normalized = ContentKey.normalize(str(source["endpoint"]), "_").endpoint
        limit = int(source.get("max_requests_per_minute") or 60)
        sources[normalized] = SourceCredentials(
            endpoint=normalized,
            api_key=api_key,
            max_requests_per_minute=limit,
        )
    return sources


def _default_transport(url: str, api_key: str, query: str, variables: dict[str, object]) -> dict[str, object]:
    payload = json.dumps({"query": query, "variables": variables}).encode("utf-8")
    http_request = request.Request(
        url,
        data=payload,
        headers={"Content-Type": "application/json", "ApiKey": api_key},
        method="POST",
    )
    try:
        with request.urlopen(http_request, timeout=HTTP_TIMEOUT_SECONDS) as response:
            return json.loads(response.read().decode("utf-8"))
    except error.URLError as exc:
        raise OSError(str(exc.reason)) from exc
