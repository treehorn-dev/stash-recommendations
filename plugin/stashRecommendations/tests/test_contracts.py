import pytest

from datetime import datetime, timezone

from rec_plugin.contracts import ContentKey, PreferenceEvent, SourceSnapshot


def test_rating_remove_serializes_without_rating() -> None:
    event = PreferenceEvent(
        schema_version=1,
        event_id="550e8400-e29b-41d4-a716-446655440000",
        client_id="550e8400-e29b-41d4-a716-446655440001",
        sequence=7,
        occurred_at=datetime.now(timezone.utc),
        content_key=ContentKey.normalize("HTTPS://BOX.EXAMPLE/GRAPHQL/", "scene-1"),
        kind="scene.rating.remove",
        origin="hook",
    )

    payload = event.to_dict()

    assert "rating" not in payload


@pytest.mark.parametrize("field", ["paths", "files", "rating100", "play_count", "custom_fields"])
def test_snapshot_construction_rejects_privacy_fields(field: str) -> None:
    with pytest.raises(ValueError, match=field):
        SourceSnapshot(
            schema_version=1,
            content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
            captured_at=datetime.now(timezone.utc),
            scenes=[{"id": "scene-1", field: "private"}],
            performers=[],
        )


def test_snapshot_construction_requires_arrays_and_nested_required_fields() -> None:
    with pytest.raises(ValueError, match="scenes"):
        SourceSnapshot(
            schema_version=1,
            content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
            captured_at=datetime.now(timezone.utc),
            scenes=None,
            performers=[],
        )

    with pytest.raises(ValueError, match="scene id"):
        SourceSnapshot(
            schema_version=1,
            content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
            captured_at=datetime.now(timezone.utc),
            scenes=[{}],
            performers=[{"id": "performer-1"}],
        )


def test_snapshot_serializes_schema_valid_arrays() -> None:
    snapshot = SourceSnapshot(
        schema_version=1,
        content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
        captured_at=datetime.now(timezone.utc),
        scenes=[{"id": "scene-1"}],
        performers=[{"id": "performer-1", "name": "Performer"}],
    )

    payload = snapshot.to_dict()

    assert isinstance(payload["scenes"], list)
    assert isinstance(payload["performers"], list)


@pytest.mark.parametrize(
    ("field", "value"),
    [("schema_version", True), ("sequence", 1.5), ("event_id", "550e8400e29b41d4a716446655440000")],
)
def test_preference_event_rejects_noncanonical_parity_values(field: str, value: object) -> None:
    values = {
        "schema_version": 1,
        "event_id": "550e8400-e29b-41d4-a716-446655440000",
        "client_id": "550e8400-e29b-41d4-a716-446655440001",
        "sequence": 7,
    }
    values[field] = value
    event = PreferenceEvent(
        **values,
        occurred_at=datetime.now(timezone.utc),
        content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
        kind="scene.rating.remove",
        origin="hook",
    )

    with pytest.raises(ValueError):
        event.validate()
