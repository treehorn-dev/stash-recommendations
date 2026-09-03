package model

const (
	minimumRatingsForPrediction = 5
	ratingPredictionPriorWeight = 3
	maxRatingNeighbors          = 20
)

type RatingEvidence struct{ Rating, Similarity float64 }

func PersonalRating(evidence []RatingEvidence, accountMean float64, ratingCount int) (float64, bool) {
	if ratingCount < minimumRatingsForPrediction {
		return 0, false
	}
	weightedRatings, weight := 0.0, 0.0
	for _, item := range evidence {
		if item.Similarity > 0 {
			weightedRatings += item.Similarity * item.Rating
			weight += item.Similarity
		}
	}
	return (ratingPredictionPriorWeight*accountMean + weightedRatings) / (ratingPredictionPriorWeight + weight), true
}
