package model

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/treehorn/stash-recommendations/server/internal/domain"
)

func TestBuilderSavesVectorProjectionInsteadOfNeighbors(t *testing.T) {
	source := &vectorBuildSource{
		catalog: []CatalogScene{{
			ContentKey: domain.ContentKey{Endpoint: "https://box.example", StashID: "scene"},
			Features:   []string{"tag:https://box.example:tag"},
		}},
	}

	version, err := NewBuilder(source, DefaultOWeight).BuildAndActivate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if version != "vector-version" {
		t.Fatalf("expected vector version, got %q", version)
	}
	if len(source.saved.SceneVectors) != 1 {
		t.Fatalf("expected one stored scene vector, got %d", len(source.saved.SceneVectors))
	}
}

type vectorBuildSource struct {
	catalog []CatalogScene
	saved   VectorProjection
}

func (source *vectorBuildSource) CurrentRatings(context.Context) ([]Rating, error) {
	return nil, nil
}

func (source *vectorBuildSource) CurrentSessions(context.Context) ([]Session, error) {
	return nil, nil
}

func (source *vectorBuildSource) CatalogScenes(context.Context) ([]CatalogScene, error) {
	return source.catalog, nil
}

func (source *vectorBuildSource) SaveAndActivateVectors(_ context.Context, projection VectorProjection) (string, error) {
	if len(projection.SceneVectors) == 0 {
		return "", errors.New("missing scene vectors")
	}
	source.saved = projection
	return "vector-version", nil
}

func TestSceneVectorIsStableAcrossFeatureOrder(t *testing.T) {
	first, ok := SceneVector([]string{"performer:https://stashdb.org:alice", "tag:https://stashdb.org:solo"})
	if !ok {
		t.Fatal("expected a vector")
	}
	second, ok := SceneVector([]string{"tag:https://stashdb.org:solo", "performer:https://stashdb.org:alice", "performer:https://stashdb.org:alice"})
	if !ok {
		t.Fatal("expected a vector")
	}
	if !slices.Equal(first, second) {
		t.Fatal("expected feature order and duplicates not to change the vector")
	}
}

func TestSceneVectorIsUnitNormalized(t *testing.T) {
	vector, ok := SceneVector([]string{"performer:https://stashdb.org:alice", "tag:https://stashdb.org:solo"})
	if !ok {
		t.Fatal("expected a vector")
	}

	var squaredLength float64
	for _, value := range vector {
		squaredLength += float64(value * value)
	}
	if math.Abs(math.Sqrt(squaredLength)-1) > 1e-6 {
		t.Fatalf("expected unit vector, got length %f", math.Sqrt(squaredLength))
	}
}

func TestSceneVectorDistinguishesFeatures(t *testing.T) {
	first, _ := SceneVector([]string{"performer:https://stashdb.org:alice"})
	second, _ := SceneVector([]string{"performer:https://stashdb.org:bob"})
	if slices.Equal(first, second) {
		t.Fatal("expected distinct feature sets to produce distinct vectors")
	}
}

func TestSceneVectorRejectsEmptyFeatures(t *testing.T) {
	if vector, ok := SceneVector([]string{"", "  "}); ok || vector != nil {
		t.Fatalf("expected no vector, got %#v", vector)
	}
}

func TestProfileVectorWeightsInteractionsWithoutPairExpansion(t *testing.T) {
	first := domain.ContentKey{Endpoint: "https://box.example", StashID: "first"}
	second := domain.ContentKey{Endpoint: "https://box.example", StashID: "second"}
	profile, known, ok := ProfileVector(
		map[domain.ContentKey][]float32{
			first:  {1, 0},
			second: {0, 1},
		},
		[]WeightedInteraction{{ContentKey: first, Weight: 1}, {ContentKey: second, Weight: 3}},
	)
	if !ok {
		t.Fatal("expected a profile vector")
	}
	if !known[first] || !known[second] {
		t.Fatal("expected interacted scenes to be excluded from results")
	}
	if profile[1] <= profile[0] {
		t.Fatalf("expected heavier interaction to dominate profile: %#v", profile)
	}
}

func TestBuildVectorProjectionBuildsOneProfileFromSessionizedBehavior(t *testing.T) {
	first := domain.ContentKey{Endpoint: "https://box.example", StashID: "first"}
	second := domain.ContentKey{Endpoint: "https://box.example", StashID: "second"}
	third := domain.ContentKey{Endpoint: "https://box.example", StashID: "third"}

	projection := buildVectorProjection(
		[]CatalogScene{
			{ContentKey: first, Features: []string{"performer:https://box.example:alex"}},
			{ContentKey: second, Features: []string{"performer:https://box.example:alex"}},
			{ContentKey: third, Features: []string{"tag:https://box.example:unrelated"}},
		},
		[]Rating{{AccountID: "account", ContentKey: first, Value: 1}},
		[]Session{{AccountID: "account", ProjectionType: "latency", Items: []SessionItem{{ContentKey: first, Kind: "scene.played"}, {ContentKey: second, Kind: "scene.o"}}}},
		DefaultOWeight,
	)

	if len(projection.SceneVectors) != 3 {
		t.Fatalf("expected one vector per catalog scene, got %d", len(projection.SceneVectors))
	}
	if len(projection.Profiles) != 1 {
		t.Fatalf("expected one account profile, got %d", len(projection.Profiles))
	}
	profile := projection.Profiles[0]
	if profile.AccountID != "account" {
		t.Fatalf("expected account profile, got %q", profile.AccountID)
	}
	if !slices.Equal(profile.Reasons, []string{"o_profile", "play_profile", "rating_profile"}) {
		t.Fatalf("unexpected profile reasons: %#v", profile.Reasons)
	}
	if similarity(profile.Embedding, projection.SceneVectors[1].Embedding) <= similarity(profile.Embedding, projection.SceneVectors[2].Embedding) {
		t.Fatal("expected shared-performer scene to rank above unrelated scene")
	}
}

func similarity(left, right []float32) float64 {
	var total float64
	for index := range left {
		total += float64(left[index] * right[index])
	}
	return total
}
