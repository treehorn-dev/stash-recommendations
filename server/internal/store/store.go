package store

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/treehorn/stash-recommendations/server/internal/auth"
	"github.com/treehorn/stash-recommendations/server/internal/domain"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version string
	path    string
}

var migrations = []migration{
	{version: "001_initial", path: "migrations/001_initial.sql"},
	{version: "002_api_key_identifier", path: "migrations/002_api_key_identifier.sql"},
	{version: "003_legacy_api_key_auth", path: "migrations/003_legacy_api_key_auth.sql"},
	{version: "004_revoke_legacy_api_keys", path: "migrations/004_revoke_legacy_api_keys.sql"},
	{version: "005_session_projections", path: "migrations/005_session_projections.sql"},
}

var ErrInvalidAPIKey = errors.New("invalid API key")
var ErrInteractionEventConflict = errors.New("interaction event conflict")

type Account struct {
	ID        string
	CreatedAt time.Time
}

type IssuedAccount struct {
	Account
	PlaintextKey string
}

// AccountRepository is the account identity boundary used by HTTP middleware.
type AccountRepository interface {
	CreateAccount(ctx context.Context) (IssuedAccount, error)
	Authenticate(ctx context.Context, plaintextKey string) (Account, error)
}

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL store: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL store: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (store *Store) Close(context.Context) {
	store.pool.Close()
}

func (store *Store) Migrate(ctx context.Context) error {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", int64(62096831461253021)); err != nil {
		return fmt.Errorf("lock initial migration: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	for _, migration := range migrations {
		var applied bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", migration.version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", migration.version, err)
		}
		if applied {
			continue
		}

		contents, err := migrationFiles.ReadFile(migration.path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migration.version, err)
		}
		for _, statement := range strings.Split(string(contents), ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err := tx.Exec(ctx, statement); err != nil {
				return fmt.Errorf("apply migration %s: %w", migration.version, err)
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", migration.version); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.version, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit initial migration: %w", err)
	}
	return nil
}

