import type { Mocked } from 'vitest';

import { vi } from 'vitest';

import type {
  ApiClient,
  CanonicalSnapshot,
  CatalogAssetList,
  ConfigurationSupport,
  CoreArtifactPage,
  DashboardContext,
  DashboardStreamSnapshot,
  MetricsHistory,
  MetricsSnapshot,
  RuntimeHistoryPage,
  RuntimeStatus,
  Session,
  StartupArtifactSummary,
  SubscriptionChannel,
  SubscriptionSource,
  SubscriptionToken,
  SystemStatus,
  TrafficPeriod,
} from '@/api/api-client';

import { DEFAULT_APPEARANCE } from '@/theme/appearance';
import { reviewedSchemaManifest } from '@/schemas/generated';

export const testSession: Session = { displayName: 'Panel administrator' };

export const testSystemStatus: SystemStatus = {
  platform: { os: 'linux', arch: 'arm64' },
  panel_version: '0.1.0',
  canonical_revision: 42,
  applied_bundle_id: 'bundle_18',
  running: true,
  running_version: '1.13.19',
  running_artifact: 'core_1',
  configuration_state: 'sing-box-1.13.19@1',
};

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
  configuration: {
    supported: true,
    label: 'Raw JSON',
    warning: null,
  },
};

export const testRevision: CanonicalSnapshot = {
  id: 'revision_42',
  sequence: 42,
  parent_id: 'revision_41',
  schema_version: 1,
  document: {
    log: { level: 'info' },
    inbounds: [
      {
        type: 'socks',
        tag: 'edge-socks',
        listen: '127.0.0.1',
        listen_port: 1080,
      },
    ],
    outbounds: [{ type: 'direct', tag: 'direct' }],
  } as unknown as CanonicalSnapshot['document'],
  document_json:
    '{"inbounds":[{"listen":"127.0.0.1","listen_port":1080,"tag":"edge-socks","type":"socks"}],"log":{"level":"info"},"outbounds":[{"tag":"direct","type":"direct"}]}',
  sha256: 'a'.repeat(64),
  created_at: '2026-08-26T07:30:00Z',
};

export const testRuntimeStatus: RuntimeStatus = {
  desired_running: true,
  target_generation: 1,
  observation_state: 'running',
  running: {
    pid: 8124, process_start_token: 'test-process', exact_core_version: '1.13.19',
    core_artifact_id: 'core_1', archive_sha256: 'b'.repeat(64), binary_sha256: 'd'.repeat(64),
    activation_bundle_id: 'bundle_18', started_at: '2026-08-26T07:32:00Z',
  },
};

export const testCatalog: CatalogAssetList = {
  validator: 'catalog-v1',
  refreshed_at: '2026-08-26T07:21:00Z',
  assets: [
    {
      repository_id: 509091576,
      release_id: 101,
      asset_id: 201,
      name: 'sing-box-1.13.19-linux-arm64-musl.tar.gz',
      download_url:
        'https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-arm64-musl.tar.gz',
      size: 12_000_000,
      version: '1.13.19',
      os: 'linux',
      arch: 'arm64',
      variant: 'musl',
      api_digest: 'b'.repeat(64),
      has_api_digest: true,
      has_catalog_digest: false,
    },
  ],
};

export const testArtifacts: CoreArtifactPage = {
  items: [
    {
      id: 'core_1',
      exact_version: '1.13.19',
      os: 'linux',
      arch: 'arm64',
      variant: 'musl',
      source_kind: 'official',
      repository_id: 509091576,
      release_id: 101,
      asset_id: 201,
      archive_sha256: 'b'.repeat(64),
      binary_sha256: 'd'.repeat(64),
      binary_path: '/var/lib/sing-box-panel/artifacts/core_1/sing-box',
      reported_version: '1.13.19',
      feature_fingerprint: { status: 'reported', features: ['with_quic'] },
      created_at: '2026-08-26T07:22:00Z',
    },
  ],
};

