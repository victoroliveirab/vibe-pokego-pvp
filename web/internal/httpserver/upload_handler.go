package httpserver

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

type mediaStorage interface {
	StoreValidated(ctx context.Context, media upload.ValidatedMedia) (upload.StoredMedia, func() error, error)
}

type uploadHandler struct {
	store    upload.Store
	validate func(ctx context.Context, file multipart.File, header *multipart.FileHeader) (upload.ValidatedMedia, error)
	storage  mediaStorage
	now      func() time.Time
}

func newUploadHandler(
	store upload.Store,
	storage mediaStorage,
	prober upload.DurationProber,
	nowFn func() time.Time,
) http.Handler {
	return &uploadHandler{
		store:   store,
		storage: storage,
		now:     nowFn,
		validate: func(ctx context.Context, file multipart.File, header *multipart.FileHeader) (upload.ValidatedMedia, error) {
			return upload.ValidateMultipartFile(ctx, file, header, prober)
		},
	}
}

func (h *uploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sess, ok := SessionFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			writeAPIError(w, http.StatusBadRequest, upload.ErrorCodeMissingFile, "Missing required file field", nil)
			return
		}
		writeAPIError(w, http.StatusBadRequest, upload.ErrorCodeMissingFile, "Missing required file field", nil)
		return
	}
	defer file.Close()
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	validated, err := h.validate(r.Context(), file, header)
	if err != nil {
		if validationErr, ok := upload.AsValidationError(err); ok {
			writeAPIError(w, validationErr.HTTPStatus, validationErr.Code, validationErr.Message, validationErr.Details)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	stored, cleanupStored, err := h.storage.StoreValidated(r.Context(), validated)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	createdUpload, createdJob, err := h.store.CreateUploadAndQueuedJob(r.Context(), upload.CreateParams{
		SessionID:   sess.ID,
		Kind:        validated.Kind,
		MediaURL:    stored.MediaURL,
		ContentType: validated.ContentType,
		ByteSize:    validated.ByteSize,
		Now:         h.now(),
	})
	if err != nil {
		if cleanupStored != nil {
			_ = cleanupStored()
		}
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"uploadId": createdUpload.ID,
		"jobId":    createdJob.ID,
	})
}

func newMediaStorageForMode(storageCfg config.StorageConfig) (mediaStorage, error) {
	switch storageCfg.Mode {
	case config.UploadStorageModeLocal:
		return upload.NewLocalMediaStorage(storageCfg.LocalDir)
	case config.UploadStorageModeUploadThing:
		client, err := upload.NewUploadThingHTTPClient(
			storageCfg.UploadThingToken,
			storageCfg.UploadThingPrepareUploadURL,
			time.Duration(storageCfg.UploadThingRequestTimeoutSecs)*time.Second,
		)
		if err != nil {
			return nil, fmt.Errorf("initialize uploadthing client: %w", err)
		}
		storage, err := upload.NewUploadThingMediaStorage(client)
		if err != nil {
			return nil, fmt.Errorf("initialize uploadthing media storage: %w", err)
		}
		return storage, nil
	default:
		return nil, fmt.Errorf("unsupported storage mode %q", storageCfg.Mode)
	}
}
