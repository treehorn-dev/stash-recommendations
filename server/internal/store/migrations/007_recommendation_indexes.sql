CREATE INDEX IF NOT EXISTS item_neighbors_active_read
    ON item_neighbors (model_version_id, source_endpoint, source_stash_id, score DESC);

CREATE INDEX IF NOT EXISTS user_recommendations_active_read
    ON user_recommendations (model_version_id, account_id, score DESC);
