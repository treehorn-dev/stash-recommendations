# Task 9 Report

Implemented Task 9 only: rating hook capture fanout, explicit rating remove
events, historical and incremental `play_history` / `o_history` import,
configured Stash-box source proxying with rate limiting, Stash-box schema
snapshot mapping, explicit sync confirmation/counts, and raw plugin entrypoint
integration for these flows. No delivery worker or UI work was added.

## Scope

- `plugin/stashRecommendations/recommendations.py`
- `plugin/stashRecommendations/rec_plugin/capture.py`
- `plugin/stashRecommendations/rec_plugin/source_client.py`
- `plugin/stashRecommendations/rec_plugin/snapshots.py`
- `plugin/stashRecommendations/rec_plugin/sync.py`
- `plugin/stashRecommendations/rec_plugin/stash_client.py`
- `plugin/stashRecommendations/tests/test_capture.py`
- `plugin/stashRecommendations/tests/test_source_client.py`
- `plugin/stashRecommendations/tests/test_snapshots.py`
- `plugin/stashRecommendations/tests/test_sync.py`
- `plugin/stashRecommendations/tests/test_stash_client.py`
- `plugin/stashRecommendations/tests/test_outbox.py`

`capture-rating` now materializes one `scene.rating.set` or
`scene.rating.remove` event per external StashID only when `rating100` is in
the hook `inputFields`, using local scene lookup for the current rating and
never attempting service delivery. `sync-ratings` previews and then queues a
full explicit rating fanout, `sync-engagement` previews and then queues only
new `scene.played` / `scene.o` events from recorded local histories, and
`sync-metadata` previews and then fetches only configured Stash-box sources to
queue schema-limited source snapshots with `source_updated_at`.

`SourceClient` normalizes configured endpoints, ignores unconfigured sources,
enforces per-endpoint request pacing, and sends Stash-box GraphQL queries using
only the source API key from local Stash configuration. `to_source_snapshot()`
maps only approved scene/performer fields into the shared contract and rejects
missing source update timestamps. `SyncState` persists a client identity,
monotonic sequence allocation, and seen history-event IDs so repeated sync runs
stay incremental.

## TDD Evidence

### RED

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_stash_client.py plugin/stashRecommendations/tests/test_sync.py -q
```

Result before the final fix: `2 failed, 8 passed`.

Failures were:
- `StashClient.find_scene()` did not request `rating100`, so the hook path
  could not reliably distinguish set vs remove.
- `build_history_event_id()` ignored `client_id`, so two clients could produce
  the same deterministic engagement event ID for identical content/timestamps.

### GREEN

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_stash_client.py plugin/stashRecommendations/tests/test_sync.py -q
```

Result after the fix: PASS, `10 passed in 0.06s`.

## Verification

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_capture.py plugin/stashRecommendations/tests/test_source_client.py plugin/stashRecommendations/tests/test_snapshots.py plugin/stashRecommendations/tests/test_sync.py -q
```

Result: PASS, `13 passed in 0.09s`.

```bash
make test-plugin
```

Result: PASS, `76 passed in 0.14s`.

```bash
git diff --check
```

Result: clean.

## Review Status

Independent review has not been performed. This report stops at the Task 9
implementer checkpoint, with delivery/UI and end-to-end work left for later
tasks.

## Fix Round 1

External review found one scoped Task 9 gap: the configured Stash-box
`findScene` query did not request scene group relationships, so group-derived
catalog signals could never enter the captured source snapshot even though the
shared contract, snapshot mapper, and downstream model support them.

### RED

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_source_client.py -q
```

Result before the fix: FAIL, `2 failed, 1 passed`.

Failures proved:
- the configured source query text omitted `groups`
- schema-compatible `groups { group { id name } }` relationships were not
  asserted anywhere in the source client contract

Snapshot regression coverage was added at the same time and already passed,
confirming the mapper retained the approved relationship shape once present.

### GREEN

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_source_client.py plugin/stashRecommendations/tests/test_snapshots.py -q
```

Result after the fix: PASS, `5 passed in 0.04s`.

### Verification

```bash
make test-plugin
```

Result: PASS, `77 passed in 0.16s`.

```bash
git diff --check
```

Result: clean.
