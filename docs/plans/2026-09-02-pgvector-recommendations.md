# Pgvector Recommendations Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the quadratic pairwise recommendation projection with pgvector-backed content and account-profile retrieval.

**Architecture:** PostgreSQL remains the system of record. A model build hashes Stash-box metadata into normalized 256-dimensional scene vectors. It derives each account profile as a weighted mean of interacted scene vectors, uses HNSW to retrieve bounded candidates, and atomically writes only the active model's vectors and For You results.

**Tech Stack:** Go, PostgreSQL 16, pgvector, pgx, Docker Compose, Go testing.

### Task 1: Add pgvector storage and feature-vector encoding

**Files:**
- Modify: `docker-compose.yml`
- Create: `server/internal/store/migrations/009_pgvector_recommendations.sql`
- Create: `server/internal/model/vector.go`
- Test: `server/internal/model/vector_test.go`

**Step 1:** Write failing tests for stable reordered metadata, unit normalization, distinct entity vectors, and empty metadata.

**Step 2:** Run `go test ./server/internal/model -run TestSceneVector -v`; expect failure because `SceneVector` does not exist.

**Step 3:** Implement `SceneVector(features []string) ([]float32, bool)` using SHA-256 signed feature hashing, 256 dimensions, sorted/deduplicated tokens, and unit L2 normalization.

**Step 4:** Use `pgvector/pgvector:pg16`; enable `vector`; add `model_scene_vectors(model_version_id, endpoint, stash_id, embedding vector(256))` and an HNSW cosine index.

**Step 5:** Rerun the focused test; expect pass. Commit `feat: add pgvector scene embeddings`.

### Task 2: Replace pairwise projection with vector retrieval

**Files:**
- Modify: `server/internal/model/interfaces.go`
- Modify: `server/internal/model/build.go`
- Modify: `server/internal/model/repository.go`
- Test: `server/internal/model/build_test.go`

**Step 1:** Write failing integration tests: two scenes sharing a performer rank together, an account with rating/play/O receives an unconsumed related scene, and a long session cannot create more vector rows than catalog scenes plus bounded results.

**Step 2:** Run `POSTGRES_TEST_DSN="$POSTGRES_TEST_DSN" go test ./server/internal/model -run 'TestBuild.*Vector' -v`; expect failure because the builder materializes pairs.

**Step 3:** Load scene feature tokens from performer, tag, group, studio, director, and code projections. Load positive ratings plus raw engagement events, with plays weight 1 and O weight `DefaultOWeight`. Do not combine both session projections because that double counts events.

**Step 4:** Build one vector per catalog scene. Query related candidates through `ORDER BY embedding <=> $query LIMIT 50`; derive normalized profile vectors and persist top-50 unconsumed For You candidates. Retain atomic model activation and reasons: `content_similarity`, `rating_profile`, `play_profile`, `o_profile`.

**Step 5:** Stop writing `item_neighbors`; retain its schema for migration compatibility. Make related reads query active model vectors, while `user_recommendations` remains the bounded durable For You projection.

**Step 6:** Rerun the focused tests; expect pass. Commit `feat: build recommendations with pgvector`.

### Task 3: Preserve API behavior and verify the stack

**Files:**
- Modify: `server/internal/httpapi/recommendations_test.go`
- Modify: `README.md`
- Test: `tests/e2e/test_rating_to_recommendation.py`

**Step 1:** Write failing API and E2E assertions that related and For You responses retain their existing JSON schema and active model version, including a no-feature cold start.

**Step 2:** Run `POSTGRES_TEST_DSN="$POSTGRES_TEST_DSN" go test ./server/internal/httpapi -run TestRecommendations -v`; expect failure until vector-backed reads exist.

**Step 3:** Make only compatibility fixes; document the pgvector image and bounded-vector build behavior.

**Step 4:** Run `make test && make test-e2e`; expect pass. Commit `test: verify pgvector recommendation flow`.
