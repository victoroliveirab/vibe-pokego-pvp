package httpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/tursodatabase/go-libsql"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestUploadIntegrationCreatesQueuedJobFromSessionUploadFlow(t *testing.T) {
	env := newUploadIntegrationEnv(t, durationProberFunc(func(context.Context, string) (float64, error) {
		return 30, nil
	}))

	sessionID := createSessionViaHTTP(t, env.client, env.server.URL)

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

	var payload struct {
		UploadID string `json:"uploadId"`
		JobID    string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected created upload payload, got: %v", err)
	}
	if payload.UploadID == "" || payload.JobID == "" {
		t.Fatalf("expected uploadId and jobId, got %#v", payload)
	}

	db := openIntegrationDB(t, env.dbPath)

	const uploadQuery = `
SELECT session_id, kind, uploadthing_url, content_type, byte_size
FROM uploads
WHERE id = ?;`

	var storedSessionID string
	var kind string
	var mediaURL string
	var contentType string
	var byteSize int64
	if err := db.QueryRow(uploadQuery, payload.UploadID).Scan(
		&storedSessionID,
		&kind,
		&mediaURL,
		&contentType,
		&byteSize,
	); err != nil {
		t.Fatalf("expected upload row to exist, got: %v", err)
	}

	if storedSessionID != sessionID {
		t.Fatalf("expected upload session id %q, got %q", sessionID, storedSessionID)
	}
	if kind != upload.KindImage {
		t.Fatalf("expected upload kind %q, got %q", upload.KindImage, kind)
	}
	if contentType != "image/png" {
		t.Fatalf("expected upload content type image/png, got %q", contentType)
	}
	if byteSize != int64(len(pngFixtureBytes())) {
		t.Fatalf("expected upload byte size %d, got %d", len(pngFixtureBytes()), byteSize)
	}
	if mediaURL == "" {
		t.Fatal("expected persisted media URL")
	}

	const jobQuery = `
SELECT upload_id, session_id, status, progress, stage
FROM jobs
WHERE id = ?;`

	var jobUploadID string
	var jobSessionID string
	var status string
	var progress int
	var stage sql.NullString
	if err := db.QueryRow(jobQuery, payload.JobID).Scan(
		&jobUploadID,
		&jobSessionID,
		&status,
		&progress,
		&stage,
	); err != nil {
		t.Fatalf("expected job row to exist, got: %v", err)
	}

	if jobUploadID != payload.UploadID {
		t.Fatalf("expected job upload id %q, got %q", payload.UploadID, jobUploadID)
	}
	if jobSessionID != sessionID {
		t.Fatalf("expected job session id %q, got %q", sessionID, jobSessionID)
	}
	if status != upload.JobStatusQueued {
		t.Fatalf("expected job status %q, got %q", upload.JobStatusQueued, status)
	}
	if progress != 0 {
		t.Fatalf("expected queued job progress 0, got %d", progress)
	}
	if stage.Valid {
		t.Fatalf("expected queued job stage to be NULL, got %q", stage.String)
	}
}

