package model

const (
	minimumRatingsForPrediction = 5
	ratingPredictionPriorWeight = 3
	maxRatingNeighbors          = 20
)

// RatingEvidence is one rated scene's similarity to a recommendation candidate.
type RatingEvidence struct {
	Rating     float64
	Similarity float64
}

// PersonalRating estimates a normalized personal rating. Negative similarity
// cannot make a scene less appealing; weak positive evidence shrinks to the
// account's mean rather than overstating an isolated neighbor.
func PersonalRating(evidence []RatingEvidence, accountMean float64, ratingCount int) (float64, bool) {
	if ratingCount < minimumRatingsForPrediction {
		return 0, false
	}

	weightedRatings := 0.0
	weight := 0.0
	for _, item := range evidence {
		if item.Similarity <= 0 {
			continue
		}
		weightedRatings += item.Similarity * item.Rating
		weight += item.Similarity
	}
	return (ratingPredictionPriorWeight*accountMean + weightedRatings) / (ratingPredictionPriorWeight + weight), true
}
