# Stash Recommendations v0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Build a Stash plugin and Go/PostgreSQL service that ingest explicit ratings, client-proxy source Stash-box metadata, and surface related and For You recommendations.

**Architecture:** A Python raw-task Stash plugin handles hooks, sync, source queries, and a SQLite outbox. A Go HTTP server persists events, Stash-box-schema snapshots, and versioned recommendation projections in PostgreSQL. Domain interfaces isolate future interaction-system and ANN changes.

**Tech Stack:** Go 1.22+, net/http, pgx/v5, PostgreSQL 16, Python 3.11+ with pytest, SQLite, Node 20+ built-in test runner, Stash raw plugins and UI Plugin API.

**Spec:** docs/superpowers/specs/2026-08-30-stash-recommendations-design.md

## Global Constraints

- A content key is exactly { endpoint, stash_id }. Normalize endpoint scheme/host casing and one trailing slash while retaining /graphql.
- Fan each explicit local rating100 to all local stash_ids. Send rating100 / 100 in [0,1]. Clearing sends scene.rating.remove without rating.
- Never upload local titles, tags, paths, files, hashes, custom fields, organization, playback, o-counter, or source credentials.
- Query source metadata only from the plugin through authenticated configured Stash-box endpoints. Upload only Stash-box-schema snapshots and remote URL/image references.
- Hooks enqueue SQLite work only. Delivery and source fetches are task-driven and independent.
- Store server API keys only as Argon2id hashes. Registration/invites, credential overrides, playback/o capture, scheduled source sync, media proxying, and ANN indexing are out of scope.
- UI defaults to locally resolvable scenes. Remote-only links require the enabled setting and a returned canonical URL.
- Use TDD in every task. Stop at review checkpoints.

---

## File Structure

~~~
contracts/v1/                           JSON schemas and shared fixtures.
server/cmd/recommendations/             Startup and model-build command.
server/internal/domain/                 Content/event/snapshot types.
server/internal/auth/                   API-key hashing and auth middleware.
server/internal/store/                  pgx repositories and migrations.
server/internal/ingest/                 Event validation and preference projection.
server/internal/catalog/                Snapshot UPSERT and catalog source.
server/internal/model/                  Batch recommendation builder.
server/internal/httpapi/                HTTP handlers.
plugin/stashRecommendations/            Manifest, raw worker, JS UI.
plugin/stashRecommendations/rec_plugin/ Settings, Stash/source/service clients, outbox, sync.
plugin/stashRecommendations/tests/      Pytest tests.
tests/ui/                               Node UI contract tests.
tests/e2e/                              Mock Stash-box and E2E tests.
~~~

### Task 1: Bootstrap the Monorepo

**Files:**
- Create: go.mod
- Create: server/cmd/recommendations/main.go
- Create: server/internal/config/config.go
- Create: server/internal/httpapi/health.go
- Create: server/internal/httpapi/health_test.go
- Create: plugin/stashRecommendations/rec_plugin/__init__.py
- Create: plugin/stashRecommendations/tests/test_smoke.py
- Create: package.json
- Create: tests/ui/smoke.test.js
- Create: docker-compose.yml
- Create: Makefile
- Create: .gitignore

**Interfaces:** config.Load() returns HTTPAddr and DatabaseURL. httpapi.NewMux() returns an http.Handler with GET /healthz. make test runs Go, Python, and Node tests.

- [x] **Step 1: Write the failing health test**

~~~go
func TestHealthz(t *testing.T) {
    recorder := httptest.NewRecorder()
    httpapi.NewMux().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
    require.Equal(t, http.StatusOK, recorder.Code)
    require.JSONEq(t, "{\"status\":\"ok\"}", recorder.Body.String())
}
~~~

- [x] **Step 2: Run it and verify it fails**

Run: go test ./server/internal/httpapi -run TestHealthz -v

Expected: FAIL because NewMux does not exist.

