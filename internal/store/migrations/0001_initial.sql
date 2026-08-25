CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY CHECK (version > 0),
    name TEXT NOT NULL CHECK (name <> ''),
    applied_at TEXT NOT NULL CHECK (applied_at <> '')
) STRICT;

CREATE TABLE canonical_revisions (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    sequence INTEGER NOT NULL UNIQUE CHECK (sequence > 0),
    parent_id TEXT REFERENCES canonical_revisions(id) ON DELETE RESTRICT,
    schema_version INTEGER NOT NULL CHECK (schema_version > 0),
    document_json TEXT NOT NULL CHECK (json_valid(document_json)),
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
    variant TEXT NOT NULL DEFAULT 'plain' CHECK (variant <> ''),
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
    verification_state TEXT NOT NULL DEFAULT 'verified'
        CHECK (verification_state IN ('verified', 'revoked', 'quarantined')),
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

CREATE UNIQUE INDEX core_artifacts_official_asset
    ON core_artifacts(repository_id, release_id, asset_id)
    WHERE repository_id IS NOT NULL AND release_id IS NOT NULL AND asset_id IS NOT NULL;

CREATE INDEX core_artifacts_version_lookup
    ON core_artifacts(exact_version, operating_system, architecture, variant);

CREATE INDEX core_artifacts_digest_lookup
    ON core_artifacts(archive_sha256);

CREATE INDEX core_artifacts_binary_digest_lookup
    ON core_artifacts(binary_sha256);

CREATE TABLE catalog_state (
    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
    validator TEXT NOT NULL DEFAULT '',
    catalog_json TEXT NOT NULL CHECK (json_valid(catalog_json)),
    diagnostics_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(diagnostics_json)),
    refreshed_at TEXT NOT NULL CHECK (refreshed_at <> '')
) STRICT;

CREATE TABLE capability_generations (
    id TEXT PRIMARY KEY
        CHECK (length(id) = 64 AND id NOT GLOB '*[^0-9a-f]*'),
    repository TEXT NOT NULL CHECK (repository <> ''),
    commit_sha TEXT NOT NULL CHECK (
        (length(commit_sha) = 40 OR length(commit_sha) = 64)
        AND commit_sha NOT GLOB '*[^0-9a-f]*'
    ),
    source_sha256 TEXT NOT NULL
        CHECK (length(source_sha256) = 64 AND source_sha256 NOT GLOB '*[^0-9a-f]*'),
    manifest_count INTEGER NOT NULL CHECK (manifest_count > 0),
    refreshed_at TEXT NOT NULL CHECK (refreshed_at <> ''),
    UNIQUE (repository, commit_sha)
) STRICT;

CREATE TABLE capability_generation_manifests (
    generation_id TEXT NOT NULL
        REFERENCES capability_generations(id) ON DELETE RESTRICT,
    exact_core_version TEXT NOT NULL CHECK (exact_core_version <> ''),
    path TEXT NOT NULL CHECK (path <> ''),
    manifest_sha256 TEXT NOT NULL
        CHECK (length(manifest_sha256) = 64 AND manifest_sha256 NOT GLOB '*[^0-9a-f]*'),
    support_level TEXT NOT NULL CHECK (
        support_level IN (
            'native_structured', 'compatible_structured', 'manual_json', 'unavailable'
        )
    ),
    manifest_json TEXT NOT NULL CHECK (json_valid(manifest_json)),
    PRIMARY KEY (generation_id, exact_core_version),
    UNIQUE (generation_id, path)
) STRICT;

CREATE INDEX capability_generation_manifest_digest
    ON capability_generation_manifests(manifest_sha256);

CREATE TABLE capability_pins (
    exact_core_version TEXT PRIMARY KEY CHECK (exact_core_version <> ''),
    repository TEXT NOT NULL CHECK (repository <> ''),
    commit_sha TEXT NOT NULL CHECK (commit_sha <> ''),
    manifest_sha256 TEXT NOT NULL
        CHECK (length(manifest_sha256) = 64 AND manifest_sha256 NOT GLOB '*[^0-9a-f]*'),
    support_level TEXT NOT NULL
        CHECK (support_level IN ('native_structured', 'compatible_structured', 'manual_json', 'unavailable')),
    pinned_at TEXT NOT NULL CHECK (pinned_at <> '')
) STRICT;

CREATE TABLE capability_quarantine (
    manifest_sha256 TEXT PRIMARY KEY
        CHECK (length(manifest_sha256) = 64 AND manifest_sha256 NOT GLOB '*[^0-9a-f]*'),
    reason_code TEXT NOT NULL CHECK (reason_code <> ''),
    diagnostics_json TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(diagnostics_json)),
    quarantined_at TEXT NOT NULL CHECK (quarantined_at <> '')
) STRICT;

CREATE TABLE startup_artifacts (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    kind TEXT NOT NULL CHECK (kind IN ('structured', 'manual')),
    canonical_revision_id TEXT NOT NULL
        REFERENCES canonical_revisions(id) ON DELETE RESTRICT,
    exact_core_version TEXT NOT NULL CHECK (exact_core_version <> ''),
    capability_commit TEXT,
    capability_digest TEXT
        CHECK (
            capability_digest IS NULL
            OR (length(capability_digest) = 64 AND capability_digest NOT GLOB '*[^0-9a-f]*')
        ),
    renderer_version TEXT NOT NULL CHECK (renderer_version <> ''),
    core_artifact_id TEXT NOT NULL
        REFERENCES core_artifacts(id) ON DELETE RESTRICT,
    config_bytes BLOB NOT NULL,
    config_sha256 TEXT NOT NULL
        CHECK (length(config_sha256) = 64 AND config_sha256 NOT GLOB '*[^0-9a-f]*'),
    diagnostics_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(diagnostics_json)),
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'ready', 'failed', 'stale')),
    checked_at TEXT,
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    CHECK (
        kind = 'manual'
        OR (capability_commit IS NOT NULL AND capability_digest IS NOT NULL)
    )
) STRICT;

