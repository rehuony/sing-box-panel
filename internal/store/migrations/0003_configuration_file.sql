-- The editable file may contain invalid JSON. Canonical revisions remain valid,
-- immutable snapshots used by startup artifacts and legacy public APIs.
CREATE TABLE configuration_file (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    revision INTEGER NOT NULL CHECK (revision > 0),
    content TEXT NOT NULL CHECK (length(CAST(content AS BLOB)) <= 2097152),
    canonical_revision_id TEXT REFERENCES canonical_revisions(id) ON DELETE RESTRICT,
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

INSERT INTO configuration_file(singleton, revision, content, canonical_revision_id, updated_at)
SELECT 1, 1, document_json, id, created_at FROM canonical_revisions
WHERE id = (SELECT head_revision_id FROM hub_state WHERE singleton = 1);
