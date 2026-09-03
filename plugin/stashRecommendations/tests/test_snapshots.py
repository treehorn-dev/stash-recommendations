from __future__ import annotations

from datetime import datetime, timezone

import pytest

from rec_plugin.snapshots import to_source_snapshot


CAPTURED_AT = datetime(2026, 8, 30, 12, 0, tzinfo=timezone.utc)


def test_to_source_snapshot_maps_stash_box_scene_without_local_only_fields() -> None:
    snapshot = to_source_snapshot(
        "https://box.example/graphql",
        CAPTURED_AT,
        {
            "id": "scene-1",
            "title": "Example Scene",
            "details": "Details",
            "release_date": "2026-08-30",
            "production_date": "2026-08-29",
            "urls": ["https://example.test/scenes/scene-1"],
            "duration": 120,
            "director": "Director",
            "code": "CODE-1",
            "updated": "2026-08-31T00:00:00Z",
            "studio": {"id": "studio-1", "name": "Studio"},
            "tags": [{"id": "tag-1", "name": "Tag"}],
            "groups": [{"group": {"id": "group-1", "name": "Group"}}],
            "images": [{"url": "https://images.example/scene.jpg"}],
            "performers": [
                {
                    "as": "Alias",
                    "performer": {
                        "id": "performer-1",
                        "name": "Performer",
                        "aliases": ["Alias"],
                        "gender": "FEMALE",
                        "country": "US",
                        "ethnicity": "ASIAN",
                        "eye_color": "BLUE",
                        "hair_color": "BLACK",
                        "cup_size": "C",
                        "band_size": 34,
                        "waist_size": 24,
                        "hip_size": 35,
                        "career_start_year": 2020,
                        "career_end_year": 2026,
                        "urls": ["https://example.test/performers/performer-1"],
                        "images": [{"url": "https://images.example/performer.jpg"}],
                    },
                }
            ],
            "paths": {"screenshot": "/private/path.jpg"},
            "play_count": 99,
            "custom_fields": {"private": True},
        },
    )

    payload = snapshot.to_dict()

    assert payload["source_updated_at"] == "2026-08-31T00:00:00Z"
    assert payload["scenes"] == [
        {
            "id": "scene-1",
            "title": "Example Scene",
            "details": "Details",
            "dates": ["2026-08-30", "2026-08-29"],
            "urls": ["https://example.test/scenes/scene-1"],
            "duration": 120,
            "director": "Director",
            "code": "CODE-1",
            "studio": {"id": "studio-1", "name": "Studio"},
            "tags": [{"id": "tag-1", "name": "Tag"}],
            "groups": [{"id": "group-1", "name": "Group"}],
            "performer_appearances": [{"performer_id": "performer-1"}],
            "remote_images": ["https://images.example/scene.jpg"],
        }
    ]
    assert payload["performers"] == [
        {
            "id": "performer-1",
            "name": "Performer",
            "aliases": ["Alias"],
            "gender": "female",
            "country": "US",
            "ethnicity": "asian",
            "eye_color": "blue",
            "hair_color": "black",
            "measurements": "34C-24-35",
            "career_years": [2020, 2026],
            "urls": ["https://example.test/performers/performer-1"],
            "remote_images": ["https://images.example/performer.jpg"],
        }
    ]


def test_to_source_snapshot_requires_source_updated_at() -> None:
    with pytest.raises(ValueError, match="source_updated_at"):
        to_source_snapshot(
            "https://box.example/graphql",
            CAPTURED_AT,
            {
                "id": "scene-1",
                "performers": [],
            },
        )


def test_to_source_snapshot_drops_invalid_optional_urls_and_dates() -> None:
    snapshot = to_source_snapshot(
        "https://box.example/graphql",
        CAPTURED_AT,
        {
            "id": "scene-1",
            "updated": "2026-08-31T00:00:00Z",
            "release_date": "not-a-date",
            "urls": ["http://example.test/scene", "https://example.test/scene"],
            "images": [{"url": "https://images.example/scene.jpg?tracking=1"}],
            "performers": [
                {
                    "performer": {
                        "id": "performer-1",
                        "name": "Performer",
                        "urls": ["ftp://example.test/profile", "https://example.test/profile"],
                        "images": [{"url": "https://images.example/performer.jpg#fragment"}],
                    }
                }
            ],
        },
    )

    assert snapshot.to_dict() == {
        "schema_version": 1,
        "content_key": {"endpoint": "https://box.example/graphql", "stash_id": "scene-1"},
        "captured_at": "2026-08-30T12:00:00Z",
        "source_updated_at": "2026-08-31T00:00:00Z",
        "scenes": [
            {
                "id": "scene-1",
                "urls": ["https://example.test/scene"],
                "performer_appearances": [{"performer_id": "performer-1"}],
            }
        ],
        "performers": [
            {
                "id": "performer-1",
                "name": "Performer",
                "urls": ["https://example.test/profile"],
            }
        ],
    }


def test_to_source_snapshot_normalizes_partial_source_dates_to_the_first_day() -> None:
    snapshot = to_source_snapshot(
        "https://box.example/graphql",
        CAPTURED_AT,
        {
            "id": "scene-1",
            "updated": "2026-08-31T00:00:00Z",
            "release_date": "2006",
            "production_date": "2007-04",
            "date": "2008-02-03",
            "performers": [],
        },
    )

    assert snapshot.to_dict()["scenes"][0]["dates"] == [
        "2006-01-01",
        "2007-04-01",
        "2008-02-03",
    ]
