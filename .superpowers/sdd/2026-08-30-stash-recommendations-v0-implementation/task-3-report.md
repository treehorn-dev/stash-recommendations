# Task 3 Report

Implemented PostgreSQL base storage, Argon2id API-key hashing, account-key
issuance/authentication, and bearer middleware. The initial migration creates
the required account, interaction, catalog, and recommendation projection
tables. Migrations run under a transaction-scoped advisory lock so concurrent
test processes cannot race table creation. No key lifecycle, disassociation,
ingestion, or model behavior was added.

TDD evidence:

- RED: `POSTGRES_TEST_DSN=... go test ./server/internal/httpapi -run TestAuthenticateAcceptsOnlyTheOwningAccountKey -v` failed because the store/authentication packages did not exist.
- GREEN: the focused auth, migration, and middleware tests passed against the
  local PostgreSQL 16 compose service.

Final verification:

- `POSTGRES_TEST_DSN=postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable go test ./server/... -v`: PASS.
- `PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q`: `46 passed`.
- `node --test tests/ui/*.test.js`: PASS, `1` test.
- `git diff --check`: clean.

## Fix Round 2

Migrations now use a transaction-scoped advisory lock plus ordered
`schema_migrations` tracking. `001_initial` retains the original legacy
`api_keys` shape; additive `002_api_key_identifier` adds `key_id`, backfills
legacy rows deterministically from their API-key UUIDs, enforces non-null, and
creates a unique index. Fresh databases apply both versions in order.

TDD evidence:

- RED: an isolated-schema regression seeded the old `api_keys` table without
  `key_id`; the old runner completed but `CreateAccount` failed with `column
  "key_id" of relation "api_keys" does not exist`.
- GREEN: the migration now records `001_initial` then
  `002_api_key_identifier`, backfills the seeded legacy row, and creates and
  authenticates a new account key.

Final verification:

- `POSTGRES_TEST_DSN=postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable go test ./server/... -v`: PASS.
- `git diff --check`: clean.

The plan's `make test-server` target is absent from the bootstrap Makefile;
the equivalent full server command above was run directly.

## Fix Round 1

Changed bearer keys to `srk_<public-id>.<secret>`. The public identifier is
independently random, persisted under the unique indexed `api_keys.key_id`,
and used to select exactly one candidate before Argon2id verifies the secret.
No bearer value is logged or persisted in plaintext.

TDD evidence:

- RED: the new parser/storage tests initially failed because `ParseAPIKey` did
  not exist; after implementation, the storage test correctly exposed the
  prior compose volume as pre-identifier schema (`column "key_id" does not
  exist`).
- GREEN: recreated the disposable PostgreSQL compose volume to validate the
  fresh v0 initial migration. Focused auth/store/http tests passed, including
  two independently issued account keys resolving to their own contexts and an
  arbitrary bearer returning `401`.

Final verification:

- `POSTGRES_TEST_DSN=postgres://stash_recommendations:stash_recommendations@localhost:5432/stash_recommendations?sslmode=disable go test ./server/... -v`: PASS.
- `PYTHONPATH=plugin/stashRecommendations pytest plugin/stashRecommendations/tests -q`: `46 passed`.
- `node --test tests/ui/*.test.js`: PASS, `1` test.
- `git diff --check`: clean.
