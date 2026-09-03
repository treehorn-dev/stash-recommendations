from __future__ import annotations

from datetime import date, datetime, timezone
from typing import Any, Iterable, Mapping
from urllib.parse import urlparse

from rec_plugin.contracts import ContentKey, SourceSnapshot


def to_source_snapshot(endpoint: str, captured_at: datetime, scene: Mapping[str, Any]) -> SourceSnapshot:
    source_updated_at = _parse_timestamp(scene.get("updated"))
    if source_updated_at is None:
        raise ValueError("source_updated_at is required")
    mapped_performers = [_map_performer(appearance) for appearance in scene.get("performers", []) or []]
    mapped_scene = {"id": str(scene["id"])}

    _copy_string(scene, mapped_scene, "title")
    _copy_string(scene, mapped_scene, "details")
    _copy_string(scene, mapped_scene, "director")
    _copy_string(scene, mapped_scene, "code")
    dates = _scene_dates(scene)
    if dates:
        mapped_scene["dates"] = dates
    urls = _https_reference_values(scene.get("urls"))
    if urls:
        mapped_scene["urls"] = urls
    duration = scene.get("duration")
    if isinstance(duration, int) and not isinstance(duration, bool):
        mapped_scene["duration"] = duration
    studio = _named_record(scene.get("studio"))
    if studio is not None:
        mapped_scene["studio"] = studio
    tags = _named_records(scene.get("tags"))
    if tags:
        mapped_scene["tags"] = tags
    groups = _groups(scene.get("groups"))
    if groups:
        mapped_scene["groups"] = groups
    remote_images = _image_urls(scene.get("images")) or _https_reference_values(scene.get("remote_images"))
    if remote_images:
        mapped_scene["remote_images"] = remote_images
    performer_ids = [{"performer_id": performer["id"]} for performer in mapped_performers]
    if performer_ids:
        mapped_scene["performer_appearances"] = performer_ids

    return SourceSnapshot(
        schema_version=1,
        content_key=ContentKey.normalize(endpoint, str(scene["id"])),
        captured_at=captured_at.astimezone(timezone.utc),
        source_updated_at=source_updated_at,
        scenes=[mapped_scene],
        performers=mapped_performers,
    )


def _map_performer(appearance: Mapping[str, Any]) -> dict[str, Any]:
    performer = appearance.get("performer", appearance)
    mapped = {
        "id": str(performer["id"]),
        "name": str(performer["name"]),
    }
    aliases = _string_values(performer.get("aliases"))
    if aliases:
        mapped["aliases"] = aliases
    for field in ("gender", "ethnicity", "eye_color", "hair_color"):
        value = performer.get(field)
        if isinstance(value, str) and value:
            mapped[field] = value.lower()
    _copy_string(performer, mapped, "country")
    measurements = _measurements_string(performer)
    if measurements:
        mapped["measurements"] = measurements
    career_years = _career_years(performer)
    if career_years:
        mapped["career_years"] = career_years
    urls = _https_reference_values(performer.get("urls"))
    if urls:
        mapped["urls"] = urls
    remote_images = _image_urls(performer.get("images")) or _https_reference_values(performer.get("remote_images"))
    if remote_images:
        mapped["remote_images"] = remote_images
    return mapped


def _scene_dates(scene: Mapping[str, Any]) -> list[str]:
    dates: list[str] = []
    for field in ("release_date", "production_date", "date"):
        value = scene.get(field)
        if isinstance(value, str) and _is_valid_date(value) and value not in dates:
            dates.append(value)
    return dates


def _career_years(record: Mapping[str, Any]) -> list[int]:
    years: list[int] = []
    for field in ("career_start_year", "career_end_year"):
        value = record.get(field)
        if isinstance(value, int) and not isinstance(value, bool) and value not in years:
            years.append(value)
    return years


def _measurements_string(record: Mapping[str, Any]) -> str | None:
    legacy = record.get("measurements")
    if isinstance(legacy, str) and legacy:
        return legacy
    band_size = record.get("band_size")
    cup_size = record.get("cup_size")
    waist_size = record.get("waist_size")
    hip_size = record.get("hip_size")
    if not isinstance(cup_size, str) or not cup_size:
        return None
    if not isinstance(band_size, int) or isinstance(band_size, bool):
        return None
    parts = [f"{band_size}{cup_size}"]
    if isinstance(waist_size, int) and not isinstance(waist_size, bool):
        parts.append(str(waist_size))
    if isinstance(hip_size, int) and not isinstance(hip_size, bool):
        parts.append(str(hip_size))
    return "-".join(parts)


def _named_records(values: object) -> list[dict[str, str]]:
    records: list[dict[str, str]] = []
    for value in values or []:
        record = _named_record(value)
        if record is not None:
            records.append(record)
    return records


def _groups(values: object) -> list[dict[str, str]]:
    groups: list[dict[str, str]] = []
    for value in values or []:
        if isinstance(value, Mapping) and "group" in value:
            record = _named_record(value.get("group"))
        else:
            record = _named_record(value)
        if record is not None:
            groups.append(record)
    return groups


def _named_record(value: object) -> dict[str, str] | None:
    if not isinstance(value, Mapping):
        return None
    record_id = value.get("id")
    name = value.get("name")
    if not isinstance(record_id, str) or not record_id.strip():
        return None
    if not isinstance(name, str) or not name.strip():
        return None
    return {"id": record_id, "name": name}


def _string_values(values: object) -> list[str]:
    result: list[str] = []
    if not isinstance(values, Iterable) or isinstance(values, (str, bytes, Mapping)):
        return result
    for value in values:
        if isinstance(value, str) and value:
            result.append(value)
        elif isinstance(value, Mapping):
            url = value.get("url")
            if isinstance(url, str) and url:
                result.append(url)
    return result


def _image_urls(values: object) -> list[str]:
    return _https_reference_values(values)


def _https_reference_values(values: object) -> list[str]:
    return [value for value in _string_values(values) if _is_public_https_url(value)]


def _copy_string(source: Mapping[str, Any], target: dict[str, Any], field: str) -> None:
    value = source.get(field)
    if isinstance(value, str) and value:
        target[field] = value


def _parse_timestamp(value: object) -> datetime | None:
    if not isinstance(value, str) or not value:
        return None
    return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)


def _is_valid_date(value: str) -> bool:
    try:
        date.fromisoformat(value)
    except ValueError:
        return False
    return True


def _is_public_https_url(value: str) -> bool:
    try:
        parsed = urlparse(value)
        username = parsed.username
    except ValueError:
        return False
    return (
        parsed.scheme.lower() == "https"
        and bool(parsed.netloc)
        and username is None
        and not parsed.query
        and not parsed.fragment
        and "?" not in value
        and "#" not in value
    )
