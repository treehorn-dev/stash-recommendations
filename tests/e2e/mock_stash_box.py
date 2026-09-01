from __future__ import annotations

from contextlib import AbstractContextManager
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import threading
from typing import Any


@dataclass(frozen=True)
class CapturedRequest:
    path: str
    headers: dict[str, str]
    body: dict[str, Any]


class MockStashBox(AbstractContextManager["MockStashBox"]):
    def __init__(self, host: str, port: int, api_key: str, scenes: dict[str, dict[str, Any]]) -> None:
        self._host = host
        self._port = port
        self._api_key = api_key
        self._scenes = {scene_id: dict(scene) for scene_id, scene in scenes.items()}
        self._requests: list[CapturedRequest] = []
        self._server = _MockStashBoxServer((host, port), self._handler())
        self._thread = threading.Thread(target=self._server.serve_forever, daemon=True)

    @property
    def endpoint(self) -> str:
        return f"https://{self._host}:{self._port}/graphql"

    @property
    def transport_url(self) -> str:
        return f"http://{self._host}:{self._port}/graphql"

    @property
    def requests(self) -> list[CapturedRequest]:
        return list(self._requests)

    def credentials_config(self) -> list[dict[str, object]]:
        return [{"endpoint": self.endpoint, "api_key": self._api_key, "max_requests_per_minute": 120}]

    def scene_key(self, stash_id: str) -> dict[str, str]:
        return {"endpoint": self.endpoint, "stash_id": stash_id}

    def __enter__(self) -> "MockStashBox":
        self._thread.start()
        return self

    def __exit__(self, exc_type, exc, exc_tb) -> None:
        self._server.shutdown()
        self._server.server_close()
        self._thread.join(timeout=5)

    def _handler(self):
        outer = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.0"

            def do_POST(self) -> None:  # noqa: N802
                length = int(self.headers.get("Content-Length", "0"))
                payload = json.loads(self.rfile.read(length).decode("utf-8"))
                outer._requests.append(
                    CapturedRequest(
                        path=self.path,
                        headers={key.lower(): value for key, value in self.headers.items()},
                        body=payload,
                    )
                )
                variables = dict(payload.get("variables", {}))
                scene = outer._scenes.get(str(variables.get("id")))
                response = {"data": {"findScene": scene}}
                encoded = json.dumps(response).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(encoded)))
                self.end_headers()
                self.wfile.write(encoded)

            def log_message(self, format: str, *args: object) -> None:
                del format, args

        return Handler


class _MockStashBoxServer(ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True
