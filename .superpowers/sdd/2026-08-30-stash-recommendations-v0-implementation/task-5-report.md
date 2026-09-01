# Task 5 Report

Implemented Task 5 only: rebuildable latency and `scene.o`-bounded engagement
session projections, fresh-install plus additive upgrade migrations for the new
session tables, and PostgreSQL integration coverage for the required boundary
cases and migration compatibility. No catalog, model, plugin, or HTTP API work
was added.

## Modified Scope

- `server/internal/session/build.go`
- `server/internal/session/build_test.go`
- `server/internal/store/migrations/001_initial.sql`
- `server/internal/store/migrations/005_session_projections.sql`
- `server/internal/store/store.go`
- `server/internal/store/store_test.go`

## RED

Command:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/session -v
```

Output:

```text
# github.com/treehorn/stash-recommendations/server/internal/session [github.com/treehorn/stash-recommendations/server/internal/session.test]
server/internal/session/build_test.go:42:32: undefined: projectionTypeLatency
server/internal/session/build_test.go:43:32: undefined: projectionTypeLatency
server/internal/session/build_test.go:44:45: undefined: projectionTypeLatency
server/internal/session/build_test.go:73:32: undefined: projectionTypeLatency
server/internal/session/build_test.go:74:32: undefined: projectionTypeLatency
server/internal/session/build_test.go:75:45: undefined: projectionTypeLatency
server/internal/session/build_test.go:122:32: undefined: projectionTypeOBounded
server/internal/session/build_test.go:123:32: undefined: projectionTypeOBounded
server/internal/session/build_test.go:124:32: undefined: projectionTypeOBounded
server/internal/session/build_test.go:197:43: undefined: Builder
server/internal/session/build_test.go:124:32: too many errors
FAIL    github.com/treehorn/stash-recommendations/server/internal/session [build failed]
FAIL
```

Command:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/store -run TestMigrateAddsSessionProjectionTablesToExistingStore -v
```

Output:

```text
=== RUN   TestMigrateAddsSessionProjectionTablesToExistingStore
    store_test.go:134:
        Error:       Should be true
--- FAIL: TestMigrateAddsSessionProjectionTablesToExistingStore (0.50s)
FAIL
FAIL    github.com/treehorn/stash-recommendations/server/internal/store    1.061s
FAIL
```

## GREEN

Focused verification after the minimal implementation:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/session -v
```

```text
=== RUN   TestRebuildKeepsTwoHourGapInSingleLatencySession
--- PASS: TestRebuildKeepsTwoHourGapInSingleLatencySession (0.49s)
=== RUN   TestRebuildStartsNewLatencySessionWhenGapExceedsTwoHours
--- PASS: TestRebuildStartsNewLatencySessionWhenGapExceedsTwoHours (0.54s)
=== RUN   TestRebuildClosesOBoundedSessionsAndTreatsOrphanOAsClosedSession
--- PASS: TestRebuildClosesOBoundedSessionsAndTreatsOrphanOAsClosedSession (0.33s)
=== RUN   TestRebuildCollapsesOnlyConsecutiveRepeats
--- PASS: TestRebuildCollapsesOnlyConsecutiveRepeats (0.36s)
PASS
ok      github.com/treehorn/stash-recommendations/server/internal/session  2.533s
```

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/store -run TestMigrateAddsSessionProjectionTablesToExistingStore -v
```

```text
=== RUN   TestMigrateAddsSessionProjectionTablesToExistingStore
--- PASS: TestMigrateAddsSessionProjectionTablesToExistingStore (0.58s)
PASS
ok      github.com/treehorn/stash-recommendations/server/internal/store    1.092s
```

