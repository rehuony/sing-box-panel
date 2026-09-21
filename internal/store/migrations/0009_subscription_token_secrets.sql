CREATE TABLE subscription_token_secrets (
    token_id TEXT PRIMARY KEY REFERENCES subscription_tokens(id) ON DELETE CASCADE,
    secret TEXT NOT NULL CHECK (length(secret) BETWEEN 1 AND 512)
);
