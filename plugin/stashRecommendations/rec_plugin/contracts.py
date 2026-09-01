from __future__ import annotations

from dataclasses import asdict, dataclass
from datetime import datetime
from typing import Any, Optional
from urllib.parse import urlparse, urlunparse
from uuid import UUID


@dataclass
class ContentKey:
    endpoint: str
    stash_id: str

    @classmethod
    def normalize(cls, endpoint: str, stash_id: str) -> "ContentKey":
        if not stash_id.strip():
            raise ValueError("stash_id is required")
        parsed = urlparse(endpoint)
        if not parsed.scheme or not parsed.netloc:
            raise ValueError("endpoint must be an absolute URL")
        path = parsed.path[:-1] if parsed.path.endswith("/") else parsed.path
        return cls(
            endpoint=urlunparse((parsed.scheme.lower(), parsed.netloc.lower(), path, parsed.params, parsed.query, parsed.fragment)),
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
        if self.schema_version != 1:
            raise ValueError("schema_version must be 1")
        UUID(self.event_id)
        UUID(self.client_id)
        if self.sequence < 1:
            raise ValueError("sequence must be at least 1")
        if self.occurred_at.tzinfo is None:
            raise ValueError("occurred_at must be timezone-aware")
        self.content_key = ContentKey.normalize(self.content_key.endpoint, self.content_key.stash_id)
        if self.kind == "scene.rating.set":
            if self.rating is None or not 0 <= self.rating <= 1:
                raise ValueError("scene.rating.set requires rating between 0 and 1")
        elif self.kind == "scene.rating.remove":
            if self.rating is not None:
                raise ValueError("scene.rating.remove prohibits rating")
        else:
            raise ValueError("unsupported preference event kind")
        if not self.origin.strip():
            raise ValueError("origin is required")

    def to_dict(self) -> dict[str, Any]:
        self.validate()
        payload = asdict(self)
        payload["occurred_at"] = self.occurred_at.isoformat().replace("+00:00", "Z")
        if self.kind == "scene.rating.remove":
            payload.pop("rating")
        return payload


@dataclass
class SourceSnapshot:
    schema_version: int
    content_key: ContentKey
    captured_at: datetime
    scenes: list[dict[str, Any]]
    performers: list[dict[str, Any]]

    def validate(self) -> None:
        if self.schema_version != 1:
            raise ValueError("schema_version must be 1")
        if self.captured_at.tzinfo is None:
            raise ValueError("captured_at must be timezone-aware")
        self.content_key = ContentKey.normalize(self.content_key.endpoint, self.content_key.stash_id)
