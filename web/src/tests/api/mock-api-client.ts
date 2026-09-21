import type { Mocked } from 'vitest';

import { vi } from 'vitest';

import type {
  ApiClient,
  CanonicalSnapshot,
  CatalogAssetList,
  ConfigurationSupport,
  CoreArtifactPage,
  DashboardContext,
  LogEntry,
  LogStreamEvent,
  MetricsHistory,
  MetricsSnapshot,
  RuntimeHistoryPage,
  Session,
  StartupArtifactSummary,
  SubscriptionChannel,
  SubscriptionSource,
  SubscriptionSourceVersion,
  SubscriptionToken,
  SystemStatus,
  Task,
  TrafficPeriod,
} from '@/api/api-client';

import { DEFAULT_APPEARANCE } from '@/theme/appearance';
import { reviewedSchemaManifest } from '@/schemas/generated';

const testSchemaVersion = '1.14.0';
const unavailableSchemaSHA256 = '0'.repeat(64);

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

export const testTask: Task = {
  id: 'task_catalog_refresh',
  lane: 'maintenance',
  kind: 'catalog-refresh',
  status: 'succeeded',
  generation: 0,
  payload: {},
  cancel_requested: false,
  attempt: 1,
  created_at: '2026-08-26T07:20:00Z',
  updated_at: '2026-08-26T07:21:00Z',
};

