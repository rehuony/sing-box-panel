import type {
  CanonicalSnapshot,
  CatalogAssetList,
  CoreArtifact,
  DashboardContext,
  LogEntry,
  MetricsHistory,
  MetricsSnapshot,
  RuntimeHistoryPage,
  RuntimeStatus,
  StartupArtifactSummary,
  SubscriptionChannel,
  SubscriptionNodeCatalog,
  SubscriptionSource,
  SubscriptionSourceVersion,
  SubscriptionToken,
  SubscriptionUser,
  SystemStatus,
  Task,
  TrafficPeriod,
} from '../api-client';

import { demoSourceNodeDetails, nodeSummary } from './demo-subscription-nodes';

export interface DemoData {
  tasks: Task[];
  logs: LogEntry[];
  cores: CoreArtifact[];
  runtime: RuntimeStatus;
  catalog: CatalogAssetList;
  users: SubscriptionUser[];
  tokens: SubscriptionToken[];
  canonical: CanonicalSnapshot;
  grants: Map<string, string[]>;
  sources: SubscriptionSource[];
  channels: SubscriptionChannel[];
  trafficPeriods: TrafficPeriod[];
  runtimeHistory: RuntimeHistoryPage;
  startupArtifacts: StartupArtifactSummary[];
  sourceVersions: Map<string, SubscriptionSourceVersion[]>;
}

export const demoCanonicalDocument = {
  log: { level: 'info', timestamp: true },
  dns: {
    servers: [
      { tag: 'cloudflare', type: 'https', server: '1.1.1.1' },
      { tag: 'local', type: 'local' },
    ],
    strategy: 'prefer_ipv4',
  },
  inbounds: [{
    type: 'mixed',
    tag: 'mixed-in',
    listen: '127.0.0.1',
    listen_port: 2080,
  }],
  outbounds: [
    { type: 'direct', tag: 'direct' },
    { type: 'block', tag: 'block' },
  ],
  route: {
    rules: [
      { action: 'reject', rule_set: ['geosite-category-ads-all'] },
      { action: 'route', outbound: 'direct', ip_is_private: true },
    ],
    final: 'direct',
  },
};

const digest = (character: string): string => character.repeat(64);

function ago(now: Date, minutes: number): string {
  return new Date(now.getTime() - minutes * 60_000).toISOString();
}

