package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/treehorn/stash-recommendations/server/internal/httpapi"
	"github.com/treehorn/stash-recommendations/server/internal/model"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

func TestRecommendationReadsAuthenticateAndReturnVersionReasonsAndCanonicalURL(t *testing.T) {
	repository, pool := openRecommendationTestStore(t)
	ctx := context.Background()
	account, err := repository.CreateAccount(ctx)
	require.NoError(t, err)
	otherAccount, err := repository.CreateAccount(ctx)
	require.NoError(t, err)
	seedAPIModelRating(t, pool, account.ID, "scene-a", 1)
	seedAPIModelRating(t, pool, account.ID, "scene-b", 1)
	seedAPIModelRating(t, pool, account.ID, "scene-c", 0)
	seedAPIModelRating(t, pool, otherAccount.ID, "scene-a", 1)
	seedAPIModelRating(t, pool, otherAccount.ID, "scene-b", 1)
	seedAPIModelRating(t, pool, otherAccount.ID, "scene-c", 0)
	seedAPIModelCatalogedScenes(t, pool, "scene-a", "scene-b")
	_, err = pool.Exec(ctx, `
		INSERT INTO source_configs (endpoint, canonical_scene_url_template)
		VALUES ($1, 'https://box.example/scenes/{stash_id}')
	`, apiModelEndpoint)
	require.NoError(t, err)

	modelRepository := model.NewRepository(repository.Pool())
	version, err := model.NewBuilder(modelRepository, model.DefaultOWeight).BuildAndActivate(ctx)
	require.NoError(t, err)
	handler := httpapi.NewMux(httpapi.Dependencies{AccountRepository: repository, RecommendationReader: modelRepository})

	query := url.Values{"endpoint": {apiModelEndpoint}, "stash_id": {"scene-a"}}
	unauthenticated := httptest.NewRequest(http.MethodGet, "/v1/recommendations/related?"+query.Encode(), nil)
	unauthenticatedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticatedRecorder, unauthenticated)
	require.Equal(t, http.StatusUnauthorized, unauthenticatedRecorder.Code)

	related := recommendationResponse(t, handler, "/v1/recommendations/related?"+query.Encode(), account.PlaintextKey)
	require.Equal(t, version, related.ModelVersion)
	require.Len(t, related.Items, 1)
	require.Equal(t, "scene-b", related.Items[0].ContentKey.StashID)
	require.Equal(t, version, related.Items[0].ModelVersion)
	require.Contains(t, related.Items[0].Reasons, "content_similarity")
	require.Equal(t, "https://box.example/scenes/scene-b", *related.Items[0].CanonicalURL)

	forYou := recommendationResponse(t, handler, "/v1/recommendations/for-you?limit=5", account.PlaintextKey)
	require.Equal(t, version, forYou.ModelVersion)
	require.NotEmpty(t, forYou.Items, "rewatch candidates remain valid recommendations")
	require.Contains(t, forYou.Items[0].Reasons, "rating_profile")
}

func TestRecommendationReadsReturnEmptyColdStartAndOmitCanonicalURL(t *testing.T) {
	repository, pool := openRecommendationTestStore(t)
	ctx := context.Background()
	account, err := repository.CreateAccount(ctx)
	require.NoError(t, err)
	modelRepository := model.NewRepository(repository.Pool())
	handler := httpapi.NewMux(httpapi.Dependencies{AccountRepository: repository, RecommendationReader: modelRepository})

	coldStart := recommendationResponse(t, handler, "/v1/recommendations/for-you", account.PlaintextKey)
	require.Empty(t, coldStart.ModelVersion)
	require.Empty(t, coldStart.Items)

	otherAccount, err := repository.CreateAccount(ctx)
	require.NoError(t, err)
	seedAPIModelRating(t, pool, account.ID, "scene-a", 1)
	seedAPIModelRating(t, pool, account.ID, "scene-b", 1)
	seedAPIModelRating(t, pool, account.ID, "scene-c", 0)
	seedAPIModelRating(t, pool, otherAccount.ID, "scene-a", 1)
	seedAPIModelRating(t, pool, otherAccount.ID, "scene-b", 1)
	seedAPIModelRating(t, pool, otherAccount.ID, "scene-c", 0)
	seedAPIModelCatalogedScenes(t, pool, "scene-a", "scene-b")
	_, err = model.NewBuilder(modelRepository, model.DefaultOWeight).BuildAndActivate(ctx)
	require.NoError(t, err)

	query := url.Values{"endpoint": {apiModelEndpoint}, "stash_id": {"scene-a"}}
	related := recommendationResponse(t, handler, "/v1/recommendations/related?"+query.Encode(), account.PlaintextKey)
	require.Len(t, related.Items, 1)
	require.Equal(t, "scene-b", related.Items[0].ContentKey.StashID)
	require.Nil(t, related.Items[0].CanonicalURL)
	request := httptest.NewRequest(http.MethodGet, "/v1/recommendations/related?"+query.Encode(), nil)
	request.Header.Set("Authorization", "Bearer "+account.PlaintextKey)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var body struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, json.RawMessage("null"), body.Items[0]["canonical_url"])
}

