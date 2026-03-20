package httpserver

import (
	"log/slog"
	"net/http"
	"time"
)

type requestLoggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	bytes      int
}

func (w *requestLoggingResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *requestLoggingResponseWriter) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}

	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func withRequestLogging(next http.Handler) http.Handler {
	logger := slog.Default()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}

		startedAt := time.Now()
		rec := &requestLoggingResponseWriter{ResponseWriter: w}

		next.ServeHTTP(rec, r)

		statusCode := rec.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		logFn := logger.Info
		if statusCode >= http.StatusInternalServerError {
			logFn = logger.Error
		} else if statusCode >= http.StatusBadRequest {
			logFn = logger.Warn
		}

		logFn(
			"http request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status_code", statusCode,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"response_bytes", rec.bytes,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
		)
	})
}
