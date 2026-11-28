-- Node database, migration 2: the core log retention the panel set.
--
-- The node keeps the policy it last received, so one that restarts while its
-- panel is unreachable keeps the panel's retention; without a row it uses its
-- built-in default.
CREATE TABLE core_log_policy (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    retention_days INTEGER NOT NULL CHECK (retention_days BETWEEN 1 AND 3650),
    max_files INTEGER NOT NULL CHECK (max_files BETWEEN 0 AND 1024),
    max_file_bytes INTEGER NOT NULL CHECK (max_file_bytes BETWEEN 1048576 AND 1073741824),
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;
