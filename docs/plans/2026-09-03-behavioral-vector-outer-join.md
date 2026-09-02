# Behavioral Vector Outer-Join Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Recommend catalog scenes with metadata and sessioned scenes without metadata through one bounded pgvector model.

**Architecture:** Build content vectors from source metadata as today. Build behavior vectors by summing the other scenes' anchor vectors within each session; anchors use content vectors when available and deterministic scene-identity vectors otherwise. Normalize the sum of content and behavioral components into a single scene embedding, so metadata-only catalog scenes and metadata-null sessioned scenes participate in the same retrieval space without materializing pairs.

**Tech Stack:** Go, pgvector, PostgreSQL, pgx, Go tests.

### Task 1: Build Behavioral Scene Vectors

**Files:**
- Modify: `server/internal/model/vector.go`
- Modify: `server/internal/model/vector_test.go`
- Modify: `server/internal/model/build.go`
- Test: `server/internal/model/vector_test.go`

**Step 1:** Write failing tests for a metadata-null scene acquiring a vector from a session with a metadata scene, a metadata-only scene retaining a vector, and a singleton session creating no behavioral recommendation signal.

**Step 2:** Run `go test ./server/internal/model -run 'Test(BuildVectorProjection|Behavioral)' -v`; expect failure because behavioral aggregation does not exist.

**Step 3:** Add deterministic identity anchors and an O(session-items * 256) centroid-minus-self behavioral aggregation. Combine content and behavioral components where either exists, retaining unit normalization.

**Step 4:** Rerun the focused test; expect pass.

**Step 5:** Commit `feat: add behavioral scene vector outer join`.

### Task 2: Preserve Profile And Retrieval Eligibility

**Files:**
- Modify: `server/internal/model/build_test.go`
- Modify: `server/internal/model/repository.go`
- Test: `server/internal/model/build_test.go`

**Step 1:** Write a failing pgvector integration test: a metadata-null scene with play/O session history is persisted, is returned by related retrieval through a metadata-bearing session neighbor, and can seed a For You profile.

**Step 2:** Run `POSTGRES_TEST_DSN="$POSTGRES_TEST_DSN" go test ./server/internal/model -run TestBuild.*Behavioral -v`; expect failure because only catalog scenes are persisted.

**Step 3:** Persist the outer union of catalog and sessioned content keys. Keep the existing active-version transaction, bounded top-50 reads, and no `item_neighbors` writes.

**Step 4:** Rerun the focused integration test; expect pass.

**Step 5:** Commit `feat: retrieve behavioral-only scenes`.

### Task 3: Verify API And Documentation

**Files:**
- Modify: `server/internal/httpapi/recommendations_test.go`
- Modify: `README.md`
- Test: `server/internal/httpapi/recommendations_test.go`

**Step 1:** Write a failing API assertion that a behavioral-only related scene preserves the existing response schema and reports `behavioral_similarity`.

**Step 2:** Run `POSTGRES_TEST_DSN="$POSTGRES_TEST_DSN" go test ./server/internal/httpapi -run TestRecommendationReads -v`; expect failure.

**Step 3:** Make only response-reason and documentation changes needed to describe metadata-plus-behavior eligibility.

**Step 4:** Run `make test && make test-contract && make test-e2e && go vet ./server/...`; expect pass.

**Step 5:** Commit `test: verify behavioral recommendation eligibility`.
