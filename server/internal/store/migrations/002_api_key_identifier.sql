ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS key_id TEXT;

UPDATE api_keys
SET key_id = 'legacy_' || replace(id::text, '-', '')
WHERE key_id IS NULL;

ALTER TABLE api_keys ALTER COLUMN key_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS api_keys_key_id_unique ON api_keys (key_id);
