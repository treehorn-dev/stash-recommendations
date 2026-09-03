package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/treehorn/stash-recommendations/server/internal/auth"
)

func TestMigrationLedgerIncludesPgvectorRecommendations(t *testing.T) {
	versions := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		versions = append(versions, migration.version)
	}
	require.Contains(t, versions, "009_pgvector_recommendations")
	require.Contains(t, versions, "010_predicted_ratings")
	require.Contains(t, versions, "011_model_account_profiles")
}

func TestMigrateCreatesBaseStorageTables(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}

	repository, err := Open(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { repository.Close(context.Background()) })
	require.NoError(t, repository.Migrate(context.Background()))

	for _, table := range []string{
		"accounts",
		"api_keys",
		"preference_events",
		"engagement_events",
		"current_preferences",
		"session_projections",
		"session_projection_items",
		"source_snapshots",
		"source_configs",
		"source_scenes",
		"source_performers",
		"source_studios",
		"source_tags",
		"source_scene_performers",
		"source_scene_tags",
		"source_groups",
		"source_scene_groups",
		"model_versions",
		"item_neighbors",
		"user_recommendations",
	} {
		var actual string
		require.NoError(t, repository.pool.QueryRow(context.Background(), "SELECT to_regclass('public.' || $1)", table).Scan(&actual))
		require.Equal(t, table, actual)
	}
	var exists bool
	require.NoError(t, repository.pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
				AND table_name = 'user_recommendations'
				AND column_name = 'predicted_rating'
		)
	`).Scan(&exists))
	require.True(t, exists)
}

func TestCreateAccountStoresHashUnderAPIKeyIdentifier(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}

	repository, err := Open(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { repository.Close(context.Background()) })
	require.NoError(t, repository.Migrate(context.Background()))

	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)
	identifier, secret, ok := auth.ParseAPIKey(account.PlaintextKey)
	require.True(t, ok)

	var storedHash string
	require.NoError(t, repository.pool.QueryRow(
		context.Background(),
		"SELECT key_hash FROM api_keys WHERE key_id = $1",
		identifier,
	).Scan(&storedHash))
	require.True(t, auth.VerifyAPIKey(storedHash, secret))
}

func TestMigrateUpgradesLegacyAPIKeySchema(t *testing.T) {
	repository := openIsolatedMigrationStore(t)
	ctx := context.Background()
	legacyAccountID, legacyKeyID, legacyBearer, err := seedLegacyAPIKeySchema(ctx, repository)
	require.NoError(t, err)

	require.NoError(t, repository.Migrate(ctx))
	var backfilledKeyID string
	require.NoError(t, repository.pool.QueryRow(ctx, "SELECT key_id FROM api_keys WHERE account_id = $1", legacyAccountID).Scan(&backfilledKeyID))
	require.Equal(t, "legacy_"+strings.ReplaceAll(legacyKeyID, "-", ""), backfilledKeyID)
	var isLegacy bool
	require.NoError(t, repository.pool.QueryRow(ctx, "SELECT legacy_key FROM api_keys WHERE account_id = $1", legacyAccountID).Scan(&isLegacy))
	require.True(t, isLegacy)
	var revokedAt time.Time
	require.NoError(t, repository.pool.QueryRow(ctx, "SELECT revoked_at FROM api_keys WHERE account_id = $1", legacyAccountID).Scan(&revokedAt))
	require.False(t, revokedAt.IsZero())
	_, err = repository.Authenticate(ctx, legacyBearer)
	require.ErrorIs(t, err, ErrInvalidAPIKey)

	account, err := repository.CreateAccount(ctx)
	require.NoError(t, err)
	authenticated, err := repository.Authenticate(ctx, account.PlaintextKey)
	require.NoError(t, err)
	require.Equal(t, account.ID, authenticated.ID)

	rows, err := repository.pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	require.NoError(t, err)
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var version string
		require.NoError(t, rows.Scan(&version))
		versions = append(versions, version)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"001_initial", "002_api_key_identifier", "003_legacy_api_key_auth", "004_revoke_legacy_api_keys", "005_session_projections", "006_source_catalog_projections", "007_recommendation_indexes", "008_source_catalog_groups", "009_pgvector_recommendations", "010_predicted_ratings"}, versions)
}

func TestMigrateAddsSessionProjectionTablesToExistingStore(t *testing.T) {
	repository := openIsolatedMigrationStore(t)
	ctx := context.Background()
	require.NoError(t, seedPreSessionProjectionSchema(ctx, repository))

	require.NoError(t, repository.Migrate(ctx))

	for _, table := range []string{"session_projections", "session_projection_items"} {
		var exists bool
		require.NoError(t, repository.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_tables
				WHERE schemaname = current_schema()
					AND tablename = $1
			)
		`, table).Scan(&exists))
		require.True(t, exists)
	}

	rows, err := repository.pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	require.NoError(t, err)
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var version string
		require.NoError(t, rows.Scan(&version))
		versions = append(versions, version)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"001_initial", "002_api_key_identifier", "003_legacy_api_key_auth", "004_revoke_legacy_api_keys", "005_session_projections", "006_source_catalog_projections", "007_recommendation_indexes", "008_source_catalog_groups", "009_pgvector_recommendations", "010_predicted_ratings"}, versions)
}

