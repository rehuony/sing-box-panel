import { describe, expect, it } from 'vitest';

import { testMetricsHistory } from '@/tests/api/mock-api-client';
import { trendChartData } from '@/pages/dashboard-page/trend-chart-data';
import { deriveTrafficRates } from '@/components/app-shell/use-telemetry';

const sample = {
  activation_bundle_id: 'bundle', pid: 42, process_start_token: 'process',
  sampled_at: '2026-08-26T07:40:00Z', upload_total: 100, download_total: 200,
  memory_bytes: 1, active_connections: 2, accepted: true,
};

describe('live traffic evidence', () => {
  it('uses real sampling times and rejects gaps, restarts, rollbacks and invalid evidence', () => {
    const next = { ...sample, sampled_at: '2026-08-26T07:40:02Z', upload_total: 120, download_total: 240 };
    expect(deriveTrafficRates(sample, next)).toEqual({ uploadBytesPerSecond: 10, downloadBytesPerSecond: 20 });
    for (const invalid of [sample, { ...next, sampled_at: '2026-08-26T07:40:10Z' },
      { ...next, pid: 43 }, { ...next, process_start_token: 'restart' }, { ...next, activation_bundle_id: 'new' },
      { ...next, upload_total: 1 }, { ...next, accepted: false }]) {
      expect(deriveTrafficRates(sample, invalid).uploadBytesPerSecond).toBeNull();
    }
  });
  it('replaces overlap at the persisted watermark and keeps explicit gaps', () => {
    const at = Date.parse(testMetricsHistory.to);
    const tail = [
      { at, connections: 2, uploadBytesPerSecond: 1024, downloadBytesPerSecond: 2048 },
      { at: at + 2000, connections: null, uploadBytesPerSecond: null, downloadBytesPerSecond: null },
      { at: at + 4000, connections: 3, uploadBytesPerSecond: 2048, downloadBytesPerSecond: 4096 },
    ];
    const first = trendChartData(testMetricsHistory, 'traffic', tail, testMetricsHistory.to);
    expect(first[0]).not.toContain(at + 1);
    expect(first[1].slice(-3)).toEqual([null, null, 4]);
    const persisted = trendChartData(testMetricsHistory, 'traffic', tail, new Date(at + 4000).toISOString());
    expect(persisted[0]).toEqual(testMetricsHistory.buckets.map(bucket => Date.parse(bucket.to)));
    expect(persisted[1].at(-1)).toBeNull();
  });
});