- [x] **Step 3: Implement the smallest runnable server**

~~~go
func NewMux() http.Handler {
    mux := http.NewServeMux()
    mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte("{\"status\":\"ok\"}"))
    })
    return mux
}
~~~

Create a PostgreSQL 16 compose service at port 5432 using database/user/password stash_recommendations. Makefile targets run go test ./server/..., python -m pytest plugin/stashRecommendations/tests, and node --test tests/ui.

- [x] **Step 4: Verify the bootstrap**

Run: make test

Expected: PASS for all smoke suites.

- [x] **Step 5: Commit**

~~~text
git add .gitignore Makefile docker-compose.yml go.mod go.sum package.json server plugin tests
git commit -m "chore: bootstrap stash recommendations monorepo"
~~~

### Task 2: Define Shared V1 Contracts

**Files:**
- Create: contracts/v1/preference-event.schema.json
- Create: contracts/v1/source-snapshot.schema.json
- Create: contracts/v1/fixtures/preference-event-valid.json
- Create: contracts/v1/fixtures/preference-event-remove-valid.json
- Create: contracts/v1/fixtures/preference-event-invalid-rating.json
- Create: contracts/v1/fixtures/source-snapshot-valid.json
- Create: server/internal/domain/types.go
- Create: server/internal/domain/types_test.go
- Create: plugin/stashRecommendations/rec_plugin/contracts.py
- Create: plugin/stashRecommendations/tests/test_contracts.py

**Interfaces:** ContentKey.Normalize(endpoint, stashID); Go/Python PreferenceEvent and SourceSnapshot; Validate methods.

- [ ] **Step 1: Write failing cross-language tests**

~~~go
func TestRatingSetNormalizesAndValidates(t *testing.T) {
    event := domain.PreferenceEvent{
        SchemaVersion: 1, EventID: uuid.New(), ClientID: uuid.New(), Sequence: 7,
        Kind: domain.RatingSet,
        Content: domain.ContentKey{Endpoint: "HTTPS://BOX.EXAMPLE/GRAPHQL/", StashID: "scene-1"},
        Rating: ptr(0.75), OccurredAt: time.Now().UTC(), Origin: "hook",
    }
    require.NoError(t, event.Validate())
    require.Equal(t, "https://box.example/GRAPHQL", event.Content.Endpoint)
}
~~~

~~~python
def test_rating_remove_omits_rating() -> None:
    event = PreferenceEvent.rating_remove(EVENT_ID, CLIENT_ID, 3, ContentKey("https://box.example/graphql", "scene-1"))
    assert "rating" not in event.to_dict()
~~~

- [ ] **Step 2: Run tests and verify failure**

Run: go test ./server/internal/domain -run TestRatingSetNormalizesAndValidates -v; python -m pytest plugin/stashRecommendations/tests/test_contracts.py -q

Expected: FAIL because domain types do not exist.

- [ ] **Step 3: Implement strict schemas and types**

Use schemas with additionalProperties false. Require v1 UUID event/client IDs, sequence >= 1, RFC3339 time, valid content key, and kinds scene.rating.set/remove. Set requires rating [0,1]; remove prohibits rating. Snapshot records include Stash-box scene/performer/studio/tag/relationship fields only. Reject paths, files, rating100, play_count, and custom_fields.

- [ ] **Step 4: Verify contract tests**

Run: go test ./server/internal/domain -v; python -m pytest plugin/stashRecommendations/tests/test_contracts.py -q

Expected: PASS including all invalid fixtures.

- [ ] **Step 5: Commit**

~~~text
git add contracts server/internal/domain plugin/stashRecommendations/rec_plugin/contracts.py plugin/stashRecommendations/tests/test_contracts.py
git commit -m "feat: define versioned recommendation contracts"
~~~

### Task 3: Add PostgreSQL Identity, Storage, and Disassociation

