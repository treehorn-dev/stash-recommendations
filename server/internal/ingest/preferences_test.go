package ingest

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/treehorn/stash-recommendations/server/internal/domain"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

func TestAcceptIgnoresOlderSequenceAndDeduplicatesEvent(t *testing.T) {
	repository, pool := openInteractionTestStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	service := NewInteractionService(repository)
	ctx := context.Background()

	accepted, err := service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440010", 2, 1.0))
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440011", 1, 0.2))
	require.NoError(t, err)
	require.True(t, accepted)

	require.Equal(t, 1.0, currentRating(t, pool, account.ID, normalizedContentKey(t)))

	accepted, err = service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440010", 2, 1.0))
	require.NoError(t, err)
	require.False(t, accepted)
	require.Equal(t, int64(2), preferenceEventCount(t, pool, account.ID))
}

func TestRemoveDeletesCurrentPreference(t *testing.T) {
	repository, pool := openInteractionTestStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	service := NewInteractionService(repository)
	ctx := context.Background()

	accepted, err := service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440012", 1, 0.8))
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = service.Accept(ctx, account.ID, ratingRemoveEvent("550e8400-e29b-41d4-a716-446655440013", 2))
	require.NoError(t, err)
	require.True(t, accepted)

	require.False(t, hasCurrentPreference(t, pool, account.ID, normalizedContentKey(t)))
}

func TestOlderRatingDoesNotRecreateProjectionAfterNewerRemove(t *testing.T) {
	repository, pool := openInteractionTestStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	service := NewInteractionService(repository)
	ctx := context.Background()
	key := normalizedContentKey(t)

	accepted, err := service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440030", 2, 0.8))
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = service.Accept(ctx, account.ID, ratingRemoveEvent("550e8400-e29b-41d4-a716-446655440031", 3))
	require.NoError(t, err)
	require.True(t, accepted)
	require.False(t, hasCurrentPreference(t, pool, account.ID, key))

	accepted, err = service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440032", 1, 0.2))
	require.NoError(t, err)
	require.True(t, accepted)
	require.False(t, hasCurrentPreference(t, pool, account.ID, key))

	accepted, err = service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440033", 4, 1.0))
	require.NoError(t, err)
	require.True(t, accepted)
	require.True(t, hasCurrentPreference(t, pool, account.ID, key))
	require.Equal(t, 1.0, currentRating(t, pool, account.ID, key))
}

func TestAcceptRejectsChangedReplay(t *testing.T) {
	repository, pool := openInteractionTestStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	service := NewInteractionService(repository)
	ctx := context.Background()

	accepted, err := service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440014", 1, 0.7))
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440014", 1, 0.4))
	require.ErrorIs(t, err, store.ErrInteractionEventConflict)
	require.False(t, accepted)
	require.Equal(t, int64(1), preferenceEventCount(t, pool, account.ID))
	require.Equal(t, 0.7, currentRating(t, pool, account.ID, normalizedContentKey(t)))
}

func TestAcceptPersistsEngagementEventsWithoutTouchingRatings(t *testing.T) {
	repository, pool := openInteractionTestStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	service := NewInteractionService(repository)
	ctx := context.Background()

	accepted, err := service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440015", 1, 0.6))
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = service.Accept(ctx, account.ID, playedEvent("550e8400-e29b-41d4-a716-446655440016", 2))
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = service.Accept(ctx, account.ID, oEvent("550e8400-e29b-41d4-a716-446655440017", 3))
	require.NoError(t, err)
	require.True(t, accepted)

	require.Equal(t, int64(2), engagementEventCount(t, pool, account.ID))
	require.Equal(t, int64(1), currentPreferenceCount(t, pool, account.ID))
	require.Equal(t, 0.6, currentRating(t, pool, account.ID, normalizedContentKey(t)))
}

func TestAcceptRejectsCrossCategoryReplayUsingSameEventID(t *testing.T) {
	repository, pool := openInteractionTestStore(t)
	account, err := repository.CreateAccount(context.Background())
	require.NoError(t, err)

	service := NewInteractionService(repository)
	ctx := context.Background()

	accepted, err := service.Accept(ctx, account.ID, ratingSetEvent("550e8400-e29b-41d4-a716-446655440034", 1, 0.6))
	require.NoError(t, err)
	require.True(t, accepted)

	accepted, err = service.Accept(ctx, account.ID, playedEvent("550e8400-e29b-41d4-a716-446655440034", 2))
	require.ErrorIs(t, err, store.ErrInteractionEventConflict)
	require.False(t, accepted)
	require.Equal(t, int64(1), preferenceEventCount(t, pool, account.ID))
	require.Equal(t, int64(0), engagementEventCount(t, pool, account.ID))
}

