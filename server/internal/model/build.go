package model

import (
	"context"
	"fmt"
	"github.com/treehorn/stash-recommendations/server/internal/domain"
	"math"
	"sort"
)

const collaborativeShrinkage = 2

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
	r, e := b.source.CurrentRatings(ctx)
	if e != nil {
		return "", fmt.Errorf("load ratings: %w", e)
	}
	s, e := b.source.CurrentSessions(ctx)
	if e != nil {
		return "", fmt.Errorf("load sessions: %w", e)
	}
	c, e := b.source.CatalogCandidates(ctx)
	if e != nil {
		return "", fmt.Errorf("load catalog candidates: %w", e)
	}
	v, e := b.source.CatalogedScenes(ctx)
	if e != nil {
		return "", fmt.Errorf("load cataloged scenes: %w", e)
	}
	id, e := b.source.SaveAndActivate(ctx, b.buildProjection(r, s, c, v))
	if e != nil {
		return "", fmt.Errorf("save and activate recommendation projection: %w", e)
	}
	return id, nil
}

type scoredCandidate struct {
	score   float64
	reasons map[string]struct{}
}
type neighborScores map[domain.ContentKey]map[domain.ContentKey]*scoredCandidate

func (b *Builder) buildProjection(r []Rating, s []Session, c []CatalogCandidate, v []domain.ContentKey) Projection {
	e := neighborScores{}
	addRatings(e, r)
	addSessions(e, s, b.oWeight)
	addCatalogCandidates(e, c)
	filterCatalogedEdges(e, v)
	return Projection{flattenNeighbors(e), deriveUserRecommendations(e, r, s, b.oWeight)}
}

type coRatingStats struct {
	numerator, leftSquares, rightSquares float64
	count                                int
}