func TestMigrateAddsSourceCatalogProjectionColumnsToExistingStore(t *testing.T) {
	repository := openIsolatedMigrationStore(t)
	ctx := context.Background()
	require.NoError(t, seedPreSourceCatalogProjectionSchema(ctx, repository))

	require.NoError(t, repository.Migrate(ctx))

	for _, column := range []struct {
		table  string
		column string
	}{
		{table: "source_scenes", column: "dates"},
		{table: "source_scenes", column: "urls"},
		{table: "source_scenes", column: "duration"},
		{table: "source_scenes", column: "director"},
		{table: "source_scenes", column: "code"},
		{table: "source_scenes", column: "studio_endpoint"},
		{table: "source_scenes", column: "studio_stash_id"},
		{table: "source_performers", column: "aliases"},
		{table: "source_performers", column: "gender"},
		{table: "source_performers", column: "country"},
		{table: "source_performers", column: "ethnicity"},
		{table: "source_performers", column: "eye_color"},
		{table: "source_performers", column: "hair_color"},
		{table: "source_performers", column: "measurements"},
		{table: "source_performers", column: "career_years"},
		{table: "source_performers", column: "urls"},
		{table: "source_performers", column: "remote_images"},
		{table: "source_scene_performers", column: "appearance_order"},
		{table: "source_scene_tags", column: "tag_order"},
	} {
		var exists bool
		require.NoError(t, repository.pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM information_schema.columns
				WHERE table_schema = current_schema()
					AND table_name = $1
					AND column_name = $2
			)
		`, column.table, column.column).Scan(&exists))
		require.Truef(t, exists, "%s.%s should exist", column.table, column.column)
	}

	rows, err := repository.pool.Query(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	require.NoError(t, err)
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var version string
		require.NoError(t, rows.Scan(&version))
		versions = append(versions, version)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"001_initial", "002_api_key_identifier", "003_legacy_api_key_auth", "004_revoke_legacy_api_keys", "005_session_projections", "006_source_catalog_projections", "007_recommendation_indexes", "008_source_catalog_groups", "009_pgvector_recommendations", "010_predicted_ratings"}, versions)
}

func openIsolatedMigrationStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}

	admin, err := Open(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(func() { admin.Close(context.Background()) })
	schema := fmt.Sprintf("task3_migration_%d", time.Now().UnixNano())
	_, err = admin.pool.Exec(context.Background(), "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	})

	schemaDSN, err := url.Parse(dsn)
	require.NoError(t, err)
	query := schemaDSN.Query()
	query.Set("search_path", schema)
	schemaDSN.RawQuery = query.Encode()
	repository, err := Open(context.Background(), schemaDSN.String())
	require.NoError(t, err)
	t.Cleanup(func() { repository.Close(context.Background()) })
	return repository
}

func seedLegacyAPIKeySchema(ctx context.Context, repository *Store) (string, string, string, error) {
	_, err := repository.pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS pgcrypto;
		CREATE TABLE accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE api_keys (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			key_hash TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
	`)
	if err != nil {
		return "", "", "", err
	}
	var accountID string
	if err := repository.pool.QueryRow(ctx, "INSERT INTO accounts DEFAULT VALUES RETURNING id").Scan(&accountID); err != nil {
		return "", "", "", err
	}
	legacyBearer := "srk_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	hash, err := auth.HashAPIKey(legacyBearer)
	if err != nil {
		return "", "", "", err
	}
	var keyID string
	if err := repository.pool.QueryRow(ctx, "INSERT INTO api_keys (account_id, key_hash) VALUES ($1, $2) RETURNING id", accountID, hash).Scan(&keyID); err != nil {
		return "", "", "", err
	}
	return accountID, keyID, legacyBearer, nil
}

