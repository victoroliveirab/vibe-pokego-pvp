package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestJobStatusIntegrationReturnsQueuedPayload(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)

	created := createUploadAndJobViaHTTP(t, env, sessionID)
	resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+created.JobID, sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var payload jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected job status payload, got: %v", err)
	}

	if payload.JobID != created.JobID {
		t.Fatalf("expected jobId %q, got %q", created.JobID, payload.JobID)
	}
	if payload.UploadID != created.UploadID {
		t.Fatalf("expected uploadId %q, got %q", created.UploadID, payload.UploadID)
	}
	if payload.Status != upload.JobStatusQueued {
		t.Fatalf("expected status %q, got %q", upload.JobStatusQueued, payload.Status)
	}
	if payload.Progress != 0 {
		t.Fatalf("expected progress 0, got %d", payload.Progress)
	}
	if payload.Stage != nil {
		t.Fatalf("expected stage to be null, got %q", *payload.Stage)
	}
	if payload.FinishedAt != nil {
		t.Fatalf("expected finishedAt to be null, got %q", *payload.FinishedAt)
	}
	if payload.Error != nil {
		t.Fatalf("expected error to be null, got %#v", payload.Error)
	}
	assertRFC3339NanoTimestamp(t, payload.CreatedAt, "createdAt")
	assertRFC3339NanoTimestamp(t, payload.UpdatedAt, "updatedAt")
}

func TestJobStatusIntegrationReturnsProcessingPayload(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)

	stage := "SAMPLING_FRAMES"
	updatedAt := time.Date(2026, time.March, 4, 17, 20, 0, 0, time.UTC)
	setJobLifecycleState(t, env.dbPath, created.JobID, upload.JobStatusProcessing, 42, &stage, updatedAt, nil, nil, nil)

	resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+created.JobID, sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var payload jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected job status payload, got: %v", err)
	}

	if payload.Status != upload.JobStatusProcessing {
		t.Fatalf("expected status %q, got %q", upload.JobStatusProcessing, payload.Status)
	}
	if payload.Progress != 42 {
		t.Fatalf("expected progress 42, got %d", payload.Progress)
	}
	if payload.Stage == nil || *payload.Stage != stage {
		t.Fatalf("expected stage %q, got %#v", stage, payload.Stage)
	}
	if payload.FinishedAt != nil {
		t.Fatalf("expected finishedAt to be null, got %q", *payload.FinishedAt)
	}
	if payload.Error != nil {
		t.Fatalf("expected error to be null, got %#v", payload.Error)
	}
}

func TestJobStatusIntegrationReturnsVideoDecodingProcessingPayload(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)

	stage := "DECODING_VIDEO"
	updatedAt := time.Date(2026, time.March, 4, 17, 25, 0, 0, time.UTC)
	setJobLifecycleState(t, env.dbPath, created.JobID, upload.JobStatusProcessing, 25, &stage, updatedAt, nil, nil, nil)

	resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+created.JobID, sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var payload jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected job status payload, got: %v", err)
	}

	if payload.Status != upload.JobStatusProcessing {
		t.Fatalf("expected status %q, got %q", upload.JobStatusProcessing, payload.Status)
	}
	if payload.Progress != 25 {
		t.Fatalf("expected progress 25, got %d", payload.Progress)
	}
	if payload.Stage == nil || *payload.Stage != stage {
		t.Fatalf("expected stage %q, got %#v", stage, payload.Stage)
	}
	if payload.FinishedAt != nil {
		t.Fatalf("expected finishedAt to be null, got %q", *payload.FinishedAt)
	}
	if payload.Error != nil {
		t.Fatalf("expected error to be null, got %#v", payload.Error)
	}
}

