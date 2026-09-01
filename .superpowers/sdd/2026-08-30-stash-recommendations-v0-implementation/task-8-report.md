# Task 8 Report

Implemented Task 8 only: raw Python plugin infrastructure, HTTPS settings,
local Stash GraphQL client, durable SQLite outbox/status, manifest task and
hook declarations, and a hook path that queues generic hook work without
performing rating capture, delivery, metadata proxying, or UI work.

## Scope

- `plugin/stashRecommendations/stashRecommendations.yml`
- `plugin/stashRecommendations/recommendations.py`
- `plugin/stashRecommendations/rec_plugin/settings.py`
- `plugin/stashRecommendations/rec_plugin/stash_client.py`
- `plugin/stashRecommendations/rec_plugin/outbox.py`
- `plugin/stashRecommendations/tests/test_settings.py`
- `plugin/stashRecommendations/tests/test_stash_client.py`
- `plugin/stashRecommendations/tests/test_outbox.py`
- `Makefile`

The manifest now declares raw Python execution, plugin settings
`service_url`, `api_key`, and `show_remote_results`, task modes
`sync-ratings`, `sync-engagement`, `sync-metadata`, `deliver-outbox`, and
`status`, plus a `Scene.Update.Post` hook routed through `capture-rating`.

`Settings.from_plugin_config` accepts plugin configuration from local Stash
GraphQL, normalizes HTTPS service URLs, trims the service API key, and emits a
redacted status view. `StashClient` talks only to the local Stash GraphQL
endpoint using the provided session cookie and supports scene lookup, rated
scene pagination, recorded `play_history`/`o_history` iteration, scene
StashID normalization, configured Stash-box reads, and plugin-config reads.

`Outbox` persists preference events, snapshots, and queued hook work in SQLite
with retry state, quarantine state, capped exponential backoff, and separate
rating/play/o/snapshot/hook counters in status output. The raw entrypoint
currently queues only generic hook work for `capture-rating`; it does not yet
materialize rating or engagement events and does not deliver anything.

## TDD Evidence

### RED

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_settings.py plugin/stashRecommendations/tests/test_stash_client.py plugin/stashRecommendations/tests/test_outbox.py -q
```

Result: collection failed because `rec_plugin.settings`,
`rec_plugin.stash_client`, and `rec_plugin.outbox` did not exist.

### GREEN

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_settings.py plugin/stashRecommendations/tests/test_stash_client.py plugin/stashRecommendations/tests/test_outbox.py -q
```

Result: PASS, `9 passed in 0.26s`.

## Verification

```bash
make test-plugin
```

Result: PASS, `60 passed in 0.25s`.

```bash
git diff --check
```

Result: clean.

## Review Status

Independent review has not been performed. This report stops at the Task 8
implementer checkpoint, with Task 9 and Task 10 untouched.
