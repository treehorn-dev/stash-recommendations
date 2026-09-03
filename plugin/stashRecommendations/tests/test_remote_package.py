"""Verify the repository can be installed through Stash's remote package UI."""

from __future__ import annotations

import importlib.util
import zipfile
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[3]
BUILD_SCRIPT = ROOT / "scripts" / "build_plugin_package.py"


def load_builder():
    spec = importlib.util.spec_from_file_location("build_plugin_package", BUILD_SCRIPT)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_remote_package_catalog_matches_distributable_archive(tmp_path: Path) -> None:
    archive_path, index_path = load_builder().build(
        "0.1.17", tmp_path, load_builder().datetime(2026, 9, 3, tzinfo=load_builder().timezone.utc)
    )
    packages = yaml.safe_load(index_path.read_text())

    assert len(packages) == 1
    package = packages[0]
    assert package["id"] == "stashRecommendations"
    assert package["path"].endswith("/releases/download/v0.1.17/stashRecommendations-0.1.17.zip")
    assert package["sha256"] == load_builder().hashlib.sha256(archive_path.read_bytes()).hexdigest()

    with zipfile.ZipFile(archive_path) as archive:
        names = archive.namelist()
        manifest = yaml.safe_load(archive.read("stashRecommendations.yml"))

    assert "stashRecommendations.yml" in names
    assert "recommendations.py" in names
    assert "ui/recommendations.js" in names
    assert "ui/stash-plugin-components.js" in names
    assert "ui/stash-plugin-components.css" in names
    assert "rec_plugin/settings.py" in names
    assert manifest["version"] == "0.1.17"
    assert manifest["ui"]["javascript"][0] == "ui/stash-plugin-components.js"
    assert manifest["ui"]["css"][0] == "ui/stash-plugin-components.css"
