package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/tursodatabase/go-libsql"
)

// Store provides persistence operations for anonymous sessions.
type Store interface {
	Create(ctx context.Context, now time.Time) (Session, error)
	GetByID(ctx context.Context, id string) (Session, error)
	Touch(ctx context.Context, id string, now time.Time) error
}

type sqliteStore struct {
	db *sql.DB
}

// NewSQLiteStore initializes a SQLite-backed session store.
func NewSQLiteStore(databaseURL string) (Store, error) {
	normalizedURL := normalizeDatabaseURL(databaseURL)
	db, err := sql.Open("libsql", normalizedURL)
	if err != nil {
		return nil, fmt.Errorf("open libsql db: %w", err)
	}

	if err := configureSQLiteDB(db, isLocalDatabaseURL(normalizedURL)); err != nil {
		_ = db.Close()
		return nil, err
	}

	store := &sqliteStore{db: db}
	if err := store.bootstrap(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func configureSQLiteDB(db *sql.DB, isLocal bool) error {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if !isLocal {
		return nil
	}

	if err := applySQLiteBusyTimeout(db); err != nil {
		return fmt.Errorf("configure sqlite busy timeout: %w", err)
	}

	return nil
}

func applySQLiteBusyTimeout(db *sql.DB) error {
	rows, err := db.Query("PRAGMA busy_timeout = 5000;")
	if err != nil {
		return err
	}
	defer rows.Close()
	return rows.Err()
}

func isLocalDatabaseURL(databaseURL string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(databaseURL))
	if trimmed == "" {
		return false
	}
	switch {
	case strings.HasPrefix(trimmed, "libsql://"),
		strings.HasPrefix(trimmed, "https://"),
		strings.HasPrefix(trimmed, "http://"),
		strings.HasPrefix(trimmed, "wss://"),
		strings.HasPrefix(trimmed, "ws://"):
		return false
	default:
		return true
	}
}

func normalizeDatabaseURL(databaseURL string) string {
	normalized := strings.TrimSpace(databaseURL)
	if normalized == "" || strings.HasPrefix(strings.ToLower(normalized), "file:") || strings.HasPrefix(normalized, ":memory:") {
		return normalized
	}
	if isLocalDatabaseURL(normalized) {
		return "file:" + normalized
	}
	return normalized
}

func (s *sqliteStore) bootstrap(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	created_at TEXT NOT NULL,
	last_seen_at TEXT NOT NULL
);`

	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("bootstrap sessions schema: %w", err)
	}

	return nil
}

func (s *sqliteStore) Create(ctx context.Context, now time.Time) (Session, error) {
	id, err := NewID()
	if err != nil {
		return Session{}, err
	}

	createdAt := now.UTC()
	timestamp := createdAt.Format(time.RFC3339Nano)

	const query = `
INSERT INTO sessions(id, created_at, last_seen_at)
VALUES (?, ?, ?);`

	if _, err := s.db.ExecContext(ctx, query, id, timestamp, timestamp); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}

	return Session{
		ID:         id,
		CreatedAt:  createdAt,
		LastSeenAt: createdAt,
	}, nil
}

func (s *sqliteStore) GetByID(ctx context.Context, id string) (Session, error) {
	const query = `
SELECT id, created_at, last_seen_at
FROM sessions
WHERE id = ?;`

	row := s.db.QueryRowContext(ctx, query, id)

	var sess Session
	var createdAtRaw string
	var lastSeenAtRaw string
	if err := row.Scan(&sess.ID, &createdAtRaw, &lastSeenAtRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrNotFound
		}
		return Session{}, fmt.Errorf("query session by id: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
	if err != nil {
		return Session{}, fmt.Errorf("parse created_at: %w", err)
	}
	lastSeenAt, err := time.Parse(time.RFC3339Nano, lastSeenAtRaw)
	if err != nil {
		return Session{}, fmt.Errorf("parse last_seen_at: %w", err)
	}

	sess.CreatedAt = createdAt
	sess.LastSeenAt = lastSeenAt

	return sess, nil
}

func (s *sqliteStore) Touch(ctx context.Context, id string, now time.Time) error {
	const query = `
UPDATE sessions
SET last_seen_at = ?
WHERE id = ?;`

	timestamp := now.UTC().Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx, query, timestamp, id)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("touch session rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}
