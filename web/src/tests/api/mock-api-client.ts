import type { Mocked } from 'vitest';

import { vi } from 'vitest';

import type {
  ApiClient,
  CanonicalSnapshot,
  CapabilityPresentation,
  CatalogAssetList,
  CoreArtifactPage,
  DashboardContext,
  EntityList,
  LogEntry,
  ManualArtifact,
  ManualReplacePreview,
  MetricsSnapshot,
  Session,
  SubscriptionChannel,
  SubscriptionSource,
  SubscriptionToken,
  Task,
  TrafficPeriod,
} from '@/api/api-client';

export const testSession: Session = {
  displayName: 'Panel administrator',
};

export const testDashboardContext: DashboardContext = {
  view: {
    exactVersion: '1.13.19',
  },
  running: {
    exactVersion: '1.13.19',
    artifactName: 'linux-amd64-musl',
    digest: 'sha256:8d5f2a1c7782e544',
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
  capability: {
    level: 'compatible_structured',
    label: 'Compatible structured',
    warning:
      'This capability matches the reported features but was not authored for this exact artifact.',
  },
};

export const testRevision: CanonicalSnapshot = {
  id: 'revision_42',
  sequence: 42,
  parent_id: 'revision_41',
  schema_version: 1,
  document: {
    schema_version: 1,
    global: {},
    nodes: [{ id: 'edge-socks', kind: 'socks', enabled: true }],
    rules: [{ id: 'private-network', enabled: true, ip_is_private: true }],
    subscription: {},
  },
  document_json: '{"schema_version":1,"global":{},"nodes":[{"enabled":true,"id":"edge-socks","kind":"socks"}],"rules":[{"enabled":true,"id":"private-network","ip_is_private":true}],"subscription":{}}',
  sha256: 'a'.repeat(64),
  created_at: '2026-08-26T07:30:00Z',
};

export const testCapabilityPresentation: CapabilityPresentation = {
  semantic_facts: [
    {
      id: 'global.mode',
      canonical_path: '/global/mode',
      classification: 'behavior_changed',
      owned_paths: ['/mode'],
    },
    {
      id: 'global.note',
      canonical_path: '/global/note',
      classification: 'supported',
      owned_paths: ['/note'],
    },
  ],
  ui: [
    {
      id: 'global.group',
      fact_id: '',
      kind: 'group',
      label: 'Global behavior',
      help: 'Fields declared by the pinned exact-version manifest.',
      order: 10,
    },
    {
      id: 'global.mode.select',
      fact_id: 'global.mode',
      kind: 'select',
      label: 'Routing mode',
      help: 'Choose how unmatched traffic is handled.',
      order: 20,
      options: [
        { value: 'direct', label: 'Direct' },
        { value: 'block', label: 'Block' },
      ],
    },
    {
      id: 'global.note.text',
      fact_id: 'global.note',
      kind: 'text',
      label: 'Operator note',
      order: 30,
      visible_when: { canonical_path: '/global/mode', equals_json: '"direct"' },
    },
  ],
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
      name: 'sing-box-1.13.19-linux-amd64.tar.gz',
      download_url: 'https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64.tar.gz',
      size: 12_000_000,
      version: '1.13.19',
      os: 'linux',
      arch: 'amd64',
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
      arch: 'amd64',
      variant: 'plain',
      source_kind: 'official',
      repository_id: 509091576,
      release_id: 101,
      asset_id: 201,
      archive_sha256: 'b'.repeat(64),
      binary_sha256: 'd'.repeat(64),
      binary_path: '/var/lib/sing-box-panel/artifacts/core_1/sing-box',
      reported_version: '1.13.19',
      feature_fingerprint: { features: [] },
      verification_state: 'verified',
      created_at: '2026-08-26T07:22:00Z',
    },
  ],
  next: undefined,
};

export const testManualArtifact: ManualArtifact = {
  id: 'startup_manual_1',
  canonical_revision_id: testRevision.id,
  exact_core_version: '1.13.19',
  core_artifact_id: 'core_1',
  config_sha256: 'c'.repeat(64),
  raw: '{\n  // exact operator bytes\n  "log": {"level": "warn"}\n}\n',
  diagnostics: [{ code: 'manual_json_exact_bytes' }],
  state: 'ready',
  checked_at: '2026-08-26T07:31:00Z',
  created_at: '2026-08-26T07:30:00Z',
};

export const testManualReplacePreview: ManualReplacePreview = {
  resolution: { exact_version: '1.13.19', source: 'explicit' },
  base: testRevision,
  core_artifact_id: 'core_1',
  config_sha256: 'c'.repeat(64),
  reverse: {
    available: false,
    reason_code: 'capability_pin_unavailable',
    owned_partial: {},
    proposed_canonical: testRevision.document,
    residual_paths: ['/log/level'],
    canonical_changed: false,
  },
};

