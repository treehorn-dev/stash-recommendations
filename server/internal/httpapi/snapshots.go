package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
)

const maxSnapshotBodyBytes = 1 << 20

type SourceSnapshotService interface {
	Upsert(ctx context.Context, snapshotJSON []byte) error
}

func PostSnapshots(service SourceSnapshotService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := AccountFromContext(r.Context()); !ok {
			unauthorized(w)
			return
		}

		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSnapshotBodyBytes))
		if err != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				http.Error(w, "request entity too large", http.StatusRequestEntityTooLarge)
				return
			}
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
