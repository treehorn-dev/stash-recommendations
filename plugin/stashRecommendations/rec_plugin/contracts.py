from __future__ import annotations

from dataclasses import asdict, dataclass
from datetime import date, datetime, timedelta
import math
import re
from typing import Any, Optional
from urllib.parse import urlparse, urlunparse
from uuid import UUID


UUID_PATTERN = re.compile(r"^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
SCENE_FIELDS = {"id", "title", "details", "dates", "urls", "duration", "director", "code", "studio", "tags", "performer_appearances", "remote_images"}
PERFORMER_FIELDS = {"id", "name", "aliases", "gender", "country", "ethnicity", "eye_color", "hair_color", "measurements", "career_years", "urls", "remote_images"}


@dataclass
class ContentKey:
    endpoint: str
    stash_id: str

    @classmethod
    def normalize(cls, endpoint: str, stash_id: str) -> "ContentKey":
        if not isinstance(endpoint, str) or not isinstance(stash_id, str) or not stash_id.strip():
            raise ValueError("stash_id is required")
        parsed = _parse_public_https_url(endpoint)
        path = parsed.path[:-1] if parsed.path.endswith("/") else parsed.path
        return cls(
            endpoint=urlunparse((parsed.scheme.lower(), parsed.netloc.lower(), path, parsed.params, "", "")),
            stash_id=stash_id,
        )


@dataclass
class PreferenceEvent:
    schema_version: int
    event_id: str
    client_id: str
    sequence: int
    occurred_at: datetime
    content_key: ContentKey
    kind: str
    origin: str
    rating: Optional[float] = None

    def validate(self) -> None:
        if type(self.schema_version) is not int or self.schema_version != 1:
            raise ValueError("schema_version must be 1")
        if not UUID_PATTERN.fullmatch(self.event_id) or not UUID_PATTERN.fullmatch(self.client_id):
            raise ValueError("event_id and client_id must be canonical UUIDs")
        UUID(self.event_id)
        UUID(self.client_id)
        if type(self.sequence) is not int or self.sequence < 1:
            raise ValueError("sequence must be at least 1")
        if not isinstance(self.occurred_at, datetime) or self.occurred_at.tzinfo is None or self.occurred_at.utcoffset() != timedelta(0):
            raise ValueError("occurred_at must be UTC")
        self.content_key = ContentKey.normalize(self.content_key.endpoint, self.content_key.stash_id)
        if self.kind == "scene.rating.set":
            if type(self.rating) not in (int, float) or not math.isfinite(self.rating) or not 0 <= self.rating <= 1:
                raise ValueError("scene.rating.set requires rating between 0 and 1")
        elif self.kind in {"scene.rating.remove", "scene.played", "scene.o"}:
            if self.rating is not None:
                raise ValueError(f"{self.kind} prohibits rating")
        else:
            raise ValueError("unsupported preference event kind")
        if not self.origin.strip():
            raise ValueError("origin is required")

    def to_dict(self) -> dict[str, Any]:
        self.validate()
        payload = asdict(self)
        payload["occurred_at"] = self.occurred_at.isoformat().replace("+00:00", "Z")
        if self.kind != "scene.rating.set":
            payload.pop("rating")
        return payload


@dataclass
class SourceSnapshot:
    schema_version: int
    content_key: ContentKey
    captured_at: datetime
    scenes: list[dict[str, Any]]
    performers: list[dict[str, Any]]

    def __post_init__(self) -> None:
        self.validate()

    def validate(self) -> None:
        if type(self.schema_version) is not int or self.schema_version != 1:
            raise ValueError("schema_version must be 1")
        if not isinstance(self.captured_at, datetime) or self.captured_at.tzinfo is None:
            raise ValueError("captured_at must be timezone-aware")
        self.content_key = ContentKey.normalize(self.content_key.endpoint, self.content_key.stash_id)
        if not isinstance(self.scenes, list) or not isinstance(self.performers, list):
            raise ValueError("scenes and performers must be arrays")
        for index, scene in enumerate(self.scenes):
            _validate_scene(scene, index)
        for index, performer in enumerate(self.performers):
            _validate_performer(performer, index)

    def to_dict(self) -> dict[str, Any]:
        self.validate()
        payload = asdict(self)
        payload["captured_at"] = self.captured_at.isoformat().replace("+00:00", "Z")
        return payload


def _validate_scene(scene: object, index: int) -> None:
    if not isinstance(scene, dict):
        raise ValueError(f"scenes[{index}] must be an object")
    _reject_unknown_fields(scene, SCENE_FIELDS, f"scenes[{index}]")
    if not isinstance(scene.get("id"), str) or not scene["id"].strip():
        raise ValueError(f"scenes[{index}] scene id is required")
    _validate_optional_strings(scene, index, {"title", "details", "director", "code"})
    _validate_date_values(scene, index)
    _validate_https_reference_collection(scene, index, "urls")
    _validate_https_reference_collection(scene, index, "remote_images")
    if "duration" in scene and (type(scene["duration"]) is not int or scene["duration"] < 0):
        raise ValueError(f"scenes[{index}].duration must be a non-negative integer")
    if "studio" in scene:
        _validate_named_record(scene["studio"], f"scenes[{index}].studio")
    tags = _collection(scene, index, "tags")
    for tag_index, tag in enumerate(tags):
        _validate_named_record(tag, f"scenes[{index}].tags[{tag_index}]")
    appearances = _collection(scene, index, "performer_appearances")
    for appearance_index, appearance in enumerate(appearances):
        if not isinstance(appearance, dict) or set(appearance) != {"performer_id"} or not isinstance(appearance["performer_id"], str) or not appearance["performer_id"].strip():
            raise ValueError(f"scenes[{index}].performer_appearances[{appearance_index}] performer_id is required")


def _validate_performer(performer: object, index: int) -> None:
    if not isinstance(performer, dict):
        raise ValueError(f"performers[{index}] must be an object")
    _reject_unknown_fields(performer, PERFORMER_FIELDS, f"performers[{index}]")
    if not isinstance(performer.get("id"), str) or not performer["id"].strip() or not isinstance(performer.get("name"), str) or not performer["name"].strip():
        raise ValueError(f"performers[{index}] performer id and name are required")
    _validate_optional_strings(performer, index, {"gender", "country", "ethnicity", "eye_color", "hair_color", "measurements"}, "performers")
    for field in ("aliases",):
        _validate_string_collection(performer, index, field, "performers")
    for field in ("urls", "remote_images"):
        _validate_https_reference_collection(performer, index, field, "performers")
    career_years = _collection(performer, index, "career_years", "performers")
    for year_index, year in enumerate(career_years):
        if type(year) is not int:
            raise ValueError(f"performers[{index}].career_years[{year_index}] must be an integer")


def _validate_named_record(record: object, label: str) -> None:
    if not isinstance(record, dict) or set(record) != {"id", "name"} or not isinstance(record["id"], str) or not record["id"].strip() or not isinstance(record["name"], str) or not record["name"].strip():
        raise ValueError(f"{label} id and name are required")


def _reject_unknown_fields(record: dict[str, Any], allowed_fields: set[str], label: str) -> None:
    unknown_fields = set(record) - allowed_fields
    if unknown_fields:
        raise ValueError(f"{label} contains unsupported field {sorted(unknown_fields)[0]}")


def _collection(record: dict[str, Any], index: int, field: str, collection_name: str = "scenes") -> list[Any]:
    if field not in record:
        return []
    value = record[field]
    if not isinstance(value, list):
        raise ValueError(f"{collection_name}[{index}].{field} must be an array")
    return value


def _validate_optional_strings(record: dict[str, Any], index: int, fields: set[str], collection_name: str = "scenes") -> None:
    for field in fields:
        if field in record and not isinstance(record[field], str):
            raise ValueError(f"{collection_name}[{index}].{field} must be a string")


def _validate_string_collection(record: dict[str, Any], index: int, field: str, collection_name: str = "scenes") -> None:
    for value_index, value in enumerate(_collection(record, index, field, collection_name)):
        if not isinstance(value, str):
            raise ValueError(f"{collection_name}[{index}].{field}[{value_index}] must be a string")


def _validate_https_reference_collection(record: dict[str, Any], index: int, field: str, collection_name: str = "scenes") -> None:
    for value_index, value in enumerate(_collection(record, index, field, collection_name)):
        if not isinstance(value, str):
            raise ValueError(f"{collection_name}[{index}].{field}[{value_index}] must be a string")
        try:
            _parse_public_https_url(value)
        except ValueError as error:
            raise ValueError(f"{collection_name}[{index}].{field}[{value_index}] must be a public HTTPS URL") from error


def _validate_date_values(scene: dict[str, Any], index: int) -> None:
    for date_index, value in enumerate(_collection(scene, index, "dates")):
        if not isinstance(value, str) or not re.fullmatch(r"\d{4}-\d{2}-\d{2}", value):
            raise ValueError(f"scenes[{index}].dates[{date_index}] must be a valid date")
        try:
            date.fromisoformat(value)
        except ValueError as error:
            raise ValueError(f"scenes[{index}].dates[{date_index}] must be a valid date") from error


def _parse_public_https_url(value: str):
    try:
        parsed = urlparse(value)
        username = parsed.username
    except ValueError as error:
        raise ValueError("must be an absolute HTTPS URL") from error
    if parsed.scheme.lower() != "https" or not parsed.netloc:
        raise ValueError("must be an absolute HTTPS URL")
    if "?" in value or "#" in value or username is not None or parsed.query or parsed.fragment:
        raise ValueError("must not contain credentials, query, or fragment")
    return parsed