export const testStartupArtifact: StartupArtifactSummary = {
  id: 'startup_1',
  canonical_revision_id: testRevision.id,
  exact_core_version: '1.13.19',
  core_artifact_id: 'core_1',
  config_sha256: 'c'.repeat(64),
  state: 'ready',
  checked_at: '2026-08-26T07:31:00Z',
  created_at: '2026-08-26T07:30:00Z',
};

export const testSubscriptionChannels: SubscriptionChannel[] = [
  {
    id: 'channel_sing_box',
    name: 'Primary sing-box',
    format: 'sing-box',
    public_host: 'proxy.example',
    config: { exclude_tags: ['private'] },
    enabled: true,
    created_at: '2026-08-26T07:00:00Z',
    updated_at: '2026-08-26T07:05:00.000000001Z',
  },
];

export const testSubscriptionSources: SubscriptionSource[] = [
  {
    id: 'source_local',
    name: 'Operator additions',
    source_kind: 'local',
    config: {},
    current_version_id: 'source_version_1',
    enabled: true,
    created_at: '2026-08-26T07:00:00Z',
    updated_at: '2026-08-26T07:06:00Z',
  },
];

export const testSubscriptionTokens: SubscriptionToken[] = [
  {
    id: 'token_primary',
    user_id: 'user_1',
    label: 'phone',
    enabled: true,
    successful_request_count: 0,
    body_response_count: 0,
    bytes_served: 0,
    created_at: '2026-08-26T07:10:00Z',
    active: true,
  },
];

const testTrafficPeriod: TrafficPeriod = {
  id: 'traffic_20260826_0730',
  activation_bundle_id: 'bundle_18',
  period_start: '2026-08-26T07:30:00Z',
  period_end: '2026-08-26T07:35:00Z',
  inbound_bytes: 2_048,
  outbound_bytes: 4_096,
  counters: { inbound: { mixed: 2_048 } },
  created_at: '2026-08-26T07:35:00Z',
};

export const testMetrics: MetricsSnapshot = {
  available: true,
  applied_bundle_id: 'bundle_18',
  monitoring_tier: 'limited',
  collected_at: '2026-08-26T07:34:00Z',
  traffic_available: true,
  quota_exceeded: false,
  current_traffic_period: testTrafficPeriod,
};

export const testMetricsHistory: MetricsHistory = {
  from: '2026-08-26T07:30:00Z',
  to: '2026-08-26T07:40:00Z',
  bucket_seconds: 300,
  activation_bundle_id: 'bundle_18',
  buckets: [
    {
      from: '2026-08-26T07:30:00Z',
      to: '2026-08-26T07:35:00Z',
      upload_bytes: 4_096,
      download_bytes: 2_048,
      memory_bytes_avg: 58_000_000,
      memory_bytes_peak: 60_000_000,
      active_connections_avg: 12.5,
      active_connections_peak: 17,
      sample_count: 10,
      coverage: 'complete',
    },
    {
      from: '2026-08-26T07:35:00Z',
      to: '2026-08-26T07:40:00Z',
      upload_bytes: null,
      download_bytes: null,
      memory_bytes_avg: null,
      memory_bytes_peak: null,
      active_connections_avg: null,
      active_connections_peak: null,
      sample_count: 0,
      coverage: 'missing',
    },
  ],
};

export const testRuntimeHistory: RuntimeHistoryPage = {
  items: [
    {
      id: 2,
      state: 'running',
      reason: 'apply_succeeded',
      activation_bundle_id: 'bundle_18',
      generation: 7,
      pid: 4182,
      process_started_at: '2026-08-26T07:32:00Z',
      occurred_at: '2026-08-26T07:32:00Z',
    },
  ],
  preceding: {
    id: 1,
    state: 'unknown',
    reason: 'history_initialized',
    occurred_at: '2026-08-26T07:00:00Z',
    uncertain_since: '2026-08-26T07:00:00Z',
  },
  history_started_at: '2026-08-26T07:00:00Z',
};

