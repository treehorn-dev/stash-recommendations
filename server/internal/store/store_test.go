package store

import (
	"context"
	"os"
	"testing"

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
