package model

import (
	"math"
	"slices"
	"testing"
)

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
