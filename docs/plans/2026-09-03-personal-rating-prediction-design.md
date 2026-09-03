# Personal Rating Prediction Design

## Goal

Return a conservative, personal five-point predicted rating for personalized
recommendations. The prediction complements recommendation ranking; it does
not decide which scenes are eligible for retrieval.

## Decision

Explicit current ratings are the only rating labels. For an account with at
least five rated scenes that have active model vectors, estimate each candidate
from its nearest rated scene vectors and shrink that estimate toward the
account's mean rating. The client receives the normalized prediction as a
five-point value and renders its existing outlined `Predicted rating` badge.

The estimate is:

```
(priorWeight * accountMean + sum(similarity * neighborRating)) /
(priorWeight + sum(similarity))
```

Only positive cosine similarities contribute. A small fixed prior weight
prevents one nearby rating from making a strong claim; sparse or weak evidence
therefore remains close to the account's established mean. The SQL uses at
most 20 nearest rated vectors per candidate, which keeps the calculation
bounded without materializing pairwise tables.

## API and Storage

`user_recommendations.predicted_rating` stores the normalized value generated
with the active model. `Recommendation.PredictedRating` serializes it as
`predicted_rating` after conversion to a 0-5 score for the existing UI.

The Related endpoint becomes account-aware, so related-scene cards receive the
same personal prediction at read time. It keeps the existing source-scene
ranking and canonical URL behavior; only the per-account rating annotation is
new. Accounts without sufficient vector-backed ratings receive `null`, so the
client correctly omits the badge rather than inventing a score.

## Non-goals

- Public/community ratings.
- Replacing behavioral or content-based recommendation ranking with ratings.
- A separate confidence UI or a new recommendation eligibility gate.
- Materialized rating-neighbor pairs or a second recommendation system of
  record.

## Verification

Unit tests cover calibration, prior shrinkage, and insufficient-history
behavior. PostgreSQL integration tests verify persisted For You predictions,
account-aware Related predictions, and JSON response shape. Existing UI tests
continue to prove the outlined predicted badge consumes the field.
