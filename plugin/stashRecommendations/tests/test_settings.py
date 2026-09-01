from __future__ import annotations

from pathlib import Path

import pytest

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


def test_manifest_declares_raw_python_tasks_hook_and_settings() -> None:
    manifest = Path(__file__).resolve().parents[1] / "stashRecommendations.yml"
    text = manifest.read_text()

    assert "interface: raw" in text
    assert '  - python3' in text
    assert "service_url:" in text
    assert "api_key:" in text
    assert "show_remote_results:" in text
    assert "sync-ratings" in text
    assert "sync-engagement" in text
    assert "sync-metadata" in text
    assert "deliver-outbox" in text
    assert "status" in text
    assert "capture-rating" in text
    assert "Scene.Update.Post" in text
