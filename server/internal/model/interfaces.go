// Package model builds and reads versioned recommendation projections.
package model

import (
	"context"

	"github.com/treehorn/stash-recommendations/server/internal/domain"
)

const DefaultOWeight = 1.5

// Recommendation is a ranked scene returned by a recommendation read.
type Recommendation struct {
	ContentKey   domain.ContentKey `json:"content_key"`
	Score        float64           `json:"score"`
	Reasons      []string          `json:"reasons"`
	ModelVersion string            `json:"model_version"`
	CanonicalURL *string           `json:"canonical_url"`
}

type Rating struct {
	AccountID  string
	ContentKey domain.ContentKey
	Value      float64
}

type SessionItem struct {
	ContentKey domain.ContentKey
	Kind       string
}

type Session struct {
	AccountID      string
	ProjectionType string
	Items          []SessionItem
}

type CatalogCandidate struct {
	Source    domain.ContentKey
	Candidate domain.ContentKey
	Reason    string
}

// CatalogScene is a source scene with its normalized Stash-box feature tokens.
type CatalogScene struct {
	ContentKey domain.ContentKey
	Features   []string
}

type SceneEmbedding struct {
	ContentKey domain.ContentKey
	Embedding  []float32
}

type AccountProfile struct {
	AccountID string
	Embedding []float32
	Reasons   []string
}

type VectorProjection struct {
	SceneVectors []SceneEmbedding
	Profiles     []AccountProfile
}

// InteractionSource supplies the current rating and session projections to a batch build.
type InteractionSource interface {
	CurrentRatings(context.Context) ([]Rating, error)
	CurrentSessions(context.Context) ([]Session, error)
}

// CatalogSource supplies source scenes with normalized Stash-box metadata.
type CatalogSource interface {
	CatalogScenes(context.Context) ([]CatalogScene, error)
}

// RecommendationStore atomically persists an inactive vector projection and activates it only when complete.
type RecommendationStore interface {
	SaveAndActivateVectors(context.Context, VectorProjection) (string, error)
}

// Reader serves recommendations from the active model version.
type Reader interface {
	Related(ctx context.Context, source domain.ContentKey, limit int) ([]Recommendation, string, error)
	ForYou(ctx context.Context, accountID string, limit int) ([]Recommendation, string, error)
}

type buildSource interface {
	InteractionSource
	CatalogSource
	RecommendationStore
}

type Neighbor struct {
	Source    domain.ContentKey
	Candidate domain.ContentKey
	Score     float64
	Reasons   []string
}

type UserRecommendation struct {
	AccountID  string
	ContentKey domain.ContentKey
	Score      float64
	Reasons    []string
}

type Projection struct {
	Neighbors           []Neighbor
	UserRecommendations []UserRecommendation
}
