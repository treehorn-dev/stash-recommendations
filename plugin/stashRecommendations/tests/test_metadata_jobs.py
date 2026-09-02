from __future__ import annotations

from pathlib import Path

from rec_plugin.metadata_jobs import MetadataJobs


def test_metadata_jobs_deduplicate_content_keys_and_claim_bounded_batch(tmp_path: Path) -> None:
    jobs = MetadataJobs(tmp_path / "recommendations.sqlite3")
    jobs.enqueue("https://box.example/graphql", "scene-1")
    jobs.enqueue("https://box.example/graphql", "scene-1")
    jobs.enqueue("https://box.example/graphql", "scene-2")

    assert jobs.claim(limit=1) == [("https://box.example/graphql", "scene-1")]
    assert jobs.status() == {"pending": 1, "in_progress": 1, "completed": 0, "failed": 0}
    jobs.complete("https://box.example/graphql", "scene-1")
    assert jobs.status() == {"pending": 1, "in_progress": 0, "completed": 1, "failed": 0}


def test_metadata_jobs_enqueue_normalizes_canonical_keys(tmp_path: Path) -> None:
    jobs = MetadataJobs(tmp_path / "recommendations.sqlite3")

    jobs.enqueue("HTTPS://BOX.EXAMPLE/graphql/", "scene-1")
    jobs.enqueue("https://box.example/graphql", "scene-1")

    assert jobs.claim(limit=10) == [("https://box.example/graphql", "scene-1")]


def test_metadata_jobs_fail_returns_claimed_job_to_pending(tmp_path: Path) -> None:
    jobs = MetadataJobs(tmp_path / "recommendations.sqlite3")
    jobs.enqueue("https://box.example/graphql", "scene-1")
    assert jobs.claim(limit=1) == [("https://box.example/graphql", "scene-1")]

    jobs.fail("https://box.example/graphql", "scene-1")

    assert jobs.status() == {"pending": 1, "in_progress": 0, "completed": 0, "failed": 0}
