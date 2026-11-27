import type { Mocked } from 'vitest';

import { vi } from 'vitest';

import type {
  ApiClient,
  CanonicalSnapshot,
  CatalogAssetList,
  ConfigurationSupport,
  CoreArtifactPage,
  DashboardStreamSnapshot,
  FleetNode,
  FleetStatus,
  MetricsHistory,
  MetricsSnapshot,
  NodeSettings,
  RuntimeHistoryPage,
  RuntimeStatus,
  Session,
  StartupArtifactSummary,
  SubscriptionChannel,
  SubscriptionChannelSummary,
  SubscriptionRuleSet,
  SubscriptionSource,
  SubscriptionToken,
  SystemUpdate,
  TrafficPeriod,
} from '@/api/api-client';

import { DEFAULT_APPEARANCE } from '@/theme/appearance';
import { configurationSchemaContract } from '@/tests/fixtures/configuration-schemas';

export const testSession: Session = { email: 'admin@example.com', csrfToken: 'csrf-test', expiresAt: '2099-01-01T00:00:00Z', displayName: 'Panel administrator' };

export const testNode: FleetNode = {
  id: 'node_edge',
  name: 'edge',
  state: 'online',
  created_at: '2026-08-20T08:00:00Z',
  enrolled_at: '2026-08-20T08:05:00Z',
  last_seen_at: '2026-08-26T07:12:00Z',
  hostname: 'edge',
  os: 'linux',
  arch: 'arm64',
  build_version: '0.1.0',
  tags: [],
  update_available: false,
  last_update: null,
};

export const testSystemUpdate: SystemUpdate = {
  current_version: 'v1.0.0',
  supported: true,
  latest_version: 'v1.1.0',
  checked_at: '2026-08-26T07:00:00Z',
  check_failure: null,
  update_available: true,
  state: 'idle',
  target_version: null,
  last_attempt: null,
};

export const testNodeSettings: NodeSettings = {
  revision: 1,
  public_host: '',
  traffic: { quota_gib: null, period_months: 1, reset_day: 1 },
  tags: [],
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
    config: { groups: [], routes: [], default_exit: { kind: 'direct' } },
    enabled: true,
    created_at: '2026-08-26T07:00:00Z',
    updated_at: '2026-08-26T07:05:00.000000001Z',
  },
];

/** The channels as the list reads them: an empty channel publishes no nodes and exits DIRECT. */
export const testSubscriptionChannelSummaries: SubscriptionChannelSummary[] = testSubscriptionChannels.map(channel => ({
  id: channel.id, name: channel.name, format: channel.format, enabled: channel.enabled,
  node_count: 0, route_count: 0, final_exit: { kind: 'direct' },
  created_at: channel.created_at, updated_at: channel.updated_at,
}));

export const testSubscriptionRuleSets: SubscriptionRuleSet[] = [
  {
    id: 'rule_set_streaming',
    client: 'mihomo',
    icon: 'tv',
    name: 'Streaming',
    rules: [
      { kind: 'domain_suffix', value: 'netflix.com' },
      { kind: 'remote', remote: { name: 'streaming', url: 'https://rules.example/streaming.mrs', format: 'mrs', behavior: 'domain', accelerated: false } },
    ],
    channels: [],
    created_at: '2026-08-26T07:00:00Z',
    updated_at: '2026-08-26T07:00:00.000000001Z',
  },
  {
    id: 'rule_set_direct',
    client: 'sing-box',
    icon: 'layers',
    name: 'Direct',
    rules: [{ kind: 'domain_suffix', value: 'cn' }],
    channels: [],
    created_at: '2026-08-26T07:00:00Z',
    updated_at: '2026-08-26T07:00:00.000000001Z',
  },
];