export function createDemoData(now = new Date()): DemoData {
  const canonicalJSON = JSON.stringify(demoCanonicalDocument);
  const canonical: CanonicalSnapshot = {
    id: 'revision_demo_7',
    sequence: 7,
    parent_id: 'revision_demo_6',
    schema_version: 1,
    document: structuredClone(demoCanonicalDocument),
    document_json: canonicalJSON,
    sha256: digest('7'),
    created_at: ago(now, 14),
  };
  const cores: CoreArtifact[] = [{
    id: 'core_demo_114',
    exact_version: '1.14.0',
    os: 'linux',
    arch: 'amd64',
    variant: 'with_quic',
    source_kind: 'official',
    repository_id: 509091576,
    release_id: 214000,
    asset_id: 114001,
    archive_sha256: digest('a'),
    binary_sha256: digest('b'),
    binary_path: '/var/lib/sing-box-panel/artifacts/core_demo_114/sing-box',
    reported_version: '1.14.0',
    feature_fingerprint: { features: ['with_quic', 'with_wireguard'], source: 'demo' },
    verification_state: 'verified',
    created_at: ago(now, 180),
  }, {
    id: 'core_demo_113',
    exact_version: '1.13.19',
    os: 'linux',
    arch: 'amd64',
    variant: 'plain',
    source_kind: 'official',
    repository_id: 509091576,
    release_id: 213019,
    asset_id: 113019,
    archive_sha256: digest('c'),
    binary_sha256: digest('d'),
    binary_path: '/var/lib/sing-box-panel/artifacts/core_demo_113/sing-box',
    reported_version: '1.13.19',
    feature_fingerprint: { features: ['with_quic'], source: 'demo' },
    verification_state: 'verified',
    created_at: ago(now, 2_880),
  }];
  const startupArtifacts: StartupArtifactSummary[] = [{
    id: 'startup_demo_ready',
    canonical_revision_id: canonical.id,
    exact_core_version: '1.14.0',
    core_artifact_id: cores[0].id,
    config_sha256: digest('e'),
    state: 'ready',
    checked_at: ago(now, 12),
    created_at: ago(now, 13),
  }, {
    id: 'startup_demo_previous',
    canonical_revision_id: canonical.parent_id ?? canonical.id,
    exact_core_version: '1.13.19',
    core_artifact_id: cores[1].id,
    config_sha256: digest('f'),
    state: 'ready',
    checked_at: ago(now, 1_430),
    created_at: ago(now, 1_440),
  }];
  const tasks: Task[] = [{
    id: 'task_demo_apply',
    lane: 'runtime',
    kind: 'runtime-apply',
    status: 'succeeded',
    generation: 4,
    canonical_revision_id: canonical.id,
    startup_artifact_id: startupArtifacts[0].id,
    activation_bundle_id: 'bundle_demo_current',
    payload: { monitoring_tier: 'limited' },
    result: { state: 'running' },
    cancel_requested: false,
    attempt: 1,
    created_at: ago(now, 11),
    updated_at: ago(now, 10),
  }, {
    id: 'task_demo_catalog',
    lane: 'maintenance',
    kind: 'catalog-refresh',
    status: 'succeeded',
    generation: 0,
    payload: { force: false },
    result: { asset_count: 3 },
    cancel_requested: false,
    attempt: 1,
    created_at: ago(now, 95),
    updated_at: ago(now, 94),
  }];
  const channels: SubscriptionChannel[] = [{
    id: 'channel_demo_singbox',
    name: 'sing-box devices',
    format: 'sing-box',
    public_host: 'panel.demo.local',
    config: { exclude_tags: ['block'] },
    enabled: true,
    created_at: ago(now, 4_320),
    updated_at: ago(now, 80),
  }, {
    id: 'channel_demo_mihomo',
    name: 'Mihomo clients',
    format: 'mihomo',
    public_host: 'panel.demo.local',
    config: {},
    enabled: true,
    created_at: ago(now, 2_880),
    updated_at: ago(now, 65),
  }];
  const users: SubscriptionUser[] = [{
    id: 'user_demo_primary',
    name: 'Primary devices',
    description: 'Laptop and phone',
    enabled: true,
    created_at: ago(now, 5_760),
    updated_at: ago(now, 75),
  }, {
    id: 'user_demo_guest',
    name: 'Guest',
    description: 'Limited access profile',
    enabled: false,
    created_at: ago(now, 1_440),
    updated_at: ago(now, 120),
  }];
  const sources: SubscriptionSource[] = [{
    id: 'source_demo_remote',
    name: 'Remote provider',
    source_kind: 'remote',
    config: { url: 'https://subscription.example/demo', refresh_interval_minutes: 60 },
    current_version_id: 'source_version_demo_2',
    enabled: true,
    created_at: ago(now, 5_760),
    updated_at: ago(now, 35),
  }, {
    id: 'source_demo_local',
    name: 'Operator additions',
    source_kind: 'local',
    config: {},
    current_version_id: 'source_version_demo_local',
    enabled: true,
    created_at: ago(now, 2_880),
    updated_at: ago(now, 50),
  }];
  const sourceVersions = new Map<string, SubscriptionSourceVersion[]>([
    [sources[0].id, [{
      id: 'source_version_demo_2', source_id: sources[0].id, format: 'mihomo-yaml',
      raw_body: 'proxies:\n  - name: Tokyo demo\n    type: trojan',
      normalized_nodes: [
        { type: 'trojan', tag: 'Tokyo demo', server: 'tokyo.example', server_port: 443 },
        { type: 'hysteria2', tag: 'Seattle demo', server: 'sea.example', server_port: 443 },
      ],
      diagnostics: [], sha256: digest('2'), fetched_at: ago(now, 35), created_at: ago(now, 35),
    }, {
      id: 'source_version_demo_1', source_id: sources[0].id, format: 'mihomo-yaml',
      normalized_nodes: [{ type: 'trojan', tag: 'Tokyo demo' }],
      diagnostics: [], sha256: digest('1'), fetched_at: ago(now, 1_475), created_at: ago(now, 1_475),
    }]],
    [sources[1].id, [{
      id: 'source_version_demo_local', source_id: sources[1].id, format: 'sing-box-json',
      raw_body: '{"outbounds":[{"type":"direct","tag":"Home LAN"}]}',
      normalized_nodes: [{ type: 'direct', tag: 'Home LAN' }], diagnostics: [],
      sha256: digest('3'), fetched_at: ago(now, 50), created_at: ago(now, 50),
    }]],
  ]);
  const tokens: SubscriptionToken[] = [{
    id: 'token_demo_phone',
    user_id: users[0].id,
    label: 'Phone',
    enabled: true,
    successful_request_count: 32,
    body_response_count: 31,
    bytes_served: 582_401,
    last_used_at: ago(now, 8),
    created_at: ago(now, 2_880),
    active: true,
  }];
  const logs: LogEntry[] = [{
    id: 'log_demo_runtime', time: ago(now, 10), source: 'core', level: 'info',
    code: 'runtime.ready', message: 'sing-box 1.14.0 passed its health check.',
    metadata: { activation_bundle_id: 'bundle_demo_current', pid: 4281 },
  }, {
    id: 'log_demo_config', time: ago(now, 12), source: 'task', level: 'info',
    code: 'configuration.checked', message: 'The startup configuration is ready to activate.',
    metadata: { startup_artifact_id: startupArtifacts[0].id },
  }, {
    id: 'log_demo_login', time: ago(now, 40), source: 'security', level: 'warn',
    code: 'session.rotated', message: 'An administrator session was renewed.',
    metadata: { address: '127.0.0.1' },
  }, {
    id: 'log_demo_catalog', time: ago(now, 94), source: 'panel', level: 'debug',
    code: 'catalog.refreshed', message: 'Core release catalog refreshed.',
    metadata: { asset_count: 3 },
  }];
  const periodStart = ago(now, 60);
  const trafficPeriods: TrafficPeriod[] = [{
    id: 'traffic_demo_current',
    activation_bundle_id: 'bundle_demo_current',
    period_start: periodStart,
    period_end: now.toISOString(),
    inbound_bytes: 268_435_456,
    outbound_bytes: 92_274_688,
    counters: { inbound: { mixed: 268_435_456 }, outbound: { direct: 92_274_688 } },
    created_at: now.toISOString(),
  }];
  return {
    canonical,
    catalog: {
      validator: 'demo-catalog-v1',
      refreshed_at: ago(now, 94),
      assets: [{
        repository_id: 509091576, release_id: 214000, asset_id: 114001,
        name: 'sing-box-1.14.0-linux-amd64.tar.gz',
        download_url: 'https://github.com/SagerNet/sing-box/releases/download/v1.14.0/sing-box-1.14.0-linux-amd64.tar.gz',
        size: 15_728_640, version: '1.14.0', os: 'linux', arch: 'amd64', variant: 'with_quic',
        api_digest: digest('a'), catalog_digest: digest('a'), has_api_digest: true, has_catalog_digest: true,
      }, {
        repository_id: 509091576, release_id: 214000, asset_id: 114002,
        name: 'sing-box-1.14.0-linux-arm64.tar.gz',
        download_url: 'https://github.com/SagerNet/sing-box/releases/download/v1.14.0/sing-box-1.14.0-linux-arm64.tar.gz',
        size: 14_680_064, version: '1.14.0', os: 'linux', arch: 'arm64', variant: 'with_quic',
        api_digest: digest('4'), catalog_digest: digest('4'), has_api_digest: true, has_catalog_digest: true,
      }, {
        repository_id: 509091576, release_id: 213019, asset_id: 113019,
        name: 'sing-box-1.13.19-linux-amd64.tar.gz',
        download_url: 'https://github.com/SagerNet/sing-box/releases/download/v1.13.19/sing-box-1.13.19-linux-amd64.tar.gz',
        size: 14_155_776, version: '1.13.19', os: 'linux', arch: 'amd64', variant: 'plain',
        has_api_digest: false, has_catalog_digest: true, catalog_digest: digest('c'),
      }],
    },
    channels,
    cores,
    grants: new Map([
      [users[0].id, [
        'source_demo_remote:0',
        'source_demo_remote:1',
        'source_demo_local:0',
      ]],
      [users[1].id, []],
    ]),
    logs,
    runtime: {
      desired_running: true,
      desired_bundle_id: 'bundle_demo_current',
      applied_bundle_id: 'bundle_demo_current',
      rollback_bundle_id: 'bundle_demo_previous',
      target_generation: 4,
      observation_state: 'running',
      running: {
        pid: 4281,
        process_start_token: 'demo-process-4281',
        exact_core_version: '1.14.0',
        core_artifact_id: cores[0].id,
        archive_sha256: cores[0].archive_sha256,
        binary_sha256: cores[0].binary_sha256,
        activation_bundle_id: 'bundle_demo_current',
        started_at: ago(now, 10),
      },
    },
    runtimeHistory: {
      items: [{
        id: 4, state: 'running', reason: 'apply_succeeded', activation_bundle_id: 'bundle_demo_current',
        generation: 4, task_id: tasks[0].id, pid: 4281, process_started_at: ago(now, 10),
        occurred_at: ago(now, 10),
      }, {
        id: 3, state: 'stopped', reason: 'restart_requested', activation_bundle_id: 'bundle_demo_previous',
        generation: 3, occurred_at: ago(now, 1_440),
      }],
      preceding: {
        id: 2, state: 'running', reason: 'start_succeeded', activation_bundle_id: 'bundle_demo_previous',
        generation: 2, occurred_at: ago(now, 2_880),
      },
      history_started_at: ago(now, 5_760),
    },
    sources,
    sourceVersions,
    startupArtifacts,
    tasks,
    tokens,
    trafficPeriods,
    users,
  };
}

