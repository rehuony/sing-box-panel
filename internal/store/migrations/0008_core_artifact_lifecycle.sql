-- Artifact availability no longer depends on a separate trust lifecycle.
-- Existing identities and runtime references remain unchanged.
ALTER TABLE core_artifacts DROP COLUMN verification_state;
