# Personal Rating Prediction Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task.

**Goal:** Attach conservative personal predicted ratings to For You and Related recommendation responses.

**Architecture:** Keep pgvector-derived ranking unchanged. A bounded nearest-rated-scene estimator uses current explicit ratings as labels and shrinks similarity evidence to the account mean. For You persists the normalized prediction with its active model version; Related calculates the same account-specific annotation at read time.

**Tech Stack:** Go, PostgreSQL, pgvector, pgx, Go integration tests, existing JavaScript UI.

### Task 1: Define calibrated personal-rating estimation

**Files:**
- Modify: `server/internal/model/interfaces.go`
- Create: `server/internal/model/rating_prediction.go`
- Create: `server/internal/model/rating_prediction_test.go`

1. Write failing unit tests for a similarity-weighted estimate, user-mean shrinkage when neighbor evidence is weak, and absent output below the minimum five vector-backed ratings.
2. Run `go test ./server/internal/model -run TestPersonalRating -v`; confirm the test fails because the estimator does not exist.
3. Implement the pure bounded estimator constants and function. Ratings remain normalized 0-1 internally; output is absent without sufficient labels.
4. Rerun the focused test; expect pass.
5. Commit `feat: add calibrated personal rating estimator`.

### Task 2: Persist predicted ratings with For You results

**Files:**
- Create: `server/internal/store/migrations/010_predicted_ratings.sql`
- Modify: `server/internal/model/interfaces.go`
- Modify: `server/internal/model/repository.go`
- Modify: `server/internal/model/build_test.go`

1. Write a failing PostgreSQL integration test that seeds five rated vectors and a similar candidate, builds a model, and asserts `ForYou` returns a non-nil five-point `PredictedRating` close to the labels.
2. Run `go test ./server/internal/model -run TestBuild.*Predicted -v`; confirm failure because `user_recommendations` has no prediction column or reader field.
3. Add nullable normalized storage, calculate each top-50 profile candidate against no more than 20 active rated vectors, and store the result. Extend reads and serialization to expose a five-point `predicted_rating`.
4. Rerun the focused test; expect pass.
5. Commit `feat: persist predicted ratings for For You`.

### Task 3: Attach personal predictions to Related results

**Files:**
- Modify: `server/internal/model/interfaces.go`
- Modify: `server/internal/model/repository.go`
- Modify: `server/internal/model/build_test.go`
- Modify: `server/internal/httpapi/recommendations.go`
- Modify: `server/internal/httpapi/recommendations_test.go`

1. Write failing integration and handler tests: Related receives the authenticated account ID, returns that account's prediction for a similar scene, and preserves `null` for an account under the rating threshold.
2. Run `go test ./server/internal/model ./server/internal/httpapi -run 'Test.*Related.*Predicted' -v`; confirm failure because Related lacks account context and prediction data.
3. Make Related account-aware and use the same bounded SQL estimator as For You. Keep source ranking, reasons, and canonical URLs unchanged.
4. Rerun the focused tests; expect pass.
5. Commit `feat: annotate related recommendations with personal ratings`.

### Task 4: Regression verification and delivery

**Files:**
- Modify: `README.md`
- Test: `server/internal/model/rating_prediction_test.go`
- Test: `server/internal/model/build_test.go`
- Test: `server/internal/httpapi/recommendations_test.go`

1. Document that `predicted_rating` is personal, normalized from explicit ratings, and omitted for cold-start accounts.
2. Run `make test`, `make test-contract`, `make test-e2e`, `go vet ./server/...`, and `git diff --check`.
3. Inspect the JSON response with a seeded account and confirm the existing UI receives a five-point predicted value without UI changes.
4. Commit `docs: describe personal predicted ratings`.
5. Push the feature branch and open a PR; do not release before review.
