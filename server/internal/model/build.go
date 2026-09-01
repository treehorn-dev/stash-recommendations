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
	if oWeight <= 0 {
		oWeight = DefaultOWeight
	}
	return &Builder{source: source, oWeight: oWeight}
}

// BuildAndActivate calculates a new projection outside request paths and atomically serves it.
func (builder *Builder) BuildAndActivate(ctx context.Context) (string, error) {
	ratings, err := builder.source.CurrentRatings(ctx)
	if err != nil {
		return "", fmt.Errorf("load ratings: %w", err)
	}
	sessions, err := builder.source.CurrentSessions(ctx)
	if err != nil {
		return "", fmt.Errorf("load sessions: %w", err)
	}
	catalogCandidates, err := builder.source.CatalogCandidates(ctx)
	if err != nil {
		return "", fmt.Errorf("load catalog candidates: %w", err)
	}

	projection := builder.buildProjection(ratings, sessions, catalogCandidates)
	version, err := builder.source.SaveAndActivate(ctx, projection)
	if err != nil {
		return "", fmt.Errorf("save and activate recommendation projection: %w", err)
	}
	return version, nil
}

type scoredCandidate struct {
	score   float64
	reasons map[string]struct{}
}

type neighborScores map[domain.ContentKey]map[domain.ContentKey]*scoredCandidate

func (builder *Builder) buildProjection(ratings []Rating, sessions []Session, catalogCandidates []CatalogCandidate) Projection {
	edges := make(neighborScores)
	addRatings(edges, ratings)
	addSessions(edges, sessions, builder.oWeight)
	addCatalogCandidates(edges, catalogCandidates)
	return Projection{
		Neighbors:           flattenNeighbors(edges),
		UserRecommendations: deriveUserRecommendations(edges, ratings, sessions, builder.oWeight),
	}
}

func addRatings(edges neighborScores, ratings []Rating) {
	byAccount := make(map[string][]Rating)
	for _, rating := range ratings {
		byAccount[rating.AccountID] = append(byAccount[rating.AccountID], rating)
	}
	for _, accountRatings := range byAccount {
		if len(accountRatings) < 2 {
			continue
		}
		mean := 0.0
		for _, rating := range accountRatings {
			mean += rating.Value
		}
		mean /= float64(len(accountRatings))
		for left := 0; left < len(accountRatings); left++ {
			for right := left + 1; right < len(accountRatings); right++ {
				// Mean-centering captures agreement beyond an account's own baseline;
				// the bounded positive preference term retains a useful signal for
				// all-positive sparse rating histories.
				score := (accountRatings[left].Value-mean)*(accountRatings[right].Value-mean) + 0.1*accountRatings[left].Value*accountRatings[right].Value
				addSymmetric(edges, accountRatings[left].ContentKey, accountRatings[right].ContentKey, score, "collaborative_rating")
			}
		}
	}
}

func addSessions(edges neighborScores, sessions []Session, oWeight float64) {
	for _, session := range sessions {
		for left := 0; left < len(session.Items); left++ {
			for right := left + 1; right < len(session.Items); right++ {
				weight := (eventWeight(session.Items[left].Kind, oWeight) + eventWeight(session.Items[right].Kind, oWeight)) / 2
				addSymmetric(edges, session.Items[left].ContentKey, session.Items[right].ContentKey, weight, "session_cooccurrence")
			}
		}
		for index := 0; index+1 < len(session.Items); index++ {
			weight := (eventWeight(session.Items[index].Kind, oWeight) + eventWeight(session.Items[index+1].Kind, oWeight)) / 2
			addEdge(edges, session.Items[index].ContentKey, session.Items[index+1].ContentKey, weight, "ordered_transition")
		}
	}
}

func eventWeight(kind string, oWeight float64) float64 {
	if kind == "scene.o" {
		return oWeight
	}
	return 1
}

