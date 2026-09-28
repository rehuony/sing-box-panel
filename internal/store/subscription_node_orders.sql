CREATE TABLE subscription_node_orders (
    collection_id TEXT PRIMARY KEY CHECK (collection_id <> ''),
    node_ids_json TEXT NOT NULL CHECK (json_valid(node_ids_json) AND json_type(node_ids_json) = 'array'),
    revision INTEGER NOT NULL CHECK (revision > 0)
) STRICT;

CREATE TRIGGER subscription_source_delete_node_order AFTER DELETE ON subscription_sources
BEGIN
    DELETE FROM subscription_node_orders WHERE collection_id = OLD.id;
END;