Fresh full-server verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/... -v
```

```text
PASS: server/internal/auth
PASS: server/internal/domain
PASS: server/internal/httpapi
PASS: server/internal/ingest
PASS: server/internal/session
PASS: server/internal/store
```

Repository verification:

```bash
git diff --check
```

```text
[clean]
```

## Migration Compatibility Evidence

- Fresh installs are covered by `TestMigrateCreatesBaseStorageTables`, which now
  asserts both `session_projections` and `session_projection_items` are created
  from `001_initial.sql`.
- Existing upgraded stores are covered by
  `TestMigrateAddsSessionProjectionTablesToExistingStore`, which seeds a schema
  with `001_initial` through `004_revoke_legacy_api_keys` already recorded,
  reruns `Store.Migrate`, and verifies both session tables now exist under the
  new ordered `005_session_projections` migration.
- The legacy API-key upgrade test now also proves the migration ledger advances
  through `005_session_projections` after upgrading a pre-identifier schema.

## Commits

- `a5fc8a9` `feat: build engagement session projections`
- `8f7131f` `docs: record task 5 session evidence`

## Verification Limitations

- PostgreSQL integration commands required escalated local access because the
  default sandbox could not connect to `localhost:5432`.
- The plan still refers to `make test-server`, but this repository currently
  exposes `go test ./server/...` instead; that direct command was used for the
  required fresh verification.

## Fix Round 1

Scope: fixed the same-account concurrent rebuild race only. `session.Builder`
now serializes rebuilds per account inside the rebuild transaction before it
allocates the next projection version. The equal-timestamp event-ID ordering
coverage note remains deferred and was not expanded into this fix round.

### RED

Command:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/session -run TestRebuildConcurrentCallsAllocateDistinctProjectionVersions -v
```

Output:

```text
# github.com/treehorn/stash-recommendations/server/internal/session [github.com/treehorn/stash-recommendations/server/internal/session.test]
server/internal/session/build_test.go:209:10: builder.afterVersionAllocated undefined (type *Builder has no field or method afterVersionAllocated)
FAIL    github.com/treehorn/stash-recommendations/server/internal/session [build failed]
FAIL
```

### GREEN

Focused regression verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/session -run TestRebuildConcurrentCallsAllocateDistinctProjectionVersions -v
```

```text
=== RUN   TestRebuildConcurrentCallsAllocateDistinctProjectionVersions
--- PASS: TestRebuildConcurrentCallsAllocateDistinctProjectionVersions (2.57s)
PASS
ok      github.com/treehorn/stash-recommendations/server/internal/session  3.057s
```

Affected package verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/session -v
```

```text
=== RUN   TestRebuildKeepsTwoHourGapInSingleLatencySession
--- PASS: TestRebuildKeepsTwoHourGapInSingleLatencySession (0.55s)
=== RUN   TestRebuildStartsNewLatencySessionWhenGapExceedsTwoHours
--- PASS: TestRebuildStartsNewLatencySessionWhenGapExceedsTwoHours (0.58s)
=== RUN   TestRebuildClosesOBoundedSessionsAndTreatsOrphanOAsClosedSession
--- PASS: TestRebuildClosesOBoundedSessionsAndTreatsOrphanOAsClosedSession (0.51s)
=== RUN   TestRebuildCollapsesOnlyConsecutiveRepeats
--- PASS: TestRebuildCollapsesOnlyConsecutiveRepeats (0.44s)
=== RUN   TestRebuildConcurrentCallsAllocateDistinctProjectionVersions
--- PASS: TestRebuildConcurrentCallsAllocateDistinctProjectionVersions (1.60s)
PASS
ok      github.com/treehorn/stash-recommendations/server/internal/session  3.908s
```

Fresh server verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/... -v
```

```text
PASS: server/internal/auth
PASS: server/internal/domain
PASS: server/internal/httpapi
PASS: server/internal/ingest
PASS: server/internal/session
PASS: server/internal/store
```

Repository verification:

```bash
git diff --check
```

```text
[clean]
```

### Commits

- `7f0fbcd` `fix: serialize concurrent session rebuilds`
- Pending evidence commit

### Verification Limitations

- PostgreSQL integration commands again required escalated local access because
  the default sandbox could not reach `localhost:5432`.