**Files:**
- Create: server/internal/store/migrations/001_initial.sql
- Create: server/internal/store/store.go
- Create: server/internal/store/store_test.go
- Create: server/internal/auth/keys.go
- Create: server/internal/auth/keys_test.go
- Create: server/internal/httpapi/auth.go
- Create: server/internal/httpapi/disassociate.go
- Create: server/internal/httpapi/disassociate_test.go

**Interfaces:** auth.HashAPIKey(string) and VerifyAPIKey(hash, plaintext); store.AccountRepository.Authenticate; POST /v1/account/disassociate returns 204.

- [ ] **Step 1: Write the failing disassociation integration test**

~~~go
func TestDisassociateRevokesKeysAndRemovesAccountLinkage(t *testing.T) {
    account := fixtureAccount(t, db)
    response := authenticated(t, server, account.PlaintextKey, http.MethodPost, "/v1/account/disassociate", nil)
    require.Equal(t, http.StatusNoContent, response.Code)
    require.False(t, canAuthenticate(t, db, account.PlaintextKey))
    require.Zero(t, accountPreferenceCount(t, db, account.ID))
    require.Positive(t, deidentifiedInteractionCount(t, db))
}
~~~

- [ ] **Step 2: Run it and verify failure**

Run: POSTGRES_TEST_DSN=postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable go test ./server/internal/httpapi -run TestDisassociateRevokesKeysAndRemovesAccountLinkage -v

Expected: FAIL because migrations and endpoint do not exist.

- [ ] **Step 3: Implement the identity boundary**

Create accounts, api_keys, deidentified_subjects, preference_events, current_preferences, source_snapshots, source catalog tables, model_versions, item_neighbors, and user_recommendations. Hash keys using Argon2id. In one transaction, disassociation revokes all keys, rewrites retained interaction references to a deidentified subject, deletes account linkage, and returns 204. Bearer values must never be logged.

- [ ] **Step 4: Verify server tests**

Run: make test-server

Expected: PASS against PostgreSQL.

- [ ] **Step 5: Commit**

~~~text
git add server/internal/store server/internal/auth server/internal/httpapi
git commit -m "feat: add account storage and disassociation"
~~~

### Task 4: Ingest Idempotent Rating Events

**Files:**
- Create: server/internal/ingest/preferences.go
- Create: server/internal/ingest/preferences_test.go
- Create: server/internal/httpapi/preferences.go
- Create: server/internal/httpapi/preferences_test.go
- Modify: server/internal/store/store.go

**Interfaces:** PreferenceService.Accept(ctx, accountID, event) returns accepted; POST /v1/events/preferences returns 202 new, 200 exact replay, and 409 changed replay.

- [ ] **Step 1: Write failing ordering and remove tests**

~~~go
func TestAcceptIgnoresOlderSequenceAndDeduplicatesEvent(t *testing.T) {
    accept(t, account, ratingSet("event-1", 2, 1.0))
    accept(t, account, ratingSet("event-2", 1, 0.2))
    require.Equal(t, 1.0, currentRating(t, db, account, key))
    require.False(t, accept(t, account, ratingSet("event-1", 2, 1.0)).Accepted)
}

func TestRemoveDeletesCurrentPreference(t *testing.T) {
    accept(t, account, ratingSet("event-1", 1, 0.8))
    accept(t, account, ratingRemove("event-2", 2))
    require.False(t, hasCurrentPreference(t, db, account, key))
}
~~~

- [ ] **Step 2: Run them and verify failure**

Run: go test ./server/internal/ingest -run TestAcceptIgnoresOlderSequenceAndDeduplicatesEvent -v

Expected: FAIL because the ingestion service does not exist.

- [ ] **Step 3: Implement transactional ingestion**

Validate before writes. Insert immutable events once by subject/event ID plus body hash. Project only higher client_id/sequence values into unique subject_id/endpoint/stash_id rows. A remove deletes only the projection. Return 400 malformed, 401 unauthorized, 409 changed replay, 200 exact replay, and 202 accepted.

