# SDD ledger - plan: docs/superpowers/plans/2026-08-30-stash-recommendations-v0-implementation.md

## Preflight

| Tasks | Shared file or interface | Finding |
| --- | --- | --- |
| 1 -> 2 | Go/Python/Node test commands and package roots | Task 1 creates every test root consumed by Task 2; consistent. |
| 2 -> 3 -> 4 | domain contracts, store schema, API authentication | Task 2 defines validation, Task 3 creates persistence/auth, Task 4 consumes both; consistent. |
| 3 -> 5 -> 6 | source catalog/model tables and interfaces | Task 3 establishes tables, Task 5 projects catalog, Task 6 consumes CatalogSource; consistent. |
| 7 -> 8 -> 9 | plugin manifest, outbox, clients, UI settings | Task 7 creates local/plugin boundaries, Task 8 queues capture, Task 9 delivers/renders; consistent. |
| 4 -> 8 -> 10 | preference event contract and end-to-end fixture | Task 4 accepts the contract Task 8 sends; Task 10 exercises both; consistent. |

| Task | Internal test/code consistency | Finding |
| --- | --- | --- |
| 1 | Health test matches NewMux response and bootstrap files. | Consistent. |
| 2 | Validation tests match explicit v1 schema rules. | Consistent. |
| 3 | Disassociation test matches separation of account linkage and retained interactions. | Consistent. |
| 4 | Sequence/replay/remove tests match projection semantics. | Consistent. |
| 5 | Newest-source test matches source update timestamp rule. | Consistent. |
| 6 | Model tests match batch/atomic activation design. | Consistent. |
| 7 | Retry test matches SQLite outbox interface. | Consistent. |
| 8 | Hook/source tests match client-only credential boundary. | Consistent. |
| 9 | Delivery/UI tests match response and remote opt-in rules. | Consistent. |
| 10 | End-to-end test and README scope match published interfaces. | Consistent. |

Ruling: Use the plan's specified PostgreSQL integration setup for Tasks 3-6, with `POSTGRES_TEST_DSN` documented by Task 1 tooling. The initial repository has no executable test suite before Task 1, so the baseline is the committed design/plan state. Cost if wrong: tests may need a test-container fallback during implementation.

Task 1: Ruling: Replace the plan's Node 22-incompatible literal `node --test tests/ui` with `node --test tests/ui/*.test.js` in the Make target. The spec requires complete Node UI-suite execution, not a directory argument; the glob discovers every conventionally named UI test while the observed directory form fails before discovery. Cost if wrong: a UI test outside the `*.test.js` convention is skipped; all plan-specified and later UI tests use that convention.

Task 1: fix round 1/5 (1 addressed, 0 open - Node UI suite discovery; commits 8f71ea4..ff637a7)
Task 1: complete (commits 1042c99..ff637a7, review clean)

Task 2: review findings open - snapshot validation does not enforce Stash-box-only/privacy fields or required nested values; Python accepts bool/float scalar values and noncanonical UUID forms; parity/privacy rejection tests are missing.
Task 3: Ruling: Remove user-facing account disassociation. On API-key rollover, atomically revoke the old key and orphan its account linkage into a pseudo-anonymous subject while retaining interaction data. Cost if wrong: users cannot independently request disassociation until a later account-management feature; this is explicitly deferred by user direction.

Task 3: Ruling supersedes prior key-rollover ruling: remove account disassociation and orphaning entirely. V0 treats every issued key/account as independent; users create a new account to sever linkage. Cost if wrong: no programmatic user data deletion/revocation flow in v0.

Plan amendment: v0 now imports recorded `play_history` and `o_history` in bulk and incrementally. It retains immutable engagement events and rebuilds overlapping latency (>2h gap) and o-bounded sessions, collapsing only consecutive repeats. An orphan o implies a one-scene session. Recommendation scoring uses session co-occurrence/transitions with configurable o weight, initially 1.5. Event privacy is limited to kind, UTC timestamp, and endpoint-qualified content.

Task 2: fix round 2/5 open - implement strict Python primitive/nested validation and fixture parity for the existing snapshot contract review findings, and extend the v1 event contract with the user-approved scene.played/scene.o engagement variants before Task 3 begins.

Task 2: complete (contracts and strict validation commits 8c5bf12, 1690a90, a4af556, 20d5797, 07f5a9d, e040ebe; evidence commits 307b984, 5b2d3b7, 821848d, e065fa4). Spec review and code-quality review approved. Verification: Go domain/server tests passed, Python contract suite 45 passed, git diff --check passed.

Task 3: complete (implementation and hardening commits 51de83d, 61cd76d, bd6ef3b, 7a7458b, a4ea1a9; evidence commits e27c838, 068a9e2, 48d5b7c, 867f0e3, 7bee80a). Ruling: after the key-id migration, unsupported pre-identifier keys are explicitly revoked and require administrator reissue; this preserves one-row, one-Argon2 authentication and avoids a multi-hash denial-of-service path. Spec and code-quality reviews approved. Verification: PostgreSQL server tests, go vet, and git diff --check passed.

