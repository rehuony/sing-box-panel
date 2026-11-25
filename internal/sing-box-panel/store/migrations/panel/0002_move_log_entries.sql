-- Log entries move to the log database, logs.db. Their table is set aside
-- here; when the store opens it copies the rows it holds to the log database
-- and then drops it, so an interrupted copy is repeated on the next start.
DROP INDEX log_entries_timeline;
DROP INDEX log_entries_source_timeline;
DROP INDEX log_entries_level_timeline;
ALTER TABLE log_entries RENAME TO log_entries_to_move;
