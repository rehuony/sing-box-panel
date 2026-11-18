-- Database schema baseline. Earlier storage formats are rejected, never converted.

CREATE TABLE canonical_revisions (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    sequence INTEGER NOT NULL CHECK (sequence > 0),
    parent_id TEXT REFERENCES canonical_revisions(id) ON DELETE CASCADE,
    schema_version INTEGER NOT NULL CHECK (schema_version = 1),
    document_json TEXT NOT NULL CHECK (
        json_valid(document_json)
        AND json_type(document_json, '$') = 'object'
    ),
    sha256 TEXT NOT NULL
        CHECK (length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'),
    command_id TEXT NOT NULL UNIQUE CHECK (command_id <> ''),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    UNIQUE (node_id, sequence),
    CHECK (parent_id IS NULL OR parent_id <> id)
) STRICT;

CREATE TABLE node_configurations (
    node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    revision INTEGER NOT NULL CHECK (revision > 0),
    content TEXT NOT NULL CHECK (length(CAST(content AS BLOB)) <= 2097152),
    -- The revision of the current text; null while the text does not parse.
    canonical_revision_id TEXT REFERENCES canonical_revisions(id) ON DELETE CASCADE,
    -- The latest revision that parsed.
    head_revision_id TEXT REFERENCES canonical_revisions(id) ON DELETE CASCADE,
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

CREATE TABLE catalog_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    validator TEXT NOT NULL DEFAULT '',
    catalog_json TEXT NOT NULL CHECK (json_valid(catalog_json)),
    diagnostics_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(diagnostics_json)),
    refreshed_at TEXT NOT NULL CHECK (refreshed_at <> '')
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

CREATE INDEX subscription_source_versions_source_created
    ON subscription_source_versions(source_id, created_at DESC, id DESC);

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

CREATE TABLE subscription_node_orders (
    collection_id TEXT PRIMARY KEY CHECK (collection_id <> ''),
    node_ids_json TEXT NOT NULL CHECK (json_valid(node_ids_json) AND json_type(node_ids_json) = 'array'),
    revision INTEGER NOT NULL CHECK (revision > 0)
) STRICT;

CREATE TRIGGER subscription_source_delete_node_order AFTER DELETE ON subscription_sources
BEGIN
    DELETE FROM subscription_node_orders WHERE collection_id = OLD.id;
END;

CREATE TABLE auth_sessions (
    token_hash BLOB PRIMARY KEY CHECK (length(token_hash) = 32),
    csrf_token TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    credential_hash BLOB NOT NULL CHECK (length(credential_hash) = 32)
) STRICT;

CREATE INDEX auth_sessions_expiry ON auth_sessions(expires_at);

CREATE TABLE fleet_identity (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    panel_id TEXT NOT NULL CHECK (length(panel_id) BETWEEN 1 AND 256)
) STRICT;

CREATE TABLE nodes (
    id TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 256),
    name TEXT NOT NULL UNIQUE COLLATE NOCASE CHECK (length(name) BETWEEN 1 AND 256),
    created_at TEXT NOT NULL,
    public_key BLOB NOT NULL CHECK (length(public_key) = 32),
    enrolled_at TEXT NOT NULL,
    hostname TEXT NOT NULL DEFAULT '' CHECK (length(hostname) <= 256),
    os TEXT NOT NULL DEFAULT '' CHECK (length(os) <= 64),
    arch TEXT NOT NULL DEFAULT '' CHECK (length(arch) <= 64),
    build_version TEXT NOT NULL DEFAULT '' CHECK (length(build_version) <= 256),
    protocol_minor INTEGER NOT NULL DEFAULT 0 CHECK (protocol_minor >= 0),
    last_seen_at TEXT,
    session_id TEXT CHECK (session_id IS NULL OR length(session_id) BETWEEN 1 AND 256),
    instance_id TEXT CHECK (instance_id IS NULL OR length(instance_id) BETWEEN 1 AND 64),
    -- The node's own traffic quota and period: a null quota is unlimited, and
    -- each period of traffic_period_months starts on traffic_reset_day (UTC).
    traffic_quota_gib INTEGER CHECK (traffic_quota_gib IS NULL OR traffic_quota_gib BETWEEN 1 AND 8589934591),
    traffic_period_months INTEGER NOT NULL DEFAULT 1 CHECK (traffic_period_months BETWEEN 1 AND 120),
    traffic_reset_day INTEGER NOT NULL DEFAULT 1 CHECK (traffic_reset_day BETWEEN 1 AND 28),
    -- The host subscriptions publish for the node's inbounds; empty uses
    -- the public IP the node reports.
    public_host TEXT NOT NULL DEFAULT '' CHECK (length(public_host) <= 253),
    -- The node's tags: a sorted JSON array of distinct names.
    tags TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(tags) AND json_type(tags) = 'array'),
    settings_revision INTEGER NOT NULL DEFAULT 1 CHECK (settings_revision >= 1),
    -- What the node last reported running, kept beside the encoded report so
    -- subscriptions can find the applied revision in the same read.
    applied_bundle_id TEXT,
    applied_revision_id TEXT,
    applied_config_sha256 TEXT,
    applied_core_version TEXT,
    reported_public_ip TEXT NOT NULL DEFAULT '' CHECK (length(reported_public_ip) <= 64),
    state_report BLOB,
    reported_at TEXT,
    CHECK ((state_report IS NULL) = (reported_at IS NULL))
) STRICT;

