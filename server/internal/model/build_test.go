package model

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/treehorn/stash-recommendations/server/internal/domain"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

func TestBuildCombinesRatingSessionAndCatalogCandidates(t *testing.T) {
	repository, pool := openModelTestStore(t)
	ctx := context.Background()

	accountA := seedModelAccount(t, pool)
	accountB := seedModelAccount(t, pool)
	seedRating(t, pool, accountA, "scene-a", 1)
	seedRating(t, pool, accountA, "scene-b", 1)
	seedRating(t, pool, accountB, "scene-a", 1)
	seedRating(t, pool, accountB, "scene-b", 1)
	seedLatestLatencySession(t, pool, accountA, []sessionSeed{{"scene-a", "scene.played"}, {"scene-c", "scene.o"}})
	seedSharedPerformer(t, pool, "scene-a", "scene-d")

	versionID, err := NewBuilder(NewRepository(repository.Pool()), DefaultOWeight).BuildAndActivate(ctx)
	require.NoError(t, err)

	items, activeVersion, err := NewRepository(repository.Pool()).Related(ctx, contentKey("scene-a"), 10)
	require.NoError(t, err)
	require.Equal(t, versionID, activeVersion)
	require.ElementsMatch(t, []string{"scene-b", "scene-c", "scene-d"}, recommendationIDs(items))
	require.Contains(t, reasonsFor(items, "scene-b"), "collaborative_rating")
	require.Contains(t, reasonsFor(items, "scene-c"), "session_cooccurrence")
	require.Contains(t, reasonsFor(items, "scene-d"), "shared_performer")
}

func TestFailedBuildKeepsActiveVersion(t *testing.T) {
	repository, pool := openModelTestStore(t)
	ctx := context.Background()
	accountID := seedModelAccount(t, pool)
	seedRating(t, pool, accountID, "scene-a", 1)
	seedRating(t, pool, accountID, "scene-b", 1)

	builder := NewBuilder(NewRepository(repository.Pool()), DefaultOWeight)
	activeVersion, err := builder.BuildAndActivate(ctx)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		CREATE FUNCTION fail_model_neighbor_insert() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'forced model build failure';
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER fail_model_neighbor_insert
		BEFORE INSERT ON item_neighbors
		FOR EACH ROW EXECUTE FUNCTION fail_model_neighbor_insert();
	`)
	require.NoError(t, err)

	_, err = builder.BuildAndActivate(ctx)
	require.ErrorContains(t, err, "forced model build failure")

	_, actualActiveVersion, err := NewRepository(repository.Pool()).Related(ctx, contentKey("scene-a"), 10)
	require.NoError(t, err)
	require.Equal(t, activeVersion, actualActiveVersion)
}

func openModelTestStore(t *testing.T) (*store.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}

	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	schema := fmt.Sprintf("task7_model_%d", time.Now().UnixNano())
	_, err = adminPool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := adminPool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	})

	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	repository, err := store.Open(ctx, parsed.String())
	require.NoError(t, err)
	t.Cleanup(func() { repository.Close(context.Background()) })
	require.NoError(t, repository.Migrate(ctx))

	pool, err := pgxpool.New(ctx, parsed.String())
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return repository, pool
}

func seedModelAccount(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var accountID string
	require.NoError(t, pool.QueryRow(context.Background(), "INSERT INTO accounts DEFAULT VALUES RETURNING id").Scan(&accountID))
	return accountID
}

func seedRating(t *testing.T, pool *pgxpool.Pool, accountID, stashID string, rating float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO current_preferences (account_id, endpoint, stash_id, rating, client_id, sequence, occurred_at)
		VALUES ($1, $2, $3, $4, '550e8400-e29b-41d4-a716-446655440001', 1, now())
	`, accountID, modelEndpoint, stashID, rating)
	require.NoError(t, err)
}

type sessionSeed struct {
	stashID string
	kind    string
}

func seedLatestLatencySession(t *testing.T, pool *pgxpool.Pool, accountID string, items []sessionSeed) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO session_projections (account_id, projection_version, projection_type)
		VALUES ($1, 1, 'latency')
	`, accountID)
	require.NoError(t, err)
	for index, item := range items {
		_, err = pool.Exec(context.Background(), `
			INSERT INTO session_projection_items (
				account_id, projection_version, projection_type, session_order, item_order,
				event_id, endpoint, stash_id, kind, occurred_at
			) VALUES (
				$1, 1, 'latency', 1, $2,
				gen_random_uuid(), $3, $4, $5, now()
			)
		`, accountID, index+1, modelEndpoint, item.stashID, item.kind)
		require.NoError(t, err)
	}
}

func seedSharedPerformer(t *testing.T, pool *pgxpool.Pool, firstID, secondID string) {
	t.Helper()
	for _, stashID := range []string{firstID, secondID} {
		_, err := pool.Exec(context.Background(), `
			INSERT INTO source_scenes (endpoint, stash_id) VALUES ($1, $2)
		`, modelEndpoint, stashID)
		require.NoError(t, err)
		_, err = pool.Exec(context.Background(), `
			INSERT INTO source_scene_performers (scene_endpoint, scene_stash_id, performer_endpoint, performer_stash_id)
			VALUES ($1, $2, $1, 'performer-1')
		`, modelEndpoint, stashID)
		require.NoError(t, err)
	}
	_, err := pool.Exec(context.Background(), `
		INSERT INTO source_performers (endpoint, stash_id, name) VALUES ($1, 'performer-1', 'Performer')
	`, modelEndpoint)
	require.NoError(t, err)
}

func contentKey(stashID string) domain.ContentKey {
	return domain.ContentKey{Endpoint: modelEndpoint, StashID: stashID}
}

func recommendationIDs(items []Recommendation) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ContentKey.StashID)
	}
	return ids
}

func reasonsFor(items []Recommendation, stashID string) []string {
	for _, item := range items {
		if item.ContentKey.StashID == stashID {
			return item.Reasons
		}
	}
	return nil
}

const modelEndpoint = "https://box.example/graphql"
