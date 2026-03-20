package logging

import (
	"log/slog"
	"os"

	slogbetterstack "github.com/samber/slog-betterstack"
)

// New builds the application logger for the provided runtime environment.
func New(cfg Config) *slog.Logger {
	if cfg.Env == "production" {
		return newProdLogger(cfg.BetterstackToken, cfg.BetterstackEndpoint)
	}

	return newDevLogger()
}

func newDevLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
}

func newProdLogger(token, endpoint string) *slog.Logger {
	return slog.New(
		slogbetterstack.Option{
			Token:    token,
			Endpoint: endpoint,
		}.NewBetterstackHandler(),
	)
}
