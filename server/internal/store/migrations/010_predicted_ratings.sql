ALTER TABLE user_recommendations
    ADD COLUMN IF NOT EXISTS predicted_rating DOUBLE PRECISION;
