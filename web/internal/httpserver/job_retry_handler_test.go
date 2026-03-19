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

func TestJobRetryHandlerMethodNotAllowed(t *testing.T) {
	handler := newJobRetryHandler(&fakeJobRetryHandlerStore{}, time.Now)
	req := newJobRetryHandlerRequest(http.MethodGet, "/jobs/job-1/retry", "job-1", "12f9f169-d9ca-4ea3-91e0-18356a1e1477")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow %q, got %q", http.MethodPost, allow)
	}
}

func TestJobRetryHandlerReturnsCreatedPayload(t *testing.T) {
	const sessionID = "12f9f169-d9ca-4ea3-91e0-18356a1e1477"
	const parentJobID = "job-parent"

	now := time.Date(2026, time.March, 5, 11, 20, 0, 0, time.UTC)
	store := &fakeJobRetryHandlerStore{
		createRetryFn: func(_ context.Context, gotParentJobID, gotSessionID string, gotNow time.Time) (upload.RetryJob, error) {
			if gotParentJobID != parentJobID {
				t.Fatalf("expected parent job id %q, got %q", parentJobID, gotParentJobID)
			}
			if gotSessionID != sessionID {
				t.Fatalf("expected session id %q, got %q", sessionID, gotSessionID)
			}
			if !gotNow.Equal(now) {
				t.Fatalf("expected now %s, got %s", now, gotNow)
			}

			return upload.RetryJob{
				ID:          "job-child",
				ParentJobID: parentJobID,
				UploadID:    "upload-1",
				SessionID:   sessionID,
				Status:      upload.JobStatusQueued,
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
	}

	handler := newJobRetryHandler(store, func() time.Time { return now })
	req := newJobRetryHandlerRequest(http.MethodPost, "/jobs/"+parentJobID+"/retry", parentJobID, sessionID)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var payload struct {
		JobID       string `json:"jobId"`
		ParentJobID string `json:"parentJobId"`
		UploadID    string `json:"uploadId"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON payload, got: %v", err)
	}

	if payload.JobID != "job-child" {
		t.Fatalf("expected jobId %q, got %q", "job-child", payload.JobID)
	}
	if payload.ParentJobID != parentJobID {
		t.Fatalf("expected parentJobId %q, got %q", parentJobID, payload.ParentJobID)
	}
	if payload.UploadID != "upload-1" {
		t.Fatalf("expected uploadId %q, got %q", "upload-1", payload.UploadID)
	}
	if payload.Status != upload.JobStatusQueued {
		t.Fatalf("expected status %q, got %q", upload.JobStatusQueued, payload.Status)
	}
}

func TestJobRetryHandlerReturnsNotFoundError(t *testing.T) {
	handler := newJobRetryHandler(&fakeJobRetryHandlerStore{
		createRetryFn: func(context.Context, string, string, time.Time) (upload.RetryJob, error) {
			return upload.RetryJob{}, upload.ErrJobNotFound
		},
	}, time.Now)

	req := newJobRetryHandlerRequest(
		http.MethodPost,
		"/jobs/job-1/retry",
		"job-1",
		"12f9f169-d9ca-4ea3-91e0-18356a1e1477",
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON API error payload, got: %v", err)
	}
	if payload.Error.Code != "JOB_NOT_FOUND" {
		t.Fatalf("expected JOB_NOT_FOUND code, got %q", payload.Error.Code)
	}
	if payload.Error.Message != "Job not found" {
		t.Fatalf("expected job not found message, got %q", payload.Error.Message)
	}
}

func TestJobRetryHandlerReturnsRetryNotAllowedError(t *testing.T) {
	handler := newJobRetryHandler(&fakeJobRetryHandlerStore{
		createRetryFn: func(context.Context, string, string, time.Time) (upload.RetryJob, error) {
			return upload.RetryJob{}, upload.ErrJobRetryNotAllowed
		},
	}, time.Now)

	req := newJobRetryHandlerRequest(
		http.MethodPost,
		"/jobs/job-1/retry",
		"job-1",
		"12f9f169-d9ca-4ea3-91e0-18356a1e1477",
	)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON API error payload, got: %v", err)
	}
	if payload.Error.Code != "JOB_RETRY_NOT_ALLOWED" {
		t.Fatalf("expected JOB_RETRY_NOT_ALLOWED code, got %q", payload.Error.Code)
	}
	if payload.Error.Message != "Only failed jobs can be retried" {
		t.Fatalf("expected retry-not-allowed message, got %q", payload.Error.Message)
	}
}

func TestJobRetryHandlerReturnsInternalErrorWhenSessionMissingFromContext(t *testing.T) {
	handler := newJobRetryHandler(&fakeJobRetryHandlerStore{
		createRetryFn: func(context.Context, string, string, time.Time) (upload.RetryJob, error) {
			t.Fatal("expected store not to be called when session context is missing")
			return upload.RetryJob{}, nil
		},
	}, time.Now)

	req := httptest.NewRequest(http.MethodPost, "/jobs/job-1/retry", nil)
	req.SetPathValue("jobId", "job-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON API error payload, got: %v", err)
	}
	if payload.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected INTERNAL_ERROR code, got %q", payload.Error.Code)
	}
}

func TestJobRetryHandlerReturnsInternalErrorWhenStoreFails(t *testing.T) {
	handler := newJobRetryHandler(&fakeJobRetryHandlerStore{
		createRetryFn: func(context.Context, string, string, time.Time) (upload.RetryJob, error) {
			return upload.RetryJob{}, errors.New("db unavailable")
		},
	}, time.Now)

	req := newJobRetryHandlerRequest(
		http.MethodPost,
		"/jobs/job-1/retry",
		"job-1",
		"12f9f169-d9ca-4ea3-91e0-18356a1e1477",
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON API error payload, got: %v", err)
	}
	if payload.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("expected INTERNAL_ERROR code, got %q", payload.Error.Code)
	}
}

type fakeJobRetryHandlerStore struct {
	createRetryFn func(ctx context.Context, parentJobID string, sessionID string, now time.Time) (upload.RetryJob, error)
}

func (s *fakeJobRetryHandlerStore) CreateUploadAndQueuedJob(context.Context, upload.CreateParams) (upload.Upload, upload.Job, error) {
	return upload.Upload{}, upload.Job{}, errors.New("not implemented")
}

func (s *fakeJobRetryHandlerStore) CreateRetryJob(
	ctx context.Context,
	parentJobID string,
	sessionID string,
	now time.Time,
) (upload.RetryJob, error) {
	if s.createRetryFn != nil {
		return s.createRetryFn(ctx, parentJobID, sessionID, now)
	}

	return upload.RetryJob{}, nil
}

func (s *fakeJobRetryHandlerStore) GetJobStatus(context.Context, string, string) (upload.JobStatusRecord, error) {
	return upload.JobStatusRecord{}, errors.New("not implemented")
}

func (s *fakeJobRetryHandlerStore) GetActiveJobStatus(context.Context, string) (upload.JobStatusRecord, error) {
	return upload.JobStatusRecord{}, upload.ErrJobNotFound
}

func (s *fakeJobRetryHandlerStore) ListPokemonResultsBySession(context.Context, string) ([]upload.PokemonResultRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeJobRetryHandlerStore) ListPendingReadingsBySession(
	context.Context,
	string,
) ([]upload.PendingSpeciesReadingRecord, error) {
	return nil, errors.New("not implemented")
}

func (s *fakeJobRetryHandlerStore) SoftDeletePokemonResult(context.Context, string, string, time.Time) error {
	return errors.New("not implemented")
}

func (s *fakeJobRetryHandlerStore) ResolvePendingReading(
	context.Context,
	upload.ResolvePendingReadingParams,
) (upload.PokemonResultRecord, error) {
	return upload.PokemonResultRecord{}, errors.New("not implemented")
}

func newJobRetryHandlerRequest(method string, path string, jobID string, sessionID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.SetPathValue("jobId", jobID)
	ctx := context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: sessionID})
	return req.WithContext(ctx)
}
