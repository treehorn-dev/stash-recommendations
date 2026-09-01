package session

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

func TestRebuildKeepsTwoHourGapInSingleLatencySession(t *testing.T) {
	builder, pool := openSessionTestStore(t)
	accountID := seedAccount(t, pool)
	base := time.Date(2026, time.August, 30, 10, 0, 0, 0, time.UTC)

	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-446655440101",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   1,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-1",
		kind:       "scene.played",
		occurredAt: base,
	})
	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-446655440102",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   2,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-2",
		kind:       "scene.played",
		occurredAt: base.Add(2 * time.Hour),
	})

	require.NoError(t, builder.Rebuild(context.Background(), accountID))
	require.Equal(t, []projectedItem{
		{version: 1, projectionType: projectionTypeLatency, sessionOrder: 1, itemOrder: 1, stashID: "scene-1", kind: "scene.played"},
		{version: 1, projectionType: projectionTypeLatency, sessionOrder: 1, itemOrder: 2, stashID: "scene-2", kind: "scene.played"},
	}, loadProjectionItems(t, pool, accountID, projectionTypeLatency))
}

func TestRebuildStartsNewLatencySessionWhenGapExceedsTwoHours(t *testing.T) {
	builder, pool := openSessionTestStore(t)
	accountID := seedAccount(t, pool)
	base := time.Date(2026, time.August, 30, 10, 0, 0, 0, time.UTC)

	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-446655440103",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   1,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-1",
		kind:       "scene.played",
		occurredAt: base,
	})
	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-446655440104",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   2,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-2",
		kind:       "scene.played",
		occurredAt: base.Add(2*time.Hour + time.Second),
	})

	require.NoError(t, builder.Rebuild(context.Background(), accountID))
	require.Equal(t, []projectedItem{
		{version: 1, projectionType: projectionTypeLatency, sessionOrder: 1, itemOrder: 1, stashID: "scene-1", kind: "scene.played"},
		{version: 1, projectionType: projectionTypeLatency, sessionOrder: 2, itemOrder: 1, stashID: "scene-2", kind: "scene.played"},
	}, loadProjectionItems(t, pool, accountID, projectionTypeLatency))
}

func TestRebuildClosesOBoundedSessionsAndTreatsOrphanOAsClosedSession(t *testing.T) {
	builder, pool := openSessionTestStore(t)
	accountID := seedAccount(t, pool)
	base := time.Date(2026, time.August, 30, 10, 0, 0, 0, time.UTC)

	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-446655440105",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   1,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-1",
		kind:       "scene.played",
		occurredAt: base,
	})
	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-446655440106",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   2,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-1",
		kind:       "scene.o",
		occurredAt: base.Add(time.Minute),
	})
	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-446655440107",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   3,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-2",
		kind:       "scene.o",
		occurredAt: base.Add(2 * time.Minute),
	})
	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-446655440108",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   4,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-3",
		kind:       "scene.played",
		occurredAt: base.Add(3 * time.Minute),
	})

	require.NoError(t, builder.Rebuild(context.Background(), accountID))
	require.Equal(t, []projectedItem{
		{version: 1, projectionType: projectionTypeOBounded, sessionOrder: 1, itemOrder: 1, stashID: "scene-1", kind: "scene.o"},
		{version: 1, projectionType: projectionTypeOBounded, sessionOrder: 2, itemOrder: 1, stashID: "scene-2", kind: "scene.o"},
		{version: 1, projectionType: projectionTypeOBounded, sessionOrder: 3, itemOrder: 1, stashID: "scene-3", kind: "scene.played"},
	}, loadProjectionItems(t, pool, accountID, projectionTypeOBounded))
}

