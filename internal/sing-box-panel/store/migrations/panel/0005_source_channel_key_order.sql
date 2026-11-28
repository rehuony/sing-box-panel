-- Sources, channels and keys each take a place in their list's order, which
-- starts as the order they were listed in: sources by name, ignoring case,
-- and channels and keys newest first.
ALTER TABLE subscription_sources ADD COLUMN position INTEGER NOT NULL DEFAULT 0
    CHECK (position >= 0);
ALTER TABLE subscription_channels ADD COLUMN position INTEGER NOT NULL DEFAULT 0
    CHECK (position >= 0);
ALTER TABLE subscription_tokens ADD COLUMN position INTEGER NOT NULL DEFAULT 0
    CHECK (position >= 0);

UPDATE subscription_sources SET position = (
    SELECT count(*) FROM subscription_sources AS other
    WHERE other.name < subscription_sources.name COLLATE NOCASE
       OR (other.name = subscription_sources.name COLLATE NOCASE AND other.id < subscription_sources.id)
);

UPDATE subscription_channels SET position = (
    SELECT count(*) FROM subscription_channels AS other
    WHERE other.created_at > subscription_channels.created_at
       OR (other.created_at = subscription_channels.created_at AND other.id > subscription_channels.id)
);

UPDATE subscription_tokens SET position = (
    SELECT count(*) FROM subscription_tokens AS other
    WHERE other.created_at > subscription_tokens.created_at
       OR (other.created_at = subscription_tokens.created_at AND other.id > subscription_tokens.id)
);

CREATE INDEX subscription_sources_order ON subscription_sources(position, id);
CREATE INDEX subscription_channels_order ON subscription_channels(position, id);
CREATE INDEX subscription_tokens_order ON subscription_tokens(position, id);
