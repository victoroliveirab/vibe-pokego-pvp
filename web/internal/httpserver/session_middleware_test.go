package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
)

func TestSessionMiddlewareMissingHeaderReturnsInvalidSession(t *testing.T) {
	store := newMiddlewareTestStore(t)
	handler := withSessionValidation(store, time.Now, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called for missing header")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected/ping", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertInvalidSessionResponse(t, rec, "missing header")
}

func TestSessionMiddlewareMalformedHeaderReturnsInvalidSession(t *testing.T) {
	store := newMiddlewareTestStore(t)
	handler := withSessionValidation(store, time.Now, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called for malformed header")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected/ping", nil)
	req.Header.Set(sessionHeaderName, "not-a-uuid")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertInvalidSessionResponse(t, rec, "malformed header")
}

func TestSessionMiddlewareUnknownSessionReturnsInvalidSession(t *testing.T) {
	store := newMiddlewareTestStore(t)
	handler := withSessionValidation(store, time.Now, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler should not be called for unknown session")
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected/ping", nil)
	req.Header.Set(sessionHeaderName, "12f9f169-d9ca-4ea3-91e0-18356a1e1477")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertInvalidSessionResponse(t, rec, "unknown session")
}

func TestSessionMiddlewareValidSessionCallsNextAndTouchesTimestamp(t *testing.T) {
	store := newMiddlewareTestStore(t)
	ctx := context.Background()

	createdAt := time.Date(2026, time.March, 1, 16, 0, 0, 0, time.UTC)
	sess, err := store.Create(ctx, createdAt)
	if err != nil {
		t.Fatalf("expected session create, got: %v", err)
	}

	touchedAt := createdAt.Add(5 * time.Minute)
	nextCalled := false
	handler := withSessionValidation(store, func() time.Time { return touchedAt }, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		ctxSession, ok := SessionFromContext(r.Context())
		if !ok {
			t.Fatal("expected session in context")
		}
		if ctxSession.ID != sess.ID {
			t.Fatalf("expected context session id %q, got %q", sess.ID, ctxSession.ID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/protected/ping", nil)
	req.Header.Set(sessionHeaderName, sess.ID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if !nextCalled {
		t.Fatal("expected next handler to be called")
	}

	updated, err := store.GetByID(ctx, sess.ID)
	if err != nil {
		t.Fatalf("expected session fetch after touch, got: %v", err)
	}
	if !updated.LastSeenAt.Equal(touchedAt) {
		t.Fatalf("expected touched timestamp %v, got %v", touchedAt, updated.LastSeenAt)
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created_at to remain %v, got %v", createdAt, updated.CreatedAt)
	}
}

func newMiddlewareTestStore(t *testing.T) session.Store {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "session-middleware.db")
	store, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite store, got: %v", err)
	}

	return store
}

func assertInvalidSessionResponse(t *testing.T, rec *httptest.ResponseRecorder, scenario string) {
	t.Helper()

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("%s: expected status %d, got %d", scenario, http.StatusUnauthorized, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("%s: expected JSON error payload, got: %v", scenario, err)
	}
	if payload.Error.Code != "INVALID_SESSION" {
		t.Fatalf("%s: expected INVALID_SESSION code, got %q", scenario, payload.Error.Code)
	}
	if payload.Error.Message != "Missing or invalid X-Session-Id" {
		t.Fatalf("%s: expected invalid session message, got %q", scenario, payload.Error.Message)
	}
}
