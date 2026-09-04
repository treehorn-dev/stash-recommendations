from __future__ import annotations

from datetime import datetime, timezone
import json
from typing import Any
from urllib import error, parse, request

from rec_plugin.delivery import ServiceResponse
from rec_plugin.settings import Settings

HTTP_TIMEOUT_SECONDS = 10.0


class ServiceClient:
    def __init__(self, settings: Settings) -> None:
        self._settings = settings

    def deliver_preference_event(self, payload: dict[str, Any]) -> ServiceResponse:
        return self._request_json("POST", "/v1/events/interactions", payload)

    def deliver_snapshot(self, payload: dict[str, Any]) -> ServiceResponse:
        return self._request_json("POST", "/v1/catalog/snapshots", payload)

    def fetch_related(self, content_keys: list[dict[str, str]], limit: int) -> dict[str, Any]:
        query = parse.urlencode(
            {
                "endpoint": content_keys[0]["endpoint"],
                "stash_id": content_keys[0]["stash_id"],
                "limit": str(limit),
            }
        )
        response = self._request_json("GET", f"/v1/recommendations/related?{query}", None)
        if response.status_code != 200:
            raise ValueError(response.error or f"service request failed with status {response.status_code}")
        return dict(response.body or {})

    def fetch_for_you(
        self, limit: int, *, offset: int = 0, filters: dict[str, Any] | None = None
    ) -> dict[str, Any]:
        parameters: dict[str, str] = {"limit": str(limit), "offset": str(max(0, offset))}
        for name in ("rating", "o_count"):
            predicate = (filters or {}).get(name)
            if not isinstance(predicate, dict) or not isinstance(predicate.get("operator"), str):
                continue
            parameters[f"{name}_operator"] = predicate["operator"]
            if predicate["operator"] not in {"is_null", "not_null"} and "value" in predicate:
                parameters[f"{name}_value"] = str(predicate["value"])
        query = parse.urlencode(parameters)
        response = self._request_json("GET", f"/v1/recommendations/for-you?{query}", None)
        if response.status_code != 200:
            raise ValueError(response.error or f"service request failed with status {response.status_code}")
        return dict(response.body or {})

    def _request_json(self, method: str, path: str, payload: dict[str, Any] | None) -> ServiceResponse:
        body = None if payload is None else json.dumps(payload).encode("utf-8")
        headers = {"Authorization": f"Bearer {self._settings.api_key}"}
        if body is not None:
            headers["Content-Type"] = "application/json"
        http_request = request.Request(self._url(path), data=body, headers=headers, method=method)
        try:
            with request.urlopen(http_request, timeout=HTTP_TIMEOUT_SECONDS) as response:
                return ServiceResponse(
                    status_code=response.status,
                    retry_after_seconds=_retry_after_seconds(response.headers.get("Retry-After")),
                    body=_decode_json(response),
                )
        except error.HTTPError as response:
            body = _decode_json(response)
            return ServiceResponse(
                status_code=response.code,
                retry_after_seconds=_retry_after_seconds(response.headers.get("Retry-After")),
                error=_decode_error(body, response),
                body=body,
            )
        except error.URLError as exc:
            raise OSError(str(exc.reason)) from exc

    def _url(self, path: str) -> str:
        return f"{self._settings.service_url}{path}"


def _decode_error(body: dict[str, Any] | None, response: Any) -> str | None:
    if isinstance(body, dict):
        detail = body.get("error") or body.get("message")
        if isinstance(detail, str) and detail:
            return detail
    return response.reason if hasattr(response, "reason") else None


def _decode_json(response: Any) -> dict[str, Any] | None:
    payload = response.read()
    if not payload.strip():
        return None
    try:
        return json.loads(payload.decode("utf-8"))
    except json.JSONDecodeError:
        return None


def _retry_after_seconds(value: str | None) -> int | None:
    if not value:
        return None
    try:
        return max(0, int(value))
    except ValueError:
        retry_at = datetime.strptime(value, "%a, %d %b %Y %H:%M:%S GMT").replace(tzinfo=timezone.utc)
        return max(0, int((retry_at - datetime.now(timezone.utc)).total_seconds()))
