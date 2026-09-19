-- Empty host delegates publication to panel settings / public-IP detection.
-- No tables reference subscription_channels; retain every existing row exactly.
CREATE TABLE subscription_channels_new (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    name TEXT NOT NULL UNIQUE CHECK (name <> ''),
    format TEXT NOT NULL CHECK (format IN ('sing-box', 'mihomo', 'loon')),
    config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json)),
    public_host TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

INSERT INTO subscription_channels_new
    (id, name, format, config_json, public_host, enabled, created_at, updated_at)
SELECT id, name, format, config_json, public_host, enabled, created_at, updated_at
FROM subscription_channels;
DROP TABLE subscription_channels;
ALTER TABLE subscription_channels_new RENAME TO subscription_channels;
