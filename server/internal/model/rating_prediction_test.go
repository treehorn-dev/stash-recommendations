package model

import (
	"math"
	"testing"
)

func TestPersonalRatingUsesPositiveSimilarityWeightedLabels(t *testing.T) {
	prediction, ok := PersonalRating(
		[]RatingEvidence{{Rating: 1, Similarity: 0.9}, {Rating: 0.5, Similarity: 0.6}, {Rating: 0, Similarity: -0.8}},
		0.4,
		5,
	)
	if !ok {
		t.Fatal("expected a prediction")
	}

	want := (ratingPredictionPriorWeight*0.4 + 0.9 + 0.6*0.5) / (ratingPredictionPriorWeight + 0.9 + 0.6)
	if math.Abs(prediction-want) > 1e-9 {
		t.Fatalf("expected %f, got %f", want, prediction)
	}
}

func TestPersonalRatingShrinksWeakEvidenceToAccountMean(t *testing.T) {
	prediction, ok := PersonalRating([]RatingEvidence{{Rating: 1, Similarity: 0.01}}, 0.2, 5)
	if !ok {
		t.Fatal("expected a prediction")
	}
	if math.Abs(prediction-0.2) > 0.01 {
		t.Fatalf("expected weak evidence to remain near account mean, got %f", prediction)
	}
}

func TestPersonalRatingRequiresMinimumVectorBackedRatings(t *testing.T) {
	if prediction, ok := PersonalRating([]RatingEvidence{{Rating: 1, Similarity: 1}}, 0.5, minimumRatingsForPrediction-1); ok || prediction != 0 {
		t.Fatalf("expected no prediction below minimum rating history, got %f, %t", prediction, ok)
	}
}
