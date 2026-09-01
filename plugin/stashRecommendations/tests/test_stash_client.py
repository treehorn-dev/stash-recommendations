from __future__ import annotations

from datetime import datetime, timezone

from rec_plugin.contracts import ContentKey
from rec_plugin.stash_client import StashClient


def test_find_scene_uses_local_graphql_and_session_cookie() -> None:
    calls: list[tuple[str, str | None, str, dict[str, object]]] = []

    def transport(url: str, cookie: str | None, query: str, variables: dict[str, object]) -> dict[str, object]:
        calls.append((url, cookie, query, variables))
        return {
            "data": {
                "findScene": {
                    "id": "44",
                    "title": "Scene 44",
                    "rating100": 75,
                    "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-44"}],
                }
            }
        }

    client = StashClient(
        {"Scheme": "https", "Host": "stash.local", "Port": 9999, "SessionCookie": {"Name": "session", "Value": "abc123"}},
        transport=transport,
    )

    scene = client.find_scene(44)

    assert scene["id"] == "44"
    assert scene["rating100"] == 75
    assert calls == [
        (
            "https://stash.local:9999/graphql",
            "session=abc123",
            calls[0][2],
            {"id": "44"},
        )
    ]
    assert "findScene" in calls[0][2]
    assert "rating100" in calls[0][2]


def test_find_scene_returns_none_when_graphql_scene_is_missing() -> None:
    def transport(url: str, cookie: str | None, query: str, variables: dict[str, object]) -> dict[str, object]:
        del url, cookie, query, variables
        return {"data": {"findScene": None}}

    client = StashClient({"Scheme": "http", "Host": "127.0.0.1", "Port": 9999}, transport=transport)

    scene = client.find_scene(44)

    assert scene is None


def test_iter_rated_scenes_paginates_across_all_pages() -> None:
    calls: list[dict[str, object]] = []
    pages = {
        1: {
            "count": 3,
            "scenes": [
                {"id": "1", "rating100": 80, "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-1"}]},
                {"id": "2", "rating100": 60, "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-2"}]},
            ],
        },
        2: {
            "count": 3,
            "scenes": [
                {"id": "3", "rating100": 40, "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-3"}]},
            ],
        },
    }

    def transport(url: str, cookie: str | None, query: str, variables: dict[str, object]) -> dict[str, object]:
        del url, cookie
        calls.append(variables)
        return {"data": {"findScenes": pages[variables["page"]]}}

    client = StashClient({"Scheme": "http", "Host": "127.0.0.1", "Port": 9999}, transport=transport, page_size=2)

    scenes = list(client.iter_rated_scenes())

    assert [scene["id"] for scene in scenes] == ["1", "2", "3"]
    assert calls == [{"page": 1, "per_page": 2}, {"page": 2, "per_page": 2}]


def test_iter_engagement_history_returns_recorded_play_and_o_timestamps() -> None:
    def transport(url: str, cookie: str | None, query: str, variables: dict[str, object]) -> dict[str, object]:
        del url, cookie, query, variables
        return {
            "data": {
                "findScenes": {
                    "count": 1,
                    "scenes": [
                        {
                            "id": "44",
                            "stash_ids": [{"endpoint": "https://box.example/graphql", "stash_id": "scene-44"}],
                            "play_history": ["2026-08-30T12:00:00Z"],
                            "o_history": ["2026-08-30T12:30:00Z"],
                        }
                    ],
                }
            }
        }

    client = StashClient({"Scheme": "http", "Host": "127.0.0.1", "Port": 9999}, transport=transport)

    history = list(client.iter_engagement_history())

    assert history == [
        {
            "id": "44",
            "stash_ids": [ContentKey.normalize("https://box.example/graphql", "scene-44")],
            "play_history": [datetime(2026, 8, 30, 12, 0, tzinfo=timezone.utc)],
            "o_history": [datetime(2026, 8, 30, 12, 30, tzinfo=timezone.utc)],
        }
    ]


def test_configured_stash_boxes_reads_local_stash_configuration() -> None:
    def transport(url: str, cookie: str | None, query: str, variables: dict[str, object]) -> dict[str, object]:
        del url, cookie, variables
        assert "configuration" in query
        return {
            "data": {
                "configuration": {
                    "general": {
                        "stashBoxes": [
                            {
                                "endpoint": "HTTPS://BOX.EXAMPLE/GRAPHQL/",
                                "api_key": "source-key",
                                "name": "Primary Box",
                                "max_requests_per_minute": 30,
                            }
                        ]
                    },
                    "plugins": {
                        "stashRecommendations": {
                            "service_url": "https://rec.example/v1",
                            "api_key": "client-key",
                            "show_remote_results": False,
                        }
                    },
                }
            }
        }

    client = StashClient({"Scheme": "http", "Host": "127.0.0.1", "Port": 9999}, transport=transport)

    boxes = client.configured_stash_boxes()
    plugin_config = client.plugin_config("stashRecommendations")

    assert boxes == [
        {
            "endpoint": "https://box.example/GRAPHQL",
            "api_key": "source-key",
            "name": "Primary Box",
            "max_requests_per_minute": 30,
        }
    ]
    assert plugin_config == {
        "service_url": "https://rec.example/v1",
        "api_key": "client-key",
        "show_remote_results": False,
    }
