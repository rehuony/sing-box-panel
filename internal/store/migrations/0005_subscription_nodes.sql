-- Publication controls never mutate core inbounds or legacy grant keys.
CREATE TABLE subscription_manual_nodes (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    revision INTEGER NOT NULL CHECK (revision > 0),
    outbound_json TEXT NOT NULL CHECK (json_valid(outbound_json) AND json_type(outbound_json) = 'object'),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
) STRICT;
CREATE TABLE subscription_node_visibility (
    publication_id TEXT PRIMARY KEY CHECK (publication_id <> ''),
    hidden INTEGER NOT NULL CHECK (hidden IN (0, 1)),
    revision INTEGER NOT NULL CHECK (revision > 0)
) STRICT;