func TestJobStatusIntegrationReturnsSucceededPayload(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)

	updatedAt := time.Date(2026, time.March, 4, 17, 30, 0, 0, time.UTC)
	finishedAt := updatedAt
	setJobLifecycleState(t, env.dbPath, created.JobID, upload.JobStatusSucceeded, 100, nil, updatedAt, &finishedAt, nil, nil)

	resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+created.JobID, sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var payload jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected job status payload, got: %v", err)
	}

	if payload.Status != upload.JobStatusSucceeded {
		t.Fatalf("expected status %q, got %q", upload.JobStatusSucceeded, payload.Status)
	}
	if payload.Progress != 100 {
		t.Fatalf("expected progress 100, got %d", payload.Progress)
	}
	if payload.Stage != nil {
		t.Fatalf("expected stage to be null, got %q", *payload.Stage)
	}
	if payload.FinishedAt == nil {
		t.Fatal("expected finishedAt to be non-null")
	}
	if payload.Error != nil {
		t.Fatalf("expected error to be null, got %#v", payload.Error)
	}
}

func TestJobStatusIntegrationReturnsFailedPayload(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)

	stage := "POSTPROCESSING"
	errorCode := "NO_APPRAISALS_FOUND"
	errorMessage := "No readable appraisals detected"
	updatedAt := time.Date(2026, time.March, 4, 17, 40, 0, 0, time.UTC)
	finishedAt := updatedAt
	setJobLifecycleState(
		t,
		env.dbPath,
		created.JobID,
		upload.JobStatusFailed,
		100,
		&stage,
		updatedAt,
		&finishedAt,
		&errorCode,
		&errorMessage,
	)

	resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+created.JobID, sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var payload jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected job status payload, got: %v", err)
	}

	if payload.Status != upload.JobStatusFailed {
		t.Fatalf("expected status %q, got %q", upload.JobStatusFailed, payload.Status)
	}
	if payload.Progress != 100 {
		t.Fatalf("expected progress 100, got %d", payload.Progress)
	}
	if payload.Stage == nil || *payload.Stage != stage {
		t.Fatalf("expected stage %q, got %#v", stage, payload.Stage)
	}
	if payload.FinishedAt == nil {
		t.Fatal("expected finishedAt to be non-null")
	}
	if payload.Error == nil {
		t.Fatal("expected error object to be non-null")
	}
	if payload.Error.Code != errorCode {
		t.Fatalf("expected error code %q, got %q", errorCode, payload.Error.Code)
	}
	if payload.Error.Message != errorMessage {
		t.Fatalf("expected error message %q, got %q", errorMessage, payload.Error.Message)
	}
}

func TestJobStatusIntegrationReturnsPendingUserDedupStatusUnchanged(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)

	updatedAt := time.Date(2026, time.March, 4, 18, 0, 0, 0, time.UTC)
	finishedAt := updatedAt
	setJobLifecycleState(t, env.dbPath, created.JobID, upload.JobStatusPendingUserDedup, 100, nil, updatedAt, &finishedAt, nil, nil)

	resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+created.JobID, sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var payload jobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected job status payload, got: %v", err)
	}

	if payload.Status != upload.JobStatusPendingUserDedup {
		t.Fatalf("expected status %q, got %q", upload.JobStatusPendingUserDedup, payload.Status)
	}
	if payload.Progress != 100 {
		t.Fatalf("expected progress 100, got %d", payload.Progress)
	}
	if payload.Stage != nil {
		t.Fatalf("expected stage to be null, got %q", *payload.Stage)
	}
	if payload.FinishedAt == nil {
		t.Fatal("expected finishedAt to be non-null")
	}
	if payload.Error != nil {
		t.Fatalf("expected error to be null, got %#v", payload.Error)
	}
}

func TestJobStatusIntegrationInvalidJobIDReturnsNotFound(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)

	resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/not-a-uuid", sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	var payload APIError
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected API error payload, got: %v", err)
	}
	if payload.Error.Code != "JOB_NOT_FOUND" {
		t.Fatalf("expected error code JOB_NOT_FOUND, got %q", payload.Error.Code)
	}
}

