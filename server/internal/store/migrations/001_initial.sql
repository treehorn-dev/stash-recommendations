CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    key_id TEXT NOT NULL UNIQUE,
    key_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS preference_events (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    event_id UUID NOT NULL,
    client_id UUID NOT NULL,
    sequence BIGINT NOT NULL,
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('scene.rating.set', 'scene.rating.remove')),
    rating DOUBLE PRECISION,
    occurred_at TIMESTAMPTZ NOT NULL,
    origin TEXT NOT NULL,
    body_hash BYTEA NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, event_id)
);

CREATE TABLE IF NOT EXISTS engagement_events (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    event_id UUID NOT NULL,
    client_id UUID NOT NULL,
    sequence BIGINT NOT NULL,
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('scene.played', 'scene.o')),
    occurred_at TIMESTAMPTZ NOT NULL,
    origin TEXT NOT NULL,
    body_hash BYTEA NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, event_id)
);

CREATE TABLE IF NOT EXISTS current_preferences (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    rating DOUBLE PRECISION NOT NULL,
    client_id UUID NOT NULL,
    sequence BIGINT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (account_id, endpoint, stash_id)
);

CREATE TABLE IF NOT EXISTS source_configs (
    endpoint TEXT PRIMARY KEY,
    canonical_scene_url_template TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS source_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    captured_at TIMESTAMPTZ NOT NULL,
    source_updated_at TIMESTAMPTZ,
    snapshot JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (endpoint, stash_id)
);

CREATE TABLE IF NOT EXISTS source_scenes (
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    title TEXT,
    details TEXT,
    source_updated_at TIMESTAMPTZ,
    remote_images JSONB NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (endpoint, stash_id)
);

CREATE TABLE IF NOT EXISTS source_performers (
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    name TEXT NOT NULL,
    source_updated_at TIMESTAMPTZ,
    PRIMARY KEY (endpoint, stash_id)
);

CREATE TABLE IF NOT EXISTS source_studios (
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    name TEXT NOT NULL,
    source_updated_at TIMESTAMPTZ,
    PRIMARY KEY (endpoint, stash_id)
);

CREATE TABLE IF NOT EXISTS source_tags (
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    name TEXT NOT NULL,
    source_updated_at TIMESTAMPTZ,
    PRIMARY KEY (endpoint, stash_id)
);

CREATE TABLE IF NOT EXISTS source_scene_performers (
    scene_endpoint TEXT NOT NULL,
    scene_stash_id TEXT NOT NULL,
    performer_endpoint TEXT NOT NULL,
    performer_stash_id TEXT NOT NULL,
    PRIMARY KEY (scene_endpoint, scene_stash_id, performer_endpoint, performer_stash_id)
);

CREATE TABLE IF NOT EXISTS source_scene_tags (
    scene_endpoint TEXT NOT NULL,
    scene_stash_id TEXT NOT NULL,
    tag_endpoint TEXT NOT NULL,
    tag_stash_id TEXT NOT NULL,
    PRIMARY KEY (scene_endpoint, scene_stash_id, tag_endpoint, tag_stash_id)
);

CREATE TABLE IF NOT EXISTS model_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at TIMESTAMPTZ,
    active BOOLEAN NOT NULL DEFAULT false
);

CREATE UNIQUE INDEX IF NOT EXISTS model_versions_one_active
    ON model_versions ((active))
    WHERE active;

CREATE TABLE IF NOT EXISTS item_neighbors (
    model_version_id UUID NOT NULL REFERENCES model_versions(id) ON DELETE CASCADE,
    source_endpoint TEXT NOT NULL,
    source_stash_id TEXT NOT NULL,
    neighbor_endpoint TEXT NOT NULL,
    neighbor_stash_id TEXT NOT NULL,
    score DOUBLE PRECISION NOT NULL,
    reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (model_version_id, source_endpoint, source_stash_id, neighbor_endpoint, neighbor_stash_id)
);

CREATE TABLE IF NOT EXISTS user_recommendations (
    model_version_id UUID NOT NULL REFERENCES model_versions(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    source_endpoint TEXT NOT NULL,
    source_stash_id TEXT NOT NULL,
    score DOUBLE PRECISION NOT NULL,
    reasons JSONB NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (model_version_id, account_id, source_endpoint, source_stash_id)
);