func seedPreSessionProjectionSchema(ctx context.Context, repository *Store) error {
	_, err := repository.pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS pgcrypto;
		CREATE TABLE accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE api_keys (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			key_hash TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			key_id TEXT NOT NULL,
			legacy_key BOOLEAN NOT NULL DEFAULT false,
			revoked_at TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX api_keys_key_id_unique ON api_keys (key_id);
		CREATE TABLE preference_events (
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			event_id UUID NOT NULL,
			client_id UUID NOT NULL,
			sequence BIGINT NOT NULL,
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('scene.rating.set', 'scene.rating.remove')),
			rating DOUBLE PRECISION,
			occurred_at TIMESTAMPTZ NOT NULL,
			origin TEXT NOT NULL,
			body_hash BYTEA NOT NULL,
			received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (account_id, event_id)
		);
		CREATE TABLE engagement_events (
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			event_id UUID NOT NULL,
			client_id UUID NOT NULL,
			sequence BIGINT NOT NULL,
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('scene.played', 'scene.o')),
			occurred_at TIMESTAMPTZ NOT NULL,
			origin TEXT NOT NULL,
			body_hash BYTEA NOT NULL,
			received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (account_id, event_id)
		);
		CREATE TABLE current_preferences (
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			rating DOUBLE PRECISION NOT NULL,
			client_id UUID NOT NULL,
			sequence BIGINT NOT NULL,
			occurred_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (account_id, endpoint, stash_id)
		);
		CREATE TABLE source_configs (
			endpoint TEXT PRIMARY KEY,
			canonical_scene_url_template TEXT,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE source_snapshots (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			captured_at TIMESTAMPTZ NOT NULL,
			source_updated_at TIMESTAMPTZ,
			snapshot JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (endpoint, stash_id)
		);
		CREATE TABLE source_scenes (
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			title TEXT,
			details TEXT,
			source_updated_at TIMESTAMPTZ,
			remote_images JSONB NOT NULL DEFAULT '[]'::jsonb,
			PRIMARY KEY (endpoint, stash_id)
		);
		CREATE TABLE source_performers (
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			name TEXT NOT NULL,
			source_updated_at TIMESTAMPTZ,
			PRIMARY KEY (endpoint, stash_id)
		);
		CREATE TABLE source_studios (
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			name TEXT NOT NULL,
			source_updated_at TIMESTAMPTZ,
			PRIMARY KEY (endpoint, stash_id)
		);
		CREATE TABLE source_tags (
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			name TEXT NOT NULL,
			source_updated_at TIMESTAMPTZ,
			PRIMARY KEY (endpoint, stash_id)
		);
		CREATE TABLE source_scene_performers (
			scene_endpoint TEXT NOT NULL,
			scene_stash_id TEXT NOT NULL,
			performer_endpoint TEXT NOT NULL,
			performer_stash_id TEXT NOT NULL,
			PRIMARY KEY (scene_endpoint, scene_stash_id, performer_endpoint, performer_stash_id)
		);
		CREATE TABLE source_scene_tags (
			scene_endpoint TEXT NOT NULL,
			scene_stash_id TEXT NOT NULL,
			tag_endpoint TEXT NOT NULL,
			tag_stash_id TEXT NOT NULL,
			PRIMARY KEY (scene_endpoint, scene_stash_id, tag_endpoint, tag_stash_id)
		);
		CREATE TABLE model_versions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			activated_at TIMESTAMPTZ,
			active BOOLEAN NOT NULL DEFAULT false
		);
		CREATE UNIQUE INDEX model_versions_one_active
			ON model_versions ((active))
			WHERE active;
		CREATE TABLE item_neighbors (
			model_version_id UUID NOT NULL REFERENCES model_versions(id) ON DELETE CASCADE,
			source_endpoint TEXT NOT NULL,
			source_stash_id TEXT NOT NULL,
			neighbor_endpoint TEXT NOT NULL,
			neighbor_stash_id TEXT NOT NULL,
			score DOUBLE PRECISION NOT NULL,
			reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
			PRIMARY KEY (model_version_id, source_endpoint, source_stash_id, neighbor_endpoint, neighbor_stash_id)
		);
		CREATE TABLE user_recommendations (
			model_version_id UUID NOT NULL REFERENCES model_versions(id) ON DELETE CASCADE,
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			source_endpoint TEXT NOT NULL,
			source_stash_id TEXT NOT NULL,
			score DOUBLE PRECISION NOT NULL,
			reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
			PRIMARY KEY (model_version_id, account_id, source_endpoint, source_stash_id)
		);
		CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		INSERT INTO schema_migrations (version) VALUES
			('001_initial'),
			('002_api_key_identifier'),
			('003_legacy_api_key_auth'),
			('004_revoke_legacy_api_keys');
	`)
	return err
}

func seedPreSourceCatalogProjectionSchema(ctx context.Context, repository *Store) error {
	_, err := repository.pool.Exec(ctx, `
		CREATE EXTENSION IF NOT EXISTS pgcrypto;
		CREATE TABLE accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE api_keys (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			key_hash TEXT NOT NULL UNIQUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			key_id TEXT NOT NULL,
			legacy_key BOOLEAN NOT NULL DEFAULT false,
			revoked_at TIMESTAMPTZ
		);
		CREATE UNIQUE INDEX api_keys_key_id_unique ON api_keys (key_id);
		CREATE TABLE preference_events (
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			event_id UUID NOT NULL,
			client_id UUID NOT NULL,
			sequence BIGINT NOT NULL,
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('scene.rating.set', 'scene.rating.remove')),
			rating DOUBLE PRECISION,
			occurred_at TIMESTAMPTZ NOT NULL,
			origin TEXT NOT NULL,
			body_hash BYTEA NOT NULL,
			received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (account_id, event_id)
		);
		CREATE TABLE engagement_events (
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			event_id UUID NOT NULL,
			client_id UUID NOT NULL,
			sequence BIGINT NOT NULL,
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('scene.played', 'scene.o')),
			occurred_at TIMESTAMPTZ NOT NULL,
			origin TEXT NOT NULL,
			body_hash BYTEA NOT NULL,
			received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (account_id, event_id)
		);
		CREATE TABLE current_preferences (
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			rating DOUBLE PRECISION NOT NULL,
			client_id UUID NOT NULL,
			sequence BIGINT NOT NULL,
			occurred_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (account_id, endpoint, stash_id)
		);
		CREATE TABLE session_projections (
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			projection_version BIGINT NOT NULL CHECK (projection_version >= 1),
			projection_type TEXT NOT NULL CHECK (projection_type IN ('latency', 'o_bounded')),
			rebuilt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (account_id, projection_version, projection_type)
		);
		CREATE TABLE session_projection_items (
			account_id UUID NOT NULL,
			projection_version BIGINT NOT NULL,
			projection_type TEXT NOT NULL CHECK (projection_type IN ('latency', 'o_bounded')),
			session_order INTEGER NOT NULL CHECK (session_order >= 1),
			item_order INTEGER NOT NULL CHECK (item_order >= 1),
			event_id UUID NOT NULL,
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			kind TEXT NOT NULL CHECK (kind IN ('scene.played', 'scene.o')),
			occurred_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (account_id, projection_version, projection_type, session_order, item_order),
			FOREIGN KEY (account_id, projection_version, projection_type)
				REFERENCES session_projections (account_id, projection_version, projection_type)
				ON DELETE CASCADE
		);
		CREATE TABLE source_configs (
			endpoint TEXT PRIMARY KEY,
			canonical_scene_url_template TEXT,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		CREATE TABLE source_snapshots (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			schema_version INTEGER NOT NULL,
			captured_at TIMESTAMPTZ NOT NULL,
			source_updated_at TIMESTAMPTZ,
			snapshot JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			UNIQUE (endpoint, stash_id)
		);
		CREATE TABLE source_scenes (
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			title TEXT,
			details TEXT,
			source_updated_at TIMESTAMPTZ,
			remote_images JSONB NOT NULL DEFAULT '[]'::jsonb,
			PRIMARY KEY (endpoint, stash_id)
		);
		CREATE TABLE source_performers (
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			name TEXT NOT NULL,
			source_updated_at TIMESTAMPTZ,
			PRIMARY KEY (endpoint, stash_id)
		);
		CREATE TABLE source_studios (
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			name TEXT NOT NULL,
			source_updated_at TIMESTAMPTZ,
			PRIMARY KEY (endpoint, stash_id)
		);
		CREATE TABLE source_tags (
			endpoint TEXT NOT NULL,
			stash_id TEXT NOT NULL,
			name TEXT NOT NULL,
			source_updated_at TIMESTAMPTZ,
			PRIMARY KEY (endpoint, stash_id)
		);
		CREATE TABLE source_scene_performers (
			scene_endpoint TEXT NOT NULL,
			scene_stash_id TEXT NOT NULL,
			performer_endpoint TEXT NOT NULL,
			performer_stash_id TEXT NOT NULL,
			PRIMARY KEY (scene_endpoint, scene_stash_id, performer_endpoint, performer_stash_id)
		);
		CREATE TABLE source_scene_tags (
			scene_endpoint TEXT NOT NULL,
			scene_stash_id TEXT NOT NULL,
			tag_endpoint TEXT NOT NULL,
			tag_stash_id TEXT NOT NULL,
			PRIMARY KEY (scene_endpoint, scene_stash_id, tag_endpoint, tag_stash_id)
		);
		CREATE TABLE model_versions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			activated_at TIMESTAMPTZ,
			active BOOLEAN NOT NULL DEFAULT false
		);
		CREATE UNIQUE INDEX model_versions_one_active
			ON model_versions ((active))
			WHERE active;
		CREATE TABLE item_neighbors (
			model_version_id UUID NOT NULL REFERENCES model_versions(id) ON DELETE CASCADE,
			source_endpoint TEXT NOT NULL,
			source_stash_id TEXT NOT NULL,
			neighbor_endpoint TEXT NOT NULL,
			neighbor_stash_id TEXT NOT NULL,
			score DOUBLE PRECISION NOT NULL,
			reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
			PRIMARY KEY (model_version_id, source_endpoint, source_stash_id, neighbor_endpoint, neighbor_stash_id)
		);
		CREATE TABLE user_recommendations (
			model_version_id UUID NOT NULL REFERENCES model_versions(id) ON DELETE CASCADE,
			account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
			source_endpoint TEXT NOT NULL,
			source_stash_id TEXT NOT NULL,
			score DOUBLE PRECISION NOT NULL,
			reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
			PRIMARY KEY (model_version_id, account_id, source_endpoint, source_stash_id)
		);
		CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		);
		INSERT INTO schema_migrations (version) VALUES
			('001_initial'),
			('002_api_key_identifier'),
			('003_legacy_api_key_auth'),
			('004_revoke_legacy_api_keys'),
			('005_session_projections');
	`)
	return err
}
