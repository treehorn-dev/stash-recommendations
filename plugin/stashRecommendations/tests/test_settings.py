from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from rec_plugin.settings import Settings


def test_settings_require_https_service_url() -> None:
    with pytest.raises(ValueError, match="HTTPS"):
        Settings.from_plugin_config(
            {
                "service_url": "http://rec.example/v1",
                "api_key": "secret-key",
                "show_remote_results": False,
            }
        )

    settings = Settings.from_plugin_config(
        {
            "service_url": "HTTPS://REC.EXAMPLE/v1/",
            "api_key": "  secret-key  ",
            "show_remote_results": True,
        }
    )

    assert settings.service_url == "https://rec.example/v1"
    assert settings.api_key == "secret-key"
    assert settings.show_remote_results is True


@pytest.mark.parametrize(
    "url",
    [
        "http://10.8.0.1:8080",
        "http://172.16.2.3:8080",
        "http://192.168.1.40:8080",
        "http://100.64.0.1:8080",
        "http://100.127.255.254:8080",
        "http://[fd7a:115c:a1e0::1]:8080",
    ],
)
def test_settings_allow_opted_in_private_and_tailnet_http_urls(url: str) -> None:
    settings = Settings.from_plugin_config(
        {
            "service_url": url,
            "api_key": "secret-key",
            "show_remote_results": False,
            "allow_private_http": True,
        }
    )

    assert settings.service_url == url
    assert settings.allow_private_http is True


@pytest.mark.parametrize(
    "url",
    [
        "http://8.8.8.8:8080",
        "http://rec.example:8080",
        "http://100.128.0.1:8080",
    ],
)
def test_settings_reject_public_http_even_when_private_http_is_enabled(url: str) -> None:
    with pytest.raises(ValueError, match="private"):
        Settings.from_plugin_config(
            {
                "service_url": url,
                "api_key": "secret-key",
                "show_remote_results": False,
                "allow_private_http": True,
            }
        )


def test_manifest_declares_raw_python_tasks_hook_and_settings() -> None:
    manifest = Path(__file__).resolve().parents[1] / "stashRecommendations.yml"
    text = manifest.read_text()

    assert "interface: raw" in text
    assert '  - python3' in text
    assert "service_url:" in text
    assert "api_key:" in text
    assert "show_remote_results:" in text
    assert "allow_private_http:" in text
    assert "sync-ratings" in text
    assert "sync-engagement" in text
    assert "sync-metadata" in text
    assert "deliver-outbox" in text
    assert "status" in text
    assert "capture-rating" in text
    assert "Scene.Update.Post" in text


def test_manifest_declares_confirmed_sync_tasks() -> None:
    manifest = Path(__file__).resolve().parents[1] / "stashRecommendations.yml"
    tasks = yaml.safe_load(manifest.read_text())["tasks"]
    defaults = {task["name"]: task["defaultArgs"] for task in tasks}

    assert defaults["sync-ratings-confirmed"] == {"mode": "sync-ratings", "confirmed": True}
    assert defaults["sync-engagement-confirmed"] == {"mode": "sync-engagement", "confirmed": True}
    assert defaults["sync-metadata-confirmed"] == {"mode": "sync-metadata", "confirmed": True}
