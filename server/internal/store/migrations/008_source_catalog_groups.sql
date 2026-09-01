CREATE TABLE IF NOT EXISTS source_groups (
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    name TEXT NOT NULL,
    source_updated_at TIMESTAMPTZ,
    PRIMARY KEY (endpoint, stash_id)
);

CREATE TABLE IF NOT EXISTS source_scene_groups (
    scene_endpoint TEXT NOT NULL,
    scene_stash_id TEXT NOT NULL,
    group_endpoint TEXT NOT NULL,
    group_stash_id TEXT NOT NULL,
    group_order INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (scene_endpoint, scene_stash_id, group_endpoint, group_stash_id)
);
