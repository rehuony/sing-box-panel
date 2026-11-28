-- Node database, migration 3: removed cores.
--
-- A removed core keeps its row, so the startup records, activation bundles and
-- append-only runtime history that name it keep their references. Its files
-- are deleted, every operation treats it as not installed, and installing it
-- again clears the mark.
ALTER TABLE core_artifacts ADD COLUMN removed_at TEXT CHECK (removed_at IS NULL OR removed_at <> '');