- [ ] **Step 4: Verify server tests**

Run: make test-server

Expected: PASS with no partial writes.

- [ ] **Step 5: Commit**

~~~text
git add server/internal/ingest server/internal/httpapi server/internal/store
git commit -m "feat: ingest idempotent scene rating events"
~~~

### Task 5: UPSERT Source-Authoritative Metadata Snapshots

**Files:**
- Create: server/internal/catalog/snapshots.go
- Create: server/internal/catalog/snapshots_test.go
- Create: server/internal/httpapi/snapshots.go
- Create: server/internal/httpapi/snapshots_test.go
- Modify: server/internal/store/migrations/001_initial.sql

**Interfaces:** SnapshotService.Upsert(ctx, snapshot); POST /v1/catalog/snapshots returns 202; CatalogSource returns content features and a nullable canonical URL.

- [ ] **Step 1: Write the failing newest-source-wins test**

~~~go
func TestSnapshotUpsertKeepsNewestSourceVersion(t *testing.T) {
    require.NoError(t, service.Upsert(ctx, snapshot("2026-08-30T10:00:00Z", "Canonical title")))
    require.NoError(t, service.Upsert(ctx, snapshot("2026-08-30T09:00:00Z", "Older title")))
    require.Equal(t, "Canonical title", sourceSceneTitle(t, db, key))
    require.Error(t, service.Upsert(ctx, snapshotWithExtraField("paths")))
}
~~~

- [ ] **Step 2: Run it and verify failure**

Run: go test ./server/internal/catalog -run TestSnapshotUpsertKeepsNewestSourceVersion -v

Expected: FAIL because catalog ingestion does not exist.

- [ ] **Step 3: Implement source snapshot storage**

Persist validated original JSON, schema/fetch/source-update versions; project endpoint-qualified scenes, performers, studios, tags, appearances, and relations. Ignore stale source updates. Add source_config with nullable canonical_scene_url_template. Store remote URLs/image URLs only, never bytes.

- [ ] **Step 4: Verify server tests**

Run: make test-server

Expected: PASS for stale, mismatch, repeat UPSERT, and extra-field cases.

- [ ] **Step 5: Commit**

~~~text
git add server/internal/catalog server/internal/httpapi server/internal/store
git commit -m "feat: upsert source-authoritative catalog snapshots"
~~~

### Task 6: Build Versioned PostgreSQL Recommendations

**Files:**
- Create: server/internal/model/interfaces.go
- Create: server/internal/model/build.go
- Create: server/internal/model/build_test.go
- Create: server/internal/model/repository.go
- Create: server/internal/httpapi/recommendations.go
- Create: server/internal/httpapi/recommendations_test.go
- Modify: server/cmd/recommendations/main.go

**Interfaces:** InteractionSource, CatalogSource, RecommendationStore; Builder.BuildAndActivate(ctx); GET /v1/recommendations/related and GET /v1/recommendations/for-you.

- [ ] **Step 1: Write failing model and rollback tests**

~~~go
func TestBuildCombinesCollaborativeAndPerformerCandidates(t *testing.T) {
    seedRatings(t, db, rating("a", "scene-a", 1), rating("a", "scene-b", 1), rating("b", "scene-a", 1), rating("b", "scene-c", 1))
    seedSharedPerformer(t, db, "scene-a", "scene-d")
    buildAndActivate(t, db)
    require.ElementsMatch(t, []string{"scene-b", "scene-c", "scene-d"}, relatedIDs(t, db, "scene-a"))
}

func TestFailedBuildKeepsActiveVersion(t *testing.T) {
    active := buildAndActivate(t, db)
    require.Error(t, failingBuilder(t, db).BuildAndActivate(ctx))
    require.Equal(t, active, activeVersion(t, db))
}
~~~

- [ ] **Step 2: Run them and verify failure**

Run: go test ./server/internal/model -run TestBuildCombinesCollaborativeAndPerformerCandidates -v

