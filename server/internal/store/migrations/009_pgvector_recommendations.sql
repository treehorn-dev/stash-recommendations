CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;

CREATE TABLE IF NOT EXISTS model_scene_vectors (
    model_version_id UUID NOT NULL REFERENCES model_versions(id) ON DELETE CASCADE,
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    embedding public.vector(256) NOT NULL,
    PRIMARY KEY (model_version_id, endpoint, stash_id)
);

CREATE INDEX IF NOT EXISTS model_scene_vectors_embedding_hnsw
    ON model_scene_vectors USING hnsw (embedding public.vector_cosine_ops);
