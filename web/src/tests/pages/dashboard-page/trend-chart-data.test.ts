import { describe, expect, it } from 'vitest';

import type { LivePoint } from '@/components/app-shell/use-telemetry';

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
    expect(first.data[0]).not.toContain(at + 1);
    expect(first.data[1].slice(-2)).toEqual([null, 4]);
    const persisted = trendChartData(testMetricsHistory, 'traffic', tail, new Date(at + 4000).toISOString());
    expect(persisted.data[0]).toEqual(testMetricsHistory.buckets.map(bucket => Date.parse(bucket.to)));
    expect(persisted.data[1].at(-1)).toBeNull();
  });
});

describe('history and live chart continuity', () => {
  const end = Date.parse(testMetricsHistory.to);
  const history = {
    ...testMetricsHistory,
    buckets: testMetricsHistory.buckets.map(bucket => ({
      ...bucket, download_bytes: 307_200, upload_bytes: 614_400,
      active_connections_avg: 12, coverage: 'partial' as const,
    })),
  };
  const point = (at: number): LivePoint => ({
    at, connections: 15, downloadBytesPerSecond: 4096, uploadBytesPerSecond: 2048,
  });

  it.each(['traffic', 'connections'] as const)('joins %s to the overlapping history bucket', kind => {
    const through = end - 6000;
    const tail = [point(through), point(through + 2000), point(through + 4000), point(end)];
    const chart = trendChartData(history, kind, tail, new Date(through).toISOString());
    expect(chart.data[0].slice(0, 3)).toEqual([Date.parse(history.buckets[0]!.to), through, through + 2000]);
    // Keep the full 300-second denominator, rather than inflating a partial bucket.
    expect(chart.data[1].slice(0, 3)).toEqual(kind === 'traffic' ? [1, 1, 4] : [12, 12, 15]);
    expect(chart.data[0].filter(at => at === through)).toHaveLength(1);
    expect(chart.data[1]).not.toContain(null);
  });

  it('does not connect across a delayed live tail or fill missing historical buckets', () => {
    const chart = trendChartData(testMetricsHistory, 'traffic', [point(end + 30_000)], testMetricsHistory.to);
    expect(chart.data[1]).toEqual([2048 / 300 / 1024, null, null, 4]);
    expect(chart.data[0].at(-2)).toBe(end + 29_999);
  });

  it('preserves a reconnect or process-change null immediately after the watermark', () => {
    const chart = trendChartData(history, 'connections', [
      { at: end + 2000, connections: null, uploadBytesPerSecond: null, downloadBytesPerSecond: null },
      point(end + 4000),
    ], history.to);
    expect(chart.data[1]).toEqual([12, 12, null, 15]);
  });

  it('replaces live points when history catches up and keeps timestamps strictly increasing', () => {
    const tail = [point(end - 2000), point(end), point(end + 2000)];
    const before = trendChartData(history, 'traffic', tail, new Date(end - 4000).toISOString());
    expect(before.data[1]).toEqual([1, 1, 4, 4, 4]);
    const after = trendChartData(history, 'traffic', tail, history.to);
    expect(after.data[1]).toEqual([1, 1, 4]);
    expect(after.data[0]).toEqual([Date.parse(history.buckets[0]!.to), end, end + 2000]);
  });

  it('keeps only non-overlapping history without a persistence watermark', () => {
    const chart = trendChartData(history, 'traffic', [point(end - 2000)], null);
    expect(chart.data[0]).toEqual([Date.parse(history.buckets[0]!.to), end - 2001, end - 2000]);
  });
});

describe('adaptive traffic units', () => {
  it.each([
    [0, 'B/s', 0],
    [512, 'B/s', 512],
    [1024, 'KB/s', 1],
    [1024 ** 2, 'MB/s', 1],
    [2.5 * 1024 ** 3, 'GB/s', 2.5],
    [1024 ** 4, 'TB/s', 1],
  ] as const)('scales both directions for a peak of %s bytes/s', (rate, unit, scaled) => {
    const chart = trendChartData(null, 'traffic', [{
      at: 1000, connections: 2, downloadBytesPerSecond: rate / 2, uploadBytesPerSecond: rate,
    }]);
    expect(chart.unit).toBe(unit);
    expect(chart.data).toEqual([[1000], [scaled / 2], [scaled]]);
  });

  it('chooses units from the selected history and live samples, preserving zeros and nulls', () => {
    const history = {
      ...testMetricsHistory,
      buckets: [
        { ...testMetricsHistory.buckets[0]!, download_bytes: 300 * 1024 ** 3, upload_bytes: 0 },
        testMetricsHistory.buckets[1]!,
      ],
    };
    const chart = trendChartData(history, 'traffic', []);
    expect(chart.unit).toBe('GB/s');
    expect(chart.data[1]).toEqual([1, null]);
    expect(chart.data[2]).toEqual([0, null]);
    expect(trendChartData(testMetricsHistory, 'traffic', []).unit).toBe('B/s');
    expect(trendChartData(null, 'traffic', []).unit).toBe('B/s');
  });
});
