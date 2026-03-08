package httpserver

import (
	"errors"
	"net/http"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

type jobRetryHandler struct {
	store upload.Store
	now   func() time.Time
}

func newJobRetryHandler(store upload.Store, nowFn func() time.Time) http.Handler {
	return &jobRetryHandler{
		store: store,
		now:   nowFn,
	}
}

func (h *jobRetryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	retryJob, err := h.store.CreateRetryJob(r.Context(), r.PathValue("jobId"), sess.ID, h.now())
	if err != nil {
		if errors.Is(err, upload.ErrJobNotFound) {
			writeAPIError(w, http.StatusNotFound, "JOB_NOT_FOUND", "Job not found", nil)
			return
		}
		if errors.Is(err, upload.ErrJobRetryNotAllowed) {
			writeAPIError(w, http.StatusConflict, "JOB_RETRY_NOT_ALLOWED", "Only failed jobs can be retried", nil)
			return
		}

		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"jobId":       retryJob.ID,
		"parentJobId": retryJob.ParentJobID,
		"uploadId":    retryJob.UploadID,
		"status":      retryJob.Status,
	})
}
