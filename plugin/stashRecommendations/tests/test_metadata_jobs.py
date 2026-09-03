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

    jobs.fail("https://box.example/graphql", "scene-1", "upstream timed out")

    assert jobs.status() == {"pending": 1, "in_progress": 0, "completed": 0, "failed": 0}
    assert jobs.diagnostics() == [
        {
            "attempts": 1,
            "endpoint": "https://box.example/graphql",
            "last_error": "upstream timed out",
            "stash_id": "scene-1",
        }
    ]


def test_metadata_jobs_recovers_interrupted_claims_on_next_invocation(tmp_path: Path) -> None:
    path = tmp_path / "recommendations.sqlite3"
    jobs = MetadataJobs(path)
    jobs.enqueue("https://box.example/graphql", "scene-1")
    assert jobs.claim(limit=1) == [("https://box.example/graphql", "scene-1")]

    recovered = MetadataJobs(path)

    assert recovered.status() == {"pending": 1, "in_progress": 0, "completed": 0, "failed": 0}


def test_metadata_jobs_does_not_retry_a_failed_job_again_in_the_same_run(tmp_path: Path) -> None:
    jobs = MetadataJobs(tmp_path / "recommendations.sqlite3")
    jobs.enqueue("https://box.example/graphql", "scene-1")

    assert jobs.claim(limit=1, attempt_run="run-1") == [("https://box.example/graphql", "scene-1")]
    jobs.fail("https://box.example/graphql", "scene-1", "upstream timed out")

    assert jobs.claim(limit=1, attempt_run="run-1") == []
    assert jobs.claim(limit=1, attempt_run="run-2") == [("https://box.example/graphql", "scene-1")]


def test_metadata_jobs_appends_failure_diagnostics_to_a_local_log(tmp_path: Path) -> None:
    path = tmp_path / "recommendations.sqlite3"
    jobs = MetadataJobs(path)
    jobs.enqueue("https://box.example/graphql", "scene-1")
    jobs.claim(limit=1)

    jobs.fail("https://box.example/graphql", "scene-1", "upstream timed out")

    assert path.with_name("recommendations.metadata.log").read_text(encoding="utf-8") == (
        "endpoint=https://box.example/graphql stash_id=scene-1 error=upstream timed out\n"
    )
