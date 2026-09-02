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

func TestBuildUsesCatalogVectorsForRelatedAndProfileRecommendations(t *testing.T) {
	repository, pool := openModelTestStore(t)
	ctx := context.Background()

	account := seedModelAccount(t, pool)
	seedSharedPerformer(t, pool, "scene-a", "scene-b")
	seedLatestLatencySession(t, pool, account, []sessionSeed{{"scene-a", "scene.played"}})
	seedRating(t, pool, account, "scene-a", 1)

	versionID, err := NewBuilder(NewRepository(repository.Pool()), DefaultOWeight).BuildAndActivate(ctx)
	require.NoError(t, err)

	items, activeVersion, err := NewRepository(repository.Pool()).Related(ctx, contentKey("scene-a"), 10)
	require.NoError(t, err)
	require.Equal(t, versionID, activeVersion)
	require.Equal(t, []string{"scene-b"}, recommendationIDs(items))
	require.Contains(t, reasonsFor(items, "scene-b"), "content_similarity")
	require.Nil(t, items[0].CanonicalURL)

	forYou, activeVersion, err := NewRepository(repository.Pool()).ForYou(ctx, account, 10)
	require.NoError(t, err)
	require.Equal(t, versionID, activeVersion)
	require.Contains(t, recommendationIDs(forYou), "scene-b")
	require.Contains(t, reasonsFor(forYou, "scene-b"), "rating_profile")
	require.Contains(t, reasonsFor(forYou, "scene-b"), "play_profile")
}

func TestCatalogCandidatesIncludeValidatedSceneAttributes(t *testing.T) {
	repository, pool := openModelTestStore(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO source_scenes (endpoint, stash_id, title, code, dates, duration) VALUES
			($1, 'code-a', 'Alpha', 'same-code', '["2026-01-01"]', 120),
			($1, 'code-b', 'Beta', 'same-code', '["2026-01-02"]', 500),
			($1, 'title-a', 'Same title', 'other-a', '["2026-02-01"]', 500),
			($1, 'title-b', 'Same title', 'other-b', '["2026-03-01"]', 900),
			($1, 'date-a', 'Date A', 'other-c', '["2026-04-01"]', 1000),
			($1, 'date-b', 'Date B', 'other-d', '["2026-04-01"]', 1500),
			($1, 'duration-a', 'Duration A', 'other-e', '["2026-05-01"]', 1000),
			($1, 'duration-b', 'Duration B', 'other-f', '["2026-06-01"]', 1050)
	`, modelEndpoint)
	require.NoError(t, err)

	candidates, err := NewRepository(repository.Pool()).CatalogCandidates(ctx)
	require.NoError(t, err)
	require.Contains(t, catalogCandidateReasons(candidates, "code-a", "code-b"), "shared_code")
	require.Contains(t, catalogCandidateReasons(candidates, "title-a", "title-b"), "shared_title")
	require.Contains(t, catalogCandidateReasons(candidates, "date-a", "date-b"), "shared_date")
	require.Contains(t, catalogCandidateReasons(candidates, "duration-a", "duration-b"), "similar_duration")
}

func TestCatalogCandidatesIncludeSharedGroups(t *testing.T) {
	repository, pool := openModelTestStore(t)
	ctx := context.Background()
	seedCatalogedScenes(t, pool, "group-a", "group-b")
	_, err := pool.Exec(ctx, `
		INSERT INTO source_groups (endpoint, stash_id, name) VALUES ($1, 'group-1', 'Series')
	`, modelEndpoint)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO source_scene_groups (scene_endpoint, scene_stash_id, group_endpoint, group_stash_id, group_order)
		VALUES ($1, 'group-a', $1, 'group-1', 1), ($1, 'group-b', $1, 'group-1', 1)
	`, modelEndpoint)
	require.NoError(t, err)

	candidates, err := NewRepository(repository.Pool()).CatalogCandidates(ctx)
	require.NoError(t, err)
	require.Contains(t, catalogCandidateReasons(candidates, "group-a", "group-b"), "shared_group")
}

func TestFailedBuildKeepsActiveVersion(t *testing.T) {
	repository, pool := openModelTestStore(t)
	ctx := context.Background()
	accountID := seedModelAccount(t, pool)
	seedSharedPerformer(t, pool, "scene-a", "scene-b")
	seedRating(t, pool, accountID, "scene-a", 1)

	builder := NewBuilder(NewRepository(repository.Pool()), DefaultOWeight)
	activeVersion, err := builder.BuildAndActivate(ctx)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
	CREATE FUNCTION fail_model_vector_insert() RETURNS trigger AS $$
		BEGIN
			RAISE EXCEPTION 'forced model build failure';
		END;
		$$ LANGUAGE plpgsql;
	CREATE TRIGGER fail_model_vector_insert
	BEFORE INSERT ON model_scene_vectors
	FOR EACH ROW EXECUTE FUNCTION fail_model_vector_insert();
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

func seedCatalogedScenes(t *testing.T, pool *pgxpool.Pool, stashIDs ...string) {
	t.Helper()
	for _, stashID := range stashIDs {
		_, err := pool.Exec(context.Background(), `INSERT INTO source_scenes (endpoint, stash_id) VALUES ($1, $2)`, modelEndpoint, stashID)
		require.NoError(t, err)
	}
}

func catalogCandidateReasons(candidates []CatalogCandidate, sourceID, candidateID string) []string {
	var reasons []string
	for _, candidate := range candidates {
		if candidate.Source.StashID == sourceID && candidate.Candidate.StashID == candidateID {
			reasons = append(reasons, candidate.Reason)
		}
	}
	return reasons
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
