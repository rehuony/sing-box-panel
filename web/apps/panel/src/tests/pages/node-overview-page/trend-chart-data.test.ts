import { describe, expect, it } from 'vitest';

import type { LivePoint } from '@/components/node-workspace';

import { testMetricsHistory } from '@/tests/api/mock-api-client';
import { liveTrafficChartData, trendChartData } from '@/pages/node-overview-page/trend-chart-data';

describe('live traffic evidence', () => {
  it('keeps the latest two minutes and the reading before them without averaging peaks or filling gaps', () => {
    const point = (at: number, downloadBytesPerSecond: number | null): LivePoint => ({
      at, downloadBytesPerSecond, uploadBytesPerSecond: downloadBytesPerSecond, connections: 2,
    });
    const chart = liveTrafficChartData([
      point(1000, 1024 ** 3), point(2000, 1024), point(3000, 512),
      point(4000, null), point(121000, 0), point(122000, 256),
    ]);
    expect(chart.from).toBe(2000);
    expect(chart.to).toBe(122000);
    // The reading before the window runs the line off its start without choosing the unit.
    expect(chart.unit).toBe('KB/s');
    expect(chart.data[0]).toEqual([1000, 2000, 3000, 4000, 62500, 121000, 122000]);
    expect(chart.data[1]).toEqual([1024 ** 2, 1, 0.5, null, null, 0, 0.25]);
  });

  it('connects a stream that samples every few seconds and breaks it only at a longer pause', () => {
    const point = (at: number): LivePoint => ({
      at, downloadBytesPerSecond: 1024, uploadBytesPerSecond: 512, connections: 2,
    });
    const chart = liveTrafficChartData([point(0), point(5000), point(10000), point(15000), point(60000), point(65000)]);
    // The break sits midway, so the cursor at either side's reading shows it.
    expect(chart.data[0]).toEqual([0, 5000, 10000, 15000, 37500, 60000, 65000]);
    expect(chart.data[1]).toEqual([1, 1, 1, 1, null, 1, 1]);
  });

  it('joins stored samples ten seconds apart to each other and the stream, breaking only where one is missing', () => {
    const point = (at: number, span?: number): LivePoint => ({
      at, downloadBytesPerSecond: 1024, uploadBytesPerSecond: 512, connections: 2, ...(span ? { span } : {}),
    });
    const stream = Array.from({ length: 10 }, (_, index) => point(51_000 + index * 1000));
    const chart = liveTrafficChartData([
      point(0, 10_000), point(10_000, 10_000), point(20_000, 10_000), point(50_000, 10_000), ...stream,
    ]);
    expect(chart.data[0].slice(0, 6)).toEqual([0, 10_000, 20_000, 35_000, 50_000, 51_000]);
    expect(chart.data[1]).toEqual([1, 1, 1, null, 1, ...stream.map(() => 1)]);
  });

  it('joins the reading after a slower sampling to the one before it by the span its rates cover', () => {
    const point = (at: number, span?: number): LivePoint => ({
      at, downloadBytesPerSecond: 1024, uploadBytesPerSecond: 512, connections: 2, ...(span ? { span } : {}),
    });
    // Seven seconds after the last ten-second reading, then every second.
    const stream = Array.from({ length: 20 }, (_, index) => point(8000 + index * 1000, 1000));
    const chart = liveTrafficChartData([point(0), point(7000, 7000), ...stream]);
    expect(chart.data[1]).not.toContain(null);
  });

  it('spans exactly two minutes up to the newest sample, however few samples it holds', () => {
    const point = (at: number): LivePoint => ({
      at, downloadBytesPerSecond: 1024, uploadBytesPerSecond: 512, connections: 2,
    });
    const few = liveTrafficChartData([point(50_000), point(55_000), point(80_000)]);
    expect(few).toMatchObject({ from: -40_000, to: 80_000 });
    const tail = Array.from({ length: 30 }, (_, index) => point(index * 5000 + 3000));
    const chart = liveTrafficChartData(tail);
    expect(chart).toMatchObject({ from: 28_000, to: 148_000 });
    expect(chart.data[0][0]).toBe(23_000);
  });

  it('does not mix live rates into historical traffic averages', () => {
    const tail = [{
      at: Date.parse(testMetricsHistory.to) + 1000,
      connections: 2, uploadBytesPerSecond: 1024 ** 2, downloadBytesPerSecond: 1024 ** 3,
    }];
    expect(trendChartData(testMetricsHistory, 'traffic', tail, testMetricsHistory.to))
      .toEqual(trendChartData(testMetricsHistory, 'traffic', []));
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

  it('joins connections to the overlapping history bucket', () => {
    const through = end - 6000;
    const tail = [point(through), point(through + 2000), point(through + 4000), point(end)];
    const chart = trendChartData(history, 'connections', tail, new Date(through).toISOString());
    expect(chart.data[0].slice(0, 3)).toEqual([Date.parse(history.buckets[0]!.to), through, through + 2000]);
    expect(chart.data[1].slice(0, 3)).toEqual([12, 12, 15]);
    expect(chart.data[0].filter(at => at === through)).toHaveLength(1);
    expect(chart.data[1]).not.toContain(null);
  });

  it('does not connect across a delayed live tail or fill missing historical buckets', () => {
    const chart = trendChartData(testMetricsHistory, 'connections', [point(end + 30_000)], testMetricsHistory.to);
    expect(chart.data[1]).toEqual([12.5, null, null, 15]);
    expect(chart.data[0].at(-2)).toBe(end + 15_000);
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
    const before = trendChartData(history, 'connections', tail, new Date(end - 4000).toISOString());
    expect(before.data[1]).toEqual([12, 12, 15, 15, 15]);
    const after = trendChartData(history, 'connections', tail, history.to);
    expect(after.data[1]).toEqual([12, 12, 15]);
    expect(after.data[0]).toEqual([Date.parse(history.buckets[0]!.to), end, end + 2000]);
  });

  it('joins live readings after a bucket known idle, not at the last sample before the core stopped', () => {
    const [sampled, idle] = testMetricsHistory.buckets;
    const stopped = { ...testMetricsHistory, buckets: [sampled!, {
      ...idle!, upload_bytes: 0, download_bytes: 0, active_connections_avg: 0, active_connections_peak: 0, coverage: 'complete' as const,
    }] };
    const chart = trendChartData(stopped, 'connections', [
      { at: end + 1000, connections: 0, uploadBytesPerSecond: 0, downloadBytesPerSecond: 0 },
    ], new Date(Date.parse(sampled!.to) - 10_000).toISOString());
    expect(chart.data[1]).toEqual([12.5, 0, 0]);
  });

  it('keeps only non-overlapping history without a persistence watermark', () => {
    const chart = trendChartData(history, 'connections', [point(end - 2000)], null);
    const bucketEnd = Date.parse(history.buckets[0]!.to);
    expect(chart.data[0]).toEqual([bucketEnd, (bucketEnd + end - 2000) / 2, end - 2000]);
  });
});

describe('historical traffic rates', () => {
  const [bucket] = testMetricsHistory.buckets;

  it('divides bucket traffic by the time its intervals cover, not the whole bucket', () => {
    const covered = { ...bucket!, download_bytes: 2500, upload_bytes: 500, traffic_seconds: 250 };
    const history = { ...testMetricsHistory, buckets: [covered] };
    expect(trendChartData(history, 'traffic', []).data.slice(1)).toEqual([[10], [2]]);
  });

  it('leaves out the newest bucket while it fills without a value', () => {
    const filling = {
      ...bucket!, from: testMetricsHistory.to, to: new Date(Date.parse(testMetricsHistory.to) + 20_000).toISOString(),
      download_bytes: null, upload_bytes: null, traffic_seconds: null, active_connections_avg: null, coverage: 'partial' as const,
    };
    const history = { ...testMetricsHistory, to: filling.to, buckets: [bucket!, filling] };
    expect(trendChartData(history, 'traffic', []).data[0]).toEqual([Date.parse(bucket!.to)]);
    expect(trendChartData(history, 'connections', []).data[0]).toEqual([Date.parse(bucket!.to)]);
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
    const chart = liveTrafficChartData([{
      at: 1000, connections: 2, downloadBytesPerSecond: rate / 2, uploadBytesPerSecond: rate,
    }]);
    expect(chart.unit).toBe(unit);
    expect(chart.data).toEqual([[1000], [scaled / 2], [scaled]]);
  });

  it('chooses units from selected history, preserving zeros and nulls', () => {
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
