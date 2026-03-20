package main

import (
	"log/slog"
	"os"

	logging "github.com/victoroliveirab/vibe-pokemongo-appraisal-app/shared/logging"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/httpserver"
)

func main() {
	cfg := config.MustLoadFromEnv()
	logger := logging.New(logging.Config{
		Env:                 cfg.AppEnv,
		BetterstackToken:    cfg.BetterstackToken,
		BetterstackEndpoint: cfg.BetterstackEndpoint,
	})
	logger.Info("starting server", "env", cfg.AppEnv)
	slog.SetDefault(logger)

	srv, err := httpserver.New(cfg, cfg.Storage)
	if err != nil {
		logger.Error("web server initialization failed", "error", err)
		os.Exit(1)
	}

	if err := srv.ListenAndServe(); err != nil {
		logger.Error("web server failed", "error", err)
		os.Exit(1)
	}
}
