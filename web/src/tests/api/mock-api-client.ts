import type { Mocked } from 'vitest';

import { vi } from 'vitest';

import type { ApiClient, CanonicalSnapshot, CatalogAssetList, CoreArtifactPage, DashboardContext, LogEntry, MetricsSnapshot, Session, StartupArtifactSummary, SubscriptionChannel, SubscriptionSource, SubscriptionToken, Task, TrafficPeriod } from '@/api/api-client';

export const testSession: Session = { displayName: 'Panel administrator' };

export const testDashboardContext: DashboardContext = {
  view: { exactVersion: '1.13.19' },
  running: {
    exactVersion: '1.13.19',
    artifactName: 'core_1',
    digest: '8d5f2a1c7782e544',
  },
  canonical: {
    revision: 42,
    savedAt: '2026-08-26T07:30:00Z',
    hasUnappliedChanges: true,
  },
  applied: {
    bundle: 'bundle_18',
    revision: 41,
    appliedAt: '2026-08-26T07:12:00Z',
  },
  adapter: {
    supported: true,
    label: 'sing-box-1.13.19@1',
    warning: null,
  },
};

export const testRevision: CanonicalSnapshot = {
  id: 'revision_42',
  sequence: 42,
  parent_id: 'revision_41',
  schema_version: 2,
  document: {
    schema_version: 2,
    configuration: {
      log: { level: 'info' },
      inbounds: [{
        _panel: { id: 'edge-socks', enabled: true },
        type: 'socks', tag: 'edge-socks', listen: '127.0.0.1', listen_port: 1080,
      }],
      outbounds: [{ _panel: { id: 'direct', enabled: true }, type: 'direct', tag: 'direct' }],
    },
  },
  document_json: '{"configuration":{"inbounds":[{"_panel":{"enabled":true,"id":"edge-socks"},"listen":"127.0.0.1","listen_port":1080,"tag":"edge-socks","type":"socks"}],"log":{"level":"info"},"outbounds":[{"_panel":{"enabled":true,"id":"direct"},"tag":"direct","type":"direct"}]},"schema_version":2}',
  sha256: 'a'.repeat(64),
  created_at: '2026-08-26T07:30:00Z',
};

export const testTask: Task = {
  id: 'task_catalog_refresh', lane: 'maintenance', kind: 'catalog-refresh',
  status: 'succeeded', generation: 0, payload: {}, cancel_requested: false,
  attempt: 1, created_at: '2026-08-26T07:20:00Z', updated_at: '2026-08-26T07:21:00Z',
};

export const testCatalog: CatalogAssetList = {
  validator: 'catalog-v1',
  refreshed_at: '2026-08-26T07:21:00Z',
  assets: [{
    repository_id: 509091576, release_id: 101, asset_id: 201,
    name: 'sing-box-1.13.19-linux-arm64.tar.gz',
    download_url: 'https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-arm64.tar.gz',
    size: 12_000_000, version: '1.13.19', os: 'linux', arch: 'arm64', variant: 'plain',
    api_digest: 'b'.repeat(64), has_api_digest: true, has_catalog_digest: false,
  }],
};

export const testArtifacts: CoreArtifactPage = {
  items: [{
    id: 'core_1', exact_version: '1.13.19', os: 'linux', arch: 'arm64', variant: 'plain',
    source_kind: 'official', repository_id: 509091576, release_id: 101, asset_id: 201,
    archive_sha256: 'b'.repeat(64), binary_sha256: 'd'.repeat(64),
    binary_path: '/var/lib/sing-box-panel/artifacts/core_1/sing-box',
    reported_version: '1.13.19', feature_fingerprint: { status: 'reported', features: ['with_quic'] },
    verification_state: 'verified', created_at: '2026-08-26T07:22:00Z',
  }],
};

