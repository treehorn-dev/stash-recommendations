# Final Hardening Report

Date: 2026-09-02

This follow-up hardening round stayed scoped to the final branch review items
only: bounded outbound HTTP calls in the plugin service/source clients,
executable README privacy smoke coverage, and direct Task 5 regressions for
equal-timestamp session ordering.

## Scope

- `plugin/stashRecommendations/rec_plugin/service_client.py`
- `plugin/stashRecommendations/rec_plugin/source_client.py`
- `plugin/stashRecommendations/tests/test_service_client.py`
- `plugin/stashRecommendations/tests/test_delivery.py`
- `plugin/stashRecommendations/tests/test_source_client.py`
- `plugin/stashRecommendations/tests/test_sync.py`
- `server/internal/session/build_test.go`
- `tests/e2e/test_readme.py`
- `README.md`

## TDD Evidence

### RED

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_service_client.py plugin/stashRecommendations/tests/test_delivery.py plugin/stashRecommendations/tests/test_source_client.py plugin/stashRecommendations/tests/test_sync.py -q
```

Initial result: failed during collection because
`rec_plugin.service_client.HTTP_TIMEOUT_SECONDS` and
`rec_plugin.source_client.HTTP_TIMEOUT_SECONDS` did not exist yet.

```bash
POSTGRES_TEST_DSN=... go test ./server/internal/session -run 'TestRebuildOrdersEqualTimestampLatencyEventsByEventID|TestRebuildOrdersEqualTimestampOBoundedEventsByEventID' -v
POSTGRES_TEST_DSN=... PYTHONPATH=plugin/stashRecommendations pytest tests/e2e/test_readme.py -q
```

Initial result: both failed under the sandbox because loopback PostgreSQL access
was blocked. That confirmed the new regression and README smoke coverage were
wired into executable suites and required the same local verification mode as
the existing PostgreSQL-backed tests.

### GREEN

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_service_client.py plugin/stashRecommendations/tests/test_delivery.py plugin/stashRecommendations/tests/test_source_client.py plugin/stashRecommendations/tests/test_sync.py -q
```

Result: PASS, `23 passed in 4.17s`.

The new Python coverage proves:

- service client POST and GET requests both supply an explicit timeout
- service client timeout failures surface as retryable `OSError`
- delivery worker retries timed-out service calls without quarantine
- source client default transport supplies an explicit timeout
- timed-out source fetches remain isolated to a single metadata key while later
  keys still queue

```bash
POSTGRES_TEST_DSN=... go test ./server/internal/session -run 'TestRebuildOrdersEqualTimestampLatencyEventsByEventID|TestRebuildOrdersEqualTimestampOBoundedEventsByEventID' -v
```

Result: PASS. The new regressions prove both latency and o-bounded projections
use `event_id` as the deterministic tie-break when timestamps are equal.

```bash
POSTGRES_TEST_DSN=... PYTHONPATH=plugin/stashRecommendations pytest tests/e2e/test_readme.py -q
```

Result: PASS, `1 passed in 6.47s`.

The new README smoke proves the documented metadata privacy validation uses the
real persisted `source_snapshots.snapshot` JSON payload and still shows source
credentials stay client-side.

## Verification

```bash
make test
```

Result: PASS. Go server packages passed, Python plugin suite passed with
`119 passed in 4.30s`, and Node UI tests passed `6/6`.

```bash
make test-contract
```

Result: PASS. Go HTTP fixture verification passed and Python contract fixtures
passed with `27 passed in 0.03s`.

```bash
make test-e2e
```

Result: PASS, `2 passed in 11.87s`.

```bash
go vet ./server/...
```

Result: PASS.

```bash
git diff --check
git diff --check origin/main..HEAD
```

Result: both clean.