export const testCatalog: CatalogAssetList = {
  validator: 'catalog-v1',
  refreshed_at: '2026-08-26T07:21:00Z',
  assets: [
    {
      repository_id: 509091576,
      release_id: 101,
      asset_id: 201,
      name: 'sing-box-1.13.19-linux-arm64.tar.gz',
      download_url:
        'https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-arm64.tar.gz',
      size: 12_000_000,
      version: '1.13.19',
      os: 'linux',
      arch: 'arm64',
      variant: 'plain',
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
      variant: 'plain',
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

export const testSubscriptionSourceVersion: SubscriptionSourceVersion = {
  id: 'version_1',
  source_id: 'source_local',
  format: 'sing-box-json',
  normalized_nodes: [],
  diagnostics: [],
  sha256: 'a'.repeat(64),
  fetched_at: '2026-08-26T07:00:00Z',
  created_at: '2026-08-26T07:00:00Z',
};

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

export const testSubscriptionUsers = [
  {
    id: 'user_1',
    name: 'Primary user',
    description: 'Personal devices',
    enabled: true,
    created_at: '2026-08-26T07:00:00Z',
    updated_at: '2026-08-26T07:00:00Z',
  },
];

export const testLogEntry: LogEntry = {
  id: 'log_runtime_ready',
  time: '2026-08-26T07:32:00Z',
  source: 'core',
  level: 'info',
  code: 'runtime.ready',
  message: 'The exact core process passed its health check.',
  metadata: { exact_version: '1.13.19', activation_bundle_id: 'bundle_18' },
};

async function* testLogStream(): AsyncGenerator<LogStreamEvent> {
  yield { id: `${testLogEntry.time}|${testLogEntry.id}`, entry: testLogEntry };
}

export const testTrafficPeriod: TrafficPeriod = {
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
      task_id: 'task_apply',
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

export function createMockApiClient(overrides: Partial<ApiClient> = {}): Mocked<ApiClient> {
  const support = {
    structured: false,
    exact_version: '1.13.19',
    reason: 'Native configuration Schema is unavailable before sing-box 1.14.',
  } satisfies ConfigurationSupport;
  const client: ApiClient = {
    newInboundDefaults: vi.fn().mockImplementation(async (type) => ({ type })),
    enableCore: vi.fn().mockResolvedValue(testTask),
    disableCore: vi.fn().mockResolvedValue(testTask),
    getConfigurationFile: vi.fn().mockResolvedValue({
      revision: 1,
      content: testRevision.document_json,
      canonical_revision_id: testRevision.id,
      syntax_valid: true,
    }),
    saveConfigurationFile: vi.fn().mockImplementation(async (input) => {
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
    getPanelSettings: vi.fn().mockResolvedValue({
      revision: 0,
      github_token_configured: false,
      identity_key_configured: false,
      restart_required: false,
      preferences: {
        listen_host: '127.0.0.1',
        listen_port: 3000,
        external_origin: '',
        public_node_host: '',
        identity_name: '',
        traffic_quota_gib: null,
        language: 'en',
        appearance: { ...DEFAULT_APPEARANCE },
      },
    }),
    savePanelSettings: vi.fn().mockImplementation(async (input) => ({
      revision: input.revision + 1,
      preferences: input.preferences,
      github_token_configured: Boolean(input.github_token),
      identity_key_configured: Boolean(input.identity_key),
      restart_required: false,
    })),
    subscribeSessionInvalidated: vi.fn().mockReturnValue(() => undefined),
    getSession: vi.fn().mockResolvedValue(testSession),
    getSystemStatus: vi.fn().mockResolvedValue(testSystemStatus),
    login: vi.fn().mockResolvedValue(testSession),
    logout: vi.fn().mockResolvedValue(undefined),
    getDashboardContext: vi.fn().mockResolvedValue(testDashboardContext),
    getCanonical: vi.fn().mockResolvedValue(testRevision),
    replaceCanonical: vi.fn().mockResolvedValue({ revision: testRevision, no_change: false }),
    patchCanonical: vi.fn().mockResolvedValue({ revision: testRevision, no_change: false }),
    listRevisions: vi.fn().mockResolvedValue({ items: [testRevision] }),
    getRevision: vi.fn().mockResolvedValue(testRevision),
    diffRevisions: vi.fn().mockResolvedValue({
      from: testRevision,
      to: testRevision,
      changes: [],
    }),
    restoreRevision: vi.fn().mockResolvedValue({ revision: testRevision, no_change: false }),
    listCatalogAssets: vi.fn().mockResolvedValue(testCatalog),
    refreshCatalog: vi.fn().mockResolvedValue({ ...testTask, status: 'queued' }),
    listCoreArtifacts: vi.fn().mockResolvedValue(testArtifacts),
    getCoreArtifact: vi.fn().mockResolvedValue(testArtifacts.items[0]),
    installCore: vi.fn().mockResolvedValue({
      ...testTask,
      id: 'task_core_install',
      kind: 'core-install',
      status: 'queued',
    }),
    importCoreArchive: vi.fn().mockResolvedValue({
      ...testTask,
      id: 'task_core_import',
      kind: 'core-import',
    }),
    removeCoreArtifact: vi.fn().mockResolvedValue(undefined),
    getConfigurationSupport: vi.fn().mockResolvedValue(support),
    getConfigurationSchema: vi.fn(async () => {
      const reviewed = await reviewedSchemaManifest[testSchemaVersion]?.load();
      return {
        exact_version: testSchemaVersion,
        schema_sha256: reviewed?.schemaSHA256 ?? unavailableSchemaSHA256,
        schema: reviewed?.schema ?? {},
      };
    }),
    previewConfiguration: vi.fn().mockResolvedValue({
      canonical_revision: testRevision,
      core_artifact: testArtifacts.items[0],
      support,
      config: { log: { level: 'info' } },
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
        exact_core_version: '1.13.19',
        core_artifact_id: 'core_1',
        config_sha256: testStartupArtifact.config_sha256,
        activation_bundle_id: 'bundle_19',
        activation_sha256: '2'.repeat(64),
        monitoring_tier: 'process_only',
      },
      task: { ...testTask, id: 'task_apply', kind: 'runtime-apply', status: 'queued' },
    }),
    getRuntimeStatus: vi.fn().mockResolvedValue({
      desired_running: true,
      target_generation: 1,
      observation_state: 'running',
    }),
    getRuntimeHistory: vi.fn().mockResolvedValue(testRuntimeHistory),
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
    listSubscriptionUsers: vi.fn().mockResolvedValue({ items: testSubscriptionUsers }),
    getSubscriptionUser: vi.fn().mockResolvedValue(testSubscriptionUsers[0]),
    createSubscriptionUser: vi.fn().mockResolvedValue(testSubscriptionUsers[0]),
    updateSubscriptionUser: vi.fn().mockResolvedValue(testSubscriptionUsers[0]),
    deleteSubscriptionUser: vi.fn().mockResolvedValue(undefined),
    getSubscriptionNode: vi.fn(),
    createSubscriptionNode: vi.fn(),
    updateSubscriptionNode: vi.fn(),
    deleteSubscriptionNode: vi.fn(),
    setSubscriptionNodeVisibility: vi.fn(),
    parseSubscriptionNode: vi.fn(),
    getSubscriptionNodeCatalog: vi.fn().mockResolvedValue({
      applied_bundle_id: 'bundle_19',
      nodes: [],
      diagnostics: [],
    }),
    getSubscriptionUserGrants: vi
      .fn()
      .mockResolvedValue({ user: testSubscriptionUsers[0], grants: [] }),
    replaceSubscriptionUserGrants: vi.fn().mockResolvedValue({
      user: testSubscriptionUsers[0],
      grants: [],
    }),
    listSubscriptionSources: vi.fn().mockResolvedValue({
      items: testSubscriptionSources.map(({ config: _config, ...source }) => ({
        ...source,
        has_version: true,
      })),
    }),
    getSubscriptionSource: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    createSubscriptionSource: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    updateSubscriptionSource: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    deleteSubscriptionSource: vi.fn().mockResolvedValue(undefined),
    refreshSubscriptionSource: vi.fn().mockResolvedValue(testTask),
    listSubscriptionSourceVersions: vi
      .fn()
      .mockResolvedValue({ items: [testSubscriptionSourceVersion] }),
    getSubscriptionSourceVersion: vi.fn().mockResolvedValue(testSubscriptionSourceVersion),
    createSubscriptionSourceVersion: vi.fn().mockResolvedValue({
      source: testSubscriptionSources[0],
      version: testSubscriptionSourceVersion,
    }),
    restoreSubscriptionSourceVersion: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    listSubscriptionTokens: vi.fn().mockResolvedValue({ items: testSubscriptionTokens }),
    getSubscriptionTokenSecret: vi.fn().mockResolvedValue({ token: 'sample-subscription-token' }),
    getSubscriptionToken: vi.fn().mockResolvedValue(testSubscriptionTokens[0]),
    createSubscriptionToken: vi.fn().mockResolvedValue({
      metadata: { ...testSubscriptionTokens[0], id: 'token_new' },
      token: 'one-time-public-token',
    }),
    rotateSubscriptionToken: vi.fn().mockResolvedValue({
      revoked: { ...testSubscriptionTokens[0], active: false },
      created: { ...testSubscriptionTokens[0], id: 'token_rotated' },
      token: 'one-time-rotated-token',
    }),
    revokeSubscriptionToken: vi
      .fn()
      .mockResolvedValue({ ...testSubscriptionTokens[0], active: false }),
    setSubscriptionTokenEnabled: vi.fn().mockResolvedValue(testSubscriptionTokens[0]),
    deleteSubscriptionToken: vi.fn().mockResolvedValue(undefined),
    listCoreLogFiles: vi.fn().mockResolvedValue({ items: [] }),
    readCoreLog: vi
      .fn()
      .mockResolvedValue({ file: '2026-09-19-000.log', text: '', next_offset: 0, size: 0 }),
    streamCoreLog: vi.fn(async function* () {}),
    listPanelLogs: vi.fn().mockResolvedValue({ items: [] }),
    retryTask: vi.fn().mockResolvedValue(testTask),
    listLogs: vi.fn().mockResolvedValue({ items: [testLogEntry] }),
    streamLogs: vi.fn(testLogStream),
    getLog: vi.fn().mockResolvedValue(testLogEntry),
    clearLogs: vi.fn().mockResolvedValue({ deleted: 1 }),
    deleteLog: vi.fn().mockImplementation(async (entryID: string) => ({
      id: entryID,
      deleted: true as const,
    })),
    streamMetrics: vi.fn(async function* (signal) {
      await new Promise<void>((resolve) => {
        if (signal?.aborted) resolve();
        else signal?.addEventListener('abort', () => resolve(), { once: true });
      });
    }),
    getMetrics: vi.fn().mockResolvedValue(testMetrics),
    getMetricsHistory: vi.fn().mockResolvedValue(testMetricsHistory),
    getTrafficStatus: vi.fn().mockResolvedValue(testMetrics),
    listTrafficPeriods: vi.fn().mockResolvedValue({ items: [testTrafficPeriod] }),
    getTrafficPeriod: vi.fn().mockResolvedValue(testTrafficPeriod),
  };
  return { ...client, ...overrides } as Mocked<ApiClient>;
}
