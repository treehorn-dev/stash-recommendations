package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/treehorn/stash-recommendations/server/internal/domain"
	"github.com/treehorn/stash-recommendations/server/internal/store"
)

type InteractionService interface {
	Accept(ctx context.Context, accountID string, event domain.PreferenceEvent) (bool, error)
}

type Dependencies struct {
	AccountRepository  store.AccountRepository
	InteractionService InteractionService
}

func PostInteractions(service InteractionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := AccountFromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}

		var event domain.PreferenceEvent
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&event); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		accepted, err := service.Accept(r.Context(), account.ID, event)
		if err != nil {
			if errors.Is(err, store.ErrInteractionEventConflict) {
				http.Error(w, "conflict", http.StatusConflict)
				return
			}
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if accepted {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
