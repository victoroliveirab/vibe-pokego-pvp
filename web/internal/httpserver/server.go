package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/session"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/upload"
)

// New builds the baseline HTTP server for the API process.
func New(cfg config.Config, storage config.StorageConfig) (*http.Server, error) {
	databaseDSN := cfg.DatabaseDSN()

	if err := upload.EnsureSQLiteSchema(databaseDSN); err != nil {
		return nil, fmt.Errorf("initialize upload/job schema: %w", err)
	}

	uploadStore, err := upload.NewSQLiteStore(databaseDSN)
	if err != nil {
		return nil, fmt.Errorf("initialize upload store: %w", err)
	}

	sessionStore, err := session.NewSQLiteStore(databaseDSN)
	if err != nil {
		return nil, fmt.Errorf("initialize session store: %w", err)
	}

	mediaStorage, err := newMediaStorageForMode(storage)
	if err != nil {
		return nil, fmt.Errorf("initialize upload media storage: %w", err)
	}

	durationProber := upload.NewFFprobeDurationProber()
	uploadsHandler := newUploadHandler(uploadStore, mediaStorage, durationProber, time.Now)
	jobsHandler := newJobStatusHandler(uploadStore)
	retryHandler := newJobRetryHandler(uploadStore, time.Now)
	pokemonHandler := newPokemonResultsHandler(uploadStore)
	deletePokemonHandler := newPokemonDeleteHandler(uploadStore, time.Now)
	pendingSpeciesHandler := newPokemonPendingSpeciesHandler(uploadStore)
	pendingSpeciesResolveHandler := newPokemonPendingSpeciesResolveHandler(uploadStore, time.Now)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":       "ok",
			"service":      "web",
			"env":          cfg.AppEnv,
			"storage_mode": storage.Mode,
		})
	})
	mux.Handle("/session", newSessionHandler(sessionStore, time.Now))
	mux.Handle("/uploads", withSessionValidation(sessionStore, time.Now, uploadsHandler))
	mux.Handle("/jobs/{jobId}", withSessionValidation(sessionStore, time.Now, jobsHandler))
	mux.Handle("/jobs/{jobId}/retry", withSessionValidation(sessionStore, time.Now, retryHandler))
	mux.Handle("/pokemon", withSessionValidation(sessionStore, time.Now, pokemonHandler))
	mux.Handle("/pokemon/{resultId}", withSessionValidation(sessionStore, time.Now, deletePokemonHandler))
	mux.Handle("/pokemon/pending-species", withSessionValidation(sessionStore, time.Now, pendingSpeciesHandler))
	mux.Handle(
		"/pokemon/pending-species/{readingId}",
		withSessionValidation(sessionStore, time.Now, pendingSpeciesResolveHandler),
	)

	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("/protected/ping", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		sess, ok := SessionFromContext(r.Context())
		if !ok {
			writeAPIError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", nil)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status":    "ok",
			"sessionId": sess.ID,
		})
	})
	mux.Handle("/protected/", withSessionValidation(sessionStore, time.Now, protectedMux))

	handler := withCORS(cfg.CORSOrigins, mux)

	return &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}, nil
}
