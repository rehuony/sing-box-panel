CREATE TABLE traffic_checkpoint (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    pid INTEGER NOT NULL CHECK (pid > 0),
    process_start_token TEXT NOT NULL CHECK (process_start_token <> ''),
    activation_bundle_id TEXT NOT NULL
        REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    last_upload_total INTEGER NOT NULL CHECK (last_upload_total >= 0),
    last_download_total INTEGER NOT NULL CHECK (last_download_total >= 0),
    sampled_at TEXT NOT NULL CHECK (sampled_at <> '')
) STRICT;
