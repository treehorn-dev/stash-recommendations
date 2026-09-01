# Task 4 Report

Implemented Task 4 only: authenticated `POST /v1/events/interactions`,
transactional idempotent ingestion for `scene.rating.set`,
`scene.rating.remove`, `scene.played`, and `scene.o`, body-hash replay
detection, current-preference projection updates for newer rating events only,
and PostgreSQL integration coverage for ordering, removal, replay, engagement
persistence, and HTTP status behavior. No session projections, catalog/model
changes, or plugin changes were made.

## RED

Command:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/ingest -run TestAcceptIgnoresOlderSequenceAndDeduplicatesEvent -v
```

Output:

```text
FAIL    github.com/treehorn/stash-recommendations/server/internal/ingest [build failed]
# github.com/treehorn/stash-recommendations/server/internal/ingest [github.com/treehorn/stash-recommendations/server/internal/ingest.test]
server/internal/ingest/preferences_test.go:23:13: undefined: NewInteractionService
server/internal/ingest/preferences_test.go:47:13: undefined: NewInteractionService
server/internal/ingest/preferences_test.go:66:13: undefined: NewInteractionService
server/internal/ingest/preferences_test.go:74:32: undefined: store.ErrInteractionEventConflict
server/internal/ingest/preferences_test.go:85:13: undefined: NewInteractionService
FAIL
```

Command:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/httpapi -run TestPostInteractionsReturnsAcceptedReplayAndConflictStatuses -v
```

Output:

```text
github.com/treehorn/stash-recommendations/server/internal/ingest: no non-test Go files in /Users/allenday/src/treehorn-dev/stash-recommendations/.worktrees/stash-recommendations-v0/server/internal/ingest
```

## GREEN

Focused verification after implementing the minimal production path:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/ingest -run TestAcceptIgnoresOlderSequenceAndDeduplicatesEvent -v
```

```text
--- PASS: TestAcceptIgnoresOlderSequenceAndDeduplicatesEvent (1.71s)
PASS
ok      github.com/treehorn/stash-recommendations/server/internal/ingest   1.929s
```

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/httpapi -run TestPostInteractionsReturnsAcceptedReplayAndConflictStatuses -v
```

```text
=== RUN   TestPostInteractionsReturnsAcceptedReplayAndConflictStatuses
--- PASS: TestPostInteractionsReturnsAcceptedReplayAndConflictStatuses (3.26s)
PASS
ok      github.com/treehorn/stash-recommendations/server/internal/httpapi  3.469s
```

Expanded package verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/ingest -v
```

```text
=== RUN   TestAcceptIgnoresOlderSequenceAndDeduplicatesEvent
--- PASS: TestAcceptIgnoresOlderSequenceAndDeduplicatesEvent (0.57s)
=== RUN   TestRemoveDeletesCurrentPreference
--- PASS: TestRemoveDeletesCurrentPreference (0.76s)
=== RUN   TestAcceptRejectsChangedReplay
--- PASS: TestAcceptRejectsChangedReplay (0.60s)
=== RUN   TestAcceptPersistsEngagementEventsWithoutTouchingRatings
--- PASS: TestAcceptPersistsEngagementEventsWithoutTouchingRatings (0.50s)
PASS
ok      github.com/treehorn/stash-recommendations/server/internal/ingest   2.676s
```

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/internal/httpapi -v
```

```text
=== RUN   TestAuthenticateAcceptsOnlyTheOwningAccountKey
--- PASS: TestAuthenticateAcceptsOnlyTheOwningAccountKey (0.43s)
=== RUN   TestHealthz
--- PASS: TestHealthz (0.00s)
=== RUN   TestPostInteractionsReturnsAcceptedReplayAndConflictStatuses
--- PASS: TestPostInteractionsReturnsAcceptedReplayAndConflictStatuses (1.01s)
=== RUN   TestPostInteractionsRejectsMalformedAndUnauthenticatedRequests
--- PASS: TestPostInteractionsRejectsMalformedAndUnauthenticatedRequests (0.60s)
PASS
ok      github.com/treehorn/stash-recommendations/server/internal/httpapi  2.258s
```

Task 4 server-wide verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' \
go test ./server/... -v
```

```text
PASS: server/internal/auth
PASS: server/internal/domain
PASS: server/internal/httpapi
PASS: server/internal/ingest
PASS: server/internal/store
```

Repository verification:

```bash
POSTGRES_TEST_DSN='postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable' make test
```

```text
ok      github.com/treehorn/stash-recommendations/server/internal/httpapi
ok      github.com/treehorn/stash-recommendations/server/internal/ingest
ok      github.com/treehorn/stash-recommendations/server/internal/store
python -m pytest plugin/stashRecommendations/tests
make: python: No such file or directory
make: *** [test-python] Error 1
```

Follow-up direct verification for the unaffected non-Go suites:

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q
```

```text
46 passed in 0.03s
```

```bash
node --test tests/ui/*.test.js
```

```text
1..1
# pass 1
```

## Commits

- `1320348` `feat: ingest idempotent scene interactions`

## Verification Limitations

- Local PostgreSQL integration commands required escalated local access from the
  Codex sandbox.
- The bootstrap `Makefile` still calls `python -m pytest`, but this environment
  does not provide a `python` alias. I verified the Python suite directly with
  `PYTHONPATH=plugin/stashRecommendations pytest ...` instead of changing
  bootstrap scope during Task 4.
- The plan references `make test-server`, but this target does not exist in the
  current `Makefile`; the equivalent verification used here was
  `POSTGRES_TEST_DSN=... go test ./server/... -v`.
