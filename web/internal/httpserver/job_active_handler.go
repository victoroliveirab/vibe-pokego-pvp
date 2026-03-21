package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

type activeJobStatusHandler struct {
	store upload.Store
}

func newActiveJobStatusHandler(store upload.Store) http.Handler {
	return &activeJobStatusHandler{store: store}
}

func (h *activeJobStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	record, err := h.store.GetActiveJobStatus(r.Context(), identity.OwnerKey())
	if err != nil {
		if errors.Is(err, upload.ErrJobNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{
				"job": nil,
			})
			return
		}

		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"job": map[string]any{
			"jobId":      record.ID,
			"uploadId":   record.UploadID,
			"status":     record.Status,
			"progress":   record.Progress,
			"stage":      record.Stage,
			"createdAt":  record.CreatedAt.Format(time.RFC3339Nano),
			"updatedAt":  record.UpdatedAt.Format(time.RFC3339Nano),
			"finishedAt": formatOptionalTime(record.FinishedAt),
			"error":      buildOptionalJobError(record.ErrorCode, record.ErrorMessage),
		},
	})
}
