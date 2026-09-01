# Task 2 Report: Define Shared V1 Contracts

## Changed Files

- `contracts/v1/preference-event.schema.json`: strict V1 preference-event JSON Schema.
- `contracts/v1/source-snapshot.schema.json`: strict V1 Stash source-snapshot JSON Schema.
- `contracts/v1/fixtures/preference-event.valid.json`: valid rating-set event fixture.
- `contracts/v1/fixtures/preference-event.invalid.json`: invalid rating-remove-with-rating fixture.
- `contracts/v1/fixtures/source-snapshot.valid.json`: valid scene and performer snapshot fixture.
- `server/internal/domain/types.go`: Go V1 content key, event, snapshot, and Stash record types with validation.
- `server/internal/domain/types_test.go`: Go normalization, validation, and UUID-version compatibility tests.
- `plugin/stashRecommendations/rec_plugin/contracts.py`: Python V1 content key, event, and snapshot types with validation/serialization.
- `plugin/stashRecommendations/tests/test_contracts.py`: Python removal serialization test.

## RED Evidence

1. `go test ./server/internal/domain`

   Output: `undefined: PreferenceEvent`, `undefined: ContentKey`, and `undefined: PreferenceEventKindSceneRatingSet`; exited `FAIL` before production code existed.

2. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests/test_contracts.py`

   Output: `ModuleNotFoundError: No module named 'rec_plugin.contracts'`; collection failed before production code existed.

3. `go test ./server/internal/domain -run TestPreferenceEventValidateAcceptsUUIDVersionSeven -count=1`

   Output: `event_id must be a UUID`; exited `FAIL`, proving the prior UUID expression incorrectly rejected a valid V1-compatible UUID shape.

## GREEN Evidence

1. `go test ./server/internal/domain -count=1`

   Output: `ok github.com/treehorn/stash-recommendations/server/internal/domain`.

2. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests/test_contracts.py`

   Output: `1 passed`.

3. `go test ./server/...`

   Output: domain and HTTP API packages passed; command package and config had no tests.

4. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests`

   Output: `2 passed`.

5. `jq empty contracts/v1/preference-event.schema.json contracts/v1/source-snapshot.schema.json contracts/v1/fixtures/preference-event.valid.json contracts/v1/fixtures/preference-event.invalid.json contracts/v1/fixtures/source-snapshot.valid.json && git diff --check`

   Output: exit `0` with no output, confirming valid JSON and no whitespace errors.

## Self-Review

- Both schemas use `additionalProperties: false` at every object boundary.
- Preference events require V1, UUID-shaped IDs, sequence at least one, RFC3339 date-time fields, content keys, supported kinds, and an origin.
- Rating set requires a 0-to-1 rating; rating removal prohibits it.
- Normalization lowercases scheme and host, removes exactly one trailing path slash, retains non-root paths such as `/GRAPHQL`, and rejects blank Stash IDs.
- Snapshot schemas model only the required scene, performer, studio, tag, relationship, URL, and remote-image fields. Therefore unsupported paths, files, `rating100`, `play_count`, and `custom_fields` are rejected.
- No database, HTTP, plugin settings, source-query, delivery, or UI code was added.

## Concerns

- The Task 1 Makefile invokes `python`, but this environment has no `python` executable; `python3` also lacks `pytest`. The installed `/opt/homebrew/bin/pytest` launcher uses Python 3.10 and was used with `PYTHONPATH=plugin/stashRecommendations` for all Python verification. The Makefile was not modified because that is outside Task 2 scope.
- No JSON Schema validator dependency is present. JSON syntax was checked with `jq`; schema behavior is encoded in the strict schemas and covered by the Go/Python contract validation tests.

## Fix Round 1: Snapshot Strictness and Go/Python Parity

### Changed Files

- `server/internal/domain/types.go`: added strict nested JSON decoding, snapshot array/required-field validation, and validated snapshot marshaling.
- `server/internal/domain/types_test.go`: added executable privacy-field, required-field, and nil-array serialization regressions.
- `plugin/stashRecommendations/rec_plugin/contracts.py`: added eager strict snapshot construction, nested record validation, schema-valid serialization, canonical UUID checks, and exact integer checks.
- `plugin/stashRecommendations/tests/test_contracts.py`: added executable privacy, snapshot, serialization, and Go/Python parity regressions.

### RED Evidence

1. `go test ./server/internal/domain -run 'TestSourceSnapshot(DecodeRejectsPrivacyFields|ValidateRequiresArraysAndNestedRequiredFields)' -count=1`

   Output: both tests failed because `json.Unmarshal` accepted `paths` and `SourceSnapshot.Validate` accepted missing arrays and nested required fields.

2. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests/test_contracts.py -k 'snapshot_construction or noncanonical_parity'`

   Output: `5 failed`; Python accepted a privacy field, a missing scenes array, `True` schema version, `1.5` sequence, and a UUID without hyphens.

3. `go test ./server/internal/domain -run TestSourceSnapshotMarshalRejectsNilArrays -count=1`

   Output: failed because Go marshaled nil snapshot slices instead of rejecting invalid `null` arrays.

### GREEN Evidence

1. `go test ./server/internal/domain -run 'TestSourceSnapshot(DecodeRejectsPrivacyFields|ValidateRequiresArraysAndNestedRequiredFields|MarshalRejectsNilArrays)' -count=1`

   Output: `ok github.com/treehorn/stash-recommendations/server/internal/domain`.

2. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests/test_contracts.py`

   Output: `11 passed` after the focused change; final full plugin suite output is `12 passed`.

3. `go test ./server/...`

   Output: all server packages passed; command and config packages have no tests.

4. `git diff --check`

   Output: exit `0` with no output.

### Self-Review

- Go now uses `DisallowUnknownFields` at the snapshot, content-key, scene, performer, studio, tag, and performer-appearance boundaries, so prohibited fields cannot be silently discarded.
- Python validates snapshot dictionaries at construction and rejects the same unknown fields, including `paths`, `files`, `rating100`, `play_count`, and `custom_fields`.
- Both implementations reject missing scene IDs, performer IDs/names, absent snapshot arrays, and invalid serialized snapshots.
- Python schema-version and sequence checks use exact `int` types to exclude booleans and floats. UUID validation requires Go-compatible hyphenated UUID text.
