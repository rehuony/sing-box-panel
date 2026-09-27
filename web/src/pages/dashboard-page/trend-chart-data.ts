import type UPlot from 'uplot';

import type { MetricsHistory } from '@/api/api-client';
import type { LivePoint } from '@/components/app-shell/use-telemetry';

export interface TrendChartData {
  live: boolean;
  unit: string | null;
  to: number | undefined;
  data: UPlot.AlignedData;
  from: number | undefined;
}

// Keep the full historical bucket denominator, even for partial coverage.
// A bucket crossing the watermark is anchored at its last persisted sample,
// so adjacent live samples can join it without dropping the entire bucket.
export function trendChartData(history: MetricsHistory | null, kind: 'traffic' | 'connections', tail: LivePoint[], watermark?: string | null): TrendChartData {
  const through = watermark ? Date.parse(watermark) : Number.NEGATIVE_INFINITY;
  const from = history ? Date.parse(history.from) : undefined;
  const live = tail.filter(point => point.at > through && (from === undefined || point.at >= from));
  const first = live[0]?.at ?? Number.POSITIVE_INFINITY;
  const boundary = live.length && Number.isFinite(through) ? through : Number.POSITIVE_INFINITY;
  const buckets = (history?.buckets ?? []).filter(bucket => Number.isFinite(boundary)
    ? Date.parse(bucket.from) < boundary
    : Date.parse(bucket.to) < first);
  const times = buckets.map(bucket => Math.min(Date.parse(bucket.to), boundary));
  const down = buckets.map(bucket => kind === 'connections'
    ? bucket.active_connections_avg
    : bucket.download_bytes === null
      ? null
      : bucket.download_bytes / ((Date.parse(bucket.to) - Date.parse(bucket.from)) / 1000));
  const up = buckets.map(bucket => bucket.upload_bytes === null
    ? null
    : bucket.upload_bytes / ((Date.parse(bucket.to) - Date.parse(bucket.from)) / 1000));
  if (live.length) {
    for (const point of live) {
      const previous = times.at(-1);
      // Match live rate derivation's three-second continuity limit. Null
      // samples already preserve reconnects, restarts and unavailable metrics.
      if (previous !== undefined && point.at - previous > 3000) {
        times.push(point.at - 1);
        down.push(null);
        up.push(null);
      }
      times.push(point.at);
      down.push(kind === 'connections'
        ? point.connections
        : point.downloadBytesPerSecond);
      up.push(point.uploadBytesPerSecond);
    }
  }
  let unit: string | null = null;
  if (kind === 'traffic') {
    const units = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s', 'PB/s'];
    let peak = 0;
    for (const series of [down, up]) {
      for (const value of series) peak = Math.max(peak, value ?? 0);
    }
    let index = 0;
    while (peak >= 1024 && index < units.length - 1) {
      peak /= 1024;
      index++;
    }
    const divisor = 1024 ** index;
    unit = units[index]!;
    for (const series of [down, up]) {
      for (let i = 0; i < series.length; i++) {
        if (series[i] !== null) series[i] = series[i]! / divisor;
      }
    }
  }
  return {
    data: kind === 'traffic' ? [times, down, up] : [times, down],
    unit,
    from: from ?? times[0],
    to: history ? Math.max(Date.parse(history.to), times.at(-1) ?? 0) : times.at(-1),
    live: live.some(point => kind === 'connections'
      ? point.connections !== null
      : point.downloadBytesPerSecond !== null || point.uploadBytesPerSecond !== null),
  };
}
