-- A rule set takes the key of an icon the Web application draws, and a place
-- in its client's order, which starts as the name order it was listed in.
ALTER TABLE subscription_rule_sets ADD COLUMN icon TEXT NOT NULL DEFAULT 'layers'
    CHECK (length(icon) BETWEEN 1 AND 32);
ALTER TABLE subscription_rule_sets ADD COLUMN position INTEGER NOT NULL DEFAULT 0
    CHECK (position >= 0);

UPDATE subscription_rule_sets SET position = (
    SELECT count(*) FROM subscription_rule_sets AS other
    WHERE other.client = subscription_rule_sets.client
      AND (other.name < subscription_rule_sets.name
        OR (other.name = subscription_rule_sets.name AND other.id < subscription_rule_sets.id))
);

CREATE INDEX subscription_rule_sets_order ON subscription_rule_sets(client, position, id);
