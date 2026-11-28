-- An entry is about the node its metadata names by node_id; an entry that
-- names none is the panel's own, with an empty node_id. The column is
-- virtual, so the rows keep their size and the migration rewrites none of
-- them; its index holds the value and serves one scope's entries newest first
-- and their count without reading the metadata.
ALTER TABLE log_entries ADD COLUMN node_id TEXT
    GENERATED ALWAYS AS (coalesce(json_extract(metadata_json, '$.node_id'), '')) VIRTUAL;

CREATE INDEX log_entries_node_timeline ON log_entries(node_id, occurred_at DESC, id DESC);
