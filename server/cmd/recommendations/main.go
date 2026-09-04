package main

import (
	"context"
	"log"
	"net/http"

	"github.com/treehorn/stash-recommendations/server/internal/catalog"
	"github.com/treehorn/stash-recommendations/server/internal/config"
	"github.com/treehorn/stash-recommendations/server/internal/httpapi"
	"github.com/treehorn/stash-recommendations/server/internal/ingest"
	"github.com/treehorn/stash-recommendations/server/internal/model"
	"github.com/treehorn/stash-recommendations/server/internal/refresh"
	"github.com/treehorn/stash-recommendations/server/internal/session"
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
	modelRepository := model.NewRepository(repository.Pool())
	modelRefresh := refresh.NewRunner(
		repository,
		session.NewBuilder(repository.Pool()),
		model.NewBuilder(modelRepository, cfg.ModelOWeight),
	)
	if cfg.RebuildModelOnce {
		version, err := modelRefresh.Refresh(ctx)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("recommendation refresh activated version=%s", version)
		return
	}
	if cfg.BuildModelOnStart {
		if _, err := modelRefresh.Refresh(ctx); err != nil {
			log.Printf("recommendation refresh failed; retaining the prior active version: %v", err)
		}
	}
	if cfg.ModelRefreshInterval > 0 {
		go refresh.RunPeriodically(ctx, cfg.ModelRefreshInterval, modelRefresh.Refresh,
			func(version string) { log.Printf("recommendation refresh activated version=%s", version) },
			func(err error) {
				log.Printf("recommendation refresh failed; retaining the prior active version: %v", err)
			},
		)
	}

	handler := httpapi.NewMux(httpapi.Dependencies{
		AccountRepository:    repository,
		InteractionService:   ingest.NewInteractionService(repository),
		SnapshotService:      catalog.NewSnapshotService(repository),
		RecommendationReader: modelRepository,
	})
	if err := http.ListenAndServe(cfg.HTTPAddr, handler); err != nil {
		log.Fatal(err)
	}
}