Expected: FAIL because model code does not exist.

- [ ] **Step 3: Implement deterministic batch scoring**

Generate collaborative item neighbors from mean-centered co-ratings with small-sample shrinkage. Add content candidates from shared endpoint-qualified performers, studios, tags, groups, and supported Stash-box attributes. Write an inactive model version, item_neighbors, and user_recommendations; atomically activate only after successful build. Response items contain content, score, reasons, model_version, and nullable canonical_url. Cold starts return empty items. Do not add global popularity or Annoy.

- [ ] **Step 4: Verify model and API tests**

Run: make test-server

Expected: PASS for authenticated reads, cold start, version rollback, and canonical URL omission.

- [ ] **Step 5: Commit**

~~~text
git add server/cmd/recommendations server/internal/model server/internal/httpapi
git commit -m "feat: build versioned postgres recommendations"
~~~

**Review checkpoint:** Review authentication/disassociation, event ordering, metadata ownership, and the model response contract before plugin work.

### Task 7: Add Plugin Settings, Local Stash Client, and SQLite Outbox

**Files:**
- Create: plugin/stashRecommendations/stashRecommendations.yml
- Create: plugin/stashRecommendations/recommendations.py
- Create: plugin/stashRecommendations/rec_plugin/settings.py
- Create: plugin/stashRecommendations/rec_plugin/stash_client.py
- Create: plugin/stashRecommendations/rec_plugin/outbox.py
- Create: plugin/stashRecommendations/tests/test_settings.py
- Create: plugin/stashRecommendations/tests/test_stash_client.py
- Create: plugin/stashRecommendations/tests/test_outbox.py

**Interfaces:** Settings.from_plugin_config; StashClient.find_scene, iter_rated_scenes, iter_scene_stash_ids, configured_stash_boxes; Outbox.enqueue, next_ready, ack, record_retry, quarantine, status.

- [ ] **Step 1: Write the failing outbox test**

~~~python
def test_outbox_preserves_event_identity_across_retry(tmp_path: Path) -> None:
    outbox = Outbox(tmp_path / "recommendations.sqlite3")
    event = PreferenceEvent.rating_set(EVENT_ID, CLIENT_ID, 1, CONTENT, 0.8)
    outbox.enqueue(event)
    outbox.record_retry(EVENT_ID, NOW)
    assert outbox.next_ready(NOW) is None
    assert outbox.next_ready(NOW + timedelta(minutes=2)).event_id == EVENT_ID
~~~

- [ ] **Step 2: Run it and verify failure**

Run: python -m pytest plugin/stashRecommendations/tests/test_settings.py plugin/stashRecommendations/tests/test_stash_client.py plugin/stashRecommendations/tests/test_outbox.py -q

Expected: FAIL because plugin infrastructure does not exist.

- [ ] **Step 3: Implement plugin infrastructure**

Declare raw Python, settings service_url/api_key/show_remote_results, tasks sync-ratings/sync-metadata/deliver-outbox/status, and Scene.Update.Post hook capture-rating. Require HTTPS. Use server_connection/session cookie for local paginated GraphQL. SQLite rows hold event/snapshot JSON, attempt count, next attempt, state, and error; retry delay caps at one hour.

- [ ] **Step 4: Verify plugin infrastructure**

Run: make test-plugin

Expected: PASS including HTTP URL rejection and no source credentials in status output.

- [ ] **Step 5: Commit**

~~~text
git add plugin/stashRecommendations
git commit -m "feat: add stash plugin settings and durable outbox"
~~~

### Task 8: Capture Ratings and Proxy Stash-box Metadata

