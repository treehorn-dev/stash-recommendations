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

## Fix Round 2: Interaction Kinds and Strict Nested Values

### Changed Paths

- `contracts/v1/preference-event.schema.json`: permits only rating-set, rating-remove, play, and o interactions; requires UTC `Z` timestamps.
- `contracts/v1/fixtures/`: adds valid play/o, invalid boolean-rating, invalid engagement-with-rating, invalid non-UTC, and invalid nested snapshot fixtures.
- `server/internal/domain/types.go`: validates only the four V1 interaction kinds, finite ratings, UTC event times, null-free required/nested fields, valid scene dates, and strict interaction JSON payloads.
- `server/internal/domain/types_test.go`: exercises local-field rejection and loads every V1 fixture with Go validation.
- `plugin/stashRecommendations/rec_plugin/contracts.py`: validates malformed nested collections and types as `ValueError` contract errors, and serializes all non-rating-set interactions without `rating`.
- `plugin/stashRecommendations/tests/test_contracts.py`: loads every V1 fixture with Python validation and asserts the schema interaction/UTC boundary.

### RED Evidence

1. `go test ./server/internal/domain -v`

   Output: `FAIL` because the new cross-language parity test referenced a fixture catalog that did not exist and relied on the test binary working directory.

2. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests/test_contracts.py`

   Output: `7 failed, 17 passed`; every failure was a missing fixture (`FileNotFoundError`), establishing the shared-fixture contract gap.

3. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests/test_contracts.py -k schema_restricts`

   Output: `KeyError: 'pattern'`; the shared schema did not require the UTC `Z` suffix enforced by the runtime validators.

### GREEN Evidence

1. `go test ./server/internal/domain -v`

   Output: `PASS`, including all 12 shared fixtures and local-field rejection checks.

2. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests/test_contracts.py`

   Output: `30 passed in 0.04s`.

3. `git diff --check`

   Output: exit `0` with no output.

### Self-Review

- Both languages load every JSON fixture and agree on its valid/invalid result, including invalid boolean rating/duration/career-years values, scalar dates, and `tags: null`.
- Python collection validation converts malformed nested input into `ValueError`; no incidental `TypeError` remains for the required cases.
- Interaction payloads accept exactly `scene.rating.set`, `scene.rating.remove`, `scene.played`, and `scene.o`. Rating is finite numeric `[0,1]` for set and prohibited for the other three kinds.
- Go rejects local IDs, duration, resume, file, and player fields by strict interaction decoding; the shared schema rejects every unlisted interaction field.
- This round contains no Task 3+ changes.

### Commit

- `a4af556 fix: enforce strict interaction contract parity`

## Fix Round 3: Invalid Semantic Date Fixture

### Changed Paths

- `contracts/v1/fixtures/source-snapshot.invalid-date.invalid.json`: adds an impossible but syntactically date-shaped scene date, `2026-02-30`.
- `server/internal/domain/types_test.go`: adds the fixture to the Go snapshot parity table.
- `plugin/stashRecommendations/tests/test_contracts.py`: adds the fixture to the Python snapshot parity table.

### RED Evidence

1. `go test ./server/internal/domain -run TestV1FixturesHaveCrossLanguageContractParity -v`

   Output: `FAIL` because `source-snapshot.invalid-date.invalid.json` did not exist; all existing fixture cases passed.

2. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests/test_contracts.py -k snapshot_fixtures`

   Output: `1 failed, 5 passed`; the sole failure was the missing shared fixture.

### GREEN Evidence

1. `go test ./server/internal/domain -v`

   Output: `PASS`, including `source-snapshot.invalid-date.invalid.json` rejected by the Go validator.

2. `PYTHONPATH=plugin/stashRecommendations /opt/homebrew/bin/pytest -q plugin/stashRecommendations/tests/test_contracts.py`

   Output: `31 passed in 0.06s`.

3. `git diff --check`

   Output: exit `0` with no output.

### Self-Review

- The new fixture remains structurally valid JSON and uses a string matching the `YYYY-MM-DD` shape, so it verifies semantic calendar validation rather than scalar-type rejection.
- Both Go and Python load the same fixture through their existing parity tables and reject it.
- No production, design, plan, or Task 3+ files changed.

### Commit

- `20d5797 test: add invalid source date fixture`
