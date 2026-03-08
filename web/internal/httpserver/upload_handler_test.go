package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

func TestUploadHandlerMethodNotAllowed(t *testing.T) {
	handler := newUploadHandler(
		&fakeUploadStore{},
		&fakeMediaStorage{},
		durationProberFunc(func(context.Context, string) (float64, error) { return 1, nil }),
		time.Now,
	)

	req := httptest.NewRequest(http.MethodGet, "/uploads", nil)
	req = req.WithContext(context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("expected Allow %q, got %q", http.MethodPost, allow)
	}
}

func TestUploadHandlerCreatesUploadAndJobForValidImage(t *testing.T) {
	store := &fakeUploadStore{
		createFn: func(_ context.Context, params upload.CreateParams) (upload.Upload, upload.Job, error) {
			if params.Kind != upload.KindImage {
				t.Fatalf("expected kind image, got %q", params.Kind)
			}
			if params.ContentType != "image/png" {
				t.Fatalf("expected content type image/png, got %q", params.ContentType)
			}
			if params.MediaURL != "local://uploads/image-1.png" {
				t.Fatalf("expected media url local://uploads/image-1.png, got %q", params.MediaURL)
			}
			if params.SessionID != "12f9f169-d9ca-4ea3-91e0-18356a1e1477" {
				t.Fatalf("expected session id to be propagated, got %q", params.SessionID)
			}

			return upload.Upload{ID: "up-123"}, upload.Job{ID: "job-456"}, nil
		},
	}
	storage := &fakeMediaStorage{
		storeFn: func(_ context.Context, media upload.ValidatedMedia) (upload.StoredMedia, func() error, error) {
			if media.Kind != upload.KindImage {
				t.Fatalf("expected media kind image, got %q", media.Kind)
			}
			return upload.StoredMedia{
				MediaURL: "local://uploads/image-1.png",
				FilePath: "/tmp/image-1.png",
			}, func() error { return nil }, nil
		},
	}

	handler := newUploadHandler(
		store,
		storage,
		nil,
		func() time.Time { return time.Date(2026, time.March, 1, 18, 0, 0, 0, time.UTC) },
	)

	req := newMultipartUploadRequest(t, "file", "avatar.png", pngFixtureBytes())
	req = req.WithContext(context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON payload, got: %v", err)
	}
	if payload["uploadId"] != "up-123" {
		t.Fatalf("expected uploadId up-123, got %#v", payload)
	}
	if payload["jobId"] != "job-456" {
		t.Fatalf("expected jobId job-456, got %#v", payload)
	}
}

func TestUploadHandlerCreatesUploadAndJobForValidVideo(t *testing.T) {
	store := &fakeUploadStore{
		createFn: func(_ context.Context, params upload.CreateParams) (upload.Upload, upload.Job, error) {
			if params.Kind != upload.KindVideo {
				t.Fatalf("expected kind video, got %q", params.Kind)
			}
			return upload.Upload{ID: "up-video"}, upload.Job{ID: "job-video"}, nil
		},
	}
	storage := &fakeMediaStorage{
		storeFn: func(_ context.Context, media upload.ValidatedMedia) (upload.StoredMedia, func() error, error) {
			if media.Kind != upload.KindVideo {
				t.Fatalf("expected media kind video, got %q", media.Kind)
			}
			return upload.StoredMedia{
				MediaURL: "local://uploads/video-1.mp4",
				FilePath: "/tmp/video-1.mp4",
			}, func() error { return nil }, nil
		},
	}
	prober := durationProberFunc(func(context.Context, string) (float64, error) {
		return 30, nil
	})

	handler := newUploadHandler(store, storage, prober, time.Now)

	req := newMultipartUploadRequest(t, "file", "clip.mp4", mp4FixtureBytes())
	req = req.WithContext(context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var payload map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected JSON payload, got: %v", err)
	}
	if payload["uploadId"] == "" || payload["jobId"] == "" {
		t.Fatalf("expected uploadId and jobId, got %#v", payload)
	}
}