**Files:**
- Create: plugin/stashRecommendations/rec_plugin/capture.py
- Create: plugin/stashRecommendations/rec_plugin/source_client.py
- Create: plugin/stashRecommendations/rec_plugin/snapshots.py
- Create: plugin/stashRecommendations/rec_plugin/sync.py
- Create: plugin/stashRecommendations/tests/test_capture.py
- Create: plugin/stashRecommendations/tests/test_source_client.py
- Create: plugin/stashRecommendations/tests/test_snapshots.py
- Create: plugin/stashRecommendations/tests/test_sync.py
- Modify: plugin/stashRecommendations/recommendations.py

**Interfaces:** handle_scene_update(hook_context, stash, outbox, identity); SourceClient.fetch_scene(endpoint, api_key, stash_id); to_source_snapshot(endpoint, fetched_at, scene); queue_rating_sync and queue_metadata_sync.

- [ ] **Step 1: Write failing fan-out and source-boundary tests**

~~~python
def test_rating_hook_fans_out_without_local_scene_data() -> None:
    outbox = FakeOutbox()
    count = handle_scene_update({"id": 44, "inputFields": ["id", "rating100"]}, FakeStash(scene_with_rating_and_two_ids()), outbox, CLIENT)
    assert count == 2
    assert [event.rating for event in outbox.events] == [0.75, 0.75]
    assert "title" not in outbox.events[0].to_dict()

def test_source_client_fetches_only_configured_endpoint() -> None:
    source = SourceClient({"https://box.example/graphql": "source-key"})
    assert source.credentials_for("https://other.example/graphql") is None
~~~

- [ ] **Step 2: Run them and verify failure**

Run: python -m pytest plugin/stashRecommendations/tests/test_capture.py plugin/stashRecommendations/tests/test_source_client.py plugin/stashRecommendations/tests/test_snapshots.py plugin/stashRecommendations/tests/test_sync.py -q

Expected: FAIL because capture/source modules do not exist.

- [ ] **Step 3: Implement capture and source proxying**

Act only if inputFields contains rating100. Fetch the current local scene, enqueue one set/remove preference event per StashID, and return without network delivery. Read Stash-box endpoint/key pairs only from local Stash configuration. For configured matching endpoints, query canonical GraphQL data within its per-minute limiter, map only Stash-box schema fields, and queue snapshots separately. Rating sync displays count and requires confirmation; metadata sync deduplicates keys.

- [ ] **Step 4: Verify plugin capture tests**

Run: make test-plugin

Expected: PASS for half-star normalization, removal, tag-only no-op, unconfigured source omission, and rejection of paths/play_count/custom_fields.

- [ ] **Step 5: Commit**

~~~text
git add plugin/stashRecommendations
git commit -m "feat: capture ratings and proxy stash-box metadata"
~~~

### Task 9: Deliver Outbox and Surface Stash UI

**Files:**
- Create: plugin/stashRecommendations/rec_plugin/service_client.py
- Create: plugin/stashRecommendations/rec_plugin/delivery.py
- Create: plugin/stashRecommendations/rec_plugin/status.py
- Create: plugin/stashRecommendations/tests/test_delivery.py
- Create: plugin/stashRecommendations/ui/recommendations.js
- Create: plugin/stashRecommendations/ui/recommendations.css
- Create: tests/ui/recommendations.test.js
- Modify: plugin/stashRecommendations/recommendations.py
- Modify: plugin/stashRecommendations/stashRecommendations.yml

**Interfaces:** DeliveryWorker.deliver_ready(now); raw status response; resolveLocalRecommendations(items, findScenes, showRemote); fetchRelated(contentKeys); fetchForYou(limit).

- [ ] **Step 1: Write failing delivery and UI tests**

~~~python
def test_401_pauses_and_422_quarantines() -> None:
    assert DeliveryWorker(seeded_outbox(), FakeServiceClient([401])).deliver_ready(NOW).paused
    assert DeliveryWorker(seeded_outbox(), FakeServiceClient([422])).deliver_ready(NOW).quarantined == 1
~~~

