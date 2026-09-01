package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/treehorn/stash-recommendations/server/internal/auth"
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
}

var ErrInvalidAPIKey = errors.New("invalid API key")

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