func TestRebuildCollapsesOnlyConsecutiveRepeats(t *testing.T) {
	builder, pool := openSessionTestStore(t)
	accountID := seedAccount(t, pool)
	base := time.Date(2026, time.August, 30, 10, 0, 0, 0, time.UTC)

	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-446655440109",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   1,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-1",
		kind:       "scene.played",
		occurredAt: base,
	})
	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-44665544010a",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   2,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-1",
		kind:       "scene.played",
		occurredAt: base.Add(time.Minute),
	})
	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-44665544010b",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   3,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-2",
		kind:       "scene.played",
		occurredAt: base.Add(2 * time.Minute),
	})
	seedEngagementEvent(t, pool, accountID, engagementSeed{
		eventID:    "550e8400-e29b-41d4-a716-44665544010c",
		clientID:   "550e8400-e29b-41d4-a716-446655440001",
		sequence:   4,
		endpoint:   "https://box.example/graphql",
		stashID:    "scene-1",
		kind:       "scene.played",
		occurredAt: base.Add(3 * time.Minute),
	})

	require.NoError(t, builder.Rebuild(context.Background(), accountID))
	require.Equal(t, []projectedItem{
		{version: 1, projectionType: projectionTypeLatency, sessionOrder: 1, itemOrder: 1, stashID: "scene-1", kind: "scene.played"},
		{version: 1, projectionType: projectionTypeLatency, sessionOrder: 1, itemOrder: 2, stashID: "scene-2", kind: "scene.played"},
		{version: 1, projectionType: projectionTypeLatency, sessionOrder: 1, itemOrder: 3, stashID: "scene-1", kind: "scene.played"},
	}, loadProjectionItems(t, pool, accountID, projectionTypeLatency))
}

type engagementSeed struct {
	eventID    string
	clientID   string
	sequence   int64
	endpoint   string
	stashID    string
	kind       string
	occurredAt time.Time
}

type projectedItem struct {
	version        int64
	projectionType string
	sessionOrder   int
	itemOrder      int
	stashID        string
	kind           string
}

func openSessionTestStore(t *testing.T) (*Builder, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is required for PostgreSQL integration tests")
	}

	ctx := context.Background()
	adminPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(adminPool.Close)

	schema := fmt.Sprintf("task5_session_%d", time.Now().UnixNano())
	_, err = adminPool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := adminPool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	})

	schemaDSN := schemaScopedSessionDSN(t, dsn, schema)
	repository, err := store.Open(ctx, schemaDSN)
	require.NoError(t, err)
	t.Cleanup(func() { repository.Close(context.Background()) })
	require.NoError(t, repository.Migrate(ctx))

	pool, err := pgxpool.New(ctx, schemaDSN)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return NewBuilder(pool), pool
}

func schemaScopedSessionDSN(t *testing.T, dsn string, schema string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func seedAccount(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var accountID string
	require.NoError(t, pool.QueryRow(context.Background(), "INSERT INTO accounts DEFAULT VALUES RETURNING id").Scan(&accountID))
	return accountID
}

func seedEngagementEvent(t *testing.T, pool *pgxpool.Pool, accountID string, event engagementSeed) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO engagement_events (
			account_id,
			event_id,
			client_id,
			sequence,
			endpoint,
			stash_id,
			kind,
			occurred_at,
			origin,
			body_hash
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, '\x01')
	`,
		accountID,
		event.eventID,
		event.clientID,
		event.sequence,
		event.endpoint,
		event.stashID,
		event.kind,
		event.occurredAt,
		"history",
	)
	require.NoError(t, err)
}

func loadProjectionItems(t *testing.T, pool *pgxpool.Pool, accountID string, projectionType string) []projectedItem {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT projection_version, projection_type, session_order, item_order, stash_id, kind
		FROM session_projection_items
		WHERE account_id = $1 AND projection_type = $2
		ORDER BY projection_version, session_order, item_order
	`, accountID, projectionType)
	require.NoError(t, err)
	defer rows.Close()

	var items []projectedItem
	for rows.Next() {
		var item projectedItem
		require.NoError(t, rows.Scan(
			&item.version,
			&item.projectionType,
			&item.sessionOrder,
			&item.itemOrder,
			&item.stashID,
			&item.kind,
		))
		items = append(items, item)
	}
	require.NoError(t, rows.Err())
	return items
}
