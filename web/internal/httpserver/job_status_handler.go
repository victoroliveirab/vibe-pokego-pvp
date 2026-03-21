package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

type jobStatusHandler struct {
	store upload.Store
}

func newJobStatusHandler(store upload.Store) http.Handler {
	return &jobStatusHandler{store: store}
}

func (h *jobStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	identity, ok := IdentityFromContext(r.Context())
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	record, err := h.store.GetJobStatus(r.Context(), r.PathValue("jobId"), identity.OwnerKey())
	if err != nil {
		if errors.Is(err, upload.ErrJobNotFound) {
			writeAPIError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}

		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jobId":      record.ID,
		"uploadId":   record.UploadID,
		"status":     record.Status,
		"progress":   record.Progress,
		"stage":      record.Stage,
		"createdAt":  record.CreatedAt.Format(time.RFC3339Nano),
		"updatedAt":  record.UpdatedAt.Format(time.RFC3339Nano),
		"finishedAt": formatOptionalTime(record.FinishedAt),
		"error":      buildOptionalJobError(record.ErrorCode, record.ErrorMessage),
	})
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}

	return value.Format(time.RFC3339Nano)
}

func buildOptionalJobError(code *string, message *string) any {
	if code == nil && message == nil {
		return nil
	}

	return map[string]string{
		"code":    derefString(code),
		"message": derefString(message),
	}
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
