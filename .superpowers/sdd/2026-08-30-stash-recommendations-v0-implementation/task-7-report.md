# Task 7 Report

Implemented Task 7 only: versioned PostgreSQL recommendation projections,
authenticated related and For You reads, model metadata in API results, and
server-side configuration for `scene.o` engagement weight. This task stops
before plugin work and awaits independent review.

## Scope

- `server/internal/model/interfaces.go`
- `server/internal/model/build.go`
- `server/internal/model/build_test.go`
- `server/internal/model/repository.go`
- `server/internal/httpapi/recommendations.go`
- `server/internal/httpapi/recommendations_test.go`
- `server/internal/httpapi/health.go`
- `server/internal/httpapi/preferences.go`
- `server/internal/config/config.go`
- `server/internal/config/config_test.go`
- `server/cmd/recommendations/main.go`
- `server/internal/store/migrations/001_initial.sql`
- `server/internal/store/migrations/007_recommendation_indexes.sql`
- `server/internal/store/store.go`
- `server/internal/store/store_test.go`

The builder scores explicit current ratings, both latest session projections
with `play=1.0` and configurable `o` weight (default `1.5`), and only the
validated catalog relations already present in V0 (performers, tags, studios,
and director metadata). The current validated snapshot contract has no group
or generic Stash-box attribute relation, so this task deliberately did not
expand catalog ingestion or plugin scope. There is no popularity fallback and
no ANN/Annoy path.

Each build inserts an inactive `model_versions` row plus its projections in a
single transaction, then deactivates the former version and activates the new
version before commit. A failed insert rolls the transaction back and retains
the active version. `GET /v1/recommendations/related` and
`GET /v1/recommendations/for-you` require bearer authentication and return
`model_version`, score, reasons, endpoint-qualified content key, and an
omitted canonical URL when no source template is configured. Cold reads return
an empty item list.

## TDD evidence

### RED: PostgreSQL model build and rollback

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/model -run 'TestBuild(CombinesRatingSessionAndCatalogCandidates|FailedKeepsActiveVersion)' -v
```

Result: failed to compile because `NewBuilder`, `NewRepository`,
`DefaultOWeight`, and `Recommendation` did not yet exist.

### GREEN: model build and rollback

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/model -run TestBuildCombinesRatingSessionAndCatalogCandidates -v
```

Result: PASS.

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/model -run TestFailedBuildKeepsActiveVersion -v
```

Result: PASS. The test uses a database trigger to fail an `item_neighbors`
insert after inactive-version creation, proving transaction rollback retains
the previous active version.

### RED: recommendation HTTP routes

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/httpapi -run TestRecommendationReads -v
```

Result: failed to compile because `httpapi.Dependencies` had no
`RecommendationReader` dependency.

### GREEN: authenticated reads, cold start, URL omission

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/httpapi -run TestRecommendationReads -v
```

Result: PASS (both authenticated metadata/canonical-URL and cold-start/URL
omission tests).

### RED/GREEN: configurable o weight

```bash
go test ./server/internal/config -run TestLoadDefaultsAndValidatesModelOWeight -v
```

RED result: failed to compile because `Config.ModelOWeight` did not exist.

GREEN result: PASS after adding validated `MODEL_O_WEIGHT` configuration with
the `1.5` default.

## Verification

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/... -v
```

Final result: PASS for all server packages, including model, HTTP, migration,
session, catalog, and ingestion coverage.

```bash
go vet ./server/...
```

Final result: PASS.

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q
```

Final result: 48 passed. The initial prescribed `python -m pytest` could not
run because `python` was absent; `python3 -m pytest` had no pytest module; the
available `pytest` runner required the plugin package path. Pytest emitted two
non-failing sandbox cache-write warnings.

```bash
node --test tests/ui/*.test.js
```

Final result: 1 passed.

```bash
git diff --check
```

Final result: clean.

## Commits

- `04b97d9` `feat: build versioned postgres recommendations`
- `docs: record task 7 recommendation evidence` (this report commit)

## Review status

Independent review has not been requested or performed: the Task 7 direction
explicitly prohibits subagents. The implementation and this evidence commit
are ready for the required independent review; no later task has been started.

## Fix Round 1/5

Addressed every scoped reviewer finding without starting plugin work.

- Replaced the raw-rating bonus with deterministic user-mean-centered,
  norm-normalized co-rating similarity and `count / (count + 2)` shrinkage.
- Filtered all model edges through validated `source_scenes`, so related and
  For You output cannot contain uncataloged scenes.
- Added validated metadata candidates for normalized code/title equality,
  shared dates, and similar duration. Groups remain unavailable because the
  schema has no validated group relation.
- Ordered pair construction and personalized accumulation; a permutation test
  proves deterministic projection output.
- Zero ratings are known-but-not-affinity seeds; nullable canonical URLs are
  serialized explicitly as JSON `null`; non-finite `MODEL_O_WEIGHT` values are
  rejected.

### TDD evidence

The focused RED command added model, HTTP, and config regressions before code:
`go test ./server/internal/model ./server/internal/httpapi ./server/internal/config -run 'Test(BuildProjectsOnlyValidatedCatalogScenes|CollaborativeSimilarityIsMeanCenteredNormalizedAndShrunk|BuildProjectionIsDeterministicForInputOrder|ForYouTreatsZeroRatingAsKnownContent|CatalogCandidatesIncludeValidatedSceneAttributes|RecommendationReadsReturnEmptyColdStartAndOmitCanonicalURL|LoadDefaultsAndValidatesModelOWeight)' -v`.

RED result: missing aggregate-similarity/catalog-input APIs, omitted
`canonical_url`, and accepted `NaN`/`+Inf` values. The same focused suite
passed after implementation, including active-version rollback coverage.

### Verification

- `POSTGRES_TEST_DSN=... go test ./server/... -v`: PASS.
- `go vet ./server/...`: PASS.
- `PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q`: 48 passed (two non-failing sandbox cache-write warnings).
- `node --test tests/ui/*.test.js`: 1 passed.
- `git diff --check`: clean.

### Files and commits

- Changed: `server/internal/model/{build.go,build_test.go,repository.go,interfaces.go}`, `server/internal/httpapi/recommendations_test.go`, and `server/internal/config/{config.go,config_test.go}`.
- `eab985f` `fix: harden recommendation model projections`
- `docs: record task 7 fix round 1 evidence` (this report commit)

Concern: group similarity remains intentionally unavailable until a validated
group relation exists in the catalog schema.
