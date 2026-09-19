-- Existing keys retain their user/grant scope. New keys may use channel policy
-- without a user. Never migrate legacy user IDs to NULL.
CREATE TABLE subscription_keys_new (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    user_id TEXT REFERENCES subscription_users(id) ON DELETE RESTRICT,
    download_limit INTEGER CHECK (download_limit BETWEEN 1 AND 1000000000),
    label TEXT NOT NULL CHECK (label <> ''),
    token_sha256 TEXT NOT NULL UNIQUE
        CHECK (length(token_sha256) = 64 AND token_sha256 NOT GLOB '*[^0-9a-f]*'),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    expires_at TEXT,
    revoked_at TEXT,
    successful_request_count INTEGER NOT NULL DEFAULT 0 CHECK (successful_request_count >= 0),
    body_response_count INTEGER NOT NULL DEFAULT 0 CHECK (body_response_count >= 0),
    bytes_served INTEGER NOT NULL DEFAULT 0 CHECK (bytes_served >= 0),
    last_used_at TEXT,
    created_at TEXT NOT NULL CHECK (created_at <> '')
) STRICT;

INSERT INTO subscription_keys_new (
    id, user_id, label, token_sha256, enabled, expires_at, revoked_at,
    successful_request_count, body_response_count, bytes_served, last_used_at, created_at
) SELECT id, user_id, label, token_sha256, enabled, expires_at, revoked_at,
    successful_request_count, body_response_count, bytes_served, last_used_at, created_at
  FROM subscription_tokens;
DROP TABLE subscription_tokens;
ALTER TABLE subscription_keys_new RENAME TO subscription_tokens;
CREATE INDEX subscription_tokens_user_created
    ON subscription_tokens(user_id, created_at DESC, id DESC);