export const testSubscriptionSources: SubscriptionSource[] = [
  {
    id: 'source_file',
    name: 'Operator additions',
    kind: 'file',
    format: 'auto',
    filter: { include: [], exclude: [] },
    node_count: 2,
    content_format: 'share-links',
    content_at: '2026-08-26T07:06:00Z',
    created_at: '2026-08-26T07:00:00Z',
    updated_at: '2026-08-26T07:06:00.000000001Z',
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
  current_traffic_period: testTrafficPeriod,
  quota_exceeded: false,
};

export const testFleetStatus: FleetStatus = { items: [{ node: testNode, metrics: testMetrics }] };

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
    nodeID: null,
    forNode: vi.fn<ApiClient['forNode']>(),
    listFleetNodes: vi.fn<ApiClient['listFleetNodes']>().mockResolvedValue({ items: [testNode] }),
    getFleetStatus: vi.fn<ApiClient['getFleetStatus']>().mockResolvedValue(testFleetStatus),
    createNode: vi.fn<ApiClient['createNode']>(),
    renameNode: vi.fn<ApiClient['renameNode']>(),
    deleteNode: vi.fn<ApiClient['deleteNode']>(),
    createNodeEnrollment: vi.fn<ApiClient['createNodeEnrollment']>(),
    cancelNodeEnrollment: vi.fn<ApiClient['cancelNodeEnrollment']>(async () => undefined),
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
    getPanelSettings: vi.fn<ApiClient['getPanelSettings']>().mockResolvedValue({ admin_email: 'admin@example.com',
      revision: 0,
      service: { data_dir: '/var/lib/sing-box-panel', base_path: '', catalog_refresh_interval_hours: 12, sample_retention_days: 90, private_source_cidrs: [] },
      github_token_configured: false,
      restart_required: false,
      preferences: {
        listen_host: '127.0.0.1',
        listen_port: 3000,
        external_origin: '',
        language: 'en',
        appearance: { ...DEFAULT_APPEARANCE },
      },
    }),
    savePanelSettings: vi.fn<ApiClient['savePanelSettings']>().mockImplementation(async (input) => ({ reauthentication_required: false, settings: { admin_email: 'admin@example.com',
      revision: input.revision + 1,
      preferences: input.preferences,
      service: input.service ?? { data_dir: '/var/lib/sing-box-panel', base_path: '', catalog_refresh_interval_hours: 12, sample_retention_days: 90, private_source_cidrs: [] },
      github_token_configured: Boolean(input.github_token),
      restart_required: false,
    } })),
    subscribeSessionInvalidated: vi.fn<ApiClient['subscribeSessionInvalidated']>().mockReturnValue(() => undefined),
    getSession: vi.fn<ApiClient['getSession']>().mockResolvedValue(testSession),
    login: vi.fn<ApiClient['login']>().mockResolvedValue(testSession),
    logout: vi.fn<ApiClient['logout']>().mockResolvedValue(undefined),

    listCatalogAssets: vi.fn<ApiClient['listCatalogAssets']>().mockResolvedValue(testCatalog),
    refreshCatalog: vi.fn<ApiClient['refreshCatalog']>().mockResolvedValue({
      refreshed_at: testCatalog.refreshed_at, not_modified: false, releases: 1, assets: 1,
    }),
    listCoreArtifacts: vi.fn<ApiClient['listCoreArtifacts']>().mockResolvedValue(testArtifacts),
    installCore: vi.fn<ApiClient['installCore']>().mockResolvedValue(testArtifacts.items[0]),
    importCoreArchive: vi.fn<ApiClient['importCoreArchive']>().mockResolvedValue(testArtifacts.items[0]),
    removeCoreArtifact: vi.fn<ApiClient['removeCoreArtifact']>().mockResolvedValue(undefined),
    getConfigurationSchema: vi.fn<ApiClient['getConfigurationSchema']>(async () =>
      configurationSchemaContract(testArtifacts.items[0].exact_version)),
    compileConfiguration: vi.fn<ApiClient['compileConfiguration']>().mockResolvedValue({
      support,
      artifact: testStartupArtifact,
    }),
    getRuntimeStatus: vi.fn<ApiClient['getRuntimeStatus']>().mockResolvedValue(testRuntimeStatus),
    startRuntime: vi.fn<ApiClient['startRuntime']>().mockResolvedValue(testRuntimeStatus),
    stopRuntime: vi.fn<ApiClient['stopRuntime']>().mockResolvedValue({ desired_running: false, target_generation: 2, observation_state: 'stopped' }),
    restartRuntime: vi.fn<ApiClient['restartRuntime']>().mockResolvedValue(testRuntimeStatus),
    rollbackRuntime: vi.fn<ApiClient['rollbackRuntime']>(),
    listSubscriptionChannels: vi.fn<ApiClient['listSubscriptionChannels']>().mockResolvedValue({ items: testSubscriptionChannelSummaries }),
    getSubscriptionChannel: vi.fn<ApiClient['getSubscriptionChannel']>().mockResolvedValue(testSubscriptionChannels[0]),
    createSubscriptionChannel: vi.fn<ApiClient['createSubscriptionChannel']>().mockResolvedValue(testSubscriptionChannels[0]),
    updateSubscriptionChannel: vi.fn<ApiClient['updateSubscriptionChannel']>().mockResolvedValue(testSubscriptionChannels[0]),
    deleteSubscriptionChannel: vi.fn<ApiClient['deleteSubscriptionChannel']>().mockResolvedValue(undefined),
    previewSubscriptionChannel: vi.fn<ApiClient['previewSubscriptionChannel']>().mockResolvedValue({
      user_id: 'user_1',
      channel: testSubscriptionChannels[0],
      node_publications: [],
      result: {
        format: 'sing-box',
        media_type: 'application/json',
        content: '',
        node_count: 0,
        diagnostics: [],
      },
    }),
    listSubscriptionNodes: vi.fn<ApiClient['listSubscriptionNodes']>().mockResolvedValue({ items: [], node_publications: [], diagnostics: [] }),
    setSubscriptionNodeOrder: vi.fn<ApiClient['setSubscriptionNodeOrder']>().mockResolvedValue({ items: [], node_publications: [], diagnostics: [] }),
    getSubscriptionNode: vi.fn<ApiClient['getSubscriptionNode']>(),
    updateSubscriptionNode: vi.fn<ApiClient['updateSubscriptionNode']>(),
    listSubscriptionSources: vi.fn<ApiClient['listSubscriptionSources']>().mockResolvedValue({ items: testSubscriptionSources }),
    getSubscriptionSource: vi.fn<ApiClient['getSubscriptionSource']>().mockResolvedValue(testSubscriptionSources[0]),
    createSubscriptionSource: vi.fn<ApiClient['createSubscriptionSource']>().mockResolvedValue(testSubscriptionSources[0]),
    updateSubscriptionSource: vi.fn<ApiClient['updateSubscriptionSource']>().mockResolvedValue(testSubscriptionSources[0]),
    deleteSubscriptionSource: vi.fn<ApiClient['deleteSubscriptionSource']>().mockResolvedValue(undefined),
    refreshSubscriptionSource: vi.fn<ApiClient['refreshSubscriptionSource']>().mockResolvedValue(testSubscriptionSources[0]),
    previewSubscriptionSource: vi.fn<ApiClient['previewSubscriptionSource']>().mockResolvedValue({ items: [] }),
    listSubscriptionRuleSets: vi.fn<ApiClient['listSubscriptionRuleSets']>().mockResolvedValue({ items: testSubscriptionRuleSets }),
    setSubscriptionRuleSetOrder: vi.fn<ApiClient['setSubscriptionRuleSetOrder']>().mockResolvedValue(undefined),
    createSubscriptionRuleSet: vi.fn<ApiClient['createSubscriptionRuleSet']>().mockImplementation(async input => ({
      ...input, icon: input.icon ?? 'layers', id: 'rule_set_new', channels: [], created_at: '2026-08-26T08:00:00Z', updated_at: '2026-08-26T08:00:00.000000001Z',
    })),
    updateSubscriptionRuleSet: vi.fn<ApiClient['updateSubscriptionRuleSet']>().mockImplementation(async (id, input) => ({
      ...testSubscriptionRuleSets.find(set => set.id === id)!, ...input, updated_at: '2026-08-26T08:00:00.000000001Z',
    })),
    deleteSubscriptionRuleSet: vi.fn<ApiClient['deleteSubscriptionRuleSet']>().mockResolvedValue(undefined),
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
    setSubscriptionTokenEnabled: vi.fn<ApiClient['setSubscriptionTokenEnabled']>().mockResolvedValue(testSubscriptionTokens[0]),
    deleteSubscriptionToken: vi.fn<ApiClient['deleteSubscriptionToken']>().mockResolvedValue(undefined),
    listCoreLogFiles: vi.fn<ApiClient['listCoreLogFiles']>().mockResolvedValue({ items: [] }),
    deleteCoreLogFile: vi.fn<ApiClient['deleteCoreLogFile']>().mockResolvedValue(undefined),
    clearCoreLog: vi.fn<ApiClient['clearCoreLog']>().mockResolvedValue(undefined),
    readCoreLog: vi.fn<ApiClient['readCoreLog']>()
      .mockResolvedValue({ file: '2026-09-19-000.log', text: '', generation: 'test-generation', next_offset: 0, size: 0 }),
    streamCoreLog: vi.fn<ApiClient['streamCoreLog']>(async function* () {}),
    listPanelLogs: vi.fn<ApiClient['listPanelLogs']>().mockResolvedValue({ items: [], total: 0 }),
    getLogUsage: vi.fn<ApiClient['getLogUsage']>().mockResolvedValue({ entries: 0, bytes: 0 }),
    clearPanelLogs: vi.fn<ApiClient['clearPanelLogs']>().mockResolvedValue({ deleted: 0, freed_bytes: 0 }),
    listFleetRuntimeHistory: vi.fn<ApiClient['listFleetRuntimeHistory']>().mockResolvedValue({ items: [], total: 0 }),
    streamMetrics: vi.fn<ApiClient['streamMetrics']>(async function* (signal) {
      await new Promise<void>((resolve) => {
        if (signal?.aborted) resolve();
        else signal?.addEventListener('abort', () => resolve(), { once: true });
      });
    }),
    streamFleetStatus: vi.fn<ApiClient['streamFleetStatus']>(async function* (signal) {
      await new Promise<void>((resolve) => {
        if (signal?.aborted) resolve();
        else signal?.addEventListener('abort', () => resolve(), { once: true });
      });
    }),
    runFleetCommand: vi.fn<ApiClient['runFleetCommand']>(async function* (command) {
      const ids = command.node_ids ?? [];
      for (const id of ids) yield { result: { node_id: id, ok: true } };
      yield { done: { failed: 0, skipped: 0, succeeded: ids.length, total: ids.length } };
    }),
    streamDashboard: vi.fn<ApiClient['streamDashboard']>(async function* (signal) {
      await new Promise<void>((resolve) => {
        if (signal?.aborted) resolve();
        else signal?.addEventListener('abort', () => resolve(), { once: true });
      });
    }),
    getMetrics: vi.fn<ApiClient['getMetrics']>().mockResolvedValue(testMetrics),
    getRecentMetrics: vi.fn<ApiClient['getRecentMetrics']>().mockResolvedValue({
      from: '2026-09-19T00:00:00Z', to: '2026-09-19T00:02:00Z', samples: [], idle: [],
    }),
    getNodeSettings: vi.fn<ApiClient['getNodeSettings']>().mockResolvedValue(testNodeSettings),
    saveNodeSettings: vi.fn<ApiClient['saveNodeSettings']>(async input => ({ ...input, revision: input.revision + 1 })),
    getSystemUpdate: vi.fn<ApiClient['getSystemUpdate']>().mockResolvedValue(testSystemUpdate),
    getSystemStatus: vi.fn<ApiClient['getSystemStatus']>().mockResolvedValue({
      platform: { os: 'linux', arch: 'amd64' }, panel_version: testSystemUpdate.current_version, started_at: '2026-09-19T00:00:00Z',
    }),
    restartPanel: vi.fn<ApiClient['restartPanel']>().mockResolvedValue({ started_at: '2026-09-19T00:00:00Z' }),
    checkSystemUpdate: vi.fn<ApiClient['checkSystemUpdate']>().mockResolvedValue(testSystemUpdate),
    startSystemUpdate: vi.fn<ApiClient['startSystemUpdate']>().mockResolvedValue({ ...testSystemUpdate, state: 'updating' }),
    updateNode: vi.fn<ApiClient['updateNode']>().mockResolvedValue({ node_id: 'node-a', version: 'v1.1.0', state: 'restarting' }),
  };
  const result = { ...client, ...overrides } as Mocked<ApiClient>;
  // A node's client shares the mocks, so tests observe node-scoped calls.
  if (!overrides.forNode) result.forNode.mockImplementation(nodeID => ({ ...result, nodeID }));
  return result;
}