-- Unused one-time enrollment tokens. A token with a name creates node node_id
-- with that name when a host enrolls with it; one without enrolls the existing
-- node again. Expired tokens are never accepted.
CREATE TABLE node_enrollments (
    token_hash BLOB PRIMARY KEY CHECK (length(token_hash) = 32),
    node_id TEXT NOT NULL UNIQUE CHECK (length(node_id) BETWEEN 1 AND 256),
    name TEXT UNIQUE COLLATE NOCASE CHECK (name IS NULL OR length(name) BETWEEN 1 AND 256),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    expires_at TEXT NOT NULL CHECK (expires_at > created_at)
) STRICT;

CREATE TABLE cached_artifacts (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('official', 'user_verified')),
    repository_id INTEGER,
    release_id INTEGER,
    asset_id INTEGER,
    user_source TEXT,
    sha256 TEXT NOT NULL CHECK (length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'),
    size INTEGER NOT NULL CHECK (size > 0),
    os TEXT NOT NULL CHECK (os <> ''),
    arch TEXT NOT NULL CHECK (arch <> ''),
    variant TEXT NOT NULL CHECK (variant <> ''),
    exact_version TEXT NOT NULL CHECK (exact_version <> ''),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    -- When an install last asked for the archive.
    used_at TEXT NOT NULL CHECK (used_at >= created_at),
    CHECK (
        (source_kind = 'official' AND user_source IS NULL
            AND repository_id IS NOT NULL AND release_id IS NOT NULL AND asset_id IS NOT NULL)
        OR
        (source_kind = 'user_verified' AND user_source IS NOT NULL AND user_source <> ''
            AND repository_id IS NULL AND release_id IS NULL AND asset_id IS NULL)
    )
) STRICT;

-- Each node's runtime history, replicated from the node's own append-only
-- history: source_id is the entry's ID in the node instance's database.
CREATE TABLE runtime_transitions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    instance_id TEXT NOT NULL CHECK (length(instance_id) BETWEEN 1 AND 64),
    source_id INTEGER NOT NULL CHECK (source_id > 0),
    state TEXT NOT NULL
        CHECK (state IN ('running', 'stopped', 'failed', 'unknown')),
    reason TEXT NOT NULL
        CHECK (reason <> '' AND length(reason) <= 128),
    activation_bundle_id TEXT,
    generation INTEGER CHECK (generation IS NULL OR generation >= 0),
    pid INTEGER CHECK (pid IS NULL OR pid > 0),
    process_started_at TEXT,
    occurred_at TEXT NOT NULL CHECK (occurred_at <> ''),
    uncertain_since TEXT,
    UNIQUE (node_id, instance_id, source_id),
    CHECK ((pid IS NULL) = (process_started_at IS NULL)),
    CHECK (state <> 'running' OR (activation_bundle_id IS NOT NULL AND pid IS NOT NULL)),
    CHECK (state <> 'unknown' OR uncertain_since IS NOT NULL),
    CHECK (uncertain_since IS NULL OR uncertain_since <= occurred_at)
) STRICT;

CREATE INDEX runtime_transitions_timeline
    ON runtime_transitions(node_id, occurred_at DESC, id DESC);

CREATE INDEX runtime_transitions_bundle_timeline
    ON runtime_transitions(node_id, activation_bundle_id, occurred_at DESC, id DESC);

CREATE TRIGGER runtime_transitions_reject_update
BEFORE UPDATE ON runtime_transitions
BEGIN
    SELECT RAISE(ABORT, 'runtime transitions are append-only');
END;

-- Traffic samples carry each node's cumulative counters. A delta is proven
-- only between samples of one node instance.
CREATE TABLE traffic_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    instance_id TEXT NOT NULL CHECK (length(instance_id) BETWEEN 1 AND 64),
    activation_bundle_id TEXT NOT NULL,
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

CREATE INDEX traffic_samples_history
    ON traffic_samples(node_id, sampled_at, id);

CREATE INDEX traffic_samples_bundle_history
    ON traffic_samples(node_id, activation_bundle_id, sampled_at, id);

CREATE INDEX traffic_samples_interval_history
    ON traffic_samples(node_id, interval_start, interval_end, id)
    WHERE interval_start IS NOT NULL;

CREATE INDEX traffic_samples_age
    ON traffic_samples(sampled_at);

-- The last accepted sample of each node, which the next delta starts from.
CREATE TABLE traffic_checkpoint (
    node_id TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    instance_id TEXT NOT NULL CHECK (length(instance_id) BETWEEN 1 AND 64),
    last_upload_total INTEGER NOT NULL CHECK (last_upload_total >= 0),
    last_download_total INTEGER NOT NULL CHECK (last_download_total >= 0),
    sampled_at TEXT NOT NULL CHECK (sampled_at <> '')
) STRICT;

-- Each node's accepted traffic, booked in the UTC day it was observed, so
-- any period that starts on a day boundary sums whole rows.
CREATE TABLE traffic_days (
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    day_start TEXT NOT NULL CHECK (day_start <> ''),
    inbound_bytes INTEGER NOT NULL CHECK (inbound_bytes >= 0),
    outbound_bytes INTEGER NOT NULL CHECK (outbound_bytes >= 0),
    first_observed_at TEXT NOT NULL CHECK (first_observed_at <> ''),
    last_sample_at TEXT NOT NULL CHECK (last_sample_at >= first_observed_at),
    PRIMARY KEY (node_id, day_start)
) STRICT;

-- The traffic periods each node has recorded, in the period settings in
-- force when they were recorded. Their totals are read from traffic_days.
CREATE TABLE traffic_periods (
    node_id TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    id TEXT NOT NULL CHECK (id <> ''),
    period_start TEXT NOT NULL CHECK (period_start <> ''),
    period_end TEXT NOT NULL CHECK (period_end <> ''),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    CHECK (period_end > period_start),
    PRIMARY KEY (node_id, id)
) STRICT;

CREATE INDEX traffic_periods_page
    ON traffic_periods(node_id, period_start DESC, id DESC);
