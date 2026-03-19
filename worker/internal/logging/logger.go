package logging

import (
	"log/slog"
	"os"

	slogbetterstack "github.com/samber/slog-betterstack"
)

func New(env, token, endpoint string) *slog.Logger {
	if env == "production" {
		return createProdLogger(token, endpoint)
	}

	return createDevLogger()
}

func createDevLogger() *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))
	return logger
}

func createProdLogger(token, endpoint string) *slog.Logger {
	logger := slog.New(
		slogbetterstack.Option{
			Token:    token,
			Endpoint: endpoint,
		}.NewBetterstackHandler(),
	)
	return logger
}
