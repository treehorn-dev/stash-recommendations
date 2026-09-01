package main

import (
	"context"
	"log"
	"net/http"

	"github.com/treehorn/stash-recommendations/server/internal/config"
	"github.com/treehorn/stash-recommendations/server/internal/httpapi"
	"github.com/treehorn/stash-recommendations/server/internal/ingest"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	repository, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer repository.Close(ctx)
	if err := repository.Migrate(ctx); err != nil {
		log.Fatal(err)
	}

	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository:  repository,
		InteractionService: ingest.NewInteractionService(repository),
	})
	if err := http.ListenAndServe(cfg.HTTPAddr, handler); err != nil {
		log.Fatal(err)
	}
}
