from __future__ import annotations

from datetime import datetime, timezone
import json
from typing import Any, Callable, Iterator, Mapping
from urllib import request

from rec_plugin.contracts import ContentKey


Transport = Callable[[str, str | None, str, dict[str, object]], dict[str, object]]


class StashClient:
    def __init__(
        self,
        server_connection: dict[str, Any],
        *,
        transport: Transport | None = None,
        page_size: int = 100,
    ) -> None:
        self._server_connection = server_connection
        self._transport = transport or _default_transport
        self._page_size = page_size
        self._config_cache: dict[str, Any] | None = None

    def find_scene(self, scene_id: int | str) -> dict[str, Any] | None:
        response = self._execute(
            """
            query FindScene($id: ID!) {
              findScene(id: $id) {
                id
                title
                rating100
                stash_ids {
                  endpoint
                  stash_id
                }
              }
            }
            """,
            {"id": str(scene_id)},
        )
        scene = response["findScene"]
        if scene is None:
            return None
        return dict(_require_mapping(scene))

    def iter_rated_scenes(self) -> Iterator[dict[str, Any]]:
        page = 1
        yielded = 0
        total = None
        while total is None or yielded < total:
            response = self._execute(
                """
                query FindRatedScenes($page: Int!, $per_page: Int!) {
                  findScenes(
                    scene_filter: { rating100: { value: 0, modifier: GREATER_THAN } }
                    filter: { page: $page, per_page: $per_page }
                  ) {
                    count
                    scenes {
                      id
                      rating100
                      stash_ids {
                        endpoint
                        stash_id
                      }
                    }
                  }
                }
                """,
                {"page": page, "per_page": self._page_size},
            )["findScenes"]
            total = int(response["count"])
            scenes = list(response["scenes"])
            if not scenes:
                return
            for scene in scenes:
                yielded += 1
                yield dict(scene)
            page += 1

    def iter_engagement_history(self) -> Iterator[dict[str, Any]]:
        page = 1
        yielded = 0
        total = None
        while total is None or yielded < total:
            response = self._execute(
                """
                query FindEngagementScenes($page: Int!, $per_page: Int!) {
                  findScenes(filter: { page: $page, per_page: $per_page }) {
                    count
                    scenes {
                      id
                      stash_ids {
                        endpoint
                        stash_id
                      }
                      play_history
                      o_history
                    }
                  }
                }
                """,
                {"page": page, "per_page": self._page_size},
            )["findScenes"]
            total = int(response["count"])
            scenes = list(response["scenes"])
            if not scenes:
                return
            for scene in scenes:
                yielded += 1
                yield {
                    "id": scene["id"],
                    "stash_ids": list(self.iter_scene_stash_ids(scene)),
                    "play_history": [_parse_utc_timestamp(value) for value in scene.get("play_history", [])],
                    "o_history": [_parse_utc_timestamp(value) for value in scene.get("o_history", [])],
                }
            page += 1

    def iter_scene_stash_ids(self, scene: dict[str, Any]) -> Iterator[ContentKey]:
        for entry in scene.get("stash_ids", []) or []:
            yield ContentKey.normalize(entry["endpoint"], entry["stash_id"])

    def configured_stash_boxes(self) -> list[dict[str, Any]]:
        general = self._configuration()["general"]
        boxes = []
        for box in general.get("stashBoxes", []):
            boxes.append(
                {
                    "endpoint": _normalize_endpoint(box["endpoint"]),
                    "api_key": box.get("api_key", ""),
                    "name": box.get("name", ""),
                    "max_requests_per_minute": box.get("max_requests_per_minute"),
                }
            )
        return boxes

    def plugin_config(self, plugin_id: str) -> dict[str, Any]:
        plugins = self._configuration().get("plugins", {})
        config = plugins.get(plugin_id, {})
        if not isinstance(config, dict):
            return {}
        return dict(config)

    def _configuration(self) -> dict[str, Any]:
        if self._config_cache is None:
            self._config_cache = self._execute(
                """
                query Configuration {
                  configuration {
                    general {
                      stashBoxes {
                        endpoint
                        api_key
                        name
                        max_requests_per_minute
                      }
                    }
                    plugins
                  }
                }
                """,
                {},
            )["configuration"]
        return self._config_cache

    def _execute(self, query: str, variables: dict[str, object]) -> dict[str, Any]:
        response = self._transport(self.graphql_url, self.session_cookie, query, variables)
        if "errors" in response:
            raise ValueError(response["errors"][0]["message"])
        return dict(response["data"])

    @property
    def graphql_url(self) -> str:
        scheme = self._server_connection.get("Scheme", "http")
        host = self._server_connection.get("Host", "127.0.0.1")
        port = self._server_connection.get("Port", 9999)
        return f"{scheme}://{host}:{port}/graphql"

    @property
    def session_cookie(self) -> str | None:
        cookie = self._server_connection.get("SessionCookie")
        if not cookie:
            return None
        name = cookie.get("Name")
        value = cookie.get("Value")
        if not name or not value:
            return None
        return f"{name}={value}"


def _default_transport(url: str, cookie: str | None, query: str, variables: dict[str, object]) -> dict[str, object]:
    payload = json.dumps({"query": query, "variables": variables}).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    if cookie:
        headers["Cookie"] = cookie
    http_request = request.Request(url, data=payload, headers=headers, method="POST")
    with request.urlopen(http_request) as response:
        return json.loads(response.read().decode("utf-8"))


def _normalize_endpoint(endpoint: str) -> str:
    return ContentKey.normalize(endpoint, "_").endpoint


def _parse_utc_timestamp(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)


def _require_mapping(value: object) -> Mapping[str, Any]:
    if not isinstance(value, Mapping):
        raise TypeError(f"expected GraphQL object response, got {type(value)!r}")
    return value