func TestForYouRejectsInvalidNumericFilters(t *testing.T) {
	repository, _ := openRecommendationTestStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)
	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository:    repository,
		RecommendationReader: model.NewRepository(repository.Pool()),
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/recommendations/for-you?rating_operator=is_null&rating_value=4", nil)
	request.Header.Set("Authorization", "Bearer "+account.PlaintextKey)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestRelatedReadsReturnPersonalPredictedRating(t *testing.T) {
	repository, pool := openRecommendationTestStore(t)
	ctx := context.Background()
	account, err := repository.CreateAccount(ctx)
	require.NoError(t, err)
	for _, stashID := range []string{"scene-a", "scene-b", "scene-c", "scene-d", "scene-e"} {
		seedAPIModelRating(t, pool, account.ID, stashID, 1)
	}
	seedAPIModelCatalogedScenes(t, pool, "scene-a", "scene-b", "scene-c", "scene-d", "scene-e", "scene-f")
	modelRepository := model.NewRepository(repository.Pool())
	_, err = model.NewBuilder(modelRepository, model.DefaultOWeight).BuildAndActivate(ctx)
	require.NoError(t, err)
	handler := httpapi.NewMux(httpapi.Dependencies{AccountRepository: repository, RecommendationReader: modelRepository})

	query := url.Values{"endpoint": {apiModelEndpoint}, "stash_id": {"scene-a"}}
	response := recommendationResponse(t, handler, "/v1/recommendations/related?"+query.Encode(), account.PlaintextKey)
	for _, item := range response.Items {
		if item.ContentKey.StashID != "scene-f" {
			continue
		}
		require.NotNil(t, item.PredictedRating)
		require.InDelta(t, 5, *item.PredictedRating, 0.01)
		return
	}
	t.Fatal("expected scene-f related recommendation")
}

type recommendationAPIResponse struct {
	ModelVersion string                 `json:"model_version"`
	Items        []model.Recommendation `json:"items"`
}

func recommendationResponse(t *testing.T, handler http.Handler, target, plaintextKey string) recommendationAPIResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("Authorization", "Bearer "+plaintextKey)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response recommendationAPIResponse
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}

func openRecommendationTestStore(t *testing.T) (*store.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)
	schema := fmt.Sprintf("task7_http_%d", time.Now().UnixNano())
	_, err = adminPool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := adminPool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	})
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema+",public")
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

func seedAPIModelRating(t *testing.T, pool *pgxpool.Pool, accountID, stashID string, rating float64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO current_preferences (account_id, endpoint, stash_id, rating, client_id, sequence, occurred_at)
		VALUES ($1, $2, $3, $4, '550e8400-e29b-41d4-a716-446655440001', 1, now())
	`, accountID, apiModelEndpoint, stashID, rating)
	require.NoError(t, err)
}

func seedAPIModelCatalogedScenes(t *testing.T, pool *pgxpool.Pool, stashIDs ...string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO source_performers (endpoint, stash_id, name) VALUES ($1, 'performer-1', 'Performer')
	`, apiModelEndpoint)
	require.NoError(t, err)
	for _, stashID := range stashIDs {
		_, err := pool.Exec(context.Background(), `INSERT INTO source_scenes (endpoint, stash_id) VALUES ($1, $2)`, apiModelEndpoint, stashID)
		require.NoError(t, err)
		_, err = pool.Exec(context.Background(), `
			INSERT INTO source_scene_performers (scene_endpoint, scene_stash_id, performer_endpoint, performer_stash_id)
			VALUES ($1, $2, $1, 'performer-1')
		`, apiModelEndpoint, stashID)
		require.NoError(t, err)
	}
}

const apiModelEndpoint = "https://box.example/graphql"