func openInteractionTestStore(t *testing.T) (*store.Store, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}

	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	schema := fmt.Sprintf("task4_ingest_%d", time.Now().UnixNano())
	_, err = adminPool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := adminPool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	})

	schemaDSN := schemaScopedDSN(t, dsn, schema)
	repository, err := store.Open(ctx, schemaDSN)
	require.NoError(t, err)
	t.Cleanup(func() { repository.Close(context.Background()) })
	require.NoError(t, repository.Migrate(ctx))

	pool, err := pgxpool.New(ctx, schemaDSN)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return repository, pool
}

func schemaScopedDSN(t *testing.T, dsn string, schema string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func normalizedContentKey(t *testing.T) domain.ContentKey {
	t.Helper()
	key, err := (domain.ContentKey{}).Normalize("HTTPS://BOX.EXAMPLE/graphql/", "scene-1")
	require.NoError(t, err)
	return key
}

func currentRating(t *testing.T, pool *pgxpool.Pool, accountID string, key domain.ContentKey) float64 {
	t.Helper()
	var rating float64
	require.NoError(t, pool.QueryRow(
		context.Background(),
		"SELECT rating FROM current_preferences WHERE account_id = $1 AND endpoint = $2 AND stash_id = $3",
		accountID,
		key.Endpoint,
		key.StashID,
	).Scan(&rating))
	return rating
}

func hasCurrentPreference(t *testing.T, pool *pgxpool.Pool, accountID string, key domain.ContentKey) bool {
	t.Helper()
	var exists bool
	require.NoError(t, pool.QueryRow(
		context.Background(),
		"SELECT EXISTS (SELECT 1 FROM current_preferences WHERE account_id = $1 AND endpoint = $2 AND stash_id = $3)",
		accountID,
		key.Endpoint,
		key.StashID,
	).Scan(&exists))
	return exists
}

func preferenceEventCount(t *testing.T, pool *pgxpool.Pool, accountID string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM preference_events WHERE account_id = $1",
		accountID,
	).Scan(&count))
	return count
}

func engagementEventCount(t *testing.T, pool *pgxpool.Pool, accountID string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM engagement_events WHERE account_id = $1",
		accountID,
	).Scan(&count))
	return count
}

func currentPreferenceCount(t *testing.T, pool *pgxpool.Pool, accountID string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM current_preferences WHERE account_id = $1",
		accountID,
	).Scan(&count))
	return count
}

func ratingSetEvent(eventID string, sequence int64, rating float64) domain.PreferenceEvent {
	return domain.PreferenceEvent{
		SchemaVersion: 1,
		EventID:       eventID,
		ClientID:      "550e8400-e29b-41d4-a716-446655440001",
		Sequence:      sequence,
		OccurredAt:    time.Date(2026, time.August, 30, 0, 0, int(sequence), 0, time.UTC),
		ContentKey: domain.ContentKey{
			Endpoint: "HTTPS://BOX.EXAMPLE/graphql/",
			StashID:  "scene-1",
		},
		Kind:   domain.PreferenceEventKindSceneRatingSet,
		Rating: &rating,
		Origin: "hook",
	}
}

func ratingRemoveEvent(eventID string, sequence int64) domain.PreferenceEvent {
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
		Kind:   domain.PreferenceEventKindSceneRatingRemove,
		Origin: "hook",
	}
}

func playedEvent(eventID string, sequence int64) domain.PreferenceEvent {
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
		Kind:   domain.PreferenceEventKindScenePlayed,
		Origin: "history",
	}
}

func oEvent(eventID string, sequence int64) domain.PreferenceEvent {
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
		Kind:   domain.PreferenceEventKindSceneO,
		Origin: "history",
	}
}

func currentPreferenceSequence(t *testing.T, pool *pgxpool.Pool, accountID string, key domain.ContentKey) int64 {
	t.Helper()
	var sequence int64
	err := pool.QueryRow(
		context.Background(),
		"SELECT sequence FROM current_preferences WHERE account_id = $1 AND endpoint = $2 AND stash_id = $3",
		accountID,
		key.Endpoint,
		key.StashID,
	).Scan(&sequence)
	if err != nil && !errorsIsNoRows(err) {
		require.NoError(t, err)
	}
	return sequence
}

func errorsIsNoRows(err error) bool {
	return err != nil && err == pgx.ErrNoRows
}