Task 6: complete (snapshot implementation/hardening commits 2396ab8, 3265648, d773349). Spec and code-quality reviews approved. Verification: PostgreSQL server tests, Python contracts 48 passed, UI test passed, go vet and git diff --check passed.

Task 4: dispatched at base 7bee80a for idempotent interaction ingestion.
Task 4: fix round 1/5 open - preserve ordering state across rating removals so stale sets cannot recreate a projection; make account/event-ID replay identity global across rating and engagement rows; reject trailing JSON after an interaction payload. Review range 7bee80a..2a7e2f1.
Task 4: fix round 1/5 (3 addressed, 0 open - removal ordering tombstone, global replay identity, complete JSON request validation; commits 2a7e2f1..7e37a34)
Task 4: complete (commits 7bee80a..7e37a34, review clean)

Task 5: Ruling: add session tables both to `001_initial.sql` for fresh installs and to a new ordered additive migration for existing stores, updating migration coverage accordingly. Task 3's migration ledger means an edited initial migration does not upgrade deployed databases. Cost if wrong: the initial migration contains intentional duplicate schema declarations across clean-install and upgrade paths, increasing migration-maintenance surface.
Task 5: minor (deferred): session ordering tests do not directly prove event-ID tie-break behavior at equal timestamps; final whole-branch review must triage this gap.
Task 5: fix round 1/5 open - concurrent `Rebuild` calls can allocate the same `MAX(projection_version)+1` and violate the unique projection key. Serialize account-scoped version allocation or add safe retry, with a regression test. Review range 7e37a34..8f7131f.
Task 5: fix round 1/5 (1 addressed, 0 open - concurrent version allocation; commits 8f7131f..2ba3d1a)
Task 5: complete (commits 7e37a34..2ba3d1a, review clean; 1 deferred minor)

Task 6: Ruling: mirror Task 5 migration handling. Keep new clean-install metadata schema in `001_initial.sql` and add an ordered additive migration plus upgrade coverage for existing stores. Cost if wrong: duplicate declarations across fresh/upgrade migration paths require ongoing synchronization.
Task 6: fix round 1/5 addressed - source snapshots now require independent `source_updated_at`; stale and equal source versions are no-ops before all catalog/relationship mutation, while `captured_at` is preserved independently. Verification: PostgreSQL `go test ./server/... -v` passed, Python tests 47 passed, UI test passed, and `git diff --check` passed. Awaiting scoped spec and code-quality review before Task 7.
Task 6: fix round 2/5 addressed - snapshot freshness normalizes source versions to PostgreSQL microseconds and exits before relation mutation on a zero-row guard; snapshot HTTP bodies are bounded at 1 MiB with 413 for oversized declared/chunked bodies; and performer appearances must resolve within the submitted snapshot. Verification: PostgreSQL `go test ./server/... -v` passed, Python tests 48 passed, UI test passed, `go vet ./server/...` passed, and `git diff --check` passed. Awaiting scoped spec and code-quality review before Task 7.

Task 7: fix round 1/5 addressed - recommendation builds now use mean-centered norm-normalized co-rating similarity with `count / (count + 2)` shrinkage, keep all recommendations within validated catalog scenes, add validated scene-attribute candidates (code/title/date/duration), accumulate deterministically, treat zero ratings as known content, serialize missing canonical URLs as explicit JSON `null`, and reject non-finite `MODEL_O_WEIGHT` values. Verification: PostgreSQL `go test ./server/... -v` passed, `go vet ./server/...` passed, Python tests 48 passed, UI test passed, and `git diff --check` passed. Awaiting scoped independent review before Task 8.
Task 7: fix round 2/5 (2 addressed, 0 open - behavioral collaborative/session edges now survive absent source metadata and group-based catalog candidates now flow through snapshot contract/storage/query; commits 658b87f..6ad7a6b)
Task 7: complete (commits d773349..6ad7a6b, review clean)
Task 7: fix round 3/5 addressed - shared source snapshot schema now requires non-blank `groups[].id` and `groups[].name`, with dedicated valid/invalid group fixtures exercised by both Go and Python cross-language parity suites. Verification: `go test ./server/internal/domain -count=1` passed, `PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_contracts.py -q` passed (50 passed), `go vet ./server/...` passed, and `git diff --check` passed. Awaiting fresh independent quality review before Task 8.
Task 8: complete (implementation pending independent review). Implemented raw Python manifest/settings/task declarations, local Stash GraphQL client access through `server_connection` plus session cookie, durable SQLite outbox retry/quarantine/status with separate rating/play/o counts, and a `capture-rating` hook path that queues generic hook work only. Verification: `PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests/test_settings.py plugin/stashRecommendations/tests/test_stash_client.py plugin/stashRecommendations/tests/test_outbox.py -q` passed (9 passed), `make test-plugin` passed (60 passed), and `git diff --check` passed.
