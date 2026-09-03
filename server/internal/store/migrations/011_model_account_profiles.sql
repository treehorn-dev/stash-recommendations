CREATE TABLE IF NOT EXISTS model_account_profiles (
    model_version_id UUID NOT NULL REFERENCES model_versions(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    embedding public.vector(256) NOT NULL,
    reasons JSONB NOT NULL,
    PRIMARY KEY (model_version_id, account_id)
);