func collaborativeNeighborScores(ratings []Rating) map[domain.ContentKey]map[domain.ContentKey]float64 {
	by := map[string][]Rating{}
	for _, r := range ratings {
		by[r.AccountID] = append(by[r.AccountID], r)
	}
	accounts := make([]string, 0, len(by))
	for a := range by {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)
	pairs := map[[2]domain.ContentKey]*coRatingStats{}
	for _, a := range accounts {
		rs := append([]Rating(nil), by[a]...)
		sort.Slice(rs, func(i, j int) bool { return contentKeyLess(rs[i].ContentKey, rs[j].ContentKey) })
		if len(rs) < 2 {
			continue
		}
		mean := 0.
		for _, r := range rs {
			mean += r.Value
		}
		mean /= float64(len(rs))
		for i := range rs {
			for j := i + 1; j < len(rs); j++ {
				k := [2]domain.ContentKey{rs[i].ContentKey, rs[j].ContentKey}
				z := pairs[k]
				if z == nil {
					z = &coRatingStats{}
					pairs[k] = z
				}
				x, y := rs[i].Value-mean, rs[j].Value-mean
				z.numerator += x * y
				z.leftSquares += x * x
				z.rightSquares += y * y
				z.count++
			}
		}
	}
	ks := make([][2]domain.ContentKey, 0, len(pairs))
	for k := range pairs {
		ks = append(ks, k)
	}
	sort.Slice(ks, func(i, j int) bool {
		if ks[i][0] != ks[j][0] {
			return contentKeyLess(ks[i][0], ks[j][0])
		}
		return contentKeyLess(ks[i][1], ks[j][1])
	})
	out := map[domain.ContentKey]map[domain.ContentKey]float64{}
	for _, k := range ks {
		z := pairs[k]
		d := math.Sqrt(z.leftSquares * z.rightSquares)
		if d == 0 {
			continue
		}
		score := z.numerator / d * float64(z.count) / float64(z.count+collaborativeShrinkage)
		if out[k[0]] == nil {
			out[k[0]] = map[domain.ContentKey]float64{}
		}
		if out[k[1]] == nil {
			out[k[1]] = map[domain.ContentKey]float64{}
		}
		out[k[0]][k[1]] = score
		out[k[1]][k[0]] = score
	}
	return out
}
func addRatings(e neighborScores, r []Rating) {
	for a, cs := range collaborativeNeighborScores(r) {
		for c, s := range cs {
			addEdge(e, a, c, s, "collaborative_rating")
		}
	}
}
func addSessions(e neighborScores, s []Session, w float64) {
	for _, x := range s {
		for i := 0; i < len(x.Items); i++ {
			for j := i + 1; j < len(x.Items); j++ {
				addSymmetric(e, x.Items[i].ContentKey, x.Items[j].ContentKey, (eventWeight(x.Items[i].Kind, w)+eventWeight(x.Items[j].Kind, w))/2, "session_cooccurrence")
			}
		}
		for i := 0; i+1 < len(x.Items); i++ {
			addEdge(e, x.Items[i].ContentKey, x.Items[i+1].ContentKey, (eventWeight(x.Items[i].Kind, w)+eventWeight(x.Items[i+1].Kind, w))/2, "ordered_transition")
		}
	}
}
func eventWeight(k string, w float64) float64 {
	if k == "scene.o" {
		return w
	}
	return 1
}
func addCatalogCandidates(e neighborScores, c []CatalogCandidate) {
	sort.Slice(c, func(i, j int) bool {
		if c[i].Source != c[j].Source {
			return contentKeyLess(c[i].Source, c[j].Source)
		}
		if c[i].Candidate != c[j].Candidate {
			return contentKeyLess(c[i].Candidate, c[j].Candidate)
		}
		return c[i].Reason < c[j].Reason
	})
	for _, x := range c {
		addEdge(e, x.Source, x.Candidate, .5, x.Reason)
	}
}
func addSymmetric(e neighborScores, a, b domain.ContentKey, s float64, r string) {
	addEdge(e, a, b, s, r)
	addEdge(e, b, a, s, r)
}
func addEdge(e neighborScores, a, b domain.ContentKey, s float64, r string) {
	if a == b {
		return
	}
	if e[a] == nil {
		e[a] = map[domain.ContentKey]*scoredCandidate{}
	}
	x := e[a][b]
	if x == nil {
		x = &scoredCandidate{reasons: map[string]struct{}{}}
		e[a][b] = x
	}
	x.score += s
	x.reasons[r] = struct{}{}
}
func filterCatalogedEdges(e neighborScores, v []domain.ContentKey) {
	ok := map[domain.ContentKey]struct{}{}
	for _, k := range v {
		ok[k] = struct{}{}
	}
	for a, cs := range e {
		if _, yes := ok[a]; !yes {
			delete(e, a)
			continue
		}
		for b := range cs {
			if _, yes := ok[b]; !yes {
				delete(cs, b)
			}
		}
	}
}
func flattenNeighbors(e neighborScores) []Neighbor {
	var o []Neighbor
	for _, a := range sortedContentKeys(e) {
		for _, b := range sortedContentKeys(e[a]) {
			x := e[a][b]
			o = append(o, Neighbor{a, b, x.score, sortedReasons(x.reasons)})
		}
	}
	return o
}
func deriveUserRecommendations(e neighborScores, r []Rating, s []Session, w float64) []UserRecommendation {
	known := map[string]map[domain.ContentKey]struct{}{}
	seeds := map[string]map[domain.ContentKey]float64{}
	knownAdd := func(a string, k domain.ContentKey) {
		if known[a] == nil {
			known[a] = map[domain.ContentKey]struct{}{}
			seeds[a] = map[domain.ContentKey]float64{}
		}
		known[a][k] = struct{}{}
	}
	seedAdd := func(a string, k domain.ContentKey, z float64) { knownAdd(a, k); seeds[a][k] += z }
	sort.Slice(r, func(i, j int) bool {
		if r[i].AccountID != r[j].AccountID {
			return r[i].AccountID < r[j].AccountID
		}
		return contentKeyLess(r[i].ContentKey, r[j].ContentKey)
	})
	for _, x := range r {
		knownAdd(x.AccountID, x.ContentKey)
		if x.Value > 0 {
			seedAdd(x.AccountID, x.ContentKey, x.Value)
		}
	}
	for _, x := range s {
		for _, i := range x.Items {
			seedAdd(x.AccountID, i.ContentKey, eventWeight(i.Kind, w))
		}
	}
	accounts := make([]string, 0, len(seeds))
	for a := range seeds {
		accounts = append(accounts, a)
	}
	sort.Strings(accounts)
	var out []UserRecommendation
	for _, a := range accounts {
		cs := map[domain.ContentKey]*scoredCandidate{}
		for _, seed := range sortedContentKeys(seeds[a]) {
			for _, candidate := range sortedContentKeys(e[seed]) {
				if _, yes := known[a][candidate]; yes {
					continue
				}
				n := e[seed][candidate]
				x := cs[candidate]
				if x == nil {
					x = &scoredCandidate{reasons: map[string]struct{}{}}
					cs[candidate] = x
				}
				x.score += seeds[a][seed] * n.score
				for reason := range n.reasons {
					x.reasons[reason] = struct{}{}
				}
			}
		}
		for _, k := range sortedContentKeys(cs) {
			x := cs[k]
			if math.Abs(x.score) >= 1e-12 {
				out = append(out, UserRecommendation{a, k, x.score, sortedReasons(x.reasons)})
			}
		}
	}
	return out
}
func sortedReasons(m map[string]struct{}) []string {
	o := make([]string, 0, len(m))
	for x := range m {
		o = append(o, x)
	}
	sort.Strings(o)
	return o
}
func sortedContentKeys[T any](m map[domain.ContentKey]T) []domain.ContentKey {
	o := make([]domain.ContentKey, 0, len(m))
	for x := range m {
		o = append(o, x)
	}
	sort.Slice(o, func(i, j int) bool { return contentKeyLess(o[i], o[j]) })
	return o
}
func contentKeyLess(a, b domain.ContentKey) bool {
	if a.Endpoint != b.Endpoint {
		return a.Endpoint < b.Endpoint
	}
	return a.StashID < b.StashID
}
