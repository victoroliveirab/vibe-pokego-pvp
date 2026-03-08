package upload

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateMultipartFileRejectsMissingFile(t *testing.T) {
	_, err := ValidateMultipartFile(context.Background(), nil, nil, nil)
	validationErr := requireValidationError(t, err)

	if validationErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, validationErr.HTTPStatus)
	}
	if validationErr.Code != ErrorCodeMissingFile {
		t.Fatalf("expected code %q, got %q", ErrorCodeMissingFile, validationErr.Code)
	}
}

func TestValidateMultipartFileRejectsUnsupportedMediaType(t *testing.T) {
	file, header, cleanup := newMultipartFile(t, "file", "notes.txt", []byte("plain text"))
	defer cleanup()

	_, err := ValidateMultipartFile(context.Background(), file, header, nil)
	validationErr := requireValidationError(t, err)

	if validationErr.HTTPStatus != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status %d, got %d", http.StatusUnsupportedMediaType, validationErr.HTTPStatus)
	}
	if validationErr.Code != ErrorCodeUnsupportedMediaType {
		t.Fatalf("expected code %q, got %q", ErrorCodeUnsupportedMediaType, validationErr.Code)
	}
}

func TestValidateMultipartFileRejectsOversizedVideo(t *testing.T) {
	file, header, cleanup := newMultipartFile(t, "file", "clip.mp4", mp4FixtureBytes())
	defer cleanup()
	header.Size = maxVideoBytes + 1

	proberCalled := false
	prober := durationProberFunc(func(context.Context, string) (float64, error) {
		proberCalled = true
		return 10, nil
	})

	_, err := ValidateMultipartFile(context.Background(), file, header, prober)
	validationErr := requireValidationError(t, err)

	if validationErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, validationErr.HTTPStatus)
	}
	if validationErr.Code != ErrorCodeVideoTooLarge {
		t.Fatalf("expected code %q, got %q", ErrorCodeVideoTooLarge, validationErr.Code)
	}
	if proberCalled {
		t.Fatal("expected duration prober not to be called for oversized videos")
	}
}

func TestValidateMultipartFileRejectsOverDurationVideo(t *testing.T) {
	file, header, cleanup := newMultipartFile(t, "file", "clip.mp4", mp4FixtureBytes())
	defer cleanup()

	prober := durationProberFunc(func(context.Context, string) (float64, error) {
		return 90.1, nil
	})

	_, err := ValidateMultipartFile(context.Background(), file, header, prober)
	validationErr := requireValidationError(t, err)

	if validationErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, validationErr.HTTPStatus)
	}
	if validationErr.Code != ErrorCodeVideoTooLong {
		t.Fatalf("expected code %q, got %q", ErrorCodeVideoTooLong, validationErr.Code)
	}
	if validationErr.Details["maxSeconds"] != maxVideoSecs {
		t.Fatalf("expected details.maxSeconds=%d, got %#v", maxVideoSecs, validationErr.Details["maxSeconds"])
	}
}

func TestValidateMultipartFileAcceptsValidImage(t *testing.T) {
	payload := pngFixtureBytes()
	file, header, cleanup := newMultipartFile(t, "file", "image.png", payload)
	defer cleanup()

	prober := durationProberFunc(func(context.Context, string) (float64, error) {
		t.Fatal("duration prober should not be called for image uploads")
		return 0, nil
	})

	validated, err := ValidateMultipartFile(context.Background(), file, header, prober)
	if err != nil {
		t.Fatalf("expected image validation to pass, got: %v", err)
	}
	defer validated.Cleanup()

	if validated.Kind != KindImage {
		t.Fatalf("expected kind %q, got %q", KindImage, validated.Kind)
	}
	if validated.ContentType != "image/png" {
		t.Fatalf("expected content type image/png, got %q", validated.ContentType)
	}
	if validated.ByteSize != int64(len(payload)) {
		t.Fatalf("expected byte size %d, got %d", len(payload), validated.ByteSize)
	}
	if _, err := os.Stat(validated.TempFilePath); err != nil {
		t.Fatalf("expected temp file to exist, got: %v", err)
	}
}

func TestValidateMultipartFileAcceptsValidVideo(t *testing.T) {
	file, header, cleanup := newMultipartFile(t, "file", "clip.mp4", mp4FixtureBytes())
	defer cleanup()

	prober := durationProberFunc(func(_ context.Context, filePath string) (float64, error) {
		if _, err := os.Stat(filePath); err != nil {
			t.Fatalf("expected duration prober path to exist, got: %v", err)
		}
		return 30, nil
	})

	validated, err := ValidateMultipartFile(context.Background(), file, header, prober)
	if err != nil {
		t.Fatalf("expected video validation to pass, got: %v", err)
	}
	defer validated.Cleanup()

	if validated.Kind != KindVideo {
		t.Fatalf("expected kind %q, got %q", KindVideo, validated.Kind)
	}
	if !strings.HasPrefix(validated.ContentType, "video/") {
		t.Fatalf("expected video content type, got %q", validated.ContentType)
	}
}

func TestValidateMultipartFileReturnsErrorWhenDurationProbeFails(t *testing.T) {
	file, header, cleanup := newMultipartFile(t, "file", "clip.mp4", mp4FixtureBytes())
	defer cleanup()

	prober := durationProberFunc(func(context.Context, string) (float64, error) {
		return 0, errors.New("probe unavailable")
	})

	_, err := ValidateMultipartFile(context.Background(), file, header, prober)
	if err == nil {
		t.Fatal("expected error when duration probing fails")
	}
	if _, ok := AsValidationError(err); ok {
		t.Fatalf("expected probing failures to be non-validation errors, got %v", err)
	}
}

