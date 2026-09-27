import type UPlot from 'uplot';

import type { MetricsHistory } from '@/api/api-client';
import type { LivePoint } from '@/components/app-shell/use-telemetry';

// Historical buckets retain their full denominator and coverage semantics.
// Live samples are shown only beyond the persistence watermark, with a null
// separator instead of fabricating a link across the two sampling resolutions.
export function trendChartData(history: MetricsHistory | null, kind: 'traffic' | 'connections', tail: LivePoint[], watermark?: string | null): UPlot.AlignedData {
  const through = watermark ? Date.parse(watermark) : Number.NEGATIVE_INFINITY;
  const live = tail.filter(point => point.at > through);
  const first = live[0]?.at ?? Number.POSITIVE_INFINITY;
  const buckets = (history?.buckets ?? []).filter(bucket => Date.parse(bucket.to) < first - 1);
  const times = buckets.map(bucket => Date.parse(bucket.to));
  const down = buckets.map(bucket => kind === 'connections'
    ? bucket.active_connections_avg
    : bucket.download_bytes === null
      ? null
      : bucket.download_bytes / ((Date.parse(bucket.to) - Date.parse(bucket.from)) / 1000) / 1024);
  const up = buckets.map(bucket => bucket.upload_bytes === null
    ? null
    : bucket.upload_bytes / ((Date.parse(bucket.to) - Date.parse(bucket.from)) / 1000) / 1024);
  if (live.length) {
    times.push(first - 1);
    down.push(null);
    up.push(null);
    for (const point of live) {
      times.push(point.at);
      down.push(kind === 'connections'
        ? point.connections
        : point.downloadBytesPerSecond === null ? null : point.downloadBytesPerSecond / 1024);
      up.push(point.uploadBytesPerSecond === null ? null : point.uploadBytesPerSecond / 1024);
    }
  }
  return kind === 'traffic' ? [times, down, up] : [times, down];
}
