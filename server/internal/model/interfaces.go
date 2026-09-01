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

// InteractionSource supplies the current rating and session projections to a batch build.
type InteractionSource interface {
	CurrentRatings(context.Context) ([]Rating, error)
	CurrentSessions(context.Context) ([]Session, error)
}

// CatalogSource supplies candidates derived from validated catalog projections.
type CatalogSource interface {
	CatalogCandidates(context.Context) ([]CatalogCandidate, error)
}

// RecommendationStore atomically persists an inactive projection and activates it only when complete.
type RecommendationStore interface {
	SaveAndActivate(context.Context, Projection) (string, error)
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