func (store *Store) CreateAccount(ctx context.Context) (IssuedAccount, error) {
	plaintextKey, err := auth.NewAPIKey()
	if err != nil {
		return IssuedAccount{}, err
	}
	keyID, secret, ok := auth.ParseAPIKey(plaintextKey)
	if !ok {
		return IssuedAccount{}, fmt.Errorf("generated invalid API key")
	}
	hash, err := auth.HashAPIKey(secret)
	if err != nil {
		return IssuedAccount{}, err
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return IssuedAccount{}, fmt.Errorf("begin account transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var account Account
	if err := tx.QueryRow(ctx, "INSERT INTO accounts DEFAULT VALUES RETURNING id, created_at").Scan(&account.ID, &account.CreatedAt); err != nil {
		return IssuedAccount{}, fmt.Errorf("create account: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO api_keys (account_id, key_id, key_hash) VALUES ($1, $2, $3)", account.ID, keyID, hash); err != nil {
		return IssuedAccount{}, fmt.Errorf("create API key: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return IssuedAccount{}, fmt.Errorf("commit account transaction: %w", err)
	}
	return IssuedAccount{Account: account, PlaintextKey: plaintextKey}, nil
}

func (store *Store) Authenticate(ctx context.Context, plaintextKey string) (Account, error) {
	keyID, secret, ok := auth.ParseAPIKey(plaintextKey)
	if !ok {
		return Account{}, ErrInvalidAPIKey
	}

	var account Account
	var hash string
	err := store.pool.QueryRow(ctx, `
		SELECT accounts.id, accounts.created_at, api_keys.key_hash
		FROM api_keys
		JOIN accounts ON accounts.id = api_keys.account_id
		WHERE api_keys.key_id = $1 AND api_keys.revoked_at IS NULL
	`, keyID).Scan(&account.ID, &account.CreatedAt, &hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, ErrInvalidAPIKey
		}
		return Account{}, fmt.Errorf("query API keys: %w", err)
	}
	if !auth.VerifyAPIKey(hash, secret) {
		return Account{}, ErrInvalidAPIKey
	}
	return account, nil
}

func (store *Store) AcceptInteractionEvent(ctx context.Context, accountID string, event domain.PreferenceEvent, bodyHash []byte) (bool, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, fmt.Errorf("begin interaction transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := lockInteractionEvent(ctx, tx, accountID, event.EventID); err != nil {
		return false, err
	}

	inserted, err := insertInteractionEvent(ctx, tx, accountID, event, bodyHash)
	if err != nil {
		return false, err
	}
	if !inserted {
		return false, nil
	}

	if isRatingEvent(event.Kind) {
		if err := applyCurrentPreference(ctx, tx, accountID, event); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit interaction transaction: %w", err)
	}
	return true, nil
}

func insertInteractionEvent(ctx context.Context, tx pgx.Tx, accountID string, event domain.PreferenceEvent, bodyHash []byte) (bool, error) {
	existingHash, exists, err := findExistingInteractionEventHash(ctx, tx, accountID, event.EventID)
	if err != nil {
		return false, err
	}
	if exists {
		if bytes.Equal(existingHash, bodyHash) {
			return false, nil
		}
		return false, ErrInteractionEventConflict
	}

	var (
		rowsAffected int64
		tableName    string
	)

	switch event.Kind {
	case domain.PreferenceEventKindSceneRatingSet, domain.PreferenceEventKindSceneRatingRemove:
		tableName = "preference_events"
		commandTag, execErr := tx.Exec(ctx, `
			INSERT INTO preference_events (
				account_id,
				event_id,
				client_id,
				sequence,
				endpoint,
				stash_id,
				kind,
				rating,
				occurred_at,
				origin,
				body_hash
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (account_id, event_id) DO NOTHING
		`,
			accountID,
			event.EventID,
			event.ClientID,
			event.Sequence,
			event.ContentKey.Endpoint,
			event.ContentKey.StashID,
			event.Kind,
			event.Rating,
			event.OccurredAt,
			event.Origin,
			bodyHash,
		)
		err = execErr
		rowsAffected = commandTag.RowsAffected()
	case domain.PreferenceEventKindScenePlayed, domain.PreferenceEventKindSceneO:
		tableName = "engagement_events"
		commandTag, execErr := tx.Exec(ctx, `
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
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			ON CONFLICT (account_id, event_id) DO NOTHING
		`,
			accountID,
			event.EventID,
			event.ClientID,
			event.Sequence,
			event.ContentKey.Endpoint,
			event.ContentKey.StashID,
			event.Kind,
			event.OccurredAt,
			event.Origin,
			bodyHash,
		)
		err = execErr
		rowsAffected = commandTag.RowsAffected()
	default:
		return false, fmt.Errorf("unsupported interaction event kind %q", event.Kind)
	}
	if err != nil {
		return false, fmt.Errorf("insert %s: %w", tableName, err)
	}
	if rowsAffected == 1 {
		return true, nil
	}
	return false, fmt.Errorf("insert %s: interaction event was not persisted", tableName)
}

func applyCurrentPreference(ctx context.Context, tx pgx.Tx, accountID string, event domain.PreferenceEvent) error {
	newerExists, err := hasStrictlyNewerPreferenceEvent(ctx, tx, accountID, event)
	if err != nil {
		return err
	}
	if newerExists {
		return nil
	}

	switch event.Kind {
	case domain.PreferenceEventKindSceneRatingSet:
		_, err = tx.Exec(ctx, `
			INSERT INTO current_preferences (
				account_id,
				endpoint,
				stash_id,
				rating,
				client_id,
				sequence,
				occurred_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (account_id, endpoint, stash_id) DO UPDATE
			SET
				rating = EXCLUDED.rating,
				client_id = EXCLUDED.client_id,
				sequence = EXCLUDED.sequence,
				occurred_at = EXCLUDED.occurred_at
			WHERE current_preferences.sequence < EXCLUDED.sequence
				OR (current_preferences.sequence = EXCLUDED.sequence AND current_preferences.client_id < EXCLUDED.client_id)
		`,
			accountID,
			event.ContentKey.Endpoint,
			event.ContentKey.StashID,
			event.Rating,
			event.ClientID,
			event.Sequence,
			event.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf("upsert current preference: %w", err)
		}
	case domain.PreferenceEventKindSceneRatingRemove:
		_, err = tx.Exec(ctx, `
			DELETE FROM current_preferences
			WHERE account_id = $1
				AND endpoint = $2
				AND stash_id = $3
				AND (
					sequence < $4
					OR (sequence = $4 AND client_id < $5)
				)
		`,
			accountID,
			event.ContentKey.Endpoint,
			event.ContentKey.StashID,
			event.Sequence,
			event.ClientID,
		)
		if err != nil {
			return fmt.Errorf("delete current preference: %w", err)
		}
	}
	return nil
}

func isRatingEvent(kind string) bool {
	return kind == domain.PreferenceEventKindSceneRatingSet || kind == domain.PreferenceEventKindSceneRatingRemove
}

func lockInteractionEvent(ctx context.Context, tx pgx.Tx, accountID string, eventID string) error {
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))", accountID, eventID); err != nil {
		return fmt.Errorf("lock interaction event: %w", err)
	}
	return nil
}

func findExistingInteractionEventHash(ctx context.Context, tx pgx.Tx, accountID string, eventID string) ([]byte, bool, error) {
	var bodyHash []byte
	err := tx.QueryRow(ctx, `
		SELECT body_hash
		FROM (
			SELECT body_hash FROM preference_events WHERE account_id = $1 AND event_id = $2
			UNION ALL
			SELECT body_hash FROM engagement_events WHERE account_id = $1 AND event_id = $2
		) AS interaction_events
		LIMIT 1
	`, accountID, eventID).Scan(&bodyHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("load existing interaction replay: %w", err)
	}
	return bodyHash, true, nil
}

func hasStrictlyNewerPreferenceEvent(ctx context.Context, tx pgx.Tx, accountID string, event domain.PreferenceEvent) (bool, error) {
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM preference_events
			WHERE account_id = $1
				AND endpoint = $2
				AND stash_id = $3
				AND (
					sequence > $4
					OR (sequence = $4 AND client_id > $5)
				)
		)
	`,
		accountID,
		event.ContentKey.Endpoint,
		event.ContentKey.StashID,
		event.Sequence,
		event.ClientID,
	).Scan(&exists); err != nil {
		return false, fmt.Errorf("query newer preference events: %w", err)
	}
	return exists, nil
}
