package session

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/treehorn/stash-recommendations/server/internal/domain"
)

const (
	projectionTypeLatency  = "latency"
	projectionTypeOBounded = "o_bounded"
	maxLatencyGap          = 2 * time.Hour
)

type Builder struct {
	pool                  *pgxpool.Pool
	afterVersionAllocated func()
}

type engagementEvent struct {
	EventID    string
	Endpoint   string
	StashID    string
	Kind       string
	OccurredAt time.Time
}

func NewBuilder(pool *pgxpool.Pool) *Builder {
	return &Builder{pool: pool}
}

func (builder *Builder) Rebuild(ctx context.Context, accountID string) error {
	tx, err := builder.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin session rebuild transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockProjectionAccount(ctx, tx, accountID); err != nil {
		return err
	}

	events, err := loadEngagementEvents(ctx, tx, accountID)
	if err != nil {
		return err
	}

	version, err := nextProjectionVersion(ctx, tx, accountID)
	if err != nil {
		return err
	}
	if builder.afterVersionAllocated != nil {
		builder.afterVersionAllocated()
	}

	latencySessions := buildLatencySessions(events)
	oBoundedSessions := buildOBoundedSessions(events)

	if err := insertProjection(ctx, tx, accountID, version, projectionTypeLatency, latencySessions); err != nil {
		return err
	}
	if err := insertProjection(ctx, tx, accountID, version, projectionTypeOBounded, oBoundedSessions); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit session rebuild transaction: %w", err)
	}
	return nil
}

func lockProjectionAccount(ctx context.Context, tx pgx.Tx, accountID string) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))", "session_projection_rebuild", accountID); err != nil {
		return fmt.Errorf("lock session projection account: %w", err)
	}
	return nil
}

func loadEngagementEvents(ctx context.Context, tx pgx.Tx, accountID string) ([]engagementEvent, error) {
	rows, err := tx.Query(ctx, `
		SELECT event_id, endpoint, stash_id, kind, occurred_at
		FROM engagement_events
		WHERE account_id = $1
		ORDER BY occurred_at, event_id
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("query engagement events: %w", err)
	}
	defer rows.Close()

	var events []engagementEvent
	for rows.Next() {
		var event engagementEvent
		if err := rows.Scan(&event.EventID, &event.Endpoint, &event.StashID, &event.Kind, &event.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan engagement event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate engagement events: %w", err)
	}
	return events, nil
}

func nextProjectionVersion(ctx context.Context, tx pgx.Tx, accountID string) (int64, error) {
	var version int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(projection_version), 0) + 1
		FROM session_projections
		WHERE account_id = $1
	`, accountID).Scan(&version); err != nil {
		return 0, fmt.Errorf("select next session projection version: %w", err)
	}
	return version, nil
}

func buildLatencySessions(events []engagementEvent) [][]engagementEvent {
	if len(events) == 0 {
		return nil
	}

	var (
		sessions [][]engagementEvent
		current  []engagementEvent
		previous *engagementEvent
	)

	for _, event := range events {
		if previous == nil || event.OccurredAt.Sub(previous.OccurredAt) > maxLatencyGap {
			if len(current) > 0 {
				sessions = append(sessions, current)
			}
			current = nil
		}
		current = appendCollapsed(current, event)
		previous = &event
	}
	if len(current) > 0 {
		sessions = append(sessions, current)
	}
	return sessions
}

func buildOBoundedSessions(events []engagementEvent) [][]engagementEvent {
	var (
		sessions [][]engagementEvent
		current  []engagementEvent
	)

	for _, event := range events {
		current = appendCollapsed(current, event)
		if event.Kind == domain.PreferenceEventKindSceneO {
			sessions = append(sessions, current)
			current = nil
		}
	}
	if len(current) > 0 {
		sessions = append(sessions, current)
	}
	return sessions
}

func appendCollapsed(session []engagementEvent, event engagementEvent) []engagementEvent {
	if len(session) == 0 {
		return append(session, event)
	}
	last := session[len(session)-1]
	if last.Endpoint == event.Endpoint && last.StashID == event.StashID {
		session[len(session)-1] = event
		return session
	}
	return append(session, event)
}

func insertProjection(ctx context.Context, tx pgx.Tx, accountID string, version int64, projectionType string, sessions [][]engagementEvent) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO session_projections (account_id, projection_version, projection_type)
		VALUES ($1, $2, $3)
	`, accountID, version, projectionType); err != nil {
		return fmt.Errorf("insert session projection: %w", err)
	}

	for sessionIndex, session := range sessions {
		for itemIndex, event := range session {
			if _, err := tx.Exec(ctx, `
				INSERT INTO session_projection_items (
					account_id,
					projection_version,
					projection_type,
					session_order,
					item_order,
					event_id,
					endpoint,
					stash_id,
					kind,
					occurred_at
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`,
				accountID,
				version,
				projectionType,
				sessionIndex+1,
				itemIndex+1,
				event.EventID,
				event.Endpoint,
				event.StashID,
				event.Kind,
				event.OccurredAt,
			); err != nil {
				return fmt.Errorf("insert session projection item: %w", err)
			}
		}
	}
	return nil
}
