package session

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteStoreCreateAndGetByID(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	created, err := store.Create(ctx, now)
	if err != nil {
		t.Fatalf("expected create to succeed, got: %v", err)
	}

	if err := ValidateID(created.ID); err != nil {
		t.Fatalf("expected UUIDv4 id, got %q: %v", created.ID, err)
	}
	if !created.CreatedAt.Equal(now) {
		t.Fatalf("expected created_at %v, got %v", now, created.CreatedAt)
	}
	if !created.LastSeenAt.Equal(now) {
		t.Fatalf("expected last_seen_at %v, got %v", now, created.LastSeenAt)
	}

	fetched, err := store.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("expected get by id to succeed, got: %v", err)
	}

	if fetched.ID != created.ID {
		t.Fatalf("expected id %q, got %q", created.ID, fetched.ID)
	}
	if !fetched.CreatedAt.Equal(now) {
		t.Fatalf("expected persisted created_at %v, got %v", now, fetched.CreatedAt)
	}
	if !fetched.LastSeenAt.Equal(now) {
		t.Fatalf("expected persisted last_seen_at %v, got %v", now, fetched.LastSeenAt)
	}
}

func TestSQLiteStoreTouchUpdatesLastSeenAtOnly(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	createdAt := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	sess, err := store.Create(ctx, createdAt)
	if err != nil {
		t.Fatalf("expected create to succeed, got: %v", err)
	}

	touchedAt := createdAt.Add(3 * time.Minute)
	if err := store.Touch(ctx, sess.ID, touchedAt); err != nil {
		t.Fatalf("expected touch to succeed, got: %v", err)
	}

	updated, err := store.GetByID(ctx, sess.ID)
	if err != nil {
		t.Fatalf("expected get after touch to succeed, got: %v", err)
	}

	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created_at to remain %v, got %v", createdAt, updated.CreatedAt)
	}
	if !updated.LastSeenAt.Equal(touchedAt) {
		t.Fatalf("expected last_seen_at %v, got %v", touchedAt, updated.LastSeenAt)
	}
}

func TestSQLiteStoreGetByIDReturnsNotFound(t *testing.T) {
	store := newTestSQLiteStore(t)
	_, err := store.GetByID(context.Background(), "12f9f169-d9ca-4ea3-91e0-18356a1e1477")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteStoreTouchReturnsNotFound(t *testing.T) {
	store := newTestSQLiteStore(t)
	err := store.Touch(context.Background(), "12f9f169-d9ca-4ea3-91e0-18356a1e1477", time.Now().UTC())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func newTestSQLiteStore(t *testing.T) Store {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "session.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite store to initialize, got: %v", err)
	}

	return store
}
