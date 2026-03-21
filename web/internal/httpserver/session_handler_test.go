package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
)

func TestSessionHandlerCreateSuccess(t *testing.T) {
	now := time.Date(2026, time.March, 1, 15, 0, 0, 0, time.UTC)
	store := &fakeSessionStore{
		createFn: func(ctx context.Context, at time.Time) (session.Session, error) {
			if !at.Equal(now) {
				t.Fatalf("expected now %v, got %v", now, at)
			}
			return session.Session{ID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"}, nil
		},
	}

	handler := newSessionHandler(store, nil, func() time.Time { return now })

	req := httptest.NewRequest(http.MethodPost, "/session", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected content type application/json, got %q", got)
	}

	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected valid JSON, got error: %v", err)
	}
	if payload["sessionId"] != "12f9f169-d9ca-4ea3-91e0-18356a1e1477" {
		t.Fatalf("expected sessionId in response, got %#v", payload)
	}
}

func TestSessionHandlerMethodNotAllowed(t *testing.T) {
	handler := newSessionHandler(&fakeSessionStore{}, nil, time.Now)

	req := httptest.NewRequest(http.MethodGet, "/session", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow header %q, got %q", http.MethodPost, allow)
	}
}

func TestSessionHandlerCreateFailureReturnsAPIError(t *testing.T) {
	store := &fakeSessionStore{
		createFn: func(context.Context, time.Time) (session.Session, error) {
			return session.Session{}, errors.New("db unavailable")
		},
	}
	handler := newSessionHandler(store, nil, time.Now)

	req := httptest.NewRequest(http.MethodPost, "/session", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected valid JSON error, got: %v", err)
	}
	if payload.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected error code INTERNAL_ERROR, got %q", payload.Error.Code)
	}
	if payload.Error.Message != "Internal server error" {
		t.Fatalf("expected internal error message, got %q", payload.Error.Message)
	}
}

func TestSessionHandlerAuthenticatedRequestReturnsConflict(t *testing.T) {
	authenticator, token := newClerkTestAuthenticator(t, clerkTestTokenConfig{
		authorizedParty: "http://localhost:4173",
		issuer:          "https://issuer.test",
		subject:         "user_123",
	})

	store := &fakeSessionStore{
		createFn: func(context.Context, time.Time) (session.Session, error) {
			t.Fatal("create should not be called for authenticated requests")
			return session.Session{}, nil
		},
	}
	handler := newSessionHandler(store, authenticator, time.Now)

	req := httptest.NewRequest(http.MethodPost, "/session", nil)
	req.Header.Set(authorizationHeaderName, "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected valid JSON error, got: %v", err)
	}
	if payload.Error.Code != "AUTHENTICATED_SESSION_CREATION_FORBIDDEN" {
		t.Fatalf("expected authenticated session conflict code, got %q", payload.Error.Code)
	}
}

type fakeSessionStore struct {
	createFn func(ctx context.Context, now time.Time) (session.Session, error)
}

func (s *fakeSessionStore) Create(ctx context.Context, now time.Time) (session.Session, error) {
	if s.createFn != nil {
		return s.createFn(ctx, now)
	}

	return session.Session{}, nil
}

func (s *fakeSessionStore) GetByID(context.Context, string) (session.Session, error) {
	return session.Session{}, nil
}

func (s *fakeSessionStore) Touch(context.Context, string, time.Time) error {
	return nil
}
