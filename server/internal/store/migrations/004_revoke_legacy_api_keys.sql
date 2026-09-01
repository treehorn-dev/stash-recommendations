ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;

UPDATE api_keys
SET revoked_at = now()
WHERE legacy_key AND revoked_at IS NULL;
