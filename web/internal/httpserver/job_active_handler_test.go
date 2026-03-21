package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestActiveJobStatusHandlerReturnsActiveJobPayload(t *testing.T) {
	const sessionID = "12f9f169-d9ca-4ea3-91e0-18356a1e1477"
	const jobID = "job-1"
	const uploadID = "upload-1"

	createdAt := time.Date(2026, time.March, 4, 12, 0, 0, 123_000_000, time.UTC)
	updatedAt := createdAt.Add(15 * time.Second)
	stage := "SAMPLING_FRAMES"

	handler := newActiveJobStatusHandler(&fakeJobStatusHandlerStore{
		getActiveJobStatusFn: func(_ context.Context, gotOwnerKey string) (upload.JobStatusRecord, error) {
			if gotOwnerKey != sessionID {
				t.Fatalf("expected owner key %q, got %q", sessionID, gotOwnerKey)
			}

			return upload.JobStatusRecord{
				ID:        jobID,
				UploadID:  uploadID,
				SessionID: sessionID,
				Status:    upload.JobStatusProcessing,
				Progress:  35,
				Stage:     &stage,
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/jobs/active", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, withTestGuestIdentity(req, sessionID))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload struct {
		Job *jobStatusHandlerResponse `json:"job"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON payload, got: %v", err)
	}
	if payload.Job == nil {
		t.Fatal("expected active job payload")
	}
	if payload.Job.JobID != jobID {
		t.Fatalf("expected jobId %q, got %q", jobID, payload.Job.JobID)
	}
	if payload.Job.UploadID != uploadID {
		t.Fatalf("expected uploadId %q, got %q", uploadID, payload.Job.UploadID)
	}
	if payload.Job.Status != upload.JobStatusProcessing {
		t.Fatalf("expected status %q, got %q", upload.JobStatusProcessing, payload.Job.Status)
	}
	if payload.Job.Progress != 35 {
		t.Fatalf("expected progress %d, got %d", 35, payload.Job.Progress)
	}
	assertOptionalStringPtrEqual(t, &stage, payload.Job.Stage, "stage")
}

func TestActiveJobStatusHandlerReturnsNilJobWhenSessionHasNoActiveWork(t *testing.T) {
	const sessionID = "12f9f169-d9ca-4ea3-91e0-18356a1e1477"

	handler := newActiveJobStatusHandler(&fakeJobStatusHandlerStore{
		getActiveJobStatusFn: func(context.Context, string) (upload.JobStatusRecord, error) {
			return upload.JobStatusRecord{}, upload.ErrJobNotFound
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/jobs/active", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, withTestGuestIdentity(req, sessionID))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var payload struct {
		Job *jobStatusHandlerResponse `json:"job"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON payload, got: %v", err)
	}
	if payload.Job != nil {
		t.Fatalf("expected nil job payload, got %#v", payload.Job)
	}
}
