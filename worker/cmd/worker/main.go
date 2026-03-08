package main

import (
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/worker"
)

func main() {
	cfg := config.MustLoadFromEnv()
	worker.Run(cfg, cfg.Storage)
}