export function demoSystemStatus(data: DemoData): SystemStatus {
  return {
    platform: { os: 'linux', arch: 'amd64' },
    panel_version: 'demo',
    canonical_revision: data.canonical.sequence,
    applied_bundle_id: data.runtime.applied_bundle_id ?? null,
    running: data.runtime.observation_state === 'running',
    running_version: data.runtime.running?.exact_core_version ?? null,
    running_artifact: data.runtime.running?.core_artifact_id ?? null,
    configuration_state: data.runtime.running?.exact_core_version === '1.14.0'
      ? 'schema@1.14.0'
      : 'raw',
  };
}

export function demoDashboardContext(data: DemoData): DashboardContext {
  const running = data.runtime.running;
  return {
    view: { exactVersion: running?.exact_core_version ?? '1.14.0' },
    running: running === undefined
      ? null
      : {
          exactVersion: running.exact_core_version,
          artifactName: running.core_artifact_id,
          digest: running.binary_sha256.slice(0, 16),
        },
    canonical: {
      revision: data.canonical.sequence,
      savedAt: data.canonical.created_at,
      hasUnappliedChanges: data.canonical.id !== data.startupArtifacts[0]?.canonical_revision_id,
    },
    applied: data.runtime.applied_bundle_id === undefined
      ? null
      : {
          bundle: data.runtime.applied_bundle_id,
          revision: data.canonical.sequence,
          appliedAt: data.runtime.running?.started_at ?? data.canonical.created_at,
        },
    configuration: {
      supported: running?.exact_core_version === '1.14.0',
      label: running?.exact_core_version === '1.14.0' ? 'Schema 1.14.0' : 'Raw JSON',
      warning: running?.exact_core_version === '1.14.0'
        ? null
        : 'Structured editing is available for sing-box 1.14 and newer.',
    },
  };
}

