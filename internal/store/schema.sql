-- Current database schema. Initialization only; existing formats are never converted.

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

CREATE TABLE catalog_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    validator TEXT NOT NULL DEFAULT '',
    catalog_json TEXT NOT NULL CHECK (json_valid(catalog_json)),
    diagnostics_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(diagnostics_json)),
    refreshed_at TEXT NOT NULL CHECK (refreshed_at <> '')
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

CREATE TABLE subscription_sources (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    name TEXT NOT NULL UNIQUE CHECK (name <> ''),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('remote', 'local')),
    config_json TEXT NOT NULL CHECK (json_valid(config_json)),
    current_version_id TEXT,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

CREATE TABLE subscription_users (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    name TEXT NOT NULL UNIQUE CHECK (name <> ''),
    description TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

CREATE TABLE subscription_user_node_grants (
    user_id TEXT NOT NULL REFERENCES subscription_users(id) ON DELETE CASCADE,
    node_key TEXT NOT NULL CHECK (node_key <> ''),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    PRIMARY KEY (user_id, node_key)
) STRICT;

CREATE TABLE subscription_source_versions (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    source_id TEXT NOT NULL REFERENCES subscription_sources(id) ON DELETE CASCADE,
    format TEXT NOT NULL CHECK (format IN ('sing-box-json', 'mihomo-yaml', 'uri-list')),
    raw_body BLOB NOT NULL,
    normalized_nodes_json TEXT NOT NULL CHECK (json_valid(normalized_nodes_json)),
    diagnostics_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(diagnostics_json)),
    sha256 TEXT NOT NULL
        CHECK (length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'),
    fetched_at TEXT NOT NULL CHECK (fetched_at <> ''),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    UNIQUE(source_id, sha256)
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

CREATE TABLE traffic_periods (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    activation_bundle_id TEXT
        REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    period_start TEXT NOT NULL CHECK (period_start <> ''),
    period_end TEXT NOT NULL CHECK (period_end <> ''),
    inbound_bytes INTEGER NOT NULL DEFAULT 0 CHECK (inbound_bytes >= 0),
    outbound_bytes INTEGER NOT NULL DEFAULT 0 CHECK (outbound_bytes >= 0),
    counters_json TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(counters_json)),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    CHECK (period_end > period_start)
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

CREATE TABLE traffic_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    activation_bundle_id TEXT NOT NULL
        REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    pid INTEGER NOT NULL CHECK (pid > 0),
    process_start_token TEXT NOT NULL CHECK (process_start_token <> ''),
    sampled_at TEXT NOT NULL CHECK (sampled_at <> ''),
    memory_bytes INTEGER NOT NULL CHECK (memory_bytes >= 0),
    active_connections INTEGER NOT NULL CHECK (active_connections >= 0),
    upload_total INTEGER NOT NULL CHECK (upload_total >= 0),
    download_total INTEGER NOT NULL CHECK (download_total >= 0),
    upload_delta INTEGER CHECK (upload_delta IS NULL OR upload_delta >= 0),
    download_delta INTEGER CHECK (download_delta IS NULL OR download_delta >= 0),
    interval_start TEXT,
    interval_end TEXT,
    coverage TEXT NOT NULL DEFAULT 'partial'
        CHECK (coverage IN ('complete', 'partial')),
    accepted INTEGER NOT NULL CHECK (accepted IN (0, 1)),
    diagnostic_code TEXT NOT NULL DEFAULT '' CHECK (length(diagnostic_code) <= 128),
    CHECK (
        (interval_start IS NULL AND interval_end IS NULL)
        OR
        (interval_start IS NOT NULL AND interval_start <> ''
            AND interval_end IS NOT NULL AND interval_end > interval_start
            AND interval_end = sampled_at
            AND upload_delta IS NOT NULL AND download_delta IS NOT NULL)
    ),
    CHECK (accepted = 1 OR interval_start IS NULL),
    CHECK (coverage <> 'complete' OR interval_start IS NOT NULL)
) STRICT;

CREATE TABLE traffic_checkpoint (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    period_start TEXT NOT NULL CHECK (period_start <> ''),
    period_end TEXT NOT NULL CHECK (period_end <> ''),
    pid INTEGER NOT NULL CHECK (pid > 0),
    process_start_token TEXT NOT NULL CHECK (process_start_token <> ''),
    activation_bundle_id TEXT NOT NULL
        REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    last_upload_total INTEGER NOT NULL CHECK (last_upload_total >= 0),
    last_download_total INTEGER NOT NULL CHECK (last_download_total >= 0),
    accumulated_upload INTEGER NOT NULL CHECK (accumulated_upload >= 0),
    accumulated_download INTEGER NOT NULL CHECK (accumulated_download >= 0),
    has_delta INTEGER NOT NULL DEFAULT 0 CHECK (has_delta IN (0, 1)),
    sampled_at TEXT NOT NULL CHECK (sampled_at <> ''),
    CHECK (period_end > period_start)
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

CREATE TABLE configuration_file (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    revision INTEGER NOT NULL CHECK (revision > 0),
    content TEXT NOT NULL CHECK (length(CAST(content AS BLOB)) <= 2097152),
    canonical_revision_id TEXT REFERENCES canonical_revisions(id) ON DELETE RESTRICT,
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

CREATE TABLE "subscription_tokens" (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    user_id TEXT REFERENCES subscription_users(id) ON DELETE RESTRICT,
    download_limit INTEGER CHECK (download_limit BETWEEN 1 AND 1000000000),
    label TEXT NOT NULL CHECK (label <> ''),
    token_sha256 TEXT NOT NULL UNIQUE
        CHECK (length(token_sha256) = 64 AND token_sha256 NOT GLOB '*[^0-9a-f]*'),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    expires_at TEXT,
    revoked_at TEXT,
    successful_request_count INTEGER NOT NULL DEFAULT 0 CHECK (successful_request_count >= 0),
    body_response_count INTEGER NOT NULL DEFAULT 0 CHECK (body_response_count >= 0),
    bytes_served INTEGER NOT NULL DEFAULT 0 CHECK (bytes_served >= 0),
    last_used_at TEXT,
    created_at TEXT NOT NULL CHECK (created_at <> '')
) STRICT;

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

CREATE TABLE "subscription_channels" (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    name TEXT NOT NULL UNIQUE CHECK (name <> ''),
    format TEXT NOT NULL CHECK (format IN ('sing-box', 'mihomo', 'loon')),
    config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json)),
    public_host TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

CREATE TABLE panel_settings_file_commits (
    settings_path TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL UNIQUE
) STRICT;

CREATE TABLE data_directory_moves (
    id TEXT PRIMARY KEY,
    source_path TEXT NOT NULL,
    target_path TEXT NOT NULL
) STRICT;

CREATE TABLE subscription_token_secrets (
    token_id TEXT PRIMARY KEY REFERENCES subscription_tokens(id) ON DELETE CASCADE,
    secret TEXT NOT NULL CHECK (length(secret) BETWEEN 1 AND 512)
);

CREATE TABLE runtime_recovery (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),
 generation INTEGER NOT NULL CHECK(generation>0),
 metadata_json TEXT NOT NULL CHECK(json_valid(metadata_json)),
 next_attempt_at TEXT,
 succeeded INTEGER NOT NULL DEFAULT 0 CHECK(succeeded IN (0,1))
) STRICT;

CREATE TABLE subscription_refresh_schedule (
 source_id TEXT PRIMARY KEY REFERENCES subscription_sources(id) ON DELETE CASCADE,
 expected_updated_at TEXT NOT NULL,
 next_at TEXT NOT NULL
) STRICT;

CREATE TABLE "log_entries" (
 id TEXT PRIMARY KEY CHECK(id<>'' AND length(id)<=80),
 occurred_at TEXT NOT NULL CHECK(occurred_at<>''),
 source TEXT NOT NULL CHECK(source IN ('panel','core','security')),
 level TEXT NOT NULL CHECK(level IN ('trace','debug','info','warn','error','fatal')),
 code TEXT NOT NULL CHECK(code<>'' AND length(code)<=128),
 message TEXT NOT NULL CHECK(message<>'' AND length(message)<=4096),
 metadata_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(metadata_json) AND json_type(metadata_json)='object' AND length(metadata_json)<=16384)
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

CREATE INDEX subscription_source_versions_source_created
    ON subscription_source_versions(source_id, created_at DESC, id DESC);

CREATE INDEX traffic_periods_range
    ON traffic_periods(period_start, period_end);

CREATE INDEX traffic_samples_timeline
    ON traffic_samples(sampled_at DESC, id DESC);

CREATE INDEX traffic_samples_bundle_timeline
    ON traffic_samples(activation_bundle_id, sampled_at DESC, id DESC);

CREATE INDEX traffic_samples_history
    ON traffic_samples(sampled_at, id);

CREATE INDEX traffic_samples_bundle_history
    ON traffic_samples(activation_bundle_id, sampled_at, id);

CREATE INDEX traffic_samples_interval_history
    ON traffic_samples(interval_start, interval_end, id)
    WHERE interval_start IS NOT NULL;

CREATE INDEX traffic_periods_page
    ON traffic_periods(period_start DESC, id DESC);

CREATE INDEX runtime_transitions_timeline
    ON runtime_transitions(occurred_at DESC, id DESC);

CREATE INDEX runtime_transitions_state_timeline
    ON runtime_transitions(state, occurred_at DESC, id DESC);

CREATE INDEX runtime_transitions_reason_timeline
    ON runtime_transitions(reason, occurred_at DESC, id DESC);

CREATE INDEX runtime_transitions_bundle_timeline
    ON runtime_transitions(activation_bundle_id, occurred_at DESC, id DESC);

CREATE INDEX subscription_tokens_user_created
    ON subscription_tokens(user_id, created_at DESC, id DESC);

CREATE INDEX log_entries_timeline ON log_entries(occurred_at DESC,id DESC);

CREATE INDEX log_entries_source_timeline ON log_entries(source,occurred_at DESC,id DESC);

CREATE INDEX log_entries_level_timeline ON log_entries(level,occurred_at DESC,id DESC);

CREATE TRIGGER subscription_sources_current_version_same_source
BEFORE UPDATE OF current_version_id ON subscription_sources
WHEN NEW.current_version_id IS NOT NULL
BEGIN
    SELECT CASE WHEN NOT EXISTS (
        SELECT 1
          FROM subscription_source_versions AS version
         WHERE version.id = NEW.current_version_id
           AND version.source_id = NEW.id
    ) THEN RAISE(ABORT, 'subscription current version belongs to another source') END;
END;

CREATE TRIGGER subscription_sources_initial_version_same_source
BEFORE INSERT ON subscription_sources
WHEN NEW.current_version_id IS NOT NULL
BEGIN
    SELECT CASE WHEN NOT EXISTS (
        SELECT 1
          FROM subscription_source_versions AS version
         WHERE version.id = NEW.current_version_id
           AND version.source_id = NEW.id
    ) THEN RAISE(ABORT, 'subscription current version belongs to another source') END;
END;

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
