# Task 6 Report

Implemented Task 6 only: source-authoritative catalog snapshot ingestion,
transactional PostgreSQL UPSERT/projection logic, authenticated
`POST /v1/catalog/snapshots`, fresh-install plus additive migration support for
the widened source catalog schema, and focused/full server verification for
newest-source, stale-update, repeat-upsert, relation projection, and
authentication behavior. No recommendation reads/builds, plugin work, session
changes, or local metadata handling were added.

## Modified Scope

- `server/cmd/recommendations/main.go`
- `server/internal/catalog/snapshots.go`
- `server/internal/catalog/snapshots_test.go`
- `server/internal/httpapi/health.go`
- `server/internal/httpapi/preferences.go`
- `server/internal/httpapi/snapshots.go`
- `server/internal/httpapi/snapshots_test.go`
- `server/internal/store/migrations/001_initial.sql`
- `server/internal/store/migrations/006_source_catalog_projections.sql`
- `server/internal/store/store.go`
- `server/internal/store/store_test.go`

## RED

Command:

```bash
go test ./server/internal/catalog -run TestSnapshotUpsertKeepsNewestSourceVersion -v
```

Output:

```text
github.com/treehorn/stash-recommendations/server/internal/catalog: no non-test Go files in /Users/allenday/src/treehorn-dev/stash-recommendations/.worktrees/stash-recommendations-v0/server/internal/catalog
FAIL	github.com/treehorn/stash-recommendations/server/internal/catalog [build failed]
FAIL
```

## GREEN

Focused newest-source and rejection verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/catalog -run TestSnapshotUpsertKeepsNewestSourceVersion -v
```

```text
=== RUN   TestSnapshotUpsertKeepsNewestSourceVersion
--- PASS: TestSnapshotUpsertKeepsNewestSourceVersion (0.57s)
PASS
ok  	github.com/treehorn/stash-recommendations/server/internal/catalog	0.895s
```

Focused catalog package verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/catalog -v
```

```text
=== RUN   TestSnapshotUpsertKeepsNewestSourceVersion
--- PASS: TestSnapshotUpsertKeepsNewestSourceVersion (0.70s)
=== RUN   TestSnapshotUpsertProjectsRelationsAndCanonicalURL
--- PASS: TestSnapshotUpsertProjectsRelationsAndCanonicalURL (0.81s)
=== RUN   TestSnapshotUpsertIgnoresStaleUpdatesAndAllowsRepeatUpserts
--- PASS: TestSnapshotUpsertIgnoresStaleUpdatesAndAllowsRepeatUpserts (0.59s)
PASS
ok  	github.com/treehorn/stash-recommendations/server/internal/catalog	2.303s
```

Focused HTTP authentication/status verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/httpapi -run 'TestPostSnapshots|TestPostInteractions|TestAuthenticate' -v
```

```text
=== RUN   TestAuthenticateAcceptsOnlyTheOwningAccountKey
--- PASS: TestAuthenticateAcceptsOnlyTheOwningAccountKey (0.40s)
=== RUN   TestPostInteractionsReturnsAcceptedReplayAndConflictStatuses
--- PASS: TestPostInteractionsReturnsAcceptedReplayAndConflictStatuses (0.87s)
=== RUN   TestPostInteractionsRejectsMalformedAndUnauthenticatedRequests
--- PASS: TestPostInteractionsRejectsMalformedAndUnauthenticatedRequests (0.54s)
=== RUN   TestPostInteractionsRejectsTrailingJSONAfterValidEvent
--- PASS: TestPostInteractionsRejectsTrailingJSONAfterValidEvent (0.56s)
=== RUN   TestPostSnapshotsReturnsAcceptedForAuthenticatedValidPayload
--- PASS: TestPostSnapshotsReturnsAcceptedForAuthenticatedValidPayload (0.49s)
=== RUN   TestPostSnapshotsRejectsMalformedAndUnauthenticatedRequests
--- PASS: TestPostSnapshotsRejectsMalformedAndUnauthenticatedRequests (0.59s)
PASS
ok  	github.com/treehorn/stash-recommendations/server/internal/httpapi	3.872s
```

Focused additive migration verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/store -run TestMigrateAddsSourceCatalogProjectionColumnsToExistingStore -v
```