export const testSubscriptionChannels: SubscriptionChannel[] = [
  {
    id: 'channel_sing_box',
    name: 'Primary sing-box',
    format: 'sing-box',
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
    latest_snapshot: { outbounds: [] },
    enabled: true,
    created_at: '2026-08-26T07:00:00Z',
    updated_at: '2026-08-26T07:06:00Z',
  },
];

export const testSubscriptionTokens: SubscriptionToken[] = [
  {
    id: 'token_primary',
    channel_id: 'channel_sing_box',
    created_at: '2026-08-26T07:10:00Z',
    active: true,
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
  monitoring_tier: 'full',
  collected_at: '2026-08-26T07:34:00Z',
  current_traffic_period: testTrafficPeriod,
};

export function createMockApiClient(
  overrides: Partial<ApiClient> = {},
): Mocked<ApiClient> {
  return {
    getSession: vi.fn().mockResolvedValue(testSession),
    login: vi.fn().mockResolvedValue(testSession),
    logout: vi.fn().mockResolvedValue(undefined),
    getDashboardContext: vi.fn().mockResolvedValue(testDashboardContext),
    listEntities: vi.fn().mockImplementation((collection: 'nodes' | 'rules') =>
      Promise.resolve({
        revision: testRevision,
        entities: testRevision.document[collection],
      } satisfies EntityList),
    ),
    saveEntity: vi.fn().mockResolvedValue({
      revision: testRevision,
      no_change: false,
    }),
    getCanonical: vi.fn().mockResolvedValue(testRevision),
    replaceCanonical: vi.fn().mockResolvedValue({
      revision: testRevision,
      no_change: false,
    }),
    patchCanonical: vi.fn().mockResolvedValue({
      revision: testRevision,
      no_change: false,
    }),
    listRevisions: vi.fn().mockResolvedValue({ items: [testRevision] }),
    listTasks: vi.fn().mockResolvedValue({ items: [testTask] }),
    cancelTask: vi.fn().mockResolvedValue({ ...testTask, cancel_requested: true }),
    listCatalogAssets: vi.fn().mockResolvedValue(testCatalog),
    refreshCatalog: vi.fn().mockResolvedValue({ ...testTask, status: 'queued' }),
    listCoreArtifacts: vi.fn().mockResolvedValue(testArtifacts),
    installCore: vi.fn().mockResolvedValue({
      ...testTask,
      id: 'task_core_install',
      kind: 'core-install',
      status: 'queued',
    }),
    removeCoreArtifact: vi.fn().mockResolvedValue(undefined),
    quarantineCoreArtifact: vi.fn().mockImplementation(async (artifactID: string) => ({
      ...testArtifacts.items.find((artifact) => artifact.id === artifactID)!,
      verification_state: 'quarantined' as const,
    })),
    revokeCoreArtifact: vi.fn().mockImplementation(async (artifactID: string) => ({
      ...testArtifacts.items.find((artifact) => artifact.id === artifactID)!,
      verification_state: 'revoked' as const,
    })),
    getCoreCapability: vi.fn().mockResolvedValue({
      resolution: { exact_version: '1.13.19', source: 'explicit' },
      support_level: 'compatible_structured',
      pinned: true,
      quarantined: false,
      presentation: testCapabilityPresentation,
    }),
    renderStructured: vi.fn().mockResolvedValue({
      resolution: { exact_version: '1.13.19', source: 'explicit' },
      pin: {
        repository: 'rehuony/sing-box-panel',
        commit_sha: 'd'.repeat(40),
        exact_core_version: '1.13.19',
        manifest_sha256: 'e'.repeat(64),
        support_level: 'compatible_structured',
        pinned_at: '2026-08-26T07:00:00Z',
      },
      artifact: {
        id: 'startup_structured_1',
        canonical_revision_id: testRevision.id,
        exact_core_version: '1.13.19',
        capability_commit: 'd'.repeat(40),
        capability_digest: 'e'.repeat(64),
        renderer_version: 'capability-projector-v1',
        core_artifact_id: 'core_1',
        config_sha256: 'f'.repeat(64),
        config: { log: { level: 'warn' } },
        diagnostics: [],
        state: 'pending',
      },
      task: { ...testTask, id: 'task_startup_check', kind: 'startup-check', status: 'queued' },
    }),
    listStartupArtifacts: vi.fn().mockResolvedValue({
      items: [{
        id: testManualArtifact.id,
        kind: 'manual',
        canonical_revision_id: testManualArtifact.canonical_revision_id,
        exact_core_version: testManualArtifact.exact_core_version,
        renderer_version: 'manual-v1',
        core_artifact_id: testManualArtifact.core_artifact_id,
        config_sha256: testManualArtifact.config_sha256,
        diagnostics: [],
        state: testManualArtifact.state,
        checked_at: testManualArtifact.checked_at,
        created_at: testManualArtifact.created_at,
      }],
    }),
    listManualArtifacts: vi.fn().mockResolvedValue({
      resolution: { exact_version: '1.13.19', source: 'explicit' },
      items: [testManualArtifact],
    }),
    getManualArtifact: vi.fn().mockResolvedValue(testManualArtifact),
    previewManualReplacement: vi.fn().mockResolvedValue(testManualReplacePreview),
    replaceManualArtifact: vi.fn().mockResolvedValue({
      resolution: { exact_version: '1.13.19', source: 'explicit' },
      preview: testManualReplacePreview,
      revision: testRevision,
      no_change: true,
      artifact: { ...testManualArtifact, id: 'startup_manual_2', state: 'pending' },
      task: { ...testTask, id: 'task_manual_check', kind: 'startup-check', status: 'queued' },
    }),
    discardManualArtifact: vi.fn().mockResolvedValue({ ...testManualArtifact, state: 'stale' }),
    activateStartupArtifact: vi.fn().mockResolvedValue({
      activation: {
        startup_artifact_id: testManualArtifact.id,
        canonical_revision_id: testRevision.id,
        exact_core_version: '1.13.19',
        core_artifact_id: 'core_1',
        config_sha256: testManualArtifact.config_sha256,
        subscription_snapshot_id: 'subscription_18',
        subscription_sha256: '1'.repeat(64),
        activation_bundle_id: 'bundle_19',
        activation_sha256: '2'.repeat(64),
        monitoring_tier: 'full',
      },
      task: { ...testTask, id: 'task_apply', kind: 'runtime-apply', status: 'queued' },
    }),
    previewManualReattach: vi.fn().mockResolvedValue({
      evidence: {
        startup_artifact_id: testManualArtifact.id,
        config_sha256: testManualArtifact.config_sha256,
        base_revision_id: testRevision.id,
        base_revision_sha256: testRevision.sha256,
        current_head_id: testRevision.id,
        current_head_sha256: testRevision.sha256,
        exact_core_version: '1.13.19',
        core_artifact_id: 'core_1',
        capability: {
          repository: 'rehuony/sing-box-panel',
          commit_sha: 'd'.repeat(40),
          manifest_sha256: 'e'.repeat(64),
          support_level: 'native_structured',
        },
      },
      base: testRevision,
      current: testRevision,
      manual: testRevision.document,
      owned_partial: {},
      merged: testRevision.document,
      residual_paths: [],
      conflicts: [],
    }),
    applyManualReattach: vi.fn().mockResolvedValue({
      preview: {} as never,
      revision: testRevision,
      artifact: testManualArtifact,
      task: { ...testTask, id: 'task_reattach', kind: 'startup-check', status: 'queued' },
    }),
    listSubscriptionChannels: vi.fn().mockResolvedValue(testSubscriptionChannels),
    createSubscriptionChannel: vi.fn().mockResolvedValue(testSubscriptionChannels[0]),
    updateSubscriptionChannel: vi.fn().mockResolvedValue(testSubscriptionChannels[0]),
    deleteSubscriptionChannel: vi.fn().mockResolvedValue(undefined),
    listSubscriptionSources: vi.fn().mockResolvedValue(testSubscriptionSources),
    createSubscriptionSource: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    updateSubscriptionSource: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    updateSubscriptionSourceSnapshot: vi.fn().mockResolvedValue(testSubscriptionSources[0]),
    deleteSubscriptionSource: vi.fn().mockResolvedValue(undefined),
    listSubscriptionTokens: vi.fn().mockResolvedValue(testSubscriptionTokens),
    createSubscriptionToken: vi.fn().mockResolvedValue({
      metadata: { ...testSubscriptionTokens[0], id: 'token_new' },
      token: 'one-time-public-token',
    }),
    rotateSubscriptionToken: vi.fn().mockResolvedValue({
      revoked: { ...testSubscriptionTokens[0], active: false, revoked_at: '2026-08-26T08:00:00Z' },
      created: { ...testSubscriptionTokens[0], id: 'token_rotated' },
      token: 'one-time-rotated-token',
    }),
    revokeSubscriptionToken: vi.fn().mockResolvedValue({
      ...testSubscriptionTokens[0],
      active: false,
      revoked_at: '2026-08-26T08:00:00Z',
    }),
    listLogs: vi.fn().mockResolvedValue({ items: [testLogEntry] }),
    getLog: vi.fn().mockResolvedValue(testLogEntry),
    clearLogs: vi.fn().mockResolvedValue({ deleted: 1 }),
    deleteLog: vi.fn().mockImplementation(async (entryID: string) => ({ id: entryID, deleted: true as const })),
    getMetrics: vi.fn().mockResolvedValue(testMetrics),
    getTrafficStatus: vi.fn().mockResolvedValue(testMetrics),
    listTrafficPeriods: vi.fn().mockResolvedValue({ items: [testTrafficPeriod] }),
    getTrafficPeriod: vi.fn().mockResolvedValue(testTrafficPeriod),
    ...overrides,
  } as Mocked<ApiClient>;
}
