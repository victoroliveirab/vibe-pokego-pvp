package httpserver

import (
	"net/http"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
)

type sessionHandler struct {
	store         session.Store
	authenticator *clerkAuthenticator
	now           func() time.Time
}

func newSessionHandler(store session.Store, authenticator *clerkAuthenticator, nowFn func() time.Time) http.Handler {
	return &sessionHandler{
		store:         store,
		authenticator: authenticator,
		now:           nowFn,
	}
}

func (h *sessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if hasAuthorizationHeader(r) {
		h.authenticator.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeAPIError(
				w,
				http.StatusConflict,
				"AUTHENTICATED_SESSION_CREATION_FORBIDDEN",
				"Signed-in users cannot create anonymous sessions",
				nil,
			)
		})).ServeHTTP(w, r)
		return
	}

	sess, err := h.store.Create(r.Context(), h.now())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"sessionId": sess.ID,
	})
}
