package model

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/treehorn/stash-recommendations/server/internal/domain"
)

type Builder struct {
	source  buildSource
	oWeight float64
}

func NewBuilder(source buildSource, oWeight float64) *Builder {
	if oWeight <= 0 || math.IsNaN(oWeight) || math.IsInf(oWeight, 0) {
		oWeight = DefaultOWeight
	}
	return &Builder{source, oWeight}
}

func (b *Builder) BuildAndActivate(ctx context.Context) (string, error) {
	ratings, err := b.source.CurrentRatings(ctx)
	if err != nil {
		return "", fmt.Errorf("load ratings: %w", err)
	}
	sessions, err := b.source.CurrentSessions(ctx)
	if err != nil {
		return "", fmt.Errorf("load sessions: %w", err)
	}
	catalog, err := b.source.CatalogScenes(ctx)
	if err != nil {
		return "", fmt.Errorf("load catalog scenes: %w", err)
	}
	versionID, err := b.source.SaveAndActivateVectors(ctx, buildVectorProjection(catalog, ratings, sessions, b.oWeight))
	if err != nil {
		return "", fmt.Errorf("save and activate vector projection: %w", err)
	}
	return versionID, nil
}

// buildVectorProjection derives bounded scene and account vectors without
// materializing scene-to-scene or interaction-to-interaction pairs.
func buildVectorProjection(catalog []CatalogScene, ratings []Rating, sessions []Session, oWeight float64) VectorProjection {
	sort.Slice(catalog, func(i, j int) bool { return contentKeyLess(catalog[i].ContentKey, catalog[j].ContentKey) })
	projection := VectorProjection{}
	sceneVectors := make(map[domain.ContentKey][]float32, len(catalog))
	for _, scene := range catalog {
		vector, ok := SceneVector(scene.Features)
		if !ok {
			continue
		}
		projection.SceneVectors = append(projection.SceneVectors, SceneEmbedding{ContentKey: scene.ContentKey, Embedding: vector})
		sceneVectors[scene.ContentKey] = vector
	}

	interactions := map[string][]WeightedInteraction{}
	reasons := map[string]map[string]struct{}{}
	addInteraction := func(accountID string, interaction WeightedInteraction, reason string) {
		if interaction.Weight <= 0 {
			return
		}
		interactions[accountID] = append(interactions[accountID], interaction)
		if reasons[accountID] == nil {
			reasons[accountID] = map[string]struct{}{}
		}
		reasons[accountID][reason] = struct{}{}
	}
	for _, rating := range ratings {
		addInteraction(rating.AccountID, WeightedInteraction{ContentKey: rating.ContentKey, Weight: rating.Value}, "rating_profile")
	}
	for _, session := range sessions {
		for _, item := range session.Items {
			reason := "play_profile"
			if item.Kind == "scene.o" {
				reason = "o_profile"
			}
			addInteraction(session.AccountID, WeightedInteraction{ContentKey: item.ContentKey, Weight: eventWeight(item.Kind, oWeight)}, reason)
		}
	}

	accounts := make([]string, 0, len(interactions))
	for accountID := range interactions {
		accounts = append(accounts, accountID)
	}
	sort.Strings(accounts)
	for _, accountID := range accounts {
		vector, _, ok := ProfileVector(sceneVectors, interactions[accountID])
		if !ok {
			continue
		}
		projection.Profiles = append(projection.Profiles, AccountProfile{
			AccountID: accountID,
			Embedding: vector,
			Reasons:   sortedReasons(reasons[accountID]),
		})
	}
	return projection
}

func eventWeight(kind string, oWeight float64) float64 {
	if kind == "scene.o" {
		return oWeight
	}
	return 1
}

func sortedReasons(reasons map[string]struct{}) []string {
	result := make([]string, 0, len(reasons))
	for reason := range reasons {
		result = append(result, reason)
	}
	sort.Strings(result)
	return result
}

func contentKeyLess(left, right domain.ContentKey) bool {
	if left.Endpoint != right.Endpoint {
		return left.Endpoint < right.Endpoint
	}
	return left.StashID < right.StashID
}
