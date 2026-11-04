-- Node database schema baseline. Earlier storage formats are rejected, never converted.
--
-- canonical_revisions holds the configurations the panel sent; their IDs are the
-- panel's revision IDs and hub_state.head_revision_id is the latest one received.

CREATE TABLE canonical_revisions (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    sequence INTEGER NOT NULL UNIQUE CHECK (sequence > 0),
    parent_id TEXT REFERENCES canonical_revisions(id) ON DELETE RESTRICT,
    schema_version INTEGER NOT NULL CHECK (schema_version = 1),
    document_json TEXT NOT NULL CHECK (
        json_valid(document_json)
        AND json_type(document_json, '$') = 'object'
    ),
    sha256 TEXT NOT NULL
        CHECK (length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'),
    command_id TEXT NOT NULL UNIQUE CHECK (command_id <> ''),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    CHECK (parent_id IS NULL OR parent_id <> id)
) STRICT;

CREATE TABLE core_artifacts (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    exact_version TEXT NOT NULL CHECK (exact_version <> ''),
    operating_system TEXT NOT NULL CHECK (operating_system <> ''),
    architecture TEXT NOT NULL CHECK (architecture <> ''),
    variant TEXT NOT NULL DEFAULT 'musl' CHECK (variant = 'musl'),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('official', 'user_verified')),
    user_source TEXT,
    repository_id TEXT,
    release_id TEXT,
    asset_id TEXT,
    archive_sha256 TEXT NOT NULL
        CHECK (length(archive_sha256) = 64 AND archive_sha256 NOT GLOB '*[^0-9a-f]*'),
    binary_sha256 TEXT NOT NULL
        CHECK (length(binary_sha256) = 64 AND binary_sha256 NOT GLOB '*[^0-9a-f]*'),
    binary_path TEXT NOT NULL CHECK (binary_path <> ''),
    reported_version TEXT NOT NULL CHECK (reported_version <> ''),
    feature_fingerprint_json TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(feature_fingerprint_json)),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    CHECK (
        (source_kind = 'official'
            AND user_source IS NULL
            AND repository_id IS NOT NULL AND release_id IS NOT NULL AND asset_id IS NOT NULL)
        OR
        (source_kind = 'user_verified'
            AND user_source IS NOT NULL AND user_source <> ''
            AND repository_id IS NULL AND release_id IS NULL AND asset_id IS NULL)
    )
) STRICT;

CREATE TABLE startup_artifacts (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    canonical_revision_id TEXT NOT NULL
        REFERENCES canonical_revisions(id) ON DELETE RESTRICT,
    exact_core_version TEXT NOT NULL CHECK (exact_core_version <> ''),
    core_artifact_id TEXT NOT NULL
        REFERENCES core_artifacts(id) ON DELETE RESTRICT,
    config_bytes BLOB NOT NULL,
    config_sha256 TEXT NOT NULL
        CHECK (length(config_sha256) = 64 AND config_sha256 NOT GLOB '*[^0-9a-f]*'),
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'ready', 'failed')),
    checked_at TEXT,
    created_at TEXT NOT NULL CHECK (created_at <> '')
) STRICT;

