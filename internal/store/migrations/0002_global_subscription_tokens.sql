CREATE TABLE subscription_tokens_v2 (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    token_sha256 TEXT NOT NULL UNIQUE
        CHECK (length(token_sha256) = 64 AND token_sha256 NOT GLOB '*[^0-9a-f]*'),
    expires_at TEXT,
    revoked_at TEXT,
    created_at TEXT NOT NULL CHECK (created_at <> '')
) STRICT;

INSERT INTO subscription_tokens_v2(
    id, token_sha256, expires_at, revoked_at, created_at
)
SELECT id, token_sha256, expires_at, revoked_at, created_at
  FROM subscription_tokens;

DROP TABLE subscription_tokens;
ALTER TABLE subscription_tokens_v2 RENAME TO subscription_tokens;