func addCatalogCandidates(edges neighborScores, candidates []CatalogCandidate) {
	for _, candidate := range candidates {
		addEdge(edges, candidate.Source, candidate.Candidate, 0.5, candidate.Reason)
	}
}

func addSymmetric(edges neighborScores, left, right domain.ContentKey, score float64, reason string) {
	addEdge(edges, left, right, score, reason)
	addEdge(edges, right, left, score, reason)
}

func addEdge(edges neighborScores, source, candidate domain.ContentKey, score float64, reason string) {
	if source == candidate {
		return
	}
	if edges[source] == nil {
		edges[source] = make(map[domain.ContentKey]*scoredCandidate)
	}
	entry := edges[source][candidate]
	if entry == nil {
		entry = &scoredCandidate{reasons: make(map[string]struct{})}
		edges[source][candidate] = entry
	}
	entry.score += score
	entry.reasons[reason] = struct{}{}
}

func flattenNeighbors(edges neighborScores) []Neighbor {
	var neighbors []Neighbor
	for source, candidates := range edges {
		for candidate, scored := range candidates {
			neighbors = append(neighbors, Neighbor{Source: source, Candidate: candidate, Score: scored.score, Reasons: sortedReasons(scored.reasons)})
		}
	}
	sort.Slice(neighbors, func(i, j int) bool {
		if neighbors[i].Source != neighbors[j].Source {
			return contentKeyLess(neighbors[i].Source, neighbors[j].Source)
		}
		return contentKeyLess(neighbors[i].Candidate, neighbors[j].Candidate)
	})
	return neighbors
}

func deriveUserRecommendations(edges neighborScores, ratings []Rating, sessions []Session, oWeight float64) []UserRecommendation {
	known := make(map[string]map[domain.ContentKey]struct{})
	seeds := make(map[string]map[domain.ContentKey]float64)
	addSeed := func(accountID string, key domain.ContentKey, weight float64) {
		if known[accountID] == nil {
			known[accountID] = make(map[domain.ContentKey]struct{})
			seeds[accountID] = make(map[domain.ContentKey]float64)
		}
		known[accountID][key] = struct{}{}
		seeds[accountID][key] += weight
	}
	for _, rating := range ratings {
		if rating.Value > 0 {
			addSeed(rating.AccountID, rating.ContentKey, rating.Value)
		}
	}
	for _, session := range sessions {
		for _, item := range session.Items {
			addSeed(session.AccountID, item.ContentKey, eventWeight(item.Kind, oWeight))
		}
	}

	var results []UserRecommendation
	for accountID, accountSeeds := range seeds {
		candidates := make(map[domain.ContentKey]*scoredCandidate)
		for seed, weight := range accountSeeds {
			for candidate, neighbor := range edges[seed] {
				if _, seen := known[accountID][candidate]; seen {
					continue
				}
				entry := candidates[candidate]
				if entry == nil {
					entry = &scoredCandidate{reasons: make(map[string]struct{})}
					candidates[candidate] = entry
				}
				entry.score += weight * neighbor.score
				for reason := range neighbor.reasons {
					entry.reasons[reason] = struct{}{}
				}
			}
		}
		for key, scored := range candidates {
			if math.Abs(scored.score) < 1e-12 {
				continue
			}
			results = append(results, UserRecommendation{AccountID: accountID, ContentKey: key, Score: scored.score, Reasons: sortedReasons(scored.reasons)})
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].AccountID != results[j].AccountID {
			return results[i].AccountID < results[j].AccountID
		}
		return contentKeyLess(results[i].ContentKey, results[j].ContentKey)
	})
	return results
}

func sortedReasons(reasons map[string]struct{}) []string {
	values := make([]string, 0, len(reasons))
	for reason := range reasons {
		values = append(values, reason)
	}
	sort.Strings(values)
	return values
}

func contentKeyLess(left, right domain.ContentKey) bool {
	if left.Endpoint != right.Endpoint {
		return left.Endpoint < right.Endpoint
	}
	return left.StashID < right.StashID
}