export const testDashboardSnapshot: DashboardStreamSnapshot = {
  collected_at: '2026-08-26T07:40:00Z',
  history_1h: { ...testMetricsHistory, bucket_seconds: 60 },
  history_24h: testMetricsHistory,
  runtime_24h: testRuntimeHistory,
  activity: { items: [], total: 0 },
};

export function createMockApiClient(overrides: Partial<ApiClient> = {}): Mocked<ApiClient> {
  const support = {
    structured: true,
    exact_version: testArtifacts.items[0].exact_version,
  } satisfies ConfigurationSupport;
  const client: ApiClient = {
    supportsNativeChannelValidation: true,
    listFilesystemEntries: vi.fn<ApiClient['listFilesystemEntries']>().mockResolvedValue({ path: '/', parent: '/', requested_path: '/', fallback: false, items: [], total: 0, offset: 0, limit: 50 }),
    resolveFilesystemPath: vi.fn<ApiClient['resolveFilesystemPath']>().mockImplementation(async input => ({ path: input.path, parent: '/', kind: input.mode === 'output-file' ? 'file' : input.mode, exists: true, symlink: false })),
    enableCore: vi.fn<ApiClient['enableCore']>().mockResolvedValue(testRuntimeStatus),
    disableCore: vi.fn<ApiClient['disableCore']>().mockResolvedValue(testRuntimeStatus),
    getConfigurationFile: vi.fn<ApiClient['getConfigurationFile']>().mockResolvedValue({
      revision: 1,
      content: testRevision.document_json,
      canonical_revision_id: testRevision.id,
      syntax_valid: true,
    }),
    saveConfigurationFile: vi.fn<ApiClient['saveConfigurationFile']>().mockImplementation(async (input) => {
      let syntaxValid = false;
      try {
        const parsed: unknown = JSON.parse(input.content);
        syntaxValid = parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed);
      } catch {
        /* incomplete text is still saved */
      }
      return {
        revision: input.revision + 1,
        content: input.content,
        syntax_valid: syntaxValid,
        canonical_revision_id: syntaxValid ? testRevision.id : undefined,
      };
    }),
    exportPanelBackup: vi.fn<ApiClient['exportPanelBackup']>(),
    restorePanelBackup: vi.fn<ApiClient['restorePanelBackup']>(),
    getPanelSettings: vi.fn<ApiClient['getPanelSettings']>().mockResolvedValue({
      revision: 0,
      service: { data_dir: '/var/lib/sing-box-panel', base_path: '', secure_cookie: false, catalog_refresh_interval_hours: 12, traffic_period_months: 1, sample_retention_days: 90, private_source_cidrs: [] },
      github_token_configured: false,
      restart_required: false,
      preferences: {
        listen_host: '127.0.0.1',
        listen_port: 3000,
        external_origin: '',
        public_node_host: '',
        traffic_quota_gib: null,
        language: 'en',
        appearance: { ...DEFAULT_APPEARANCE },
      },
    }),
    savePanelSettings: vi.fn<ApiClient['savePanelSettings']>().mockImplementation(async (input) => ({
      revision: input.revision + 1,
      preferences: input.preferences,
      service: input.service ?? { data_dir: '/var/lib/sing-box-panel', base_path: '', secure_cookie: false, catalog_refresh_interval_hours: 12, traffic_period_months: 1, sample_retention_days: 90, private_source_cidrs: [] },
      github_token_configured: Boolean(input.github_token),
      restart_required: false,
    })),
    invalidateReadCache: vi.fn<ApiClient['invalidateReadCache']>(),
    subscribeSessionInvalidated: vi.fn<ApiClient['subscribeSessionInvalidated']>().mockReturnValue(() => undefined),
    getSession: vi.fn<ApiClient['getSession']>().mockResolvedValue(testSession),
    getSystemStatus: vi.fn<ApiClient['getSystemStatus']>().mockResolvedValue(testSystemStatus),
    login: vi.fn<ApiClient['login']>().mockResolvedValue(testSession),
    logout: vi.fn<ApiClient['logout']>().mockResolvedValue(undefined),
    getDashboardContext: vi.fn<ApiClient['getDashboardContext']>().mockResolvedValue(testDashboardContext),

    listCatalogAssets: vi.fn<ApiClient['listCatalogAssets']>().mockResolvedValue(testCatalog),
    refreshCatalog: vi.fn<ApiClient['refreshCatalog']>().mockResolvedValue({
      refreshed_at: testCatalog.refreshed_at, not_modified: false, releases: 1, assets: 1,
    }),
    listCoreArtifacts: vi.fn<ApiClient['listCoreArtifacts']>().mockResolvedValue(testArtifacts),
    getCoreArtifact: vi.fn<ApiClient['getCoreArtifact']>(),
    installCore: vi.fn<ApiClient['installCore']>().mockResolvedValue(testArtifacts.items[0]),
    importCoreArchive: vi.fn<ApiClient['importCoreArchive']>().mockResolvedValue(testArtifacts.items[0]),
    removeCoreArtifact: vi.fn<ApiClient['removeCoreArtifact']>().mockResolvedValue(undefined),
    getConfigurationSupport: vi.fn<ApiClient['getConfigurationSupport']>(),
    getConfigurationSchema: vi.fn<ApiClient['getConfigurationSchema']>(async () => {
      const exactVersion = testArtifacts.items[0].exact_version;
      const reviewed = await reviewedSchemaManifest[exactVersion].load();
      return {
        exact_version: exactVersion,
        schema_sha256: reviewed.schemaSHA256,
        schema: reviewed.schema,
      };
    }),
    previewConfiguration: vi.fn<ApiClient['previewConfiguration']>(),
    compileConfiguration: vi.fn<ApiClient['compileConfiguration']>().mockResolvedValue({
      support,
      artifact: testStartupArtifact,
    }),
    listStartupArtifacts: vi.fn<ApiClient['listStartupArtifacts']>(),
    checkStartupArtifact: vi.fn<ApiClient['checkStartupArtifact']>(),
    activateStartupArtifact: vi.fn<ApiClient['activateStartupArtifact']>(),
    getRuntimeStatus: vi.fn<ApiClient['getRuntimeStatus']>().mockResolvedValue(testRuntimeStatus),
    getRuntimeHistory: vi.fn<ApiClient['getRuntimeHistory']>().mockResolvedValue(testRuntimeHistory),
    startRuntime: vi.fn<ApiClient['startRuntime']>().mockResolvedValue(testRuntimeStatus),
    stopRuntime: vi.fn<ApiClient['stopRuntime']>().mockResolvedValue({ desired_running: false, target_generation: 2, observation_state: 'stopped' }),
    restartRuntime: vi.fn<ApiClient['restartRuntime']>().mockResolvedValue(testRuntimeStatus),
    rollbackRuntime: vi.fn<ApiClient['rollbackRuntime']>(),
    listSubscriptionChannels: vi.fn<ApiClient['listSubscriptionChannels']>().mockResolvedValue({ items: testSubscriptionChannels }),
    getSubscriptionChannel: vi.fn<ApiClient['getSubscriptionChannel']>().mockResolvedValue(testSubscriptionChannels[0]),
    createSubscriptionChannel: vi.fn<ApiClient['createSubscriptionChannel']>().mockResolvedValue(testSubscriptionChannels[0]),
    updateSubscriptionChannel: vi.fn<ApiClient['updateSubscriptionChannel']>().mockResolvedValue(testSubscriptionChannels[0]),
    deleteSubscriptionChannel: vi.fn<ApiClient['deleteSubscriptionChannel']>().mockResolvedValue(undefined),
    previewSubscriptionChannel: vi.fn<ApiClient['previewSubscriptionChannel']>().mockResolvedValue({
      user_id: 'user_1',
      applied_bundle_id: 'bundle_19',
      channel: testSubscriptionChannels[0],
      startup_artifact_id: testStartupArtifact.id,
      canonical_revision_id: testRevision.id,
      exact_core_version: '1.13.19',
      artifact_state: 'ready',
      result: {
        format: 'sing-box',
        media_type: 'application/json',
        content: '',
        node_count: 0,
        diagnostics: [],
      },
    }),
    listSubscriptionUsers: vi.fn<ApiClient['listSubscriptionUsers']>(),
    getSubscriptionUser: vi.fn<ApiClient['getSubscriptionUser']>(),
    createSubscriptionUser: vi.fn<ApiClient['createSubscriptionUser']>(),
    updateSubscriptionUser: vi.fn<ApiClient['updateSubscriptionUser']>(),
    deleteSubscriptionUser: vi.fn<ApiClient['deleteSubscriptionUser']>(),
    getSubscriptionNode: vi.fn<ApiClient['getSubscriptionNode']>(),
    createSubscriptionNode: vi.fn<ApiClient['createSubscriptionNode']>(),
    updateSubscriptionNode: vi.fn<ApiClient['updateSubscriptionNode']>(),
    deleteSubscriptionNode: vi.fn<ApiClient['deleteSubscriptionNode']>(),
    setSubscriptionNodeVisibility: vi.fn<ApiClient['setSubscriptionNodeVisibility']>(),
    parseSubscriptionNode: vi.fn<ApiClient['parseSubscriptionNode']>(),
    getSubscriptionNodeCatalog: vi.fn<ApiClient['getSubscriptionNodeCatalog']>().mockResolvedValue({
      applied_bundle_id: 'bundle_19',
      nodes: [],
      diagnostics: [],
    }),
    getSubscriptionUserGrants: vi.fn<ApiClient['getSubscriptionUserGrants']>(),
    replaceSubscriptionUserGrants: vi.fn<ApiClient['replaceSubscriptionUserGrants']>(),
    listSubscriptionSources: vi.fn<ApiClient['listSubscriptionSources']>().mockResolvedValue({
      items: testSubscriptionSources.map(({ config: _config, ...source }) => ({
        ...source,
        has_version: true,
      })),
    }),
    getSubscriptionSource: vi.fn<ApiClient['getSubscriptionSource']>().mockResolvedValue(testSubscriptionSources[0]),
    createSubscriptionSource: vi.fn<ApiClient['createSubscriptionSource']>().mockResolvedValue(testSubscriptionSources[0]),
    updateSubscriptionSource: vi.fn<ApiClient['updateSubscriptionSource']>().mockResolvedValue(testSubscriptionSources[0]),
    deleteSubscriptionSource: vi.fn<ApiClient['deleteSubscriptionSource']>().mockResolvedValue(undefined),
    refreshSubscriptionSource: vi.fn<ApiClient['refreshSubscriptionSource']>().mockResolvedValue({ source_id: testSubscriptionSources[0].id, version_id: testSubscriptionSources[0].current_version_id!, format: 'sing-box', sha256: 'a'.repeat(64), node_count: 0, fetched_at: '2026-09-21T00:00:00Z' }),
    listSubscriptionSourceVersions: vi.fn<ApiClient['listSubscriptionSourceVersions']>(),
    getSubscriptionSourceVersion: vi.fn<ApiClient['getSubscriptionSourceVersion']>(),
    createSubscriptionSourceVersion: vi.fn<ApiClient['createSubscriptionSourceVersion']>(),
    restoreSubscriptionSourceVersion: vi.fn<ApiClient['restoreSubscriptionSourceVersion']>(),
    listSubscriptionTokens: vi.fn<ApiClient['listSubscriptionTokens']>().mockResolvedValue({
      items: testSubscriptionTokens, total: testSubscriptionTokens.length,
    }),
    getSubscriptionTokenSecret: vi.fn<ApiClient['getSubscriptionTokenSecret']>().mockResolvedValue({ token: 'sample-subscription-token' }),
    getSubscriptionToken: vi.fn<ApiClient['getSubscriptionToken']>().mockResolvedValue(testSubscriptionTokens[0]),
    createSubscriptionToken: vi.fn<ApiClient['createSubscriptionToken']>().mockResolvedValue({
      metadata: { ...testSubscriptionTokens[0], id: 'token_new' },
      token: 'one-time-public-token',
    }),
    rotateSubscriptionToken: vi.fn<ApiClient['rotateSubscriptionToken']>().mockResolvedValue({
      revoked: { ...testSubscriptionTokens[0], active: false },
      created: { ...testSubscriptionTokens[0], id: 'token_rotated' },
      token: 'one-time-rotated-token',
    }),
    revokeSubscriptionToken: vi.fn<ApiClient['revokeSubscriptionToken']>()
      .mockResolvedValue({ ...testSubscriptionTokens[0], active: false }),
    setSubscriptionTokenEnabled: vi.fn<ApiClient['setSubscriptionTokenEnabled']>().mockResolvedValue(testSubscriptionTokens[0]),
    deleteSubscriptionToken: vi.fn<ApiClient['deleteSubscriptionToken']>().mockResolvedValue(undefined),
    listCoreLogFiles: vi.fn<ApiClient['listCoreLogFiles']>().mockResolvedValue({ items: [] }),
    deleteCoreLogFile: vi.fn<ApiClient['deleteCoreLogFile']>().mockResolvedValue(undefined),
    clearCoreLog: vi.fn<ApiClient['clearCoreLog']>().mockResolvedValue(undefined),
    readCoreLog: vi.fn<ApiClient['readCoreLog']>()
      .mockResolvedValue({ file: '2026-09-19-000.log', text: '', generation: 'test-generation', next_offset: 0, size: 0 }),
    streamCoreLog: vi.fn<ApiClient['streamCoreLog']>(async function* () {}),
    listPanelLogs: vi.fn<ApiClient['listPanelLogs']>().mockResolvedValue({ items: [], total: 0 }),
    listLogs: vi.fn<ApiClient['listLogs']>(),
    streamLogs: vi.fn<ApiClient['streamLogs']>(),
    getLog: vi.fn<ApiClient['getLog']>(),
    clearLogs: vi.fn<ApiClient['clearLogs']>(),
    deleteLog: vi.fn<ApiClient['deleteLog']>(),
    streamMetrics: vi.fn<ApiClient['streamMetrics']>(async function* (signal) {
      await new Promise<void>((resolve) => {
        if (signal?.aborted) resolve();
        else signal?.addEventListener('abort', () => resolve(), { once: true });
      });
    }),
    streamDashboard: vi.fn<ApiClient['streamDashboard']>(async function* (signal) {
      await new Promise<void>((resolve) => {
        if (signal?.aborted) resolve();
        else signal?.addEventListener('abort', () => resolve(), { once: true });
      });
    }),
    getMetrics: vi.fn<ApiClient['getMetrics']>().mockResolvedValue(testMetrics),
    getMetricsHistory: vi.fn<ApiClient['getMetricsHistory']>().mockResolvedValue(testMetricsHistory),
    getTrafficStatus: vi.fn<ApiClient['getTrafficStatus']>().mockResolvedValue(testMetrics),
    listTrafficPeriods: vi.fn<ApiClient['listTrafficPeriods']>(),
    getTrafficPeriod: vi.fn<ApiClient['getTrafficPeriod']>(),
  };
  return { ...client, ...overrides } as Mocked<ApiClient>;
}
