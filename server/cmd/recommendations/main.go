package main

import (
	"log"
	"net/http"

	"github.com/treehorn/stash-recommendations/server/internal/config"
	"github.com/treehorn/stash-recommendations/server/internal/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	if err := http.ListenAndServe(cfg.HTTPAddr, httpapi.NewMux()); err != nil {
		log.Fatal(err)
	}
}
