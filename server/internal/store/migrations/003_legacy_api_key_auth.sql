ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS legacy_key BOOLEAN NOT NULL DEFAULT false;

UPDATE api_keys
SET legacy_key = true
WHERE left(key_id, 7) = 'legacy_';

CREATE INDEX IF NOT EXISTS api_keys_legacy_key_candidates
ON api_keys (id)
WHERE legacy_key;
