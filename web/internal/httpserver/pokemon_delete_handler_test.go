package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestPokemonDeleteHandlerMethodNotAllowed(t *testing.T) {
	handler := newPokemonDeleteHandler(&fakePokemonDeleteHandlerStore{}, time.Now)
	req := newPokemonDeleteHandlerRequest(http.MethodGet, "/pokemon/result-1", "result-1", "session-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodDelete {
		t.Fatalf("expected Allow %q, got %q", http.MethodDelete, allow)
	}
}

func TestPokemonDeleteHandlerReturnsInternalErrorWhenSessionMissing(t *testing.T) {
	handler := newPokemonDeleteHandler(&fakePokemonDeleteHandlerStore{
		deleteFn: func(context.Context, string, string, time.Time) error {
			t.Fatal("expected store not to be called")
			return nil
		},
	}, time.Now)
	req := httptest.NewRequest(http.MethodDelete, "/pokemon/result-1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestPokemonDeleteHandlerDeletesResult(t *testing.T) {
	now := time.Date(2026, time.March, 7, 15, 0, 0, 0, time.UTC)
	handler := newPokemonDeleteHandler(&fakePokemonDeleteHandlerStore{
		deleteFn: func(_ context.Context, resultID string, sessionID string, gotNow time.Time) error {
			if resultID != "result-1" {
				t.Fatalf("expected result id %q, got %q", "result-1", resultID)
			}
			if sessionID != "session-1" {
				t.Fatalf("expected session id %q, got %q", "session-1", sessionID)
			}
			if !gotNow.Equal(now) {
				t.Fatalf("expected now %s, got %s", now, gotNow)
			}
			return nil
		},
	}, func() time.Time { return now })
	req := newPokemonDeleteHandlerRequest(http.MethodDelete, "/pokemon/result-1", "result-1", "session-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %q", rec.Body.String())
	}
}

func TestPokemonDeleteHandlerMapsNotFound(t *testing.T) {
	handler := newPokemonDeleteHandler(&fakePokemonDeleteHandlerStore{
		deleteFn: func(context.Context, string, string, time.Time) error {
			return upload.ErrPokemonResultNotFound
		},
	}, time.Now)
	req := newPokemonDeleteHandlerRequest(http.MethodDelete, "/pokemon/result-1", "result-1", "session-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON API error payload, got %v", err)
	}
	if payload.Error.Code != "RESULT_NOT_FOUND" {
		t.Fatalf("expected RESULT_NOT_FOUND, got %q", payload.Error.Code)
	}
}

func TestPokemonDeleteHandlerReturnsInternalErrorWhenStoreFails(t *testing.T) {
	handler := newPokemonDeleteHandler(&fakePokemonDeleteHandlerStore{
		deleteFn: func(context.Context, string, string, time.Time) error {
			return errors.New("db unavailable")
		},
	}, time.Now)
	req := newPokemonDeleteHandlerRequest(http.MethodDelete, "/pokemon/result-1", "result-1", "session-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

type fakePokemonDeleteHandlerStore struct {
	deleteFn func(ctx context.Context, resultID string, sessionID string, now time.Time) error
}

func (s *fakePokemonDeleteHandlerStore) CreateUploadAndQueuedJob(context.Context, upload.CreateParams) (upload.Upload, upload.Job, error) {
	return upload.Upload{}, upload.Job{}, errors.New("not implemented")
}

func (s *fakePokemonDeleteHandlerStore) CreateRetryJob(context.Context, string, string, time.Time) (upload.RetryJob, error) {
	return upload.RetryJob{}, errors.New("not implemented")
}

func (s *fakePokemonDeleteHandlerStore) GetJobStatus(context.Context, string, string) (upload.JobStatusRecord, error) {
	return upload.JobStatusRecord{}, errors.New("not implemented")
}

func (s *fakePokemonDeleteHandlerStore) ListPokemonResultsBySession(context.Context, string) ([]upload.PokemonResultRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *fakePokemonDeleteHandlerStore) SoftDeletePokemonResult(
	ctx context.Context,
	resultID string,
	sessionID string,
	now time.Time,
) error {
	if s.deleteFn != nil {
		return s.deleteFn(ctx, resultID, sessionID, now)
	}

	return nil
}

func (s *fakePokemonDeleteHandlerStore) ListPendingReadingsBySession(context.Context, string) ([]upload.PendingSpeciesReadingRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *fakePokemonDeleteHandlerStore) ResolvePendingReading(
	context.Context,
	upload.ResolvePendingReadingParams,
) (upload.PokemonResultRecord, error) {
	return upload.PokemonResultRecord{}, errors.New("not implemented")
}

func newPokemonDeleteHandlerRequest(method string, path string, resultID string, sessionID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.SetPathValue("resultId", resultID)
	ctx := context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: sessionID})
	return req.WithContext(ctx)
}
