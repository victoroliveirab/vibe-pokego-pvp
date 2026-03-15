package httpserver

import (
	"net/http"
	"strings"
)

const (
	corsAllowMethods = "GET, POST, PATCH, DELETE, OPTIONS"
	corsAllowHeaders = "Content-Type, X-Session-Id"
)

func withCORS(allowedOrigins []string, next http.Handler) http.Handler {
	allowAll := false
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" {
			allowAll = true
			continue
		}
		allowed[trimmed] = struct{}{}
	}

	isAllowed := func(origin string) bool {
		if allowAll {
			return true
		}
		_, ok := allowed[origin]
		return ok
	}

	setOriginHeaders := func(w http.ResponseWriter, r *http.Request, origin string) {
		if origin == "" {
			return
		}

		if !isAllowed(origin) {
			return
		}

		if allowAll {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			addVaryHeader(w, "Origin")
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))

		if r.Method == http.MethodOptions {
			if origin != "" && !isAllowed(origin) {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}

			setOriginHeaders(w, r, origin)
			w.Header().Set("Access-Control-Allow-Methods", corsAllowMethods)

			requestedHeaders := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers"))
			if requestedHeaders == "" {
				w.Header().Set("Access-Control-Allow-Headers", corsAllowHeaders)
			} else {
				w.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
				addVaryHeader(w, "Access-Control-Request-Headers")
			}
			addVaryHeader(w, "Access-Control-Request-Method")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		setOriginHeaders(w, r, origin)
		next.ServeHTTP(w, r)
	})
}

func addVaryHeader(w http.ResponseWriter, value string) {
	current := w.Header().Get("Vary")
	if current == "" {
		w.Header().Set("Vary", value)
		return
	}

	parts := strings.Split(current, ",")
	for _, part := range parts {
		if strings.EqualFold(strings.TrimSpace(part), value) {
			return
		}
	}

	w.Header().Set("Vary", current+", "+value)
}
