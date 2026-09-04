package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/treehorn/stash-recommendations/server/internal/domain"
	"github.com/treehorn/stash-recommendations/server/internal/model"
)

type recommendationFilterSet = model.ForYouFilters

func recommendationFilters(r *http.Request) (recommendationFilterSet, error) {
	parse := func(name string) (model.NumericPredicate, error) {
		op := r.URL.Query().Get(name + "_operator")
		if op == "" {
			return model.NumericPredicate{}, nil
		}
		switch op {
		case "gt", "gte", "lt", "lte", "eq", "is_null", "not_null":
		default:
			return model.NumericPredicate{}, fmt.Errorf("invalid %s operator", name)
		}
		if op == "is_null" || op == "not_null" {
			if r.URL.Query().Has(name + "_value") {
				return model.NumericPredicate{}, fmt.Errorf("%s value is not allowed", name)
			}
			return model.NumericPredicate{Operator: op}, nil
		}
		value, err := strconv.ParseFloat(r.URL.Query().Get(name+"_value"), 64)
		if err != nil {
			return model.NumericPredicate{}, fmt.Errorf("invalid %s value", name)
		}
		return model.NumericPredicate{Operator: op, Value: value}, nil
	}
	rating, err := parse("rating")
	if err != nil {
		return recommendationFilterSet{}, err
	}
	oCount, err := parse("o_count")
	if err != nil {
		return recommendationFilterSet{}, err
	}
	return recommendationFilterSet{Rating: rating, OCount: oCount}, nil
}

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
		offset, err := recommendationOffset(r)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		filters, err := recommendationFilters(r)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		items, version, err := reader.ForYou(r.Context(), account.ID, limit, offset, filters)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeRecommendations(w, version, items)
	})
}

func recommendationOffset(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("offset")
	if raw == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(raw)
	if err != nil || offset < 0 {
		return 0, strconv.ErrSyntax
	}
	return offset, nil
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
