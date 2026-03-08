package upload

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUploadThingMediaStorageStoreValidatedUsesPrepareAndUploadEndpoints(t *testing.T) {
	const apiToken = "uploadthing-token"
	fileContent := []byte("fixture-image-content")
	prepareCalled := false
	uploadCalled := false

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepareUpload":
			prepareCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("expected prepareUpload method POST, got %s", r.Method)
			}
			if got := r.Header.Get("x-uploadthing-api-key"); got != apiToken {
				t.Fatalf("expected x-uploadthing-api-key %q, got %q", apiToken, got)
			}

			rawBody, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("expected prepareUpload request body to be readable: %v", err)
			}

			var payload map[string]any
			if err := json.Unmarshal(rawBody, &payload); err != nil {
				t.Fatalf("expected prepareUpload payload to be JSON object: %v", err)
			}
			if got := payload["fileName"]; got != "avatar.png" {
				t.Fatalf("expected fileName to be avatar.png, got %#v", got)
			}
			if got := payload["fileType"]; got != "image/png" {
				t.Fatalf("expected fileType to be image/png, got %#v", got)
			}
			if got := payload["fileSize"]; got != float64(len(fileContent)) {
				t.Fatalf("expected fileSize to match content length, got %#v", got)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"url":"` + server.URL + `/upload","key":"file-key","fileUrl":"https://utfs.io/f/file-key"}`))
		case "/upload":
			uploadCalled = true
			if r.Method != http.MethodPut {
				t.Fatalf("expected upload method PUT, got %s", r.Method)
			}
			if err := r.ParseMultipartForm(4 * 1024 * 1024); err != nil {
				t.Fatalf("expected upload multipart parsing to succeed: %v", err)
			}
			uploadedFile, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("expected file field in upload payload: %v", err)
			}
			defer uploadedFile.Close()
			gotContent, err := io.ReadAll(uploadedFile)
			if err != nil {
				t.Fatalf("expected uploaded file to be readable: %v", err)
			}
			if string(gotContent) != string(fileContent) {
				t.Fatalf("expected uploaded content %q, got %q", string(fileContent), string(gotContent))
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewUploadThingHTTPClient(apiToken, server.URL+"/prepareUpload", 10*time.Second)
	if err != nil {
		t.Fatalf("expected uploadthing client to initialize: %v", err)
	}
	storage, err := NewUploadThingMediaStorage(client)
	if err != nil {
		t.Fatalf("expected uploadthing storage to initialize: %v", err)
	}

	media := writeValidatedTempMedia(t, fileContent, KindImage, "avatar.png", "image/png")
	stored, cleanup, err := storage.StoreValidated(context.Background(), media)
	if err != nil {
		t.Fatalf("expected uploadthing storage to succeed: %v", err)
	}

	if !prepareCalled {
		t.Fatal("expected prepareUpload endpoint to be called")
	}
	if !uploadCalled {
		t.Fatal("expected upload endpoint to be called")
	}
	if stored.MediaURL != "https://utfs.io/f/file-key" {
		t.Fatalf("expected uploadthing media URL, got %q", stored.MediaURL)
	}
	if cleanup == nil {
		t.Fatal("expected non-nil cleanup callback")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("expected cleanup callback to be safe no-op, got %v", err)
	}
	if _, statErr := os.Stat(media.TempFilePath); !os.IsNotExist(statErr) {
		t.Fatalf("expected temp media file to be removed, stat err: %v", statErr)
	}
}

func TestUploadThingMediaStorageStoreValidatedSupportsPutObjectUploadURLs(t *testing.T) {
	const apiToken = "uploadthing-token"
	fileContent := []byte("fixture-video-content")
	putCalled := false

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prepareUpload":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[` +
				`{"url":"` + server.URL + `/put-upload?x-id=PutObject","key":"put-key"}` +
				`]`))
		case "/put-upload":
			putCalled = true
			if r.Method != http.MethodPut {
				t.Fatalf("expected upload method PUT, got %s", r.Method)
			}
			rawBody, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("expected put-upload body to be readable: %v", err)
			}
			if string(rawBody) != string(fileContent) {
				t.Fatalf("expected PUT payload %q, got %q", string(fileContent), string(rawBody))
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewUploadThingHTTPClient(apiToken, server.URL+"/prepareUpload", 10*time.Second)
	if err != nil {
		t.Fatalf("expected uploadthing client to initialize: %v", err)
	}
	storage, err := NewUploadThingMediaStorage(client)
	if err != nil {
		t.Fatalf("expected uploadthing storage to initialize: %v", err)
	}

	media := writeValidatedTempMedia(t, fileContent, KindVideo, "clip.mp4", "video/mp4")
	stored, _, err := storage.StoreValidated(context.Background(), media)
	if err != nil {
		t.Fatalf("expected uploadthing storage to succeed: %v", err)
	}

	if !putCalled {
		t.Fatal("expected PUT upload endpoint to be called")
	}
	if stored.MediaURL != "https://utfs.io/f/put-key" {
		t.Fatalf("expected fallback media URL from key, got %q", stored.MediaURL)
	}
}

func TestUploadThingMediaStorageStoreValidatedReturnsActionableErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream unavailable"}`))
	}))
	defer server.Close()

	client, err := NewUploadThingHTTPClient("token", server.URL, 10*time.Second)
	if err != nil {
		t.Fatalf("expected uploadthing client to initialize: %v", err)
	}
	storage, err := NewUploadThingMediaStorage(client)
	if err != nil {
		t.Fatalf("expected uploadthing storage to initialize: %v", err)
	}

	media := writeValidatedTempMedia(t, []byte("image"), KindImage, "avatar.png", "image/png")
	_, _, err = storage.StoreValidated(context.Background(), media)
	if err == nil {
		t.Fatal("expected uploadthing storage to fail on non-2xx prepare response")
	}
	if !strings.Contains(err.Error(), "prepare-upload failed") {
		t.Fatalf("expected actionable prepare-upload failure, got: %v", err)
	}
}

func writeValidatedTempMedia(
	t *testing.T,
	content []byte,
	kind string,
	originalFileName string,
	contentType string,
) ValidatedMedia {
	t.Helper()

	tempDir := t.TempDir()
	tempPath := filepath.Join(tempDir, "upload.bin")
	if err := os.WriteFile(tempPath, content, 0o644); err != nil {
		t.Fatalf("expected temp media file to be written: %v", err)
	}

	return ValidatedMedia{
		Kind:             kind,
		ContentType:      contentType,
		ByteSize:         int64(len(content)),
		OriginalFilename: originalFileName,
		TempFilePath:     tempPath,
	}
}
