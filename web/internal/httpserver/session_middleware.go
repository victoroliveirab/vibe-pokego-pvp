package httpserver

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
)

const sessionHeaderName = "X-Session-Id"

type sessionContextKey struct{}

func withSessionValidation(store session.Store, nowFn func() time.Time, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Header.Get(sessionHeaderName)
		if err := session.ValidateID(sessionID); err != nil {
			writeAPIError(w, http.StatusUnauthorized, "INVALID_SESSION", "Missing or invalid X-Session-Id", nil)
			return
		}

		sess, err := store.GetByID(r.Context(), sessionID)
		if err != nil {
			if errors.Is(err, session.ErrNotFound) {
				writeAPIError(w, http.StatusUnauthorized, "INVALID_SESSION", "Missing or invalid X-Session-Id", nil)
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
			return
		}

		if err := store.Touch(r.Context(), sessionID, nowFn()); err != nil {
			if errors.Is(err, session.ErrNotFound) {
				writeAPIError(w, http.StatusUnauthorized, "INVALID_SESSION", "Missing or invalid X-Session-Id", nil)
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
			return
		}

		ctx := context.WithValue(r.Context(), sessionContextKey{}, sess)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SessionFromContext returns the validated session injected by middleware.
func SessionFromContext(ctx context.Context) (session.Session, bool) {
	sess, ok := ctx.Value(sessionContextKey{}).(session.Session)
	return sess, ok
}