func TestUploadIntegrationValidationFailuresReturnContractErrors(t *testing.T) {
	testCases := []struct {
		name           string
		prober         upload.DurationProber
		requestBuilder func(t *testing.T, url string) *http.Request
		status         int
		code           string
		assertDetails  func(t *testing.T, details interface{})
	}{
		{
			name: "missing file",
			prober: durationProberFunc(func(context.Context, string) (float64, error) {
				return 30, nil
			}),
			requestBuilder: func(t *testing.T, url string) *http.Request {
				return newMultipartUploadRequestWithoutFileToURL(t, url)
			},
			status: http.StatusBadRequest,
			code:   upload.ErrorCodeMissingFile,
		},
		{
			name: "unsupported media type",
			prober: durationProberFunc(func(context.Context, string) (float64, error) {
				return 30, nil
			}),
			requestBuilder: func(t *testing.T, url string) *http.Request {
				return newMultipartUploadClientRequest(t, url, "file", "notes.txt", []byte("plain text"))
			},
			status: http.StatusUnsupportedMediaType,
			code:   upload.ErrorCodeUnsupportedMediaType,
		},
		{
			name: "video too long",
			prober: durationProberFunc(func(context.Context, string) (float64, error) {
				return 90.1, nil
			}),
			requestBuilder: func(t *testing.T, url string) *http.Request {
				return newMultipartUploadClientRequest(t, url, "file", "clip.mp4", mp4FixtureBytes())
			},
			status: http.StatusBadRequest,
			code:   upload.ErrorCodeVideoTooLong,
			assertDetails: func(t *testing.T, details interface{}) {
				t.Helper()

				typedDetails, ok := details.(map[string]interface{})
				if !ok {
					t.Fatalf("expected details map, got %#v", details)
				}
				if typedDetails["maxSeconds"] != float64(90) {
					t.Fatalf("expected details.maxSeconds=90, got %#v", typedDetails["maxSeconds"])
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			env := newUploadIntegrationEnv(t, tc.prober)
			sessionID := createSessionViaHTTP(t, env.client, env.server.URL)

			req := tc.requestBuilder(t, env.server.URL+"/uploads")
			req.Header.Set(sessionHeaderName, sessionID)

			resp, err := env.client.Do(req)
			if err != nil {
				t.Fatalf("expected upload request to succeed, got: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.status {
				t.Fatalf("expected status %d, got %d", tc.status, resp.StatusCode)
			}

			var payload APIError
			if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
				t.Fatalf("expected JSON error payload, got: %v", err)
			}
			if payload.Error.Code != tc.code {
				t.Fatalf("expected error code %q, got %q", tc.code, payload.Error.Code)
			}

			if tc.assertDetails != nil {
				tc.assertDetails(t, payload.Error.Details)
			}

			assertNoSessionUploadRows(t, env.dbPath, sessionID)
		})
	}
}

type uploadIntegrationEnv struct {
	server *httptest.Server
	client *http.Client
	dbPath string
}

func newUploadIntegrationEnv(t *testing.T, prober upload.DurationProber) uploadIntegrationEnv {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "upload-integration.db")
	uploadDir := filepath.Join(t.TempDir(), "uploads")

	sessionStore, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite session store, got: %v", err)
	}

	uploadStore, err := upload.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite upload store, got: %v", err)
	}

	mediaStorage, err := upload.NewLocalMediaStorage(uploadDir)
	if err != nil {
		t.Fatalf("expected local media storage, got: %v", err)
	}

	if prober == nil {
		prober = durationProberFunc(func(context.Context, string) (float64, error) {
			return 30, nil
		})
	}

	mux := http.NewServeMux()
	mux.Handle("/session", newSessionHandler(sessionStore, time.Now))
	mux.Handle("/uploads", withSessionValidation(sessionStore, time.Now, newUploadHandler(uploadStore, mediaStorage, prober, time.Now)))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return uploadIntegrationEnv{
		server: server,
		client: server.Client(),
		dbPath: dbPath,
	}
}

func createSessionViaHTTP(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()

	resp, err := client.Post(baseURL+"/session", "application/json", nil)
	if err != nil {
		t.Fatalf("expected create session request to succeed, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, resp.StatusCode)
	}

	var payload struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("expected create session response JSON, got: %v", err)
	}
	if err := session.ValidateID(payload.SessionID); err != nil {
		t.Fatalf("expected UUIDv4 session id, got %q: %v", payload.SessionID, err)
	}

	return payload.SessionID
}

func newMultipartUploadClientRequest(
	t *testing.T,
	url string,
	fieldName string,
	fileName string,
	payload []byte,
) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("expected create form file to succeed, got: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("expected writing payload to succeed, got: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("expected closing multipart writer to succeed, got: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		t.Fatalf("expected request creation to succeed, got: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func newMultipartUploadRequestWithoutFileToURL(t *testing.T, url string) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("name", "value"); err != nil {
		t.Fatalf("expected writing field to succeed, got: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("expected closing multipart writer to succeed, got: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, &body)
	if err != nil {
		t.Fatalf("expected request creation to succeed, got: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func assertNoSessionUploadRows(t *testing.T, dbPath string, sessionID string) {
	t.Helper()

	db := openIntegrationDB(t, dbPath)

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM uploads WHERE session_id = ?;", sessionID).Scan(&count); err != nil {
		t.Fatalf("expected upload row count query to succeed, got: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no uploads for session %q, got %d", sessionID, count)
	}
}

func openIntegrationDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()

	db, err := sql.Open("libsql", "file:"+dbPath)
	if err != nil {
		t.Fatalf("expected sqlite db open to succeed, got: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}