func TestJobStatusIntegrationReturnsNotFoundForNonOwnedJob(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionA := createSessionViaHTTP(t, env.client, env.server.URL)
	sessionB := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionA)

	resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+created.JobID, sessionB)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	var payload APIError
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected API error payload, got: %v", err)
	}
	if payload.Error.Code != "JOB_NOT_FOUND" {
		t.Fatalf("expected error code JOB_NOT_FOUND, got %q", payload.Error.Code)
	}
	if payload.Error.Message != "Job not found" {
		t.Fatalf("expected error message %q, got %q", "Job not found", payload.Error.Message)
	}
}

func TestJobStatusIntegrationReturnsInvalidSessionErrors(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)

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
			resp := sendJobStatusRequest(t, env.client, http.MethodGet, env.server.URL+"/jobs/"+created.JobID, tc.sessionID)
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

func TestJobStatusIntegrationMethodNotAllowed(t *testing.T) {
	env := newJobStatusIntegrationEnv(t)
	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)
	created := createUploadAndJobViaHTTP(t, env, sessionID)

	resp := sendJobStatusRequest(t, env.client, http.MethodPost, env.server.URL+"/jobs/"+created.JobID, sessionID)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow %q, got %q", http.MethodGet, allow)
	}
}

type jobStatusIntegrationEnv struct {
	server *httptest.Server
	client *http.Client
	dbPath string
}

func newJobStatusIntegrationEnv(t *testing.T) jobStatusIntegrationEnv {
	return newJobStatusIntegrationEnvWithAuthenticator(t, nil)
}

func newJobStatusIntegrationEnvWithAuthenticator(t *testing.T, authenticator *clerkAuthenticator) jobStatusIntegrationEnv {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "job-status-integration.db")
	storageDir := filepath.Join(t.TempDir(), "uploads")
	storageCfg := config.StorageConfig{
		Mode:     config.UploadStorageModeLocal,
		LocalDir: storageDir,
	}

	sessionStore, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite session store, got: %v", err)
	}

	uploadStore, err := upload.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite upload store, got: %v", err)
	}

	mediaStorage, err := newMediaStorageForMode(storageCfg)
	if err != nil {
		t.Fatalf("expected media storage to initialize, got: %v", err)
	}

	durationProber := durationProberFunc(func(context.Context, string) (float64, error) {
		return 30, nil
	})
	uploadsHandler := newUploadHandler(uploadStore, mediaStorage, durationProber, time.Now)
	jobsHandler := newJobStatusHandler(uploadStore)
	activeJobHandler := newActiveJobStatusHandler(uploadStore)
	retryHandler := newJobRetryHandler(uploadStore, time.Now)
	pokemonHandler := newPokemonResultsHandler(uploadStore)
	deletePokemonHandler := newPokemonDeleteHandler(uploadStore, time.Now)
	pendingSpeciesHandler := newPokemonPendingSpeciesHandler(uploadStore)
	pendingSpeciesResolveHandler := newPokemonPendingSpeciesResolveHandler(uploadStore, time.Now)

	mux := http.NewServeMux()
	mux.Handle("/session", newSessionHandler(sessionStore, authenticator, time.Now))
	mux.Handle("/uploads", withSessionValidation(sessionStore, authenticator, time.Now, uploadsHandler))
	mux.Handle("/jobs/active", withSessionValidation(sessionStore, authenticator, time.Now, activeJobHandler))
	mux.Handle("/jobs/{jobId}", withSessionValidation(sessionStore, authenticator, time.Now, jobsHandler))
	mux.Handle("/jobs/{jobId}/retry", withSessionValidation(sessionStore, authenticator, time.Now, retryHandler))
	mux.Handle("/pokemon", withSessionValidation(sessionStore, authenticator, time.Now, pokemonHandler))
	mux.Handle("/pokemon/{resultId}", withSessionValidation(sessionStore, authenticator, time.Now, deletePokemonHandler))
	mux.Handle("/pokemon/pending-species", withSessionValidation(sessionStore, authenticator, time.Now, pendingSpeciesHandler))
	mux.Handle(
		"/pokemon/pending-species/{readingId}",
		withSessionValidation(sessionStore, authenticator, time.Now, pendingSpeciesResolveHandler),
	)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return jobStatusIntegrationEnv{
		server: server,
		client: server.Client(),
		dbPath: dbPath,
	}
}

