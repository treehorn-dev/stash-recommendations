from __future__ import annotations

import json
from pathlib import Path

import pytest

from rec_plugin.contracts import ContentKey, PreferenceEvent, SourceSnapshot


@pytest.mark.parametrize("fixture_name", [
    "preference-event.valid.json",
    "interaction-event.played.valid.json",
    "interaction-event.o.valid.json",
    "interaction-event.uppercase-endpoint.valid.json",
])
def test_valid_event_fixtures_round_trip_through_plugin_contracts(fixture_name: str) -> None:
    payload = _load_fixture(fixture_name)

    event = _event_from_payload(payload)

    assert event.to_dict()["content_key"]["stash_id"]


@pytest.mark.parametrize("fixture_name", [
    "preference-event.invalid.json",
    "preference-event.boolean-rating.invalid.json",
    "interaction-event.with-rating.invalid.json",
    "interaction-event.non-utc-timestamp.invalid.json",
    "interaction-event.credential-endpoint.invalid.json",
    "interaction-event.query-fragment-endpoint.invalid.json",
    "interaction-event.http-endpoint.invalid.json",
])
def test_invalid_event_fixtures_are_rejected_by_plugin_contracts(fixture_name: str) -> None:
    payload = _load_fixture(fixture_name)

    with pytest.raises(ValueError):
        _event_from_payload(payload).validate()


@pytest.mark.parametrize("fixture_name", [
    "source-snapshot.valid.json",
    "source-snapshot.group.valid.json",
])
def test_valid_snapshot_fixtures_round_trip_through_plugin_contracts(fixture_name: str) -> None:
    payload = _load_fixture(fixture_name)

    snapshot = _snapshot_from_payload(payload)

    assert snapshot.to_dict()["source_updated_at"].endswith("Z")


@pytest.mark.parametrize("fixture_name", [
    "source-snapshot.missing-appearance-performer.invalid.json",
    "source-snapshot.missing-source-updated-at.invalid.json",
    "source-snapshot.boolean-duration.invalid.json",
    "source-snapshot.scalar-dates.invalid.json",
    "source-snapshot.invalid-date.invalid.json",
    "source-snapshot.nested-null.invalid.json",
    "source-snapshot.boolean-career-years.invalid.json",
    "source-snapshot.invalid-remote-url.invalid.json",
    "source-snapshot.credential-remote-image.invalid.json",
    "source-snapshot.query-fragment-remote-reference.invalid.json",
    "source-snapshot.http-remote-reference.invalid.json",
    "source-snapshot.empty-query-remote-reference.invalid.json",
    "source-snapshot.empty-fragment-remote-reference.invalid.json",
    "source-snapshot.blank-group.invalid.json",
])
def test_invalid_snapshot_fixtures_are_rejected_by_plugin_contracts(fixture_name: str) -> None:
    payload = _load_fixture(fixture_name)

    with pytest.raises(ValueError):
        _snapshot_from_payload(payload)


def _event_from_payload(payload: dict[str, object]) -> PreferenceEvent:
    occurred_at = _timestamp(str(payload["occurred_at"]))
    content_key = ContentKey(**dict(payload["content_key"]))
    return PreferenceEvent(
        schema_version=payload["schema_version"],
        event_id=str(payload["event_id"]),
        client_id=str(payload["client_id"]),
        sequence=payload["sequence"],
        occurred_at=occurred_at,
        content_key=content_key,
        kind=str(payload["kind"]),
        origin=str(payload["origin"]),
        rating=payload.get("rating"),
    )


def _snapshot_from_payload(payload: dict[str, object]) -> SourceSnapshot:
    source_updated_at_value = payload.get("source_updated_at")
    return SourceSnapshot(
        schema_version=payload["schema_version"],
        content_key=ContentKey(**dict(payload["content_key"])),
        captured_at=_timestamp(str(payload["captured_at"])),
        source_updated_at=_timestamp(str(source_updated_at_value)) if source_updated_at_value is not None else None,
        scenes=list(payload["scenes"]),
        performers=list(payload["performers"]),
    )


def _timestamp(value: str):
    from datetime import datetime

    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def _load_fixture(name: str) -> dict[str, object]:
    path = Path(__file__).parents[3] / "contracts" / "v1" / "fixtures" / name
    with path.open() as fixture:
        return json.load(fixture)