export const testStartupArtifact: StartupArtifactSummary = {
  id: 'startup_1', canonical_revision_id: testRevision.id, exact_core_version: '1.13.19',
  adapter_id: 'sing-box-1.13.19', adapter_revision: '1', core_artifact_id: 'core_1',
  config_sha256: 'c'.repeat(64), diagnostics: [], state: 'ready',
  checked_at: '2026-08-26T07:31:00Z', created_at: '2026-08-26T07:30:00Z',
};

export const testSubscriptionChannels: SubscriptionChannel[] = [{
  id: 'channel_sing_box', name: 'Primary sing-box', format: 'sing-box',
  public_host: 'proxy.example', config: { exclude_tags: ['private'] }, enabled: true,
  created_at: '2026-08-26T07:00:00Z', updated_at: '2026-08-26T07:05:00.000000001Z',
}];

export const testSubscriptionSources: SubscriptionSource[] = [{
  id: 'source_local', name: 'Operator additions', source_kind: 'local', config: {},
  current_version_id: 'source_version_1', enabled: true,
  created_at: '2026-08-26T07:00:00Z', updated_at: '2026-08-26T07:06:00Z',
}];

export const testSubscriptionTokens: SubscriptionToken[] = [{
  id: 'token_primary', user_id: 'user_1', label: 'phone', enabled: true,
  successful_request_count: 0, body_response_count: 0, bytes_served: 0,
  created_at: '2026-08-26T07:10:00Z', active: true,
}];

export const testSubscriptionUsers = [{
  id: 'user_1', name: 'Primary user', description: 'Personal devices', enabled: true,
  created_at: '2026-08-26T07:00:00Z', updated_at: '2026-08-26T07:00:00Z',
}];

export const testLogEntry: LogEntry = {
  id: 'log_runtime_ready', time: '2026-08-26T07:32:00Z', source: 'core', level: 'info',
  code: 'runtime.ready', message: 'The exact core process passed its health check.',
  metadata: { exact_version: '1.13.19', activation_bundle_id: 'bundle_18' },
};

export const testTrafficPeriod: TrafficPeriod = {
  id: 'traffic_20260826_0730', activation_bundle_id: 'bundle_18',
  period_start: '2026-08-26T07:30:00Z', period_end: '2026-08-26T07:35:00Z',
  inbound_bytes: 2_048, outbound_bytes: 4_096, counters: { inbound: { mixed: 2_048 } },
  created_at: '2026-08-26T07:35:00Z',
};

export const testMetrics: MetricsSnapshot = {
  available: true, applied_bundle_id: 'bundle_18', monitoring_tier: 'limited',
  collected_at: '2026-08-26T07:34:00Z', quota_exceeded: false,
  current_traffic_period: testTrafficPeriod,
};

