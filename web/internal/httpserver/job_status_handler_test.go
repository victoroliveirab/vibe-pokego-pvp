package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestJobStatusHandlerMethodNotAllowed(t *testing.T) {
	handler := newJobStatusHandler(&fakeJobStatusHandlerStore{})
	req := httptest.NewRequest(http.MethodPost, "/jobs/job-1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow %q, got %q", http.MethodGet, allow)
	}
}

func TestJobStatusHandlerReturnsMappedPayloadForQueuedFailedAndPendingUserDedup(t *testing.T) {
	const sessionID = "12f9f169-d9ca-4ea3-91e0-18356a1e1477"
	const jobID = "job-1"
	const uploadID = "upload-1"

	createdAt := time.Date(2026, time.March, 4, 12, 0, 0, 123_000_000, time.UTC)
	updatedAt := createdAt.Add(15 * time.Second)
	finishedAt := createdAt.Add(3 * time.Minute)
	failedStage := "POSTPROCESSING"
	failedCode := "NO_APPRAISALS_FOUND"
	failedMessage := "No readable appraisals detected"

	tests := []struct {
		name             string
		record           upload.JobStatusRecord
		assertError      func(t *testing.T, got *jobStatusHandlerError)
		expectedStage    *string
		expectedFinished *string
	}{
		{
			name: "queued",
			record: upload.JobStatusRecord{
				ID:        jobID,
				UploadID:  uploadID,
				SessionID: sessionID,
				Status:    upload.JobStatusQueued,
				Progress:  0,
				Stage:     nil,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			},
			assertError:      func(t *testing.T, got *jobStatusHandlerError) { t.Helper(); assertNilErrorObject(t, got) },
			expectedStage:    nil,
			expectedFinished: nil,
		},
		{
			name: "failed",
			record: upload.JobStatusRecord{
				ID:           jobID,
				UploadID:     uploadID,
				SessionID:    sessionID,
				Status:       upload.JobStatusFailed,
				Progress:     100,
				Stage:        &failedStage,
				CreatedAt:    createdAt,
				UpdatedAt:    updatedAt,
				FinishedAt:   &finishedAt,
				ErrorCode:    &failedCode,
				ErrorMessage: &failedMessage,
			},
			assertError: func(t *testing.T, got *jobStatusHandlerError) {
				t.Helper()

				if got == nil {
					t.Fatal("expected error object")
				}
				if got.Code != failedCode {
					t.Fatalf("expected error code %q, got %q", failedCode, got.Code)
				}
				if got.Message != failedMessage {
					t.Fatalf("expected error message %q, got %q", failedMessage, got.Message)
				}
			},
			expectedStage:    &failedStage,
			expectedFinished: stringPtr(finishedAt.Format(time.RFC3339Nano)),
		},
		{
			name: "pending user dedup",
			record: upload.JobStatusRecord{
				ID:         jobID,
				UploadID:   uploadID,
				SessionID:  sessionID,
				Status:     upload.JobStatusPendingUserDedup,
				Progress:   100,
				Stage:      nil,
				CreatedAt:  createdAt,
				UpdatedAt:  updatedAt,
				FinishedAt: &finishedAt,
			},
			assertError:      func(t *testing.T, got *jobStatusHandlerError) { t.Helper(); assertNilErrorObject(t, got) },
			expectedStage:    nil,
			expectedFinished: stringPtr(finishedAt.Format(time.RFC3339Nano)),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeJobStatusHandlerStore{
				getJobStatusFn: func(_ context.Context, gotJobID, gotOwnerKey string) (upload.JobStatusRecord, error) {
					if gotJobID != jobID {
						t.Fatalf("expected job id %q, got %q", jobID, gotJobID)
					}
					if gotOwnerKey != sessionID {
						t.Fatalf("expected owner key %q, got %q", sessionID, gotOwnerKey)
					}
					return tc.record, nil
				},
			}

			handler := newJobStatusHandler(store)
			req := newJobStatusHandlerRequest(http.MethodGet, "/jobs/"+jobID, jobID, sessionID)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
			}

			var payload jobStatusHandlerResponse
			if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
				t.Fatalf("expected JSON payload, got: %v", err)
			}
			if payload.JobID != jobID {
				t.Fatalf("expected jobId %q, got %q", jobID, payload.JobID)
			}
			if payload.UploadID != uploadID {
				t.Fatalf("expected uploadId %q, got %q", uploadID, payload.UploadID)
			}
			if payload.Status != tc.record.Status {
				t.Fatalf("expected status %q, got %q", tc.record.Status, payload.Status)
			}
			if payload.Progress != tc.record.Progress {
				t.Fatalf("expected progress %d, got %d", tc.record.Progress, payload.Progress)
			}
			assertOptionalStringPtrEqual(t, tc.expectedStage, payload.Stage, "stage")
			assertOptionalStringPtrEqual(t, tc.expectedFinished, payload.FinishedAt, "finishedAt")
			if payload.CreatedAt != createdAt.Format(time.RFC3339Nano) {
				t.Fatalf("expected createdAt %q, got %q", createdAt.Format(time.RFC3339Nano), payload.CreatedAt)
			}
			if payload.UpdatedAt != updatedAt.Format(time.RFC3339Nano) {
				t.Fatalf("expected updatedAt %q, got %q", updatedAt.Format(time.RFC3339Nano), payload.UpdatedAt)
			}
			tc.assertError(t, payload.Error)
		})
	}
}