func TestUploadHandlerMissingFileReturnsContractError(t *testing.T) {
	handler := newUploadHandler(
		&fakeUploadStore{},
		&fakeMediaStorage{},
		nil,
		time.Now,
	)

	req := newMultipartUploadRequestWithoutFile(t)
	req = req.WithContext(context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertAPIErrorCode(t, rec, http.StatusBadRequest, upload.ErrorCodeMissingFile)
}

func TestUploadHandlerUnsupportedMediaReturnsContractError(t *testing.T) {
	handler := newUploadHandler(
		&fakeUploadStore{},
		&fakeMediaStorage{},
		nil,
		time.Now,
	)

	req := newMultipartUploadRequest(t, "file", "notes.txt", []byte("plain text"))
	req = req.WithContext(context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertAPIErrorCode(t, rec, http.StatusUnsupportedMediaType, upload.ErrorCodeUnsupportedMediaType)
}

func TestUploadHandlerValidationErrorsCodes(t *testing.T) {
	testCases := []struct {
		name       string
		status     int
		code       string
		details    map[string]interface{}
		assertBody func(t *testing.T, rec *httptest.ResponseRecorder)
	}{
		{
			name:    "video too large",
			status:  http.StatusBadRequest,
			code:    upload.ErrorCodeVideoTooLarge,
			details: nil,
		},
		{
			name:   "video too long",
			status: http.StatusBadRequest,
			code:   upload.ErrorCodeVideoTooLong,
			details: map[string]interface{}{
				"maxSeconds": 90,
			},
			assertBody: func(t *testing.T, rec *httptest.ResponseRecorder) {
				t.Helper()

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
				}

				var payload APIError
				if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
					t.Fatalf("expected json payload: %v", err)
				}
				if payload.Error.Code != upload.ErrorCodeVideoTooLong {
					t.Fatalf("expected error code %q, got %q", upload.ErrorCodeVideoTooLong, payload.Error.Code)
				}
				details, ok := payload.Error.Details.(map[string]interface{})
				if !ok {
					t.Fatalf("expected details map, got %#v", payload.Error.Details)
				}
				if details["maxSeconds"] != float64(90) {
					t.Fatalf("expected details.maxSeconds=90, got %#v", details["maxSeconds"])
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler, ok := newUploadHandler(
				&fakeUploadStore{},
				&fakeMediaStorage{},
				nil,
				time.Now,
			).(*uploadHandler)
			if !ok {
				t.Fatalf("expected *uploadHandler")
			}

			handler.validate = func(context.Context, multipart.File, *multipart.FileHeader) (upload.ValidatedMedia, error) {
				return upload.ValidatedMedia{}, &upload.ValidationError{
					HTTPStatus: tc.status,
					Code:       tc.code,
					Message:    "validation failed",
					Details:    tc.details,
				}
			}

			req := newMultipartUploadRequest(t, "file", "payload.bin", []byte{0x01})
			req = req.WithContext(context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if tc.assertBody != nil {
				tc.assertBody(t, rec)
				return
			}
			assertAPIErrorCode(t, rec, tc.status, tc.code)
		})
	}
}

func TestUploadHandlerInvalidSessionViaMiddleware(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "upload-handler-session.db")
	sessionStore, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("expected sqlite session store, got: %v", err)
	}

	handler := withSessionValidation(
		sessionStore,
		time.Now,
		newUploadHandler(&fakeUploadStore{}, &fakeMediaStorage{}, nil, time.Now),
	)

	req := newMultipartUploadRequest(t, "file", "avatar.png", pngFixtureBytes())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertAPIErrorCode(t, rec, http.StatusUnauthorized, "INVALID_SESSION")
}