~~~javascript
test("remote-only results stay hidden until enabled", () => {
  const items = [{ content: { endpoint: "https://box.example/graphql", stash_id: "1" }, canonical_url: "https://box.example/scenes/1" }];
  assert.deepEqual(resolveLocalRecommendations(items, () => [], false), []);
  assert.equal(resolveLocalRecommendations(items, () => [], true)[0].kind, "remote");
});
~~~

- [ ] **Step 2: Run them and verify failure**

Run: python -m pytest plugin/stashRecommendations/tests/test_delivery.py -q; node --test tests/ui/recommendations.test.js

Expected: FAIL because delivery and UI modules do not exist.

- [ ] **Step 3: Implement delivery and UI**

Map 200/202 to ack, 401/403 to pause, 400/409/422 to quarantine, 429 to Retry-After, and network/5xx to retry. Deliver snapshots independently. Use window.PluginApi to register ScenePage.Tabs, ScenePage.TabContent, route /plugins/stash-recommendations, and nav item. Resolve server content keys through local Stash GraphQL. Render local cards by default, remote canonical links only when enabled, and loading/configuration/cold-start states. Scope CSS below .stash-recommendations.

- [ ] **Step 4: Verify plugin, UI, and server tests**

Run: make test-plugin; make test-ui; make test-server

Expected: PASS; tests prove API keys are absent from markup and remote items are hidden by default.

- [ ] **Step 5: Commit**

~~~text
git add plugin/stashRecommendations tests/ui
git commit -m "feat: deliver and surface stash recommendations"
~~~

**Review checkpoint:** Review non-blocking hooks, source credential containment, retry behavior, and local-first UI results before end-to-end verification.

### Task 10: Add Contract, End-to-End, and Manual Smoke Verification

**Files:**
- Create: server/internal/httpapi/contract_test.go
- Create: plugin/stashRecommendations/tests/test_contract_fixtures.py
- Create: tests/e2e/mock_stash_box.py
- Create: tests/e2e/test_rating_to_recommendation.py
- Create: tests/e2e/test_readme.py
- Create: README.md
- Modify: Makefile

**Interfaces:** make test-contract; make test-e2e; mock Stash-box request capture verifies source API keys never reach the service.

- [ ] **Step 1: Write the failing end-to-end test**

~~~python
def test_hook_to_snapshot_to_recommendation(service, mock_stash_box) -> None:
    plugin = configured_plugin(service.url, service.api_key, mock_stash_box.config())
    plugin.handle_hook(scene_update_with_rating("local-44", 80, [mock_stash_box.scene_key("a")]))
    plugin.deliver_all()
    service.build_model()
    assert service.related(mock_stash_box.scene_key("a")).status_code == 200
~~~

- [ ] **Step 2: Run it and verify failure**

Run: python -m pytest tests/e2e/test_rating_to_recommendation.py -q

Expected: FAIL until fixture wiring is written.

- [ ] **Step 3: Implement shared fixture verification and README**

Load every valid/invalid fixture through Go and Python validation. Mock source responses/rate limits and assert credentials appear only in plugin-to-source calls. Document PostgreSQL setup, admin key provisioning, plugin configuration, explicit sync, status, model build, disassociation, privacy, and manual smoke: multi-ID rate, clear rating, source refresh, cold start, remote toggle, outage/retry, and old-key rejection.

- [ ] **Step 4: Run complete verification**

Run: make test; make test-contract; make test-e2e; go vet ./server/...; git diff --check

Expected: PASS. Run the README manual smoke against local Stash if available; report separately if only automated evidence was possible.

- [ ] **Step 5: Commit**

~~~text
git add README.md Makefile server plugin tests contracts
git commit -m "test: verify end-to-end recommendation flow"
~~~

## Review Checkpoints

- After Task 6: review authentication/disassociation, event ordering, metadata ownership, and the model response contract.
- After Task 9: review that hooks do not make network calls, source credentials/local metadata remain client-side, and remote results are opt-in.
- After Task 10: review automated output and manual Stash evidence before packaging or release.
