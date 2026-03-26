package httpserver

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestJobRetryIntegrationCreatesQueuedChildAndPreservesParentRow(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)
	failedAt := time.Date(2026, time.March, 5, 12, 0, 0, 0, time.UTC)
	failedStage := "POSTPROCESSING"
	failedCode := "NO_APPRAISALS_FOUND"
	failedMessage := "No readable appraisals detected"
	setJobLifecycleState(
		t,
		env.dbPath,
		created.JobID,
		upload.JobStatusFailed,
		100,
		&failedStage,
		nil,
		failedAt,
		&failedAt,
		&failedCode,
		&failedMessage,
	)

	parentBefore := readIntegrationJobSnapshot(t, env.dbPath, created.JobID)
	resp := sendJobStatusRequest(t, env.client, http.MethodPost, env.server.URL+"/jobs/"+created.JobID+"/retry", sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, resp.StatusCode)
	}

	var payload retryJobCreatedResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected retry payload, got: %v", err)
	}
	if payload.JobID == "" {
		t.Fatal("expected response jobId")
	}
	if payload.JobID == created.JobID {
		t.Fatalf("expected child job id to differ from parent job id %q", created.JobID)
	}
	if payload.ParentJobID != created.JobID {
		t.Fatalf("expected parentJobId %q, got %q", created.JobID, payload.ParentJobID)
	}
	if payload.UploadID != created.UploadID {
		t.Fatalf("expected uploadId %q, got %q", created.UploadID, payload.UploadID)
	}
	if payload.Status != upload.JobStatusQueued {
		t.Fatalf("expected status %q, got %q", upload.JobStatusQueued, payload.Status)
	}

	childSnapshot := readIntegrationJobSnapshot(t, env.dbPath, payload.JobID)
	if childSnapshot.UploadID != created.UploadID {
		t.Fatalf("expected child upload_id %q, got %q", created.UploadID, childSnapshot.UploadID)
	}
	if childSnapshot.SessionID != sessionID {
		t.Fatalf("expected child session_id %q, got %q", sessionID, childSnapshot.SessionID)
	}
	if !childSnapshot.ParentJobID.Valid || childSnapshot.ParentJobID.String != created.JobID {
		t.Fatalf("expected child parent_job_id %q, got %#v", created.JobID, childSnapshot.ParentJobID)
	}
	if childSnapshot.Status != upload.JobStatusQueued {
		t.Fatalf("expected child status %q, got %q", upload.JobStatusQueued, childSnapshot.Status)
	}
	if childSnapshot.Progress != 0 {
		t.Fatalf("expected child progress 0, got %v", childSnapshot.Progress)
	}
	for _, field := range []struct {
		name  string
		value sql.NullString
	}{
		{name: "stage", value: childSnapshot.Stage},
		{name: "worker_id", value: childSnapshot.WorkerID},
		{name: "claimed_at", value: childSnapshot.ClaimedAt},
		{name: "heartbeat_at", value: childSnapshot.HeartbeatAt},
		{name: "error_code", value: childSnapshot.ErrorCode},
		{name: "error_message", value: childSnapshot.ErrorMessage},
		{name: "finished_at", value: childSnapshot.FinishedAt},
	} {
		if field.value.Valid {
			t.Fatalf("expected child %s to be NULL, got %q", field.name, field.value.String)
		}
	}
	assertRFC3339NanoTimestamp(t, childSnapshot.CreatedAtRaw, "created_at")
	assertRFC3339NanoTimestamp(t, childSnapshot.UpdatedAtRaw, "updated_at")
	if childSnapshot.CreatedAtRaw != childSnapshot.UpdatedAtRaw {
		t.Fatalf(
			"expected child created_at and updated_at to match, got created_at=%q updated_at=%q",
			childSnapshot.CreatedAtRaw,
			childSnapshot.UpdatedAtRaw,
		)
	}

	parentAfter := readIntegrationJobSnapshot(t, env.dbPath, created.JobID)
	if parentBefore != parentAfter {
		t.Fatalf("expected parent job row to remain unchanged, before=%#v after=%#v", parentBefore, parentAfter)
	}

	childStatusResp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+payload.JobID, sessionID)
	defer childStatusResp.Body.Close()
	if childStatusResp.StatusCode != http.StatusOK {
		t.Fatalf("expected child status poll to return %d, got %d", http.StatusOK, childStatusResp.StatusCode)
	}
}