func TestUploadHandlerCleansUpStoredFileWhenPersistenceFails(t *testing.T) {
	cleanupCalls := 0
	storage := &fakeMediaStorage{
		storeFn: func(_ context.Context, _ upload.ValidatedMedia) (upload.StoredMedia, func() error, error) {
			return upload.StoredMedia{
					MediaURL: "local://uploads/image.png",
					FilePath: "/tmp/image.png",
				}, func() error {
					cleanupCalls++
					return nil
				}, nil
		},
	}
	store := &fakeUploadStore{
		createFn: func(context.Context, upload.CreateParams) (upload.Upload, upload.Job, error) {
			return upload.Upload{}, upload.Job{}, errors.New("db unavailable")
		},
	}

	handler := newUploadHandler(store, storage, nil, time.Now)
	req := newMultipartUploadRequest(t, "file", "avatar.png", pngFixtureBytes())
	req = req.WithContext(context.WithValue(req.Context(), sessionContextKey{}, session.Session{ID: "12f9f169-d9ca-4ea3-91e0-18356a1e1477"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assertAPIErrorCode(t, rec, http.StatusInternalServerError, "INTERNAL_ERROR")
	if cleanupCalls != 1 {
		t.Fatalf("expected cleanup to be called once, got %d", cleanupCalls)
	}
}

func TestNewMediaStorageForModeUploadThingBuildsConcreteStorage(t *testing.T) {
	storage, err := newMediaStorageForMode(config.StorageConfig{
		Mode:                          config.UploadStorageModeUploadThing,
		UploadThingToken:              "uploadthing-token",
		UploadThingPrepareUploadURL:   "https://api.uploadthing.com/v7/prepareUpload",
		UploadThingRequestTimeoutSecs: 30,
	})
	if err != nil {
		t.Fatalf("expected uploadthing mode media storage to initialize, got: %v", err)
	}
	if storage == nil {
		t.Fatal("expected uploadthing media storage instance")
	}
}

func TestNewMediaStorageForModeUploadThingRejectsMissingToken(t *testing.T) {
	_, err := newMediaStorageForMode(config.StorageConfig{
		Mode:                          config.UploadStorageModeUploadThing,
		UploadThingToken:              "",
		UploadThingPrepareUploadURL:   "https://api.uploadthing.com/v7/prepareUpload",
		UploadThingRequestTimeoutSecs: 30,
	})
	if err == nil {
		t.Fatal("expected uploadthing mode initialization to fail without token")
	}
}

type fakeUploadStore struct {
	createFn func(ctx context.Context, params upload.CreateParams) (upload.Upload, upload.Job, error)
}

func (s *fakeUploadStore) CreateUploadAndQueuedJob(ctx context.Context, params upload.CreateParams) (upload.Upload, upload.Job, error) {
	if s.createFn != nil {
		return s.createFn(ctx, params)
	}

	return upload.Upload{ID: "upload-default"}, upload.Job{ID: "job-default"}, nil
}

func (s *fakeUploadStore) GetJobStatus(context.Context, string, string) (upload.JobStatusRecord, error) {
	return upload.JobStatusRecord{}, upload.ErrJobNotFound
}

func (s *fakeUploadStore) CreateRetryJob(context.Context, string, string, time.Time) (upload.RetryJob, error) {
	return upload.RetryJob{}, errors.New("not implemented")
}

func (s *fakeUploadStore) ListPokemonResultsBySession(context.Context, string) ([]upload.PokemonResultRecord, error) {
	return nil, nil
}

func (s *fakeUploadStore) ListPendingReadingsBySession(context.Context, string) ([]upload.PendingSpeciesReadingRecord, error) {
	return nil, nil
}

func (s *fakeUploadStore) ResolvePendingReading(
	context.Context,
	upload.ResolvePendingReadingParams,
) (upload.PokemonResultRecord, error) {
	return upload.PokemonResultRecord{}, errors.New("not implemented")
}

type fakeMediaStorage struct {
	storeFn func(ctx context.Context, media upload.ValidatedMedia) (upload.StoredMedia, func() error, error)
}

func (s *fakeMediaStorage) StoreValidated(ctx context.Context, media upload.ValidatedMedia) (upload.StoredMedia, func() error, error) {
	if s.storeFn != nil {
		return s.storeFn(ctx, media)
	}
	return upload.StoredMedia{
			MediaURL: "local://uploads/default.bin",
			FilePath: "/tmp/default.bin",
		}, func() error {
			return nil
		}, nil
}

type durationProberFunc func(ctx context.Context, filePath string) (float64, error)

func (f durationProberFunc) DurationSeconds(ctx context.Context, filePath string) (float64, error) {
	return f(ctx, filePath)
}

func newMultipartUploadRequest(t *testing.T, fieldName, fileName string, payload []byte) *http.Request {
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

	req := httptest.NewRequest(http.MethodPost, "/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func newMultipartUploadRequestWithoutFile(t *testing.T) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("name", "value"); err != nil {
		t.Fatalf("expected writing field to succeed, got: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("expected closing multipart writer to succeed, got: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func assertAPIErrorCode(t *testing.T, rec *httptest.ResponseRecorder, expectedStatus int, expectedCode string) {
	t.Helper()

	if rec.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d", expectedStatus, rec.Code)
	}

	var payload APIError
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("expected json error payload, got: %v", err)
	}
	if payload.Error.Code != expectedCode {
		t.Fatalf("expected error code %q, got %q", expectedCode, payload.Error.Code)
	}
}

func pngFixtureBytes() []byte {
	return []byte{
		0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n',
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00,
	}
}

func mp4FixtureBytes() []byte {
	return []byte{
		0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70,
		0x69, 0x73, 0x6f, 0x6d, 0x00, 0x00, 0x02, 0x00,
		0x69, 0x73, 0x6f, 0x6d, 0x69, 0x73, 0x6f, 0x32,
		0x00, 0x00, 0x00, 0x08, 0x6d, 0x6f, 0x6f, 0x76,
	}
}
