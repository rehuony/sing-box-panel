-- Recovery markers only. Panel values are migrated out of panel_settings by
-- the server once its selected settings file is known and writable.
CREATE TABLE panel_settings_file_commits (
    settings_path TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL UNIQUE
) STRICT;

-- Completed path rebases identify a recoverable data relocation snapshot.
CREATE TABLE data_directory_moves (
    id TEXT PRIMARY KEY,
    source_path TEXT NOT NULL,
    target_path TEXT NOT NULL
) STRICT;
