package httpapi

import (
	"context"
	"io"
	"net/http"
)

type SourceSnapshotService interface {
	Upsert(ctx context.Context, snapshotJSON []byte) error
}

func PostSnapshots(service SourceSnapshotService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := AccountFromContext(r.Context()); !ok {
			unauthorized(w)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := service.Upsert(r.Context(), body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	})
}