CREATE INDEX startup_artifacts_projection_lookup
    ON startup_artifacts(canonical_revision_id, exact_core_version, core_artifact_id, state);

CREATE TABLE subscription_channels (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    name TEXT NOT NULL UNIQUE CHECK (name <> ''),
    format TEXT NOT NULL CHECK (format IN ('sing-box', 'mihomo', 'loon')),
    config_json TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(config_json)),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

CREATE TABLE subscription_sources (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    name TEXT NOT NULL UNIQUE CHECK (name <> ''),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('remote', 'local')),
    config_json TEXT NOT NULL CHECK (json_valid(config_json)),
    latest_snapshot_json TEXT
        CHECK (latest_snapshot_json IS NULL OR json_valid(latest_snapshot_json)),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

CREATE TABLE subscription_snapshots (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    canonical_revision_id TEXT NOT NULL
        REFERENCES canonical_revisions(id) ON DELETE RESTRICT,
    startup_artifact_id TEXT NOT NULL
        REFERENCES startup_artifacts(id) ON DELETE RESTRICT,
    content_json TEXT NOT NULL CHECK (json_valid(content_json)),
    sha256 TEXT NOT NULL
        CHECK (length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'),
    created_at TEXT NOT NULL CHECK (created_at <> '')
) STRICT;

CREATE TABLE activation_bundles (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    startup_artifact_id TEXT NOT NULL
        REFERENCES startup_artifacts(id) ON DELETE RESTRICT,
    subscription_snapshot_id TEXT NOT NULL
        REFERENCES subscription_snapshots(id) ON DELETE RESTRICT,
    public_addresses_json TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(public_addresses_json)),
    source_snapshots_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(source_snapshots_json)),
    monitoring_tier TEXT NOT NULL
        CHECK (monitoring_tier IN ('full', 'limited', 'process_only')),
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

INSERT INTO hub_state(singleton, updated_at)
VALUES (1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

CREATE TABLE tasks (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    idempotency_key TEXT,
    lane TEXT NOT NULL CHECK (lane IN ('runtime', 'maintenance')),
    kind TEXT NOT NULL CHECK (kind <> ''),
    status TEXT NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled', 'superseded')),
    generation INTEGER NOT NULL DEFAULT 0 CHECK (generation >= 0),
    canonical_revision_id TEXT
        REFERENCES canonical_revisions(id) ON DELETE RESTRICT,
    startup_artifact_id TEXT
        REFERENCES startup_artifacts(id) ON DELETE RESTRICT,
    activation_bundle_id TEXT
        REFERENCES activation_bundles(id) ON DELETE RESTRICT,
    payload_json TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(payload_json)),
    result_json TEXT CHECK (result_json IS NULL OR json_valid(result_json)),
    error_json TEXT CHECK (error_json IS NULL OR json_valid(error_json)),
    cancel_requested INTEGER NOT NULL DEFAULT 0 CHECK (cancel_requested IN (0, 1)),
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    lease_owner TEXT,
    lease_expires_at TEXT,
    not_before TEXT,
    created_at TEXT NOT NULL CHECK (created_at <> ''),
    updated_at TEXT NOT NULL CHECK (updated_at <> '')
) STRICT;

CREATE UNIQUE INDEX tasks_lane_idempotency
    ON tasks(lane, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX tasks_claim_queue
    ON tasks(lane, status, not_before, created_at);

CREATE TABLE subscription_tokens (
    id TEXT PRIMARY KEY CHECK (id <> ''),
    token_sha256 TEXT NOT NULL UNIQUE
        CHECK (length(token_sha256) = 64 AND token_sha256 NOT GLOB '*[^0-9a-f]*'),
    channel_id TEXT REFERENCES subscription_channels(id) ON DELETE RESTRICT,
    expires_at TEXT,
    revoked_at TEXT,
    created_at TEXT NOT NULL CHECK (created_at <> '')
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

CREATE INDEX traffic_periods_range
    ON traffic_periods(period_start, period_end);

CREATE TABLE log_entries (
    id TEXT PRIMARY KEY
        CHECK (id <> '' AND length(id) <= 80),
    occurred_at TEXT NOT NULL CHECK (occurred_at <> ''),
    source TEXT NOT NULL
        CHECK (source IN ('panel', 'core', 'task', 'security')),
    level TEXT NOT NULL
        CHECK (level IN ('trace', 'debug', 'info', 'warn', 'error', 'fatal')),
    code TEXT NOT NULL
        CHECK (code <> '' AND length(code) <= 128),
    message TEXT NOT NULL
        CHECK (message <> '' AND length(message) <= 4096),
    metadata_json TEXT NOT NULL DEFAULT '{}'
        CHECK (json_valid(metadata_json) AND json_type(metadata_json) = 'object' AND length(metadata_json) <= 16384)
) STRICT;

CREATE INDEX log_entries_timeline
    ON log_entries(occurred_at DESC, id DESC);

CREATE INDEX log_entries_source_timeline
    ON log_entries(source, occurred_at DESC, id DESC);

CREATE INDEX log_entries_level_timeline
    ON log_entries(level, occurred_at DESC, id DESC);

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
    observed_at TEXT NOT NULL CHECK (observed_at <> '')
) STRICT;
