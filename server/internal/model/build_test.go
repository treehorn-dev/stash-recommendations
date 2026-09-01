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

func TestBuildProjectsOnlyValidatedCatalogScenes(t *testing.T) {
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
	require.Equal(t, []string{"scene-d"}, recommendationIDs(items))
	require.Contains(t, reasonsFor(items, "scene-d"), "shared_performer")
}

func TestCollaborativeSimilarityIsMeanCenteredNormalizedAndShrunk(t *testing.T) {
	ratings := []Rating{
		{AccountID: "account-1", ContentKey: contentKey("scene-a"), Value: 1},
		{AccountID: "account-1", ContentKey: contentKey("scene-b"), Value: 1},
		{AccountID: "account-1", ContentKey: contentKey("scene-c"), Value: 0},
		{AccountID: "account-2", ContentKey: contentKey("scene-a"), Value: 1},
		{AccountID: "account-2", ContentKey: contentKey("scene-b"), Value: 1},
		{AccountID: "account-2", ContentKey: contentKey("scene-c"), Value: 0},
		{AccountID: "account-3", ContentKey: contentKey("scene-a"), Value: 1},
		{AccountID: "account-3", ContentKey: contentKey("scene-d"), Value: 1},
		{AccountID: "account-3", ContentKey: contentKey("scene-c"), Value: 0},
	}

	scores := collaborativeNeighborScores(ratings)
	require.InDelta(t, 0.5, scores[contentKey("scene-a")][contentKey("scene-b")], 1e-12)
	require.InDelta(t, 1.0/3.0, scores[contentKey("scene-a")][contentKey("scene-d")], 1e-12)
	require.Greater(t, scores[contentKey("scene-a")][contentKey("scene-b")], scores[contentKey("scene-a")][contentKey("scene-d")])
}

func TestBuildProjectionIsDeterministicForInputOrder(t *testing.T) {
	inputs := []Rating{
		{AccountID: "b", ContentKey: contentKey("scene-a"), Value: 1}, {AccountID: "b", ContentKey: contentKey("scene-b"), Value: 1}, {AccountID: "b", ContentKey: contentKey("scene-c"), Value: 0},
		{AccountID: "a", ContentKey: contentKey("scene-a"), Value: 1}, {AccountID: "a", ContentKey: contentKey("scene-b"), Value: 1}, {AccountID: "a", ContentKey: contentKey("scene-c"), Value: 0},
	}
	cataloged := []domain.ContentKey{contentKey("scene-a"), contentKey("scene-b"), contentKey("scene-c")}
	first := NewBuilder(nil, DefaultOWeight).buildProjection(inputs, nil, nil, cataloged)
	for index := 0; index < 20; index++ {
		reversed := append([]Rating(nil), inputs...)
		for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
			reversed[left], reversed[right] = reversed[right], reversed[left]
		}
		actual := NewBuilder(nil, DefaultOWeight).buildProjection(reversed, nil, nil, cataloged)
		require.Equal(t, first, actual)
	}
}

func TestForYouTreatsZeroRatingAsKnownContent(t *testing.T) {
	repository, pool := openModelTestStore(t)
	ctx := context.Background()
	account := seedModelAccount(t, pool)
	other := seedModelAccount(t, pool)
	seedCatalogedScenes(t, pool, "scene-a", "scene-b")
	seedRating(t, pool, account, "scene-a", 1)
	seedRating(t, pool, account, "scene-b", 0)
	seedRating(t, pool, other, "scene-a", 1)
	seedRating(t, pool, other, "scene-b", 1)

	_, err := NewBuilder(NewRepository(repository.Pool()), DefaultOWeight).BuildAndActivate(ctx)
	require.NoError(t, err)
	items, _, err := NewRepository(repository.Pool()).ForYou(ctx, account, 10)
	require.NoError(t, err)
	require.Empty(t, items)
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

func TestFailedBuildKeepsActiveVersion(t *testing.T) {
	repository, pool := openModelTestStore(t)
	ctx := context.Background()
	accountID := seedModelAccount(t, pool)
	seedCatalogedScenes(t, pool, "scene-a", "scene-b", "scene-c")
	seedRating(t, pool, accountID, "scene-a", 1)
	seedRating(t, pool, accountID, "scene-b", 1)
	seedRating(t, pool, accountID, "scene-c", 0)

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