export function createMockApiClient(overrides: Partial<ApiClient> = {}): Mocked<ApiClient> {
  const support = {
    supported: true,
    profile: {
      exact_version: '1.13.19', os: 'linux', arch: 'arm64', variant: 'plain',
      feature_fingerprint: { status: 'reported', features: ['with_quic'] },
    },
    adapter_id: 'sing-box-1.13.19',
    adapter_revision: '1',
    provenance: {
      upstream_tag: 'v1.13.19', upstream_commit: 'b'.repeat(40), source: 'compiled',
    },
  };
  const client: ApiClient = {
    getSession: vi.fn().mockResolvedValue(testSession),
    login: vi.fn().mockResolvedValue(testSession),
    logout: vi.fn().mockResolvedValue(undefined),
    getDashboardContext: vi.fn().mockResolvedValue(testDashboardContext),
    getCanonical: vi.fn().mockResolvedValue(testRevision),
    replaceCanonical: vi.fn().mockResolvedValue({ revision: testRevision, no_change: false }),
    patchCanonical: vi.fn().mockResolvedValue({ revision: testRevision, no_change: false }),
    listRevisions: vi.fn().mockResolvedValue({ items: [testRevision] }),
    getRevision: vi.fn().mockResolvedValue(testRevision),
    diffRevisions: vi.fn().mockResolvedValue({
      from: testRevision, to: testRevision, changes: [],
    }),
    restoreRevision: vi.fn().mockResolvedValue({ revision: testRevision, no_change: false }),
    listCatalogAssets: vi.fn().mockResolvedValue(testCatalog),
    refreshCatalog: vi.fn().mockResolvedValue({ ...testTask, status: 'queued' }),
    listCoreArtifacts: vi.fn().mockResolvedValue(testArtifacts),
    getCoreArtifact: vi.fn().mockResolvedValue(testArtifacts.items[0]),
    installCore: vi.fn().mockResolvedValue({
      ...testTask, id: 'task_core_install', kind: 'core-install', status: 'queued',
    }),
    importCoreArchive: vi.fn().mockResolvedValue({
      ...testTask, id: 'task_core_import', kind: 'core-import',
    }),
    removeCoreArtifact: vi.fn().mockResolvedValue(undefined),
    quarantineCoreArtifact: vi.fn().mockResolvedValue({
      ...testArtifacts.items[0], verification_state: 'quarantined',
    }),
    revokeCoreArtifact: vi.fn().mockResolvedValue({
      ...testArtifacts.items[0], verification_state: 'revoked',
    }),
    getConfigurationSupport: vi.fn().mockResolvedValue(support),
    previewConfiguration: vi.fn().mockResolvedValue({
      canonical_revision: testRevision, core_artifact: testArtifacts.items[0],
      support, config: { log: { level: 'info' } }, diagnostics: [],
    }),
    compileConfiguration: vi.fn().mockResolvedValue({
      support,
      artifact: { ...testStartupArtifact, state: 'pending' },
      task: { ...testTask, id: 'task_startup_check', kind: 'startup-check', status: 'queued' },
    }),
    listStartupArtifacts: vi.fn().mockResolvedValue({ items: [testStartupArtifact] }),
    checkStartupArtifact: vi.fn().mockResolvedValue(testTask),
    activateStartupArtifact: vi.fn().mockResolvedValue({
      activation: {
        startup_artifact_id: testStartupArtifact.id,
        canonical_revision_id: testRevision.id,
        exact_core_version: '1.13.19', core_artifact_id: 'core_1',
        config_sha256: testStartupArtifact.config_sha256,
        activation_bundle_id: 'bundle_19', activation_sha256: '2'.repeat(64),
        monitoring_tier: 'process_only',
      },
      task: { ...testTask, id: 'task_apply', kind: 'runtime-apply', status: 'queued' },
    }),
    getRuntimeStatus: vi.fn().mockResolvedValue({
      desired_running: true, target_generation: 1, observation_state: 'running',
    }),
    startRuntime: vi.fn().mockResolvedValue(testTask),
    stopRuntime: vi.fn().mockResolvedValue(testTask),
    restartRuntime: vi.fn().mockResolvedValue(testTask),
    rollbackRuntime: vi.fn().mockResolvedValue(testTask),
    listTasks: vi.fn().mockResolvedValue({ items: [testTask] }),
    getTask: vi.fn().mockResolvedValue(testTask),
    cancelTask: vi.fn().mockResolvedValue({ ...testTask, cancel_requested: true }),
    listSubscriptionChannels: vi.fn().mockResolvedValue({ items: testSubscriptionChannels }),
    getSubscriptionChannel: vi.fn().mockResolvedValue(testSubscriptionChannels[0]),
    createSubscriptionChannel: vi.fn().mockResolvedValue(testSubscriptionChannels[0]),
    updateSubscriptionChannel: vi.fn().mockResolvedValue(testSubscriptionChannels[0]),
    deleteSubscriptionChannel: vi.fn().mockResolvedValue(undefined),
    previewSubscriptionChannel: vi.fn().mockResolvedValue({
      user_id: 'user_1', applied_bundle_id: 'bundle_19', channel: testSubscriptionChannels[0],
      startup_artifact_id: testStartupArtifact.id, canonical_revision_id: testRevision.id,
      exact_core_version: '1.13.19', artifact_state: 'ready',
      result: {
        format: 'sing-box', media_type: 'application/json', content: '',
        node_count: 0, diagnostics: [],
      },
    }),
    listSubscriptionUsers: vi.fn().mockResolvedValue({ items: testSubscriptionUsers }),
    getSubscriptionUser: vi.fn().mockResolvedValue(testSubscriptionUsers[0]),
    createSubscriptionUser: vi.fn().mockResolvedValue(testSubscriptionUsers[0]),
    updateSubscriptionUser: vi.fn().mockResolvedValue(testSubscriptionUsers[0]),
    deleteSubscriptionUser: vi.fn().mockResolvedValue(undefined),
    getSubscriptionNodeCatalog: vi.fn().mockResolvedValue({
      applied_bundle_id: 'bundle_19', nodes: [], diagnostics: [],
    }),
    getSubscriptionUserGrants: vi.fn().mockResolvedValue({ user: testSubscriptionUsers[0], grants: [] }),
    replaceSubscriptionUserGrants: vi.fn().mockResolvedValue({
      user: testSubscriptionUsers[0], grants: [],
    }),
    listSubscriptionSources: vi.fn().mockResolvedValue({
      items: testSubscriptionSources.map(({ config: _config, ...source }) => ({
        ...source, has_version: true,
      })),
    }),
    getSubscriptionSource: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    createSubscriptionSource: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    updateSubscriptionSource: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    deleteSubscriptionSource: vi.fn().mockResolvedValue(undefined),
    refreshSubscriptionSource: vi.fn().mockResolvedValue(testTask),
    listSubscriptionSourceVersions: vi.fn().mockResolvedValue({ items: [] }),
    createSubscriptionSourceVersion: vi.fn().mockResolvedValue({
      source: testSubscriptionSources[0],
      version: {
        id: 'version_1', source_id: 'source_local', format: 'sing-box-json',
        normalized_nodes: [], diagnostics: [], sha256: 'a'.repeat(64),
        fetched_at: '2026-08-26T07:00:00Z', created_at: '2026-08-26T07:00:00Z',
      },
    }),
    restoreSubscriptionSourceVersion: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    listSubscriptionTokens: vi.fn().mockResolvedValue({ items: testSubscriptionTokens }),
    createSubscriptionToken: vi.fn().mockResolvedValue({
      metadata: { ...testSubscriptionTokens[0], id: 'token_new' },
      token: 'one-time-public-token',
    }),
    rotateSubscriptionToken: vi.fn().mockResolvedValue({
      revoked: { ...testSubscriptionTokens[0], active: false },
      created: { ...testSubscriptionTokens[0], id: 'token_rotated' },
      token: 'one-time-rotated-token',
    }),
    revokeSubscriptionToken: vi.fn().mockResolvedValue({ ...testSubscriptionTokens[0], active: false }),
    setSubscriptionTokenEnabled: vi.fn().mockResolvedValue(testSubscriptionTokens[0]),
    deleteSubscriptionToken: vi.fn().mockResolvedValue(undefined),
    listLogs: vi.fn().mockResolvedValue({ items: [testLogEntry] }),
    getLog: vi.fn().mockResolvedValue(testLogEntry),
    clearLogs: vi.fn().mockResolvedValue({ deleted: 1 }),
    deleteLog: vi.fn().mockImplementation(async (entryID: string) => ({
      id: entryID, deleted: true as const,
    })),
    getMetrics: vi.fn().mockResolvedValue(testMetrics),
    getTrafficStatus: vi.fn().mockResolvedValue(testMetrics),
    listTrafficPeriods: vi.fn().mockResolvedValue({ items: [testTrafficPeriod] }),
    getTrafficPeriod: vi.fn().mockResolvedValue(testTrafficPeriod),
  };
  return { ...client, ...overrides } as Mocked<ApiClient>;
}
