# Task 10 Report

User-directed Task 10 implemented only: authenticated service delivery and
status handling, raw plugin read proxies for recommendation fetches, and the
Stash UI route/nav/scene-tab surfaces with local-first recommendation
rendering. End-to-end tests, README/docs, and Task 11 work were not touched.

Note: the written plan labels this delivery/UI slice as Task 9. The user
explicitly drove it as Task 10 and explicitly deferred the plan's E2E/docs
slice, so this report follows the user-directed numbering while preserving the
scope actually implemented.

## Scope

- `plugin/stashRecommendations/recommendations.py`
- `plugin/stashRecommendations/stashRecommendations.yml`
- `plugin/stashRecommendations/rec_plugin/outbox.py`
- `plugin/stashRecommendations/rec_plugin/delivery.py`
- `plugin/stashRecommendations/rec_plugin/service_client.py`
- `plugin/stashRecommendations/rec_plugin/status.py`
- `plugin/stashRecommendations/tests/test_delivery.py`
- `plugin/stashRecommendations/ui/recommendations.js`
- `plugin/stashRecommendations/ui/recommendations.css`
- `tests/ui/recommendations.test.js`

`Outbox` now persists delivery pause state alongside pending/quarantined/
delivered counters and supports Retry-After-aware retry scheduling. The new
`DeliveryWorker` maps service responses exactly within the approved scope:
`200/202` acknowledge, `401/403` pause delivery, `400/409/422` quarantine,
`429` retries using server guidance, and network/`5xx` failures retry without
quarantine. Snapshot rows deliver independently from preference-event rows.

`ServiceClient` sends authenticated preference-event, snapshot, related, and
For You requests over HTTPS using the locally configured recommendation API
key, while `status.py` emits only redacted configuration state.

The browser module registers:
- scene-page recommendations tab
- `/plugins/stash-recommendations` route
- main-nav "For You" entry

The UI fetches status and recommendation reads through raw plugin operations,
never exposing the API key to markup. It resolves returned content keys back to
local Stash scenes by default and only shows remote canonical links when the
plugin setting enables them. Loading, configuration, paused-authentication, and
cold-start states are rendered explicitly, and CSS stays scoped beneath
`.stash-recommendations`.

## TDD Evidence

### Python RED

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_delivery.py -q
```

Initial result: collection failed with
`ModuleNotFoundError: No module named 'rec_plugin.delivery'`.

After the first green pass, expanded proxy tests failed with:
- `ValueError: unsupported mode: fetch-related`
- `ValueError: unsupported mode: fetch-for-you`

These failures proved the delivery worker/modules and authenticated raw
read-proxy modes did not yet exist.

### Python GREEN

```bash
PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_delivery.py -q
```

Result after implementation: PASS, `6 passed in 0.07s`.

### UI RED

```bash
node --test tests/ui/recommendations.test.js
```

Initial result: `MODULE_NOT_FOUND` for
`plugin/stashRecommendations/ui/recommendations.js`.

This proved the route/nav/scene-tab recommendation UI module did not yet
exist.

### UI GREEN

```bash
node --test tests/ui/recommendations.test.js
```

Result after implementation: PASS, `5 passed`.

## Verification

```bash
make test-plugin
```

Result: PASS, `85 passed in 2.26s`.

```bash
make test-ui
```

Result: PASS, `6 passed`.

```bash
make test-go
```

Result: PASS, all `server/...` Go packages green.

```bash
git diff --check
```

Result: clean.

## Review Status

Independent review has not been performed. This report stops at the Task 10
implementer checkpoint, with the next step being the scoped review checkpoint
and E2E/docs still deferred exactly as instructed.
