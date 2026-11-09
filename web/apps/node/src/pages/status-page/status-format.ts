import type { Connection, ConsoleStatus, Enrollment, TrafficReading } from '@/api/generated';

/** The colours of each link state: its dot and its words. */
export const connectionTone: Record<Connection['state'], { dot: string; text: string }> = {
  connected: { dot: 'bg-[var(--color-success)]', text: 'text-[var(--color-success-text)]' },
  connecting: { dot: 'bg-[var(--color-info)]', text: 'text-[var(--color-info-text)]' },
  disconnected: { dot: 'bg-[var(--color-warning)]', text: 'text-[var(--color-warning-text)]' },
  unenrolled: { dot: 'bg-[var(--color-text-faint)]', text: 'text-muted-foreground' },
};

/** The panel the node is enrolled with: its node service address and base path. */
export function panelAddress(enrollment: Enrollment): string {
  return `${enrollment.panel_endpoint}${enrollment.panel_path}`;
}

export interface TrafficRates {
  upload: number;
  download: number;
}

/**
 * Rates between two live readings of the core's cumulative counters, over the
 * time between the readings. Null without two live readings, or when the
 * counters went back.
 */
export function trafficRates(previous: TrafficReading | null, next: TrafficReading | null): TrafficRates | null {
  if (!previous?.live || !next?.live) return null;
  const seconds = (Date.parse(next.live.sampled_at) - Date.parse(previous.live.sampled_at)) / 1_000;
  const upload = next.upload_bytes - previous.upload_bytes;
  const download = next.download_bytes - previous.download_bytes;
  if (!(seconds > 0) || upload < 0 || download < 0) return null;
  return { download: download / seconds, upload: upload / seconds };
}

/** A share of a total as a whole percentage; null when either is unknown. */
export function percentOf(used: number | null, total: number | null): number | null {
  if (used === null || total === null || total <= 0) return null;
  return Math.min(100, Math.max(0, (used / total) * 100));
}

/** The node's own values, which the field behind the terminal mixes into its text. */
export function nodeWords(status: ConsoleStatus): string[] {
  const { enrollment, node, runtime } = status;
  const words = [node.hostname, node.version, `${node.os}/${node.arch}`, node.os, node.arch, `protocol ${node.protocol}`];
  if (node.public_ip) words.push(node.public_ip);
  const core = runtime?.running_version ?? runtime?.enabled_version;
  if (core) words.push(core, `sing-box ${core}`);
  if (enrollment) {
    const port = /:(\d+)$/.exec(enrollment.panel_endpoint)?.[1];
    const { node_id: nodeId, panel_id: panelId } = enrollment;
    words.push(panelAddress(enrollment), nodeId, nodeId.slice(0, 9), nodeId.slice(-8), panelId);
    if (port) words.push(`:${port}`);
    words.push(enrollment.trust === 'pin' ? 'pinned' : 'system ca');
  }
  return words;
}
