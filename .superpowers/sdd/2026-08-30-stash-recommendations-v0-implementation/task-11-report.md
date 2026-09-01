# Task 11 Report

User-directed Task 11 implemented only: the final contract/E2E/docs verification
slice for the stash recommendations v0 plan. Scope stayed bounded to shared
fixture verification, mock Stash-box end-to-end coverage, README/operator
documentation, and `Makefile` verification targets.

## Scope

- `server/internal/httpapi/contract_test.go`
- `plugin/stashRecommendations/tests/test_contract_fixtures.py`
- `tests/__init__.py`
- `tests/e2e/__init__.py`
- `tests/e2e/mock_stash_box.py`
- `tests/e2e/test_rating_to_recommendation.py`
- `README.md`
- `Makefile`

The new Go contract test posts shared v1 preference-event and source-snapshot
fixtures through the authenticated HTTP handlers and proves valid fixtures are
accepted while invalid fixtures are rejected with `400`.

The new Python contract fixture target loads the same fixture set through the
plugin-side contract constructors and validation boundary. The new E2E harness
starts a real Go service against PostgreSQL, captures a real raw-plugin
`capture-rating` event, proxies mock Stash-box metadata through the plugin's
source boundary, builds a recommendation model through an operator helper, and
verifies related recommendations are returned without leaking the source API
key into persisted snapshot payloads.

`Makefile` now exposes `test-contract` and `test-e2e`, uses a loopback-safe
default PostgreSQL DSN, and passes that DSN into the Go test suite. `README.md`
documents PostgreSQL startup, v0 API-key bootstrap, plugin configuration,
explicit rating/engagement/metadata sync tasks, status output, model build,
privacy boundaries, and a manual smoke checklist.

## TDD Evidence

### RED

```bash
make test-contract
make test-e2e
```

Initial result: both failed with:

- `make: *** No rule to make target 'test-contract'.  Stop.`
- `make: *** No rule to make target 'test-e2e'.  Stop.`

That proved the Task 11 verification targets did not exist yet.

After the targets were added, the new end-to-end suite failed until the final
Task 11 surfaces were implemented:

- local PostgreSQL verification required loopback-safe DSN wiring and
  unsandboxed verification
- the mock Stash-box harness needed an HTTPS-valid content-key boundary plus
  loopback transport rewriting
- the model build step needed an explicit operator helper path for the E2E flow
- the privacy assertion needed to inspect the persisted `source_snapshots.snapshot`
  column rather than a nonexistent raw payload column

Each failure moved forward only after the corresponding missing Task 11
verification surface was implemented.

### GREEN

```bash
make test-contract
```

Result: PASS. Go HTTP contract fixture suite passed for valid and invalid event
plus snapshot fixtures, and Python fixture parity passed with `27 passed in 0.05s`.

```bash
make test-e2e
```

Result: PASS, `1 passed in 10.69s`.

The E2E assertion verifies:

- one delivered rating event plus two delivered snapshots
- a built model version is returned by the service
- related recommendations include the second shared-performer scene
- mock Stash-box requests carry the source API key
- persisted snapshot payloads do not contain that source API key

## Verification

```bash
make test
```

Result: PASS.

- Go server packages passed, including `catalog`, `httpapi`, `ingest`, `model`,
  `session`, and `store`
- Python plugin suite passed with `114 passed in 2.25s`
- Node UI suite passed with `6 tests`, `6 pass`, `0 fail`

```bash
make test-contract
```

Result: PASS.

```bash
make test-e2e
```

Result: PASS, `1 passed in 10.69s`.

```bash
go vet ./server/...
```

Result: PASS.

```bash
git diff --check
```

Result: clean.

## Manual Smoke

Live Stash manual smoke was attempted only as an availability probe.

```bash
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9999/
```

Result: `curl: (7) Failed to connect to 127.0.0.1 port 9999`.

No local Stash instance was reachable in this environment, so the README manual
smoke flow could not be executed live.
