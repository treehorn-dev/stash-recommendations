from datetime import datetime, timezone
import json
from pathlib import Path

import pytest

from rec_plugin.contracts import ContentKey, PreferenceEvent, SourceSnapshot


def test_preference_event_schema_restricts_v1_interactions_to_utc_timestamps() -> None:
    schema_path = Path(__file__).parents[3] / "contracts" / "v1" / "preference-event.schema.json"
    schema = json.loads(schema_path.read_text())

    assert schema["properties"]["kind"]["enum"] == [
        "scene.rating.set",
        "scene.rating.remove",
        "scene.played",
        "scene.o",
    ]
    assert schema["properties"]["occurred_at"]["pattern"] == "Z$"
    assert schema["additionalProperties"] is False
    assert schema["$defs"]["content_key"]["properties"]["endpoint"]["pattern"] == r"^[Hh][Tt][Tt][Pp][Ss]://[^/?#@]+(?:/[^?#]*)?$"


def test_source_snapshot_schema_requires_public_https_references() -> None:
    schema_path = Path(__file__).parents[3] / "contracts" / "v1" / "source-snapshot.schema.json"
    schema = json.loads(schema_path.read_text())

    assert schema["$defs"]["content_key"]["properties"]["endpoint"]["pattern"] == r"^[Hh][Tt][Tt][Pp][Ss]://[^/?#@]+(?:/[^?#]*)?$"
    assert schema["$defs"]["scene"]["properties"]["urls"]["items"] == {"$ref": "#/$defs/https_reference"}
    assert schema["$defs"]["performer"]["properties"]["remote_images"]["items"] == {"$ref": "#/$defs/https_reference"}
    assert schema["$defs"]["group"]["properties"]["id"]["pattern"] == r".*\S.*"
    assert schema["$defs"]["group"]["properties"]["name"]["pattern"] == r".*\S.*"
    assert "source_updated_at" in schema["required"]


def test_source_snapshot_accepts_named_groups() -> None:
    snapshot = SourceSnapshot(
        schema_version=1,
        content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
        captured_at=datetime.now(timezone.utc),
        source_updated_at=datetime.now(timezone.utc),
        scenes=[{"id": "scene-1", "groups": [{"id": "group-1", "name": "Series"}]}],
        performers=[],
    )

    assert snapshot.to_dict()["scenes"][0]["groups"] == [{"id": "group-1", "name": "Series"}]


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
            source_updated_at=datetime.now(timezone.utc),
            scenes=[{"id": "scene-1", field: "private"}],
            performers=[],
        )


def test_snapshot_construction_requires_arrays_and_nested_required_fields() -> None:
    with pytest.raises(ValueError, match="scenes"):
        SourceSnapshot(
            schema_version=1,
            content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
            captured_at=datetime.now(timezone.utc),
            source_updated_at=datetime.now(timezone.utc),
            scenes=None,
            performers=[],
        )

    with pytest.raises(ValueError, match="scene id"):
        SourceSnapshot(
            schema_version=1,
            content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
            captured_at=datetime.now(timezone.utc),
            source_updated_at=datetime.now(timezone.utc),
            scenes=[{}],
            performers=[{"id": "performer-1"}],
        )


def test_snapshot_serializes_schema_valid_arrays() -> None:
    snapshot = SourceSnapshot(
        schema_version=1,
        content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
        captured_at=datetime.now(timezone.utc),
        source_updated_at=datetime.now(timezone.utc),
        scenes=[{"id": "scene-1"}],
        performers=[{"id": "performer-1", "name": "Performer"}],
    )

    payload = snapshot.to_dict()

    assert isinstance(payload["scenes"], list)
    assert isinstance(payload["performers"], list)
    assert payload["captured_at"].endswith("Z")
    assert payload["source_updated_at"].endswith("Z")


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


@pytest.mark.parametrize("kind", ["scene.played", "scene.o"])
def test_interaction_events_prohibit_rating(kind: str) -> None:
    event = PreferenceEvent(
        schema_version=1,
        event_id="550e8400-e29b-41d4-a716-446655440000",
        client_id="550e8400-e29b-41d4-a716-446655440001",
        sequence=7,
        occurred_at=datetime.now(timezone.utc),
        content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
        kind=kind,
        origin="hook",
    )

    event.validate()
    event.rating = 0.5
    with pytest.raises(ValueError):
        event.validate()


