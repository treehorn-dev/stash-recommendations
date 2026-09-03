package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/treehorn/stash-recommendations/server/internal/domain"
	"github.com/treehorn/stash-recommendations/server/internal/model"
)

type recommendationsResponse struct {
	ModelVersion string                 `json:"model_version"`
	Items        []model.Recommendation `json:"items"`
}

func GetRelated(reader model.Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := AccountFromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		key, err := (domain.ContentKey{}).Normalize(r.URL.Query().Get("endpoint"), r.URL.Query().Get("stash_id"))
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		limit, err := recommendationLimit(r)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		items, version, err := reader.Related(r.Context(), account.ID, key, limit)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeRecommendations(w, version, items)
	})
}

func GetForYou(reader model.Reader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		account, ok := AccountFromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		limit, err := recommendationLimit(r)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		items, version, err := reader.ForYou(r.Context(), account.ID, limit)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeRecommendations(w, version, items)
	})
}

func recommendationLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 20, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > 100 {
		return 0, strconv.ErrSyntax
	}
	return limit, nil
}

func writeRecommendations(w http.ResponseWriter, version string, items []model.Recommendation) {
	if items == nil {
		items = []model.Recommendation{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(recommendationsResponse{ModelVersion: version, Items: items})
}