```text
=== RUN   TestMigrateAddsSourceCatalogProjectionColumnsToExistingStore
--- PASS: TestMigrateAddsSourceCatalogProjectionColumnsToExistingStore (0.44s)
PASS
ok  	github.com/treehorn/stash-recommendations/server/internal/store	0.753s
```

Full server verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/... -v
```

```text
?   	github.com/treehorn/stash-recommendations/server/cmd/recommendations	[no test files]
ok  	github.com/treehorn/stash-recommendations/server/internal/auth	(cached)
ok  	github.com/treehorn/stash-recommendations/server/internal/catalog	(cached)
?   	github.com/treehorn/stash-recommendations/server/internal/config	[no test files]
ok  	github.com/treehorn/stash-recommendations/server/internal/domain	(cached)
ok  	github.com/treehorn/stash-recommendations/server/internal/httpapi	5.656s
ok  	github.com/treehorn/stash-recommendations/server/internal/ingest	6.274s
ok  	github.com/treehorn/stash-recommendations/server/internal/session	6.236s
ok  	github.com/treehorn/stash-recommendations/server/internal/store	4.553s
```

Repository verification:

```bash
git diff --check
```

```text
[clean]
```

## Migration Compatibility Evidence

- Fresh installs are covered by `TestMigrateCreatesBaseStorageTables`; the base
  schema now includes the widened source catalog columns and relation-order
  fields in `001_initial.sql`.
- Existing upgraded stores are covered by
  `TestMigrateAddsSourceCatalogProjectionColumnsToExistingStore`, which seeds
  the Task 5-era source catalog schema through `005_session_projections`, reruns
  `Store.Migrate`, and verifies the additive `006_source_catalog_projections`
  migration adds the required scene, performer, and relation-order columns.
- `006_source_catalog_projections.sql` uses `ADD COLUMN IF NOT EXISTS` so fresh
  installs can apply the ordered migration list after `001_initial.sql` without
  duplicate-column failures.

## Commits

- `2396ab8` `feat: upsert source-authoritative catalog snapshots`
- `docs: record task 6 snapshot evidence` (this report commit)

## Verification Limitations

- PostgreSQL-backed verification required escalated local access because the
  sandbox could not connect to `localhost:5432`.

## Fix Round 1

The v1 contract now requires a distinct `source_updated_at` timestamp. The
server persists `captured_at` as fetch time and uses only `source_updated_at`
for snapshot and projection freshness. Older and equal source versions return
before any field or relation mutation, making equal deliveries idempotent no-
ops. Added shared Go/Python fixture coverage for a missing source version and
PostgreSQL regression coverage for delayed delivery, separate persisted times,
and equal-version relation replacement.

TDD evidence: the new catalog tests were added before the contract/storage
change; the prior implementation rejected their new contract field and used
capture time for freshness. The full server rerun initially exposed only a
test-location comparison issue, corrected by comparing timestamp instants.

Verification after the fix:

```text
POSTGRES_TEST_DSN=... go test ./server/... -v: PASS
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q: 47 passed
node --test tests/ui/*.test.js: 1 passed
git diff --check: clean
```

## Fix Round 2

- Store snapshot freshness truncates `source_updated_at` to PostgreSQL's
  microsecond precision before comparison and every source projection write.
  A guarded source-snapshot UPSERT that changes zero rows returns before any
  relation delete/insert operations.
- `POST /v1/catalog/snapshots` limits reads with `http.MaxBytesReader` to 1
  MiB and returns `413` for both declared-length and chunked oversized bodies.
- Snapshot validation rejects every performer appearance whose performer is not
  supplied by the same snapshot, with shared Go/Python fixture coverage.

TDD RED evidence: before implementation, the sub-microsecond collision
regression replaced `tag-1` with `tag-2`; both oversized request tests returned
`400`; and the dangling performer fixture was accepted by Go and Python.

Verification after the fix:

```text
POSTGRES_TEST_DSN=... go test ./server/... -v: PASS
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q: 48 passed
node --test tests/ui/*.test.js: 1 passed
go vet ./server/...: PASS
git diff --check: clean
```