export function demoMetrics(data: DemoData, now = new Date()): MetricsSnapshot {
  const running = data.runtime.observation_state === 'running';
  const period = data.trafficPeriods[0];
  return {
    host: {
      sampled_at: now.toISOString(), cpu_count: 4, cpu_percent: 12.8, load_one: 0.64,
      memory_total: 965004492, memory_used: 639797978, disk_total: 26306674688, disk_used: 8207682503,
    },
    available: running,
    reason_code: running ? undefined : 'no_collector_sample',
    applied_bundle_id: data.runtime.applied_bundle_id,
    monitoring_tier: 'limited',
    collected_at: now.toISOString(),
    current_traffic_period: period,
    latest_sample: running
      ? {
          id: Math.floor(now.getTime() / 10_000),
          activation_bundle_id: data.runtime.applied_bundle_id ?? 'bundle_demo_current',
          pid: data.runtime.running?.pid ?? 0,
          process_start_token: data.runtime.running?.process_start_token ?? 'demo-stopped',
          sampled_at: now.toISOString(),
          memory_bytes: 68_157_440,
          active_connections: 18,
          upload_total: period?.outbound_bytes ?? 0,
          download_total: period?.inbound_bytes ?? 0,
          upload_delta: 124_820,
          download_delta: 486_210,
          coverage: 'complete',
          accepted: true,
        }
      : undefined,
    traffic_available: running,
    quota_bytes: 1_099_511_627_776,
    quota_exceeded: false,
  };
}

export function demoMetricsHistory(from: string, to: string, bucketSeconds: number): MetricsHistory {
  const start = new Date(from).getTime();
  const end = new Date(to).getTime();
  const step = Math.max(1, bucketSeconds) * 1_000;
  const buckets: MetricsHistory['buckets'] = [];
  for (let cursor = start, index = 0; cursor < end && index < 240; cursor += step, index += 1) {
    const next = Math.min(end, cursor + step);
    const wave = (Math.sin(index / 2.4) + 1.4) / 2.4;
    buckets.push({
      from: new Date(cursor).toISOString(),
      to: new Date(next).toISOString(),
      upload_bytes: Math.round(1_800_000 + wave * 4_200_000),
      download_bytes: Math.round(5_200_000 + wave * 12_000_000),
      memory_bytes_avg: Math.round(61_000_000 + wave * 8_000_000),
      memory_bytes_peak: Math.round(68_000_000 + wave * 9_000_000),
      active_connections_avg: Math.round(8 + wave * 14),
      active_connections_peak: Math.round(14 + wave * 20),
      sample_count: Math.max(1, Math.round((next - cursor) / 10_000)),
      coverage: index % 11 === 10 ? 'partial' : 'complete',
    });
  }
  return { from, to, bucket_seconds: bucketSeconds, activation_bundle_id: 'bundle_demo_current', buckets };
}

export function demoNodeCatalog(data: DemoData): SubscriptionNodeCatalog {
  return {
    applied_bundle_id: data.runtime.applied_bundle_id ?? '',
    nodes: demoSourceNodeDetails(data).map(nodeSummary),
    diagnostics: [],
  };
}
