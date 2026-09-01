package httpapi_test

import (
	"bytes"
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
	"github.com/treehorn/stash-recommendations/server/internal/domain"
	"github.com/treehorn/stash-recommendations/server/internal/httpapi"
	"github.com/treehorn/stash-recommendations/server/internal/ingest"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

func TestPostInteractionsReturnsAcceptedReplayAndConflictStatuses(t *testing.T) {
	repository := openInteractionHTTPStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository:  repository,
		InteractionService: ingest.NewInteractionService(repository),
	})

	event := ratingSetEvent("550e8400-e29b-41d4-a716-446655440020", 1, 0.9)
	recorder := postInteraction(t, handler, account.PlaintextKey, mustJSON(t, event))
	require.Equal(t, http.StatusAccepted, recorder.Code)

	recorder = postInteraction(t, handler, account.PlaintextKey, mustJSON(t, event))
	require.Equal(t, http.StatusOK, recorder.Code)

	recorder = postInteraction(t, handler, account.PlaintextKey, mustJSON(t, ratingSetEvent("550e8400-e29b-41d4-a716-446655440020", 1, 0.1)))
	require.Equal(t, http.StatusConflict, recorder.Code)
}

func TestPostInteractionsRejectsMalformedAndUnauthenticatedRequests(t *testing.T) {
	repository := openInteractionHTTPStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository:  repository,
		InteractionService: ingest.NewInteractionService(repository),
	})

	recorder := postInteraction(t, handler, account.PlaintextKey, []byte(`{"schema_version":1,"event_id":"550e8400-e29b-41d4-a716-446655440021","client_id":"550e8400-e29b-41d4-a716-446655440001","sequence":1,"occurred_at":"2026-08-30T00:00:01Z","content_key":{"endpoint":"https://box.example/graphql","stash_id":"scene-1"},"kind":"scene.played","rating":0.4,"origin":"history"}`))
	require.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder = postInteraction(t, handler, "", mustJSON(t, ratingSetEvent("550e8400-e29b-41d4-a716-446655440022", 1, 0.8)))
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestPostInteractionsRejectsTrailingJSONAfterValidEvent(t *testing.T) {
	repository := openInteractionHTTPStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository:  repository,
		InteractionService: ingest.NewInteractionService(repository),
	})

	body := append(
		mustJSON(t, ratingSetEvent("550e8400-e29b-41d4-a716-446655440023", 1, 0.8)),
		[]byte(` {"ignored":true}`)...,
	)

	recorder := postInteraction(t, handler, account.PlaintextKey, body)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func openInteractionHTTPStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}

	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	schema := fmt.Sprintf("task4_http_%d", time.Now().UnixNano())
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
	return repository
}

func postInteraction(t *testing.T, handler http.Handler, plaintextKey string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/v1/events/interactions", bytes.NewReader(body))
	if plaintextKey != "" {
		request.Header.Set("Authorization", "Bearer "+plaintextKey)
	}
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func mustJSON(t *testing.T, event domain.PreferenceEvent) []byte {
	t.Helper()
	data, err := json.Marshal(event)
	require.NoError(t, err)
	return data
}

func ratingSetEvent(eventID string, sequence int64, rating float64) domain.PreferenceEvent {
	return domain.PreferenceEvent{
		SchemaVersion: 1,
		EventID:       eventID,
		ClientID:      "550e8400-e29b-41d4-a716-446655440001",
		Sequence:      sequence,
		OccurredAt:    time.Date(2026, time.August, 30, 0, 0, int(sequence), 0, time.UTC),
		ContentKey: domain.ContentKey{
			Endpoint: "https://box.example/graphql",
			StashID:  "scene-1",
		},
		Kind:   domain.PreferenceEventKindSceneRatingSet,
		Rating: &rating,
		Origin: "hook",
	}
}