@pytest.mark.parametrize(
    ("scene", "performers"),
    [
        ({"id": "scene-1", "duration": True}, []),
        ({"id": "scene-1", "dates": ["not-a-date"]}, []),
        ({"id": "scene-1", "tags": None}, []),
        ({"id": "scene-1"}, [{"id": "performer-1", "name": "Performer", "career_years": [True]}]),
    ],
)
def test_snapshot_rejects_invalid_allowed_field_types(scene: dict[str, object], performers: list[dict[str, object]]) -> None:
    with pytest.raises(ValueError):
        SourceSnapshot(
            schema_version=1,
            content_key=ContentKey.normalize("https://box.example/graphql", "scene-1"),
            captured_at=datetime.now(timezone.utc),
            source_updated_at=datetime.now(timezone.utc),
            scenes=[scene],
            performers=performers,
        )


@pytest.mark.parametrize(
    ("fixture_name", "valid"),
    [
        ("preference-event.valid.json", True),
        ("preference-event.invalid.json", False),
        ("preference-event.boolean-rating.invalid.json", False),
        ("interaction-event.played.valid.json", True),
        ("interaction-event.o.valid.json", True),
        ("interaction-event.with-rating.invalid.json", False),
        ("interaction-event.non-utc-timestamp.invalid.json", False),
        ("interaction-event.credential-endpoint.invalid.json", False),
        ("interaction-event.query-fragment-endpoint.invalid.json", False),
        ("interaction-event.http-endpoint.invalid.json", False),
        ("interaction-event.uppercase-endpoint.valid.json", True),
        ("interaction-event.empty-query-endpoint.invalid.json", False),
        ("interaction-event.empty-fragment-endpoint.invalid.json", False),
    ],
)
def test_event_fixtures_have_cross_language_contract_parity(fixture_name: str, valid: bool) -> None:
    payload = _load_fixture(fixture_name)
    occurred_at = datetime.fromisoformat(payload.pop("occurred_at").replace("Z", "+00:00"))
    content_key = ContentKey(**payload.pop("content_key"))
    event = PreferenceEvent(
        **payload,
        occurred_at=occurred_at,
        content_key=content_key,
    )

    if valid:
        event.validate()
    else:
        with pytest.raises(ValueError):
            event.validate()


@pytest.mark.parametrize(
    ("fixture_name", "valid"),
    [
        ("source-snapshot.valid.json", True),
        ("source-snapshot.group.valid.json", True),
        ("source-snapshot.missing-appearance-performer.invalid.json", False),
        ("source-snapshot.missing-source-updated-at.invalid.json", False),
        ("source-snapshot.boolean-duration.invalid.json", False),
        ("source-snapshot.scalar-dates.invalid.json", False),
        ("source-snapshot.invalid-date.invalid.json", False),
        ("source-snapshot.nested-null.invalid.json", False),
        ("source-snapshot.boolean-career-years.invalid.json", False),
        ("source-snapshot.invalid-remote-url.invalid.json", False),
        ("source-snapshot.credential-remote-image.invalid.json", False),
        ("source-snapshot.query-fragment-remote-reference.invalid.json", False),
        ("source-snapshot.http-remote-reference.invalid.json", False),
        ("source-snapshot.empty-query-remote-reference.invalid.json", False),
        ("source-snapshot.empty-fragment-remote-reference.invalid.json", False),
        ("source-snapshot.blank-group.invalid.json", False),
    ],
)
def test_snapshot_fixtures_have_cross_language_contract_parity(fixture_name: str, valid: bool) -> None:
    payload = _load_fixture(fixture_name)
    captured_at = datetime.fromisoformat(payload.pop("captured_at").replace("Z", "+00:00"))
    source_updated_at_value = payload.pop("source_updated_at", None)
    source_updated_at = (
        datetime.fromisoformat(source_updated_at_value.replace("Z", "+00:00"))
        if source_updated_at_value is not None
        else None
    )
    content_key = ContentKey(**payload.pop("content_key"))

    if valid:
        SourceSnapshot(
            **payload,
            captured_at=captured_at,
            source_updated_at=source_updated_at,
            content_key=content_key,
        )
    else:
        with pytest.raises(ValueError):
            SourceSnapshot(
                **payload,
                captured_at=captured_at,
                source_updated_at=source_updated_at,
                content_key=content_key,
            )


def test_uppercase_endpoint_fixture_normalizes() -> None:
    payload = _load_fixture("interaction-event.uppercase-endpoint.valid.json")
    occurred_at = datetime.fromisoformat(payload.pop("occurred_at").replace("Z", "+00:00"))
    content_key = ContentKey(**payload.pop("content_key"))
    event = PreferenceEvent(
        **payload,
        occurred_at=occurred_at,
        content_key=content_key,
    )

    event.validate()

    assert event.content_key.endpoint == "https://box.example/GRAPHQL"


def _load_fixture(name: str) -> dict[str, object]:
    path = Path(__file__).parents[3] / "contracts" / "v1" / "fixtures" / name
    with path.open() as fixture:
        return json.load(fixture)