func TestJobRetryIntegrationReturnsNotFoundForNonOwnedParent(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionA := createSessionViaHTTP(t, env.client, env.server.URL)
	sessionB := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionA)

	resp := sendJobStatusRequest(t, env.client, http.MethodPost, env.server.URL+"/jobs/"+created.JobID+"/retry", sessionB)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	var payload APIError
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected API error payload, got: %v", err)
	}
	if payload.Error.Code != "JOB_NOT_FOUND" {
		t.Fatalf("expected JOB_NOT_FOUND code, got %q", payload.Error.Code)
	}
}

func TestJobRetryIntegrationReturnsConflictForOwnedNonFailedParent(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)

	resp := sendJobStatusRequest(t, env.client, http.MethodPost, env.server.URL+"/jobs/"+created.JobID+"/retry", sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, resp.StatusCode)
	}

	var payload APIError
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected API error payload, got: %v", err)
	}
	if payload.Error.Code != "JOB_RETRY_NOT_ALLOWED" {
		t.Fatalf("expected JOB_RETRY_NOT_ALLOWED code, got %q", payload.Error.Code)
	}
}

func TestJobRetryIntegrationReturnsNotFoundForUnknownParent(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)

	resp := sendJobStatusRequest(t, env.client, http.MethodPost, env.server.URL+"/jobs/not-a-uuid/retry", sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	var payload APIError
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected API error payload, got: %v", err)
	}
	if payload.Error.Code != "JOB_NOT_FOUND" {
		t.Fatalf("expected JOB_NOT_FOUND code, got %q", payload.Error.Code)
	}
}

func TestJobRetryIntegrationReturnsInvalidSessionErrors(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	validSessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, validSessionID)

	testCases := []struct {
		name      string
		sessionID string
	}{
		{name: "missing header", sessionID: ""},
		{name: "malformed header", sessionID: "not-a-uuid"},
		{name: "unknown session", sessionID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := sendJobStatusRequest(t, env.client, http.MethodPost, env.server.URL+"/jobs/"+created.JobID+"/retry", tc.sessionID)
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, resp.StatusCode)
			}

			var payload APIError
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatalf("expected API error payload, got: %v", err)
			}
			if payload.Error.Code != "INVALID_SESSION" {
				t.Fatalf("expected INVALID_SESSION code, got %q", payload.Error.Code)
			}
		})
	}
}

func TestJobRetryIntegrationMethodNotAllowed(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)

	resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+created.JobID+"/retry", sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow %q, got %q", http.MethodPost, allow)
	}
}

type retryJobCreatedResponse struct {
	JobID       string `json:"jobId"`
	ParentJobID string `json:"parentJobId"`
	UploadID    string `json:"uploadId"`
	Status      string `json:"status"`
}

type integrationJobSnapshot struct {
	UploadID     string
	SessionID    string
	ParentJobID  sql.NullString
	Status       string
	Progress     float64
	Stage        sql.NullString
	WorkerID     sql.NullString
	ClaimedAt    sql.NullString
	HeartbeatAt  sql.NullString
	ErrorCode    sql.NullString
	ErrorMessage sql.NullString
	CreatedAtRaw string
	UpdatedAtRaw string
	FinishedAt   sql.NullString
}

func readIntegrationJobSnapshot(t *testing.T, dbPath string, jobID string) integrationJobSnapshot {
	t.Helper()

	db := openIntegrationDB(t, dbPath)

	const query = `
SELECT upload_id, session_id, parent_job_id, status, progress, stage, worker_id, claimed_at,
       heartbeat_at, error_code, error_message, created_at, updated_at, finished_at
FROM jobs
WHERE id = ?;`

	var snapshot integrationJobSnapshot
	if err := db.QueryRow(query, jobID).Scan(
		&snapshot.UploadID,
		&snapshot.SessionID,
		&snapshot.ParentJobID,
		&snapshot.Status,
		&snapshot.Progress,
		&snapshot.Stage,
		&snapshot.WorkerID,
		&snapshot.ClaimedAt,
		&snapshot.HeartbeatAt,
		&snapshot.ErrorCode,
		&snapshot.ErrorMessage,
		&snapshot.CreatedAtRaw,
		&snapshot.UpdatedAtRaw,
		&snapshot.FinishedAt,
	); err != nil {
		t.Fatalf("expected integration job snapshot query to succeed for %q: %v", jobID, err)
	}

	return snapshot
}
