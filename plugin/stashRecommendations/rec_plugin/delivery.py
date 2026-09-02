from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Any

from rec_plugin.outbox import Outbox, OutboxItem


@dataclass(frozen=True)
class ServiceResponse:
    status_code: int
    retry_after_seconds: int | None = None
    error: str | None = None
    body: dict[str, Any] | None = None


@dataclass
class DeliverySummary:
    delivered: int = 0
    retried: int = 0
    quarantined: int = 0
    paused: bool = False


class DeliveryWorker:
    def __init__(self, outbox: Outbox, service_client: Any, *, pause_key: str | None = None) -> None:
        self._outbox = outbox
        self._service_client = service_client
        self._pause_key = pause_key

    def deliver_ready(self, now: datetime) -> DeliverySummary:
        summary = DeliverySummary()
        if self._outbox.sync_delivery_pause(self._pause_key):
            summary.paused = True
            return summary
        while True:
            item = self._outbox.next_ready(now)
            if item is None:
                return summary
            outcome = self._deliver_item(item, now)
            if outcome == "ack":
                summary.delivered += 1
                self._outbox.ack(item.row_id)
                continue
            if outcome == "pause":
                summary.paused = True
                return summary
            if outcome == "quarantine":
                summary.quarantined += 1
                continue
            summary.retried += 1

    def _deliver_item(self, item: OutboxItem, now: datetime) -> str:
        try:
            response = self._send(item)
        except OSError as error:
            self._outbox.record_retry(item.row_id, now, str(error))
            self._outbox.record_delivery_attempt(item, now, "retry", error=str(error))
            return "retry"
        if response.status_code in (200, 202):
            self._outbox.record_delivery_attempt(item, now, "delivered", status_code=response.status_code)
            return "ack"
        if response.status_code in (401, 403):
            self._outbox.pause_delivery("service authentication failed", pause_key=self._pause_key)
            self._outbox.record_delivery_attempt(
                item, now, "paused", status_code=response.status_code, error=response.error
            )
            return "pause"
        if response.status_code in (400, 409, 422):
            self._outbox.quarantine(
                item.row_id,
                response.error or f"service rejected payload with status {response.status_code}",
            )
            self._outbox.record_delivery_attempt(
                item, now, "quarantined", status_code=response.status_code, error=response.error
            )
            return "quarantine"
        if response.status_code == 429:
            self._outbox.record_retry_after(
                item.row_id,
                now,
                response.retry_after_seconds,
                response.error or "service rate limited",
            )
            self._outbox.record_delivery_attempt(item, now, "retry", status_code=response.status_code, error=response.error)
            return "retry"
        self._outbox.record_retry(
            item.row_id,
            now,
            response.error or f"service unavailable ({response.status_code})",
        )
        self._outbox.record_delivery_attempt(item, now, "retry", status_code=response.status_code, error=response.error)
        return "retry"

    def _send(self, item: OutboxItem) -> ServiceResponse:
        if item.item_type == "preference_event":
            return self._service_client.deliver_preference_event(item.payload)
        if item.item_type == "source_snapshot":
            return self._service_client.deliver_snapshot(item.payload)
        raise ValueError(f"unsupported deliverable outbox item type: {item.item_type}")
