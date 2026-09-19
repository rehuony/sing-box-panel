CREATE TABLE panel_settings (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    revision INTEGER NOT NULL CHECK (revision > 0),
    document TEXT NOT NULL CHECK (json_valid(document))
) STRICT;