CREATE TABLE activation_bundles (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    startup_artifact_id TEXT NOT NULL
        REFERENCES startup_artifacts(id) ON DELETE RESTRICT,
    monitoring_tier TEXT NOT NULL
        CHECK (monitoring_tier IN ('limited', 'process_only')),
    sha256 TEXT NOT NULL UNIQUE
        CHECK (length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'),
    created_at TEXT NOT NULL CHECK (created_at <> '')
) STRICT;

CREATE TABLE hub_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    head_revision_id TEXT REFERENCES canonical_revisions(id) ON DELETE RESTRICT,
    desired_bundle_id TEXT REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    applied_bundle_id TEXT REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    rollback_bundle_id TEXT REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    applied_at TEXT,
    target_generation INTEGER NOT NULL DEFAULT 0 CHECK (target_generation >= 0),
    desired_running INTEGER NOT NULL DEFAULT 0 CHECK (desired_running IN (0, 1)),
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

CREATE TABLE runtime_observation (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    pid INTEGER NOT NULL CHECK (pid > 0),
    process_start_token TEXT NOT NULL CHECK (process_start_token <> ''),
    core_artifact_id TEXT NOT NULL
        REFERENCES core_artifacts(id) ON DELETE RESTRICT,
    activation_bundle_id TEXT NOT NULL
        REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    exact_core_version TEXT NOT NULL CHECK (exact_core_version <> ''),
    archive_sha256 TEXT NOT NULL
        CHECK (length(archive_sha256) = 64 AND archive_sha256 NOT GLOB '*[^0-9a-f]*'),
    binary_sha256 TEXT NOT NULL
        CHECK (length(binary_sha256) = 64 AND binary_sha256 NOT GLOB '*[^0-9a-f]*'),
    started_at TEXT NOT NULL CHECK (started_at <> ''),
    observed_at TEXT NOT NULL CHECK (observed_at <> ''),
    stable_observed_at TEXT
) STRICT;

CREATE TABLE runtime_transitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    dedupe_key TEXT NOT NULL UNIQUE
        CHECK (dedupe_key <> '' AND length(dedupe_key) <= 256),
    state TEXT NOT NULL
        CHECK (state IN ('running', 'stopped', 'failed', 'unknown')),
    reason TEXT NOT NULL
        CHECK (reason <> '' AND length(reason) <= 128),
    activation_bundle_id TEXT
        REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    generation INTEGER CHECK (generation IS NULL OR generation >= 0),
    pid INTEGER CHECK (pid IS NULL OR pid > 0),
    process_start_token TEXT,
    process_started_at TEXT,
    occurred_at TEXT NOT NULL CHECK (occurred_at <> ''),
    uncertain_since TEXT,
    CHECK (
        (pid IS NULL AND process_start_token IS NULL AND process_started_at IS NULL)
        OR
        (pid IS NOT NULL AND process_start_token IS NOT NULL AND process_start_token <> ''
            AND process_started_at IS NOT NULL AND process_started_at <> '')
    ),
    CHECK (state <> 'running' OR (activation_bundle_id IS NOT NULL AND pid IS NOT NULL)),
    CHECK (state <> 'unknown' OR uncertain_since IS NOT NULL),
    CHECK (uncertain_since IS NULL OR uncertain_since <= occurred_at)
) STRICT;

CREATE TABLE runtime_recovery (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 generation INTEGER NOT NULL CHECK(generation>0),
 metadata_json TEXT NOT NULL CHECK(json_valid(metadata_json)),
 next_attempt_at TEXT,
 succeeded INTEGER NOT NULL DEFAULT 0 CHECK(succeeded IN (0,1))
) STRICT;

CREATE UNIQUE INDEX core_artifacts_official_asset
    ON core_artifacts(repository_id, release_id, asset_id)
    WHERE repository_id IS NOT NULL AND release_id IS NOT NULL AND asset_id IS NOT NULL;

CREATE INDEX core_artifacts_version_lookup
    ON core_artifacts(exact_version, operating_system, architecture, variant);

CREATE INDEX core_artifacts_digest_lookup
    ON core_artifacts(archive_sha256);

CREATE INDEX core_artifacts_binary_digest_lookup
    ON core_artifacts(binary_sha256);

CREATE INDEX startup_artifacts_configuration_lookup
    ON startup_artifacts(canonical_revision_id, exact_core_version, core_artifact_id, state);

CREATE INDEX runtime_transitions_timeline
    ON runtime_transitions(occurred_at DESC, id DESC);

CREATE INDEX runtime_transitions_state_timeline
    ON runtime_transitions(state, occurred_at DESC, id DESC);

CREATE INDEX runtime_transitions_reason_timeline
    ON runtime_transitions(reason, occurred_at DESC, id DESC);

CREATE INDEX runtime_transitions_bundle_timeline
    ON runtime_transitions(activation_bundle_id, occurred_at DESC, id DESC);

CREATE TRIGGER runtime_transitions_reject_update
BEFORE UPDATE ON runtime_transitions
BEGIN
    SELECT RAISE(ABORT, 'runtime transitions are append-only');
END;

CREATE TRIGGER runtime_transitions_reject_delete
BEFORE DELETE ON runtime_transitions
BEGIN
    SELECT RAISE(ABORT, 'runtime transitions are append-only');
END;

INSERT INTO hub_state(singleton, updated_at)
VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

INSERT INTO runtime_transitions(
    dedupe_key, state, reason, occurred_at, uncertain_since
) VALUES (
    'history_initialized',
    'unknown',
    'history_initialized',
    strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now'),
    strftime('%Y-%m-%dT%H:%M:%f000000Z', 'now')
);

CREATE TABLE command_results (
    request_id TEXT PRIMARY KEY CHECK (length(request_id) BETWEEN 1 AND 256),
    result BLOB NOT NULL,
    completed_at TEXT NOT NULL CHECK (completed_at <> '')
) STRICT;

CREATE INDEX command_results_completed ON command_results(completed_at);
