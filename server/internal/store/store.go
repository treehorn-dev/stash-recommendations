package store

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/treehorn/stash-recommendations/server/internal/auth"
	"github.com/treehorn/stash-recommendations/server/internal/catalog"
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
	{version: "006_source_catalog_projections", path: "migrations/006_source_catalog_projections.sql"},
	{version: "007_recommendation_indexes", path: "migrations/007_recommendation_indexes.sql"},
	{version: "008_source_catalog_groups", path: "migrations/008_source_catalog_groups.sql"},
	{version: "009_pgvector_recommendations", path: "migrations/009_pgvector_recommendations.sql"},
	{version: "010_predicted_ratings", path: "migrations/010_predicted_ratings.sql"},
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

func (store *Store) Pool() *pgxpool.Pool {
	return store.pool
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

func (store *Store) UpsertSnapshot(ctx context.Context, snapshot domain.SourceSnapshot, raw json.RawMessage) error {
	sourceUpdatedAt := snapshot.SourceUpdatedAt.UTC().Truncate(time.Microsecond)

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin source snapshot transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))", snapshot.ContentKey.Endpoint, snapshot.ContentKey.StashID); err != nil {
		return fmt.Errorf("lock source snapshot: %w", err)
	}

	currentVersion, exists, err := currentSourceSnapshotVersion(ctx, tx, snapshot.ContentKey)
	if err != nil {
		return err
	}
	if exists && !sourceUpdatedAt.After(currentVersion) {
		return nil
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO source_configs (endpoint)
		VALUES ($1)
		ON CONFLICT (endpoint) DO NOTHING
	`, snapshot.ContentKey.Endpoint); err != nil {
		return fmt.Errorf("ensure source config: %w", err)
	}

	result, err := tx.Exec(ctx, `
		INSERT INTO source_snapshots (
			endpoint,
			stash_id,
			schema_version,
			captured_at,
			source_updated_at,
			snapshot
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb)
		ON CONFLICT (endpoint, stash_id) DO UPDATE
		SET
			schema_version = EXCLUDED.schema_version,
			captured_at = EXCLUDED.captured_at,
			source_updated_at = EXCLUDED.source_updated_at,
			snapshot = EXCLUDED.snapshot
		WHERE source_snapshots.source_updated_at IS NULL
			OR source_snapshots.source_updated_at < EXCLUDED.source_updated_at
	`, snapshot.ContentKey.Endpoint, snapshot.ContentKey.StashID, snapshot.SchemaVersion, snapshot.CapturedAt, sourceUpdatedAt, []byte(raw))
	if err != nil {
		return fmt.Errorf("upsert source snapshot: %w", err)
	}
	if result.RowsAffected() == 0 {
		return nil
	}

	for _, performer := range snapshot.Performers {
		aliases, err := json.Marshal(performer.Aliases)
		if err != nil {
			return fmt.Errorf("marshal performer aliases: %w", err)
		}
		careerYears, err := json.Marshal(performer.CareerYears)
		if err != nil {
			return fmt.Errorf("marshal performer career years: %w", err)
		}
		urls, err := json.Marshal(performer.URLs)
		if err != nil {
			return fmt.Errorf("marshal performer urls: %w", err)
		}
		remoteImages, err := json.Marshal(performer.RemoteImages)
		if err != nil {
			return fmt.Errorf("marshal performer remote images: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO source_performers (
				endpoint,
				stash_id,
				name,
				aliases,
				gender,
				country,
				ethnicity,
				eye_color,
				hair_color,
				measurements,
				career_years,
				urls,
				remote_images,
				source_updated_at
			) VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9, $10, $11::jsonb, $12::jsonb, $13::jsonb, $14)
			ON CONFLICT (endpoint, stash_id) DO UPDATE
			SET
				name = EXCLUDED.name,
				aliases = EXCLUDED.aliases,
				gender = EXCLUDED.gender,
				country = EXCLUDED.country,
				ethnicity = EXCLUDED.ethnicity,
				eye_color = EXCLUDED.eye_color,
				hair_color = EXCLUDED.hair_color,
				measurements = EXCLUDED.measurements,
				career_years = EXCLUDED.career_years,
				urls = EXCLUDED.urls,
				remote_images = EXCLUDED.remote_images,
				source_updated_at = EXCLUDED.source_updated_at
			WHERE source_performers.source_updated_at IS NULL
				OR source_performers.source_updated_at < EXCLUDED.source_updated_at
		`, snapshot.ContentKey.Endpoint, performer.ID, performer.Name, aliases, performer.Gender, performer.Country, performer.Ethnicity, performer.EyeColor, performer.HairColor, performer.Measurements, careerYears, urls, remoteImages, sourceUpdatedAt); err != nil {
			return fmt.Errorf("upsert source performer %s: %w", performer.ID, err)
		}
	}

	for _, scene := range snapshot.Scenes {
		dates, err := json.Marshal(scene.Dates)
		if err != nil {
			return fmt.Errorf("marshal scene dates: %w", err)
		}
		urls, err := json.Marshal(scene.URLs)
		if err != nil {
			return fmt.Errorf("marshal scene urls: %w", err)
		}
		remoteImages, err := json.Marshal(scene.RemoteImages)
		if err != nil {
			return fmt.Errorf("marshal scene remote images: %w", err)
		}

		var studioEndpoint any
		var studioStashID any
		if scene.Studio != nil {
			studioEndpoint = snapshot.ContentKey.Endpoint
			studioStashID = scene.Studio.ID
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_studios (endpoint, stash_id, name, source_updated_at)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (endpoint, stash_id) DO UPDATE
				SET
					name = EXCLUDED.name,
					source_updated_at = EXCLUDED.source_updated_at
				WHERE source_studios.source_updated_at IS NULL
					OR source_studios.source_updated_at < EXCLUDED.source_updated_at
			`, snapshot.ContentKey.Endpoint, scene.Studio.ID, scene.Studio.Name, sourceUpdatedAt); err != nil {
				return fmt.Errorf("upsert source studio %s: %w", scene.Studio.ID, err)
			}
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO source_scenes (
				endpoint,
				stash_id,
				title,
				details,
				dates,
				urls,
				duration,
				director,
				code,
				studio_endpoint,
				studio_stash_id,
				source_updated_at,
				remote_images
			) VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8, $9, $10, $11, $12, $13::jsonb)
			ON CONFLICT (endpoint, stash_id) DO UPDATE
			SET
				title = EXCLUDED.title,
				details = EXCLUDED.details,
				dates = EXCLUDED.dates,
				urls = EXCLUDED.urls,
				duration = EXCLUDED.duration,
				director = EXCLUDED.director,
				code = EXCLUDED.code,
				studio_endpoint = EXCLUDED.studio_endpoint,
				studio_stash_id = EXCLUDED.studio_stash_id,
				source_updated_at = EXCLUDED.source_updated_at,
				remote_images = EXCLUDED.remote_images
			WHERE source_scenes.source_updated_at IS NULL
				OR source_scenes.source_updated_at < EXCLUDED.source_updated_at
		`, snapshot.ContentKey.Endpoint, scene.ID, scene.Title, scene.Details, dates, urls, scene.Duration, scene.Director, scene.Code, studioEndpoint, studioStashID, sourceUpdatedAt, remoteImages); err != nil {
			return fmt.Errorf("upsert source scene %s: %w", scene.ID, err)
		}

		if _, err := tx.Exec(ctx, `
			DELETE FROM source_scene_performers
			WHERE scene_endpoint = $1 AND scene_stash_id = $2
		`, snapshot.ContentKey.Endpoint, scene.ID); err != nil {
			return fmt.Errorf("clear source scene performers %s: %w", scene.ID, err)
		}
		for index, appearance := range scene.PerformerAppearances {
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_scene_performers (
					scene_endpoint,
					scene_stash_id,
					performer_endpoint,
					performer_stash_id,
					appearance_order
				) VALUES ($1, $2, $3, $4, $5)
			`, snapshot.ContentKey.Endpoint, scene.ID, snapshot.ContentKey.Endpoint, appearance.PerformerID, index+1); err != nil {
				return fmt.Errorf("insert source scene performer %s/%s: %w", scene.ID, appearance.PerformerID, err)
			}
		}

		if _, err := tx.Exec(ctx, `
			DELETE FROM source_scene_tags
			WHERE scene_endpoint = $1 AND scene_stash_id = $2
		`, snapshot.ContentKey.Endpoint, scene.ID); err != nil {
			return fmt.Errorf("clear source scene tags %s: %w", scene.ID, err)
		}
		for index, tag := range scene.Tags {
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_tags (endpoint, stash_id, name, source_updated_at)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (endpoint, stash_id) DO UPDATE
				SET
					name = EXCLUDED.name,
					source_updated_at = EXCLUDED.source_updated_at
				WHERE source_tags.source_updated_at IS NULL
					OR source_tags.source_updated_at < EXCLUDED.source_updated_at
			`, snapshot.ContentKey.Endpoint, tag.ID, tag.Name, sourceUpdatedAt); err != nil {
				return fmt.Errorf("upsert source tag %s: %w", tag.ID, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_scene_tags (
					scene_endpoint,
					scene_stash_id,
					tag_endpoint,
					tag_stash_id,
					tag_order
				) VALUES ($1, $2, $3, $4, $5)
			`, snapshot.ContentKey.Endpoint, scene.ID, snapshot.ContentKey.Endpoint, tag.ID, index+1); err != nil {
				return fmt.Errorf("insert source scene tag %s/%s: %w", scene.ID, tag.ID, err)
			}
		}

		if _, err := tx.Exec(ctx, `
			DELETE FROM source_scene_groups
			WHERE scene_endpoint = $1 AND scene_stash_id = $2
		`, snapshot.ContentKey.Endpoint, scene.ID); err != nil {
			return fmt.Errorf("clear source scene groups %s: %w", scene.ID, err)
		}
		for index, group := range scene.Groups {
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_groups (endpoint, stash_id, name, source_updated_at)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (endpoint, stash_id) DO UPDATE
				SET
					name = EXCLUDED.name,
					source_updated_at = EXCLUDED.source_updated_at
				WHERE source_groups.source_updated_at IS NULL
					OR source_groups.source_updated_at < EXCLUDED.source_updated_at
			`, snapshot.ContentKey.Endpoint, group.ID, group.Name, sourceUpdatedAt); err != nil {
				return fmt.Errorf("upsert source group %s: %w", group.ID, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_scene_groups (
					scene_endpoint,
					scene_stash_id,
					group_endpoint,
					group_stash_id,
					group_order
				) VALUES ($1, $2, $3, $4, $5)
			`, snapshot.ContentKey.Endpoint, scene.ID, snapshot.ContentKey.Endpoint, group.ID, index+1); err != nil {
				return fmt.Errorf("insert source scene group %s/%s: %w", scene.ID, group.ID, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit source snapshot transaction: %w", err)
	}
	return nil
}

func (store *Store) CatalogSource(ctx context.Context, key domain.ContentKey) (catalog.Source, bool, error) {
	var (
		source                 catalog.Source
		datesJSON              []byte
		urlsJSON               []byte
		remoteImagesJSON       []byte
		canonicalSceneTemplate *string
		studioEndpoint         *string
		studioStashID          *string
	)

	err := store.pool.QueryRow(ctx, `
		SELECT
			source_scenes.title,
			source_scenes.details,
			source_scenes.dates,
			source_scenes.urls,
			source_scenes.duration,
			source_scenes.director,
			source_scenes.code,
			source_scenes.remote_images,
			source_scenes.studio_endpoint,
			source_scenes.studio_stash_id,
			source_configs.canonical_scene_url_template
		FROM source_scenes
		LEFT JOIN source_configs ON source_configs.endpoint = source_scenes.endpoint
		WHERE source_scenes.endpoint = $1 AND source_scenes.stash_id = $2
	`, key.Endpoint, key.StashID).Scan(
		&source.Title,
		&source.Details,
		&datesJSON,
		&urlsJSON,
		&source.Duration,
		&source.Director,
		&source.Code,
		&remoteImagesJSON,
		&studioEndpoint,
		&studioStashID,
		&canonicalSceneTemplate,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return catalog.Source{}, false, nil
		}
		return catalog.Source{}, false, fmt.Errorf("query source scene: %w", err)
	}

	source.ContentKey = key
	if err := json.Unmarshal(datesJSON, &source.Dates); err != nil {
		return catalog.Source{}, false, fmt.Errorf("decode source scene dates: %w", err)
	}
	if err := json.Unmarshal(urlsJSON, &source.URLs); err != nil {
		return catalog.Source{}, false, fmt.Errorf("decode source scene urls: %w", err)
	}
	if err := json.Unmarshal(remoteImagesJSON, &source.RemoteImages); err != nil {
		return catalog.Source{}, false, fmt.Errorf("decode source scene remote images: %w", err)
	}

	if studioEndpoint != nil && studioStashID != nil {
		var studio catalog.EntityReference
		studio.ContentKey = domain.ContentKey{Endpoint: *studioEndpoint, StashID: *studioStashID}
		if err := store.pool.QueryRow(ctx, `
			SELECT name
			FROM source_studios
			WHERE endpoint = $1 AND stash_id = $2
		`, *studioEndpoint, *studioStashID).Scan(&studio.Name); err != nil {
			return catalog.Source{}, false, fmt.Errorf("query source studio: %w", err)
		}
		source.Studio = &studio
	}

	performerRows, err := store.pool.Query(ctx, `
		SELECT source_performers.endpoint, source_performers.stash_id, source_performers.name
		FROM source_scene_performers
		JOIN source_performers
			ON source_performers.endpoint = source_scene_performers.performer_endpoint
			AND source_performers.stash_id = source_scene_performers.performer_stash_id
		WHERE source_scene_performers.scene_endpoint = $1 AND source_scene_performers.scene_stash_id = $2
		ORDER BY source_scene_performers.appearance_order
	`, key.Endpoint, key.StashID)
	if err != nil {
		return catalog.Source{}, false, fmt.Errorf("query source performers: %w", err)
	}
	defer performerRows.Close()
	for performerRows.Next() {
		var performer catalog.EntityReference
		if err := performerRows.Scan(&performer.ContentKey.Endpoint, &performer.ContentKey.StashID, &performer.Name); err != nil {
			return catalog.Source{}, false, fmt.Errorf("scan source performer: %w", err)
		}
		source.Performers = append(source.Performers, performer)
	}
	if err := performerRows.Err(); err != nil {
		return catalog.Source{}, false, fmt.Errorf("iterate source performers: %w", err)
	}

	tagRows, err := store.pool.Query(ctx, `
		SELECT source_tags.endpoint, source_tags.stash_id, source_tags.name
		FROM source_scene_tags
		JOIN source_tags
			ON source_tags.endpoint = source_scene_tags.tag_endpoint
			AND source_tags.stash_id = source_scene_tags.tag_stash_id
		WHERE source_scene_tags.scene_endpoint = $1 AND source_scene_tags.scene_stash_id = $2
		ORDER BY source_scene_tags.tag_order
	`, key.Endpoint, key.StashID)
	if err != nil {
		return catalog.Source{}, false, fmt.Errorf("query source tags: %w", err)
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var tag catalog.EntityReference
		if err := tagRows.Scan(&tag.ContentKey.Endpoint, &tag.ContentKey.StashID, &tag.Name); err != nil {
			return catalog.Source{}, false, fmt.Errorf("scan source tag: %w", err)
		}
		source.Tags = append(source.Tags, tag)
	}
	if err := tagRows.Err(); err != nil {
		return catalog.Source{}, false, fmt.Errorf("iterate source tags: %w", err)
	}

	groupRows, err := store.pool.Query(ctx, `
		SELECT source_groups.endpoint, source_groups.stash_id, source_groups.name
		FROM source_scene_groups
		JOIN source_groups
			ON source_groups.endpoint = source_scene_groups.group_endpoint
			AND source_groups.stash_id = source_scene_groups.group_stash_id
		WHERE source_scene_groups.scene_endpoint = $1 AND source_scene_groups.scene_stash_id = $2
		ORDER BY source_scene_groups.group_order
	`, key.Endpoint, key.StashID)
	if err != nil {
		return catalog.Source{}, false, fmt.Errorf("query source groups: %w", err)
	}
	defer groupRows.Close()
	for groupRows.Next() {
		var group catalog.EntityReference
		if err := groupRows.Scan(&group.ContentKey.Endpoint, &group.ContentKey.StashID, &group.Name); err != nil {
			return catalog.Source{}, false, fmt.Errorf("scan source group: %w", err)
		}
		source.Groups = append(source.Groups, group)
	}
	if err := groupRows.Err(); err != nil {
		return catalog.Source{}, false, fmt.Errorf("iterate source groups: %w", err)
	}

	if canonicalSceneTemplate != nil {
		canonicalURL := strings.ReplaceAll(*canonicalSceneTemplate, "{stash_id}", key.StashID)
		canonicalURL = strings.ReplaceAll(canonicalURL, "{id}", key.StashID)
		source.CanonicalURL = &canonicalURL
	}

	return source, true, nil
}

func currentSourceSnapshotVersion(ctx context.Context, tx pgx.Tx, key domain.ContentKey) (time.Time, bool, error) {
	var sourceUpdatedAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT source_updated_at
		FROM source_snapshots
		WHERE endpoint = $1 AND stash_id = $2
	`, key.Endpoint, key.StashID).Scan(&sourceUpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("query source snapshot version: %w", err)
	}
	return sourceUpdatedAt, true, nil
}
