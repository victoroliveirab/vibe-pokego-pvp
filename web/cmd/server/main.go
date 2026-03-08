package main

import (
	"log"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/config"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/web/internal/httpserver"
)

func main() {
	cfg := config.MustLoadFromEnv()
	srv, err := httpserver.New(cfg, cfg.Storage)
	if err != nil {
		log.Fatal(err)
	}

	log.Fatal(srv.ListenAndServe())
}
