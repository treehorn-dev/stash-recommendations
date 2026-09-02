#!/usr/bin/env python3
"""Build the Stash remote-package ZIP and its corresponding package index."""

from __future__ import annotations

import argparse
import hashlib
from datetime import datetime, timezone
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile


REPOSITORY = "https://github.com/treehorn-dev/stash-recommendations"
PLUGIN_ROOT = Path("plugin/stashRecommendations")


def archive_members() -> list[Path]:
    members = [
        PLUGIN_ROOT / "stashRecommendations.yml",
        PLUGIN_ROOT / "recommendations.py",
    ]
    members.extend(sorted((PLUGIN_ROOT / "rec_plugin").glob("*.py")))
    members.extend(sorted((PLUGIN_ROOT / "ui").glob("*")))
    return members


def build(version: str, output_dir: Path, released_at: datetime) -> tuple[Path, Path]:
    output_dir.mkdir(parents=True, exist_ok=True)
    archive_name = f"stashRecommendations-{version}.zip"
    archive_path = output_dir / archive_name

    with ZipFile(archive_path, "w", compression=ZIP_DEFLATED) as archive:
        for member in archive_members():
            archive.write(member, member.relative_to(PLUGIN_ROOT))

    checksum = hashlib.sha256(archive_path.read_bytes()).hexdigest()
    release_tag = f"v{version}"
    index_path = output_dir / "index.yml"
    index_path.write_text(
        "\n".join(
            [
                "- id: stashRecommendations",
                "  name: Stash Recommendations",
                f'  version: "{version}"',
                f'  date: "{released_at.strftime("%Y-%m-%d %H:%M:%S")}"',
                f"  path: {REPOSITORY}/releases/download/{release_tag}/{archive_name}",
                f"  sha256: {checksum}",
                "  metadata:",
                "    description: Recommendation sync plugin and local recommendation surfaces for Stash.",
                "    authors:",
                "      - Treehorn Dev",
                "",
            ]
        )
    )
    return archive_path, index_path


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", required=True)
    parser.add_argument("--output-dir", type=Path, default=Path("dist"))
    args = parser.parse_args()
    build(args.version, args.output_dir, datetime.now(timezone.utc))


if __name__ == "__main__":
    main()