func TestJobStatusHandlerReturnsNotFoundError(t *testing.T) {
	handler := newJobStatusHandler(&fakeJobStatusHandlerStore{
		getJobStatusFn: func(context.Context, string, string) (upload.JobStatusRecord, error) {
			return upload.JobStatusRecord{}, upload.ErrJobNotFound
		},
	})

	req := newJobStatusHandlerRequest(
		http.MethodGet,
		"/jobs/job-1",
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

func TestJobStatusHandlerReturnsInternalErrorWhenSessionMissingFromContext(t *testing.T) {
	handler := newJobStatusHandler(&fakeJobStatusHandlerStore{
		getJobStatusFn: func(context.Context, string, string) (upload.JobStatusRecord, error) {
			t.Fatal("expected store not to be called when session context is missing")
			return upload.JobStatusRecord{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/jobs/job-1", nil)
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

func TestJobStatusHandlerFailedPayloadNormalizesMissingErrorField(t *testing.T) {
	const sessionID = "12f9f169-d9ca-4ea3-91e0-18356a1e1477"
	const jobID = "job-1"

	errorCode := "FAILED_WITHOUT_MESSAGE"
	stage := "POSTPROCESSING"
	finishedAt := time.Date(2026, time.March, 4, 13, 0, 0, 0, time.UTC)
	record := upload.JobStatusRecord{
		ID:         jobID,
		UploadID:   "upload-1",
		SessionID:  sessionID,
		Status:     upload.JobStatusFailed,
		Progress:   100,
		Stage:      &stage,
		CreatedAt:  finishedAt.Add(-time.Minute),
		UpdatedAt:  finishedAt,
		FinishedAt: &finishedAt,
		ErrorCode:  &errorCode,
	}

	handler := newJobStatusHandler(&fakeJobStatusHandlerStore{
		getJobStatusFn: func(context.Context, string, string) (upload.JobStatusRecord, error) {
			return record, nil
		},
	})

	req := newJobStatusHandlerRequest(http.MethodGet, "/jobs/"+jobID, jobID, sessionID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload jobStatusHandlerResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON payload, got: %v", err)
	}
	if payload.Error == nil {
		t.Fatal("expected error object")
	}
	if payload.Error.Code != errorCode {
		t.Fatalf("expected error code %q, got %q", errorCode, payload.Error.Code)
	}
	if payload.Error.Message != "" {
		t.Fatalf("expected normalized empty error message, got %q", payload.Error.Message)
	}
}

type fakeJobStatusHandlerStore struct {
	getJobStatusFn       func(ctx context.Context, jobID string, ownerKey string) (upload.JobStatusRecord, error)
	getActiveJobStatusFn func(ctx context.Context, ownerKey string) (upload.JobStatusRecord, error)
}

func (s *fakeJobStatusHandlerStore) CreateUploadAndQueuedJob(context.Context, upload.CreateParams) (upload.Upload, upload.Job, error) {
	return upload.Upload{}, upload.Job{}, errors.New("not implemented")
}

func (s *fakeJobStatusHandlerStore) CreateRetryJob(context.Context, string, string, time.Time) (upload.RetryJob, error) {
	return upload.RetryJob{}, errors.New("not implemented")
}

func (s *fakeJobStatusHandlerStore) GetJobStatus(
	ctx context.Context,
	jobID string,
	ownerKey string,
) (upload.JobStatusRecord, error) {
	if s.getJobStatusFn != nil {
		return s.getJobStatusFn(ctx, jobID, ownerKey)
	}

	return upload.JobStatusRecord{}, nil
}

func (s *fakeJobStatusHandlerStore) GetActiveJobStatus(ctx context.Context, ownerKey string) (upload.JobStatusRecord, error) {
	if s.getActiveJobStatusFn != nil {
		return s.getActiveJobStatusFn(ctx, ownerKey)
	}

	return upload.JobStatusRecord{}, upload.ErrJobNotFound
}

func (s *fakeJobStatusHandlerStore) ListPokemonResults(context.Context, string) ([]upload.PokemonResultRecord, error) {
	return nil, nil
}

func (s *fakeJobStatusHandlerStore) ListPendingReadings(
	context.Context,
	string,
) ([]upload.PendingSpeciesReadingRecord, error) {
	return nil, nil
}

func (s *fakeJobStatusHandlerStore) SoftDeletePokemonResult(context.Context, string, string, time.Time) error {
	return errors.New("not implemented")
}

func (s *fakeJobStatusHandlerStore) DismissPendingReading(context.Context, upload.DismissPendingReadingParams) error {
	return errors.New("not implemented")
}

func (s *fakeJobStatusHandlerStore) ResolvePendingReading(
	context.Context,
	upload.ResolvePendingReadingParams,
) (upload.PokemonResultRecord, error) {
	return upload.PokemonResultRecord{}, errors.New("not implemented")
}

type jobStatusHandlerResponse struct {
	JobID      string                 `json:"jobId"`
	UploadID   string                 `json:"uploadId"`
	Status     string                 `json:"status"`
	Progress   int                    `json:"progress"`
	Stage      *string                `json:"stage"`
	CreatedAt  string                 `json:"createdAt"`
	UpdatedAt  string                 `json:"updatedAt"`
	FinishedAt *string                `json:"finishedAt"`
	Error      *jobStatusHandlerError `json:"error"`
}

type jobStatusHandlerError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func newJobStatusHandlerRequest(method string, path string, jobID string, sessionID string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.SetPathValue("jobId", jobID)
	return withTestGuestIdentity(req, sessionID)
}

func assertNilErrorObject(t *testing.T, got *jobStatusHandlerError) {
	t.Helper()
	if got != nil {
		t.Fatalf("expected nil error object, got %#v", got)
	}
}

func assertOptionalStringPtrEqual(t *testing.T, expected *string, actual *string, field string) {
	t.Helper()
	switch {
	case expected == nil && actual == nil:
		return
	case expected == nil && actual != nil:
		t.Fatalf("expected %s nil, got %q", field, *actual)
	case expected != nil && actual == nil:
		t.Fatalf("expected %s %q, got nil", field, *expected)
	case *expected != *actual:
		t.Fatalf("expected %s %q, got %q", field, *expected, *actual)
	}
}

func stringPtr(value string) *string {
	return &value
}
