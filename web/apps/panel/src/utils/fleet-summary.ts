import type { FleetStatus, MetricsSnapshot } from '@/api/api-client';

export type NodeStatus = FleetStatus['items'][number];

/** The core version a node runs, or else the one it has enabled, as `v1.14.4`; null while it has neither. */
export function coreVersionLabel({ node }: NodeStatus): string | null {
  const exact = node.runtime?.running?.exact_core_version ?? node.runtime?.enabled_core?.exact_core_version;
  return exact ? `v${exact.replace(/^v/i, '')}` : null;
}

/** A share from 0 to 100, or null when either side is unknown. */
export function percentOf(used: number | null | undefined, total: number | null | undefined): number | null {
  if (used === null || used === undefined || !total) return null;
  return Math.min(100, Math.max(0, used / total * 100));
}

/** Upload and download bytes per second; null while unknown. */
export interface Rates {
  upload: number | null;
  download: number | null;
}

/**
 * A node's rates: its live sample's, a known zero while no core runs, and
 * unknown while a core runs without counters or the node is away.
 */
export function nodeRates({ metrics, node }: NodeStatus): Rates {
  const live = metrics.live_sample;
  if (live) return { download: live.download_bytes_per_second, upload: live.upload_bytes_per_second };
  const idle = node.state === 'online' && node.runtime?.observation_state === 'stopped';
  return idle ? { download: 0, upload: 0 } : { download: null, upload: null };
}

/** CPU, memory and disk shares while the node reports them; unknown while it is away. */
export interface HostLoad {
  cpu: number | null;
  disk: number | null;
  memory: number | null;
}

export function hostLoad({ metrics, node }: NodeStatus): HostLoad {
  if (node.state !== 'online') return { cpu: null, disk: null, memory: null };
  const host = metrics.host;
  return {
    cpu: host?.cpu_percent ?? null,
    disk: percentOf(host?.disk_used, host?.disk_total),
    memory: percentOf(host?.memory_used, host?.memory_total),
  };
}

/** Upload and download together in the node's current period. */
export function periodUsage(metrics: MetricsSnapshot): number {
  const period = metrics.current_traffic_period;
  return period.inbound_bytes + period.outbound_bytes;
}

export interface FleetFigures {
  /** Summed rates; a node with unknown rates makes the sum unknown only when no node reports any. */
  rates: Rates;
  total: number;
  online: number;
  /** Summed usage of every node's own current period; quotas are never summed. */
  upload: number;
  offline: number;
  /** Online nodes whose core runs, and whose core is stopped; every other node's core is unknown. */
  running: number;
  stopped: number;
  unknown: number;
  download: number;
  /** Nodes that reached their own quota. */
  exhausted: number;
  connections: number;
}

function sum(values: (number | null)[]): number | null {
  const known = values.filter((value): value is number => value !== null);
  return known.length === 0 ? null : known.reduce((total, value) => total + value, 0);
}

export function fleetFigures(items: readonly NodeStatus[]): FleetFigures {
  const rates = items.map(nodeRates);
  const online = items.filter(item => item.node.state === 'online');
  const running = online.filter(item => item.node.runtime?.observation_state === 'running').length;
  const stopped = online.filter(item => item.node.runtime?.observation_state === 'stopped').length;
  return {
    connections: items.reduce((total, item) => total + (item.metrics.live_sample?.active_connections ?? 0), 0),
    download: items.reduce((total, item) => total + item.metrics.current_traffic_period.inbound_bytes, 0),
    exhausted: items.filter(item => item.metrics.quota_exceeded).length,
    offline: items.filter(item => item.node.state === 'offline').length,
    online: online.length,
    rates: { download: sum(rates.map(rate => rate.download)), upload: sum(rates.map(rate => rate.upload)) },
    running,
    stopped,
    total: items.length,
    unknown: items.length - running - stopped,
    upload: items.reduce((total, item) => total + item.metrics.current_traffic_period.outbound_bytes, 0),
  };
}

/** What needs a look, most severe first. */
export type AttentionKind = 'offline' | 'quotaReached' | 'coreDown' | 'coreUnknown' | 'quotaNear' | 'diskNear' | 'memoryNear';

export interface Attention {
  item: NodeStatus;
  /** The share a "near" item is about. */
  percent?: number;
  kind: AttentionKind;
  tone: 'danger' | 'warning';
}

/** Shares from this percent up count as nearly full. */
const NEAR_FULL = 90;

const order: AttentionKind[] = ['offline', 'quotaReached', 'coreDown', 'coreUnknown', 'quotaNear', 'diskNear', 'memoryNear'];

export function attentionItems(items: readonly NodeStatus[]): Attention[] {
  const found: Attention[] = [];
  for (const item of items) {
    const { metrics, node } = item;
    if (node.state === 'offline') {
      found.push({ item, kind: 'offline', tone: 'danger' });
      continue;
    }
    const runtime = node.runtime;
    if (runtime?.desired_running && runtime.observation_state === 'stopped') found.push({ item, kind: 'coreDown', tone: 'danger' });
    if (runtime?.observation_state === 'stale' || runtime?.observation_state === 'inspection_unavailable') {
      found.push({ item, kind: 'coreUnknown', tone: 'warning' });
    }
    const quota = percentOf(periodUsage(metrics), metrics.quota_bytes);
    if (metrics.quota_exceeded) found.push({ item, kind: 'quotaReached', tone: 'danger' });
    else if (quota !== null && quota >= NEAR_FULL) found.push({ item, kind: 'quotaNear', percent: quota, tone: 'warning' });
    const disk = percentOf(metrics.host?.disk_used, metrics.host?.disk_total);
    if (disk !== null && disk >= NEAR_FULL) found.push({ item, kind: 'diskNear', percent: disk, tone: 'warning' });
    const memory = percentOf(metrics.host?.memory_used, metrics.host?.memory_total);
    if (memory !== null && memory >= NEAR_FULL) found.push({ item, kind: 'memoryNear', percent: memory, tone: 'warning' });
  }
  return found.sort((first, second) => order.indexOf(first.kind) - order.indexOf(second.kind));
}

/** The fleet's own values the backdrop mixes into its text. */
export function fleetWords(items: readonly NodeStatus[], formatRate: (value: number) => string): string[] {
  const words = new Set<string>();
  for (const { metrics, node } of items) {
    for (const value of [node.name, node.hostname, node.public_ip, node.build_version, `${node.os}/${node.arch}`,
      node.runtime?.enabled_core?.exact_core_version]) {
      if (value) words.add(value);
    }
    const rate = metrics.live_sample?.download_bytes_per_second;
    if (typeof rate === 'number' && rate > 0) words.add(formatRate(rate));
  }
  return [...words];
}
