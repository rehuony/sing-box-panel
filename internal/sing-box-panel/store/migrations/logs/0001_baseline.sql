-- The panel's log, kept in its own database file so that clearing it can
-- shrink the file without rewriting the main database.

-- The panel's log: its operations, security events and core events, newest first.
CREATE TABLE log_entries (
    id TEXT PRIMARY KEY CHECK (id <> '' AND length(id) <= 80),
    occurred_at TEXT NOT NULL CHECK (occurred_at <> ''),
    source TEXT NOT NULL CHECK (source IN ('panel', 'core', 'security')),
    level TEXT NOT NULL CHECK (level IN ('trace', 'debug', 'info', 'warn', 'error', 'fatal')),
    code TEXT NOT NULL CHECK (code <> '' AND length(code) <= 128),
    message TEXT NOT NULL CHECK (message <> '' AND length(message) <= 4096),
    metadata_json TEXT NOT NULL DEFAULT '{}' CHECK (
        json_valid(metadata_json) AND json_type(metadata_json) = 'object' AND length(metadata_json) <= 16384
    )
) STRICT;

CREATE INDEX log_entries_timeline ON log_entries(occurred_at DESC, id DESC);
CREATE INDEX log_entries_source_timeline ON log_entries(source, occurred_at DESC, id DESC);
CREATE INDEX log_entries_level_timeline ON log_entries(level, occurred_at DESC, id DESC);