type uploadAndJobCreated struct {
	UploadID string `json:"uploadId"`
	JobID    string `json:"jobId"`
}

func createUploadAndJobViaHTTP(t *testing.T, env jobStatusIntegrationEnv, sessionID string) uploadAndJobCreated {
	t.Helper()

	req := newMultipartUploadClientRequest(t, env.server.URL+"/uploads", "file", "avatar.png", pngFixtureBytes())
	req.Header.Set(sessionHeaderName, sessionID)

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("expected upload request to succeed, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, resp.StatusCode)
	}

	var created uploadAndJobCreated
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("expected upload response payload, got: %v", err)
	}
	if created.UploadID == "" || created.JobID == "" {
		t.Fatalf("expected uploadId/jobId in response, got %#v", created)
	}

	return created
}

func createUploadAndJobViaAuthorization(t *testing.T, env jobStatusIntegrationEnv, token string) uploadAndJobCreated {
	t.Helper()

	req := newMultipartUploadClientRequest(t, env.server.URL+"/uploads", "file", "avatar.png", pngFixtureBytes())
	req.Header.Set(authorizationHeaderName, "Bearer "+token)

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("expected authorized upload request to succeed, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, resp.StatusCode)
	}

	var created uploadAndJobCreated
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("expected upload response payload, got: %v", err)
	}

	return created
}

func sendJobStatusRequest(t *testing.T, client *http.Client, method string, url string, sessionID string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatalf("expected request creation to succeed, got: %v", err)
	}
	if sessionID != "" {
		req.Header.Set(sessionHeaderName, sessionID)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("expected request to succeed, got: %v", err)
	}

	return resp
}

type jobStatusResponse struct {
	JobID      string                `json:"jobId"`
	UploadID   string                `json:"uploadId"`
	Status     string                `json:"status"`
	Progress   int                   `json:"progress"`
	Stage      *string               `json:"stage"`
	CreatedAt  string                `json:"createdAt"`
	UpdatedAt  string                `json:"updatedAt"`
	FinishedAt *string               `json:"finishedAt"`
	Error      *jobStatusErrorObject `json:"error"`
}

type jobStatusErrorObject struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func assertRFC3339NanoTimestamp(t *testing.T, raw string, field string) {
	t.Helper()

	if raw == "" {
		t.Fatalf("expected %s to be non-empty", field)
	}
	if _, err := time.Parse(time.RFC3339Nano, raw); err != nil {
		t.Fatalf("expected %s to be RFC3339Nano timestamp, got %q: %v", field, raw, err)
	}
}

func setJobLifecycleState(
	t *testing.T,
	dbPath string,
	jobID string,
	status string,
	progress int,
	stage *string,
	updatedAt time.Time,
	finishedAt *time.Time,
	errorCode *string,
	errorMessage *string,
) {
	t.Helper()

	db := openIntegrationDB(t, dbPath)

	var stageValue interface{}
	if stage != nil {
		stageValue = *stage
	}

	var finishedAtValue interface{}
	if finishedAt != nil {
		finishedAtValue = finishedAt.UTC().Format(time.RFC3339Nano)
	}

	var errorCodeValue interface{}
	if errorCode != nil {
		errorCodeValue = *errorCode
	}

	var errorMessageValue interface{}
	if errorMessage != nil {
		errorMessageValue = *errorMessage
	}

	const query = `
UPDATE jobs
SET status = ?, progress = ?, stage = ?, updated_at = ?, finished_at = ?, error_code = ?, error_message = ?
WHERE id = ?;`

	result, err := db.Exec(
		query,
		status,
		progress,
		stageValue,
		updatedAt.UTC().Format(time.RFC3339Nano),
		finishedAtValue,
		errorCodeValue,
		errorMessageValue,
		jobID,
	)
	if err != nil {
		t.Fatalf("expected job lifecycle update to succeed, got: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("expected rows affected query to succeed, got: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("expected one row updated for job %q, got %d", jobID, rowsAffected)
	}
}
