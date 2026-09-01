CREATE TABLE IF NOT EXISTS session_projections (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    projection_version BIGINT NOT NULL CHECK (projection_version >= 1),
    projection_type TEXT NOT NULL CHECK (projection_type IN ('latency', 'o_bounded')),
    rebuilt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, projection_version, projection_type)
);

CREATE TABLE IF NOT EXISTS session_projection_items (
    account_id UUID NOT NULL,
    projection_version BIGINT NOT NULL,
    projection_type TEXT NOT NULL CHECK (projection_type IN ('latency', 'o_bounded')),
    session_order INTEGER NOT NULL CHECK (session_order >= 1),
    item_order INTEGER NOT NULL CHECK (item_order >= 1),
    event_id UUID NOT NULL,
    endpoint TEXT NOT NULL,
    stash_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('scene.played', 'scene.o')),
    occurred_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (account_id, projection_version, projection_type, session_order, item_order),
    FOREIGN KEY (account_id, projection_version, projection_type)
        REFERENCES session_projections (account_id, projection_version, projection_type)
        ON DELETE CASCADE
);
