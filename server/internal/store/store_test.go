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
		"source_snapshots",
		"source_configs",
		"source_scenes",
		"source_performers",
		"source_studios",
		"source_tags",
		"source_scene_performers",
		"source_scene_tags",
		"model_versions",
		"item_neighbors",
		"user_recommendations",
	} {
		var actual string
		require.NoError(t, repository.pool.QueryRow(context.Background(), "SELECT to_regclass('public.' || $1)", table).Scan(&actual))
		require.Equal(t, table, actual)
	}
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
	require.Equal(t, []string{"001_initial", "002_api_key_identifier", "003_legacy_api_key_auth", "004_revoke_legacy_api_keys"}, versions)
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