func TestLocalMediaStoragePersistsValidatedMediaUnderConfiguredDirectory(t *testing.T) {
	payload := pngFixtureBytes()
	file, header, cleanup := newMultipartFile(t, "file", "avatar.png", payload)
	defer cleanup()

	validated, err := ValidateMultipartFile(context.Background(), file, header, nil)
	if err != nil {
		t.Fatalf("expected image validation to pass, got: %v", err)
	}

	localDir := filepath.Join(t.TempDir(), "uploads")
	storage, err := NewLocalMediaStorage(localDir)
	if err != nil {
		t.Fatalf("expected local storage init to pass, got: %v", err)
	}

	stored, cleanupStored, err := storage.StoreValidated(context.Background(), validated)
	if err != nil {
		t.Fatalf("expected store validated media to pass, got: %v", err)
	}

	if !strings.HasPrefix(stored.FilePath, localDir+string(os.PathSeparator)) {
		t.Fatalf("expected stored file path under %q, got %q", localDir, stored.FilePath)
	}
	if _, err := os.Stat(stored.FilePath); err != nil {
		t.Fatalf("expected stored file to exist, got: %v", err)
	}

	storedPayload, err := os.ReadFile(stored.FilePath)
	if err != nil {
		t.Fatalf("expected reading stored file to succeed, got: %v", err)
	}
	if !bytes.Equal(storedPayload, payload) {
		t.Fatalf("stored payload mismatch")
	}

	expectedURL := "local://uploads/" + filepath.Base(stored.FilePath)
	if stored.MediaURL != expectedURL {
		t.Fatalf("expected media URL %q, got %q", expectedURL, stored.MediaURL)
	}

	if _, err := os.Stat(validated.TempFilePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected validated temp file to be removed, got: %v", err)
	}

	if err := cleanupStored(); err != nil {
		t.Fatalf("expected cleanup to succeed, got: %v", err)
	}
	if err := cleanupStored(); err != nil {
		t.Fatalf("expected cleanup to be idempotent, got: %v", err)
	}
	if _, err := os.Stat(stored.FilePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected stored file to be removed by cleanup, got: %v", err)
	}
}

func TestLocalMediaStorageCleanupSkipsPreExistingFile(t *testing.T) {
	payload := pngFixtureBytes()
	localDir := filepath.Join(t.TempDir(), "uploads")

	storage, err := NewLocalMediaStorage(localDir)
	if err != nil {
		t.Fatalf("expected local storage init to pass, got: %v", err)
	}

	firstFile, firstHeader, firstCleanup := newMultipartFile(t, "file", "duplicate.png", payload)
	defer firstCleanup()
	firstValidated, err := ValidateMultipartFile(context.Background(), firstFile, firstHeader, nil)
	if err != nil {
		t.Fatalf("expected first validation to succeed, got: %v", err)
	}
	firstStored, firstStoredCleanup, err := storage.StoreValidated(context.Background(), firstValidated)
	if err != nil {
		t.Fatalf("expected first store to succeed, got: %v", err)
	}

	secondFile, secondHeader, secondCleanup := newMultipartFile(t, "file", "duplicate.png", payload)
	defer secondCleanup()
	secondValidated, err := ValidateMultipartFile(context.Background(), secondFile, secondHeader, nil)
	if err != nil {
		t.Fatalf("expected second validation to succeed, got: %v", err)
	}
	secondStored, secondStoredCleanup, err := storage.StoreValidated(context.Background(), secondValidated)
	if err != nil {
		t.Fatalf("expected second store to succeed, got: %v", err)
	}

	if firstStored.FilePath != secondStored.FilePath {
		t.Fatalf("expected deterministic duplicate path, got %q and %q", firstStored.FilePath, secondStored.FilePath)
	}

	if err := secondStoredCleanup(); err != nil {
		t.Fatalf("expected second cleanup to succeed, got: %v", err)
	}
	if _, err := os.Stat(firstStored.FilePath); err != nil {
		t.Fatalf("expected pre-existing file to remain after second cleanup, got: %v", err)
	}

	if err := firstStoredCleanup(); err != nil {
		t.Fatalf("expected first cleanup to succeed, got: %v", err)
	}
	if _, err := os.Stat(firstStored.FilePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected file removed after first cleanup, got: %v", err)
	}
}

type durationProberFunc func(ctx context.Context, filePath string) (float64, error)

func (f durationProberFunc) DurationSeconds(ctx context.Context, filePath string) (float64, error) {
	return f(ctx, filePath)
}

func newMultipartFile(
	t *testing.T,
	fieldName string,
	fileName string,
	payload []byte,
) (multipart.File, *multipart.FileHeader, func()) {
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
	if err := req.ParseMultipartForm(int64(len(payload) + 1024)); err != nil {
		t.Fatalf("expected ParseMultipartForm to succeed, got: %v", err)
	}

	file, header, err := req.FormFile(fieldName)
	if err != nil {
		t.Fatalf("expected FormFile to succeed, got: %v", err)
	}

	cleanup := func() {
		_ = file.Close()
		if req.MultipartForm != nil {
			_ = req.MultipartForm.RemoveAll()
		}
	}

	return file, header, cleanup
}

func requireValidationError(t *testing.T, err error) *ValidationError {
	t.Helper()

	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	validationErr, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("expected validation error type, got: %v", err)
	}

	return validationErr
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
