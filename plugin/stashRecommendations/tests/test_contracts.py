from datetime import datetime, timezone

from rec_plugin.contracts import ContentKey, PreferenceEvent


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
