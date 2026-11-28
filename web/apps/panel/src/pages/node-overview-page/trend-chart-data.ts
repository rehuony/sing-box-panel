import type UPlot from 'uplot';

import type { MetricsHistory } from '@/api/api-client';
import type { LivePoint } from '@/components/node-workspace';

export interface TrendChartData {
  unit: string | null;
  to: number | undefined;
  data: UPlot.AlignedData;
  from: number | undefined;
}

/** How far back the live charts reach: their window is always this long. */
export const LIVE_WINDOW = 120_000;

/**
 * The longest step a live line spans: two and a half times the stream's
 * usual spacing, and never under three seconds, so a node sampling each
 * second or every few seconds draws a line while a pause, reconnect or
 * restart still breaks it.
 */
function gapLimit(times: number[]): number {
  const steps = times.slice(1).map((time, index) => time - times[index]!);
  const spacing = steps.filter(step => step > 0).sort((a, b) => a - b);
  return Math.max(3000, 2.5 * (spacing[Math.floor(spacing.length / 2)] ?? 0));
}

/**
 * The live tail's last two minutes as aligned series, broken across
 * collection gaps, with the reading before them, so the lines run off the
 * window's start rather than starting inside it. A step to or from a point
 * may also reach one and a half of the span its rates cover, so the stored
 * samples a window opens with, about ten seconds apart, join each other and
 * the stream's first readings while the stream's own readings keep their
 * limit. A break sits midway between its two points, so the cursor at either
 * one shows its values. The span is always LIVE_WINDOW, ending at the newest
 * sample, so the chart never rescales as old samples leave; `first` is the
 * index of the first time inside it.
 */
function liveSeries(tail: LivePoint[], pick: ((point: LivePoint) => number | null)[]) {
  const to = tail.at(-1)?.at;
  const from = to === undefined ? undefined : to - LIVE_WINDOW;
  const start = from === undefined ? 0 : tail.findIndex(point => point.at >= from);
  const recent = tail.slice(Math.max(0, start - 1));
  const limit = gapLimit(recent.map(point => point.at));
  const times: number[] = [];
  const series = pick.map(() => [] as (number | null)[]);
  let last: LivePoint | undefined;
  for (const point of recent) {
    const allowed = Math.max(limit, 1.5 * Math.max(point.span ?? 0, last?.span ?? 0));
    if (last !== undefined && point.at - last.at > allowed) {
      times.push((last.at + point.at) / 2);
      for (const values of series) values.push(null);
    }
    times.push(point.at);
    pick.forEach((value, index) => series[index]!.push(value(point)));
    last = point;
  }
  const first = from === undefined ? 0 : Math.max(0, times.findIndex(time => time >= from));
  return { times, series, from, to, first };
}

export function liveTrafficChartData(tail: LivePoint[]): TrendChartData {
  const { times, series: [down = [], up = []], from, to, first } = liveSeries(tail, [
    point => point.downloadBytesPerSecond, point => point.uploadBytesPerSecond,
  ]);
  return { data: [times, down, up], unit: scaleTrafficRates(down, up, first), from, to };
}

export function liveConnectionsChartData(tail: LivePoint[]): TrendChartData {
  const { times, series: [connections = []], from, to } = liveSeries(tail, [point => point.connections]);
  return { data: [times, connections], unit: null, from, to };
}

/** A bucket's traffic as a rate over the time its intervals cover, which may be less than the bucket. */
function bucketRate(bytes: number | null, bucket: MetricsHistory['buckets'][number]): number | null {
  const seconds = bucket.traffic_seconds;
  return bytes === null || seconds === null || seconds <= 0 ? null : bytes / seconds;
}

// Historical traffic uses bucket averages only. Connections can still join the
// latest persisted bucket to live readings at the persistence watermark, or
// after the last bucket known without samples because no core ran.
export function trendChartData(history: MetricsHistory | null, kind: 'traffic' | 'connections', tail: LivePoint[], watermark?: string | null): TrendChartData {
  const idleThrough = (history?.buckets ?? [])
    .filter(bucket => bucket.sample_count === 0 && bucket.coverage === 'complete')
    .reduce((latest, bucket) => Math.max(latest, Date.parse(bucket.to)), Number.NEGATIVE_INFINITY);
  const through = Math.max(watermark ? Date.parse(watermark) : Number.NEGATIVE_INFINITY, idleThrough);
  const from = history ? Date.parse(history.from) : undefined;
  // The newest bucket fills until the range's end and is left out until it has a value.
  const all = history?.buckets ?? [];
  const newest = all.at(-1);
  const filling = newest !== undefined
    && Date.parse(newest.to) - Date.parse(newest.from) < history!.bucket_seconds * 1000
    && (kind === 'connections'
      ? newest.active_connections_avg === null
      : bucketRate(newest.download_bytes, newest) === null);
  const known = filling ? all.slice(0, -1) : all;
  const live = kind === 'connections'
    ? tail.filter(point => point.at > through && (from === undefined || point.at >= from))
    : [];
  const first = live[0]?.at ?? Number.POSITIVE_INFINITY;
  const boundary = live.length && Number.isFinite(through) ? through : Number.POSITIVE_INFINITY;
  const buckets = known.filter(bucket => Number.isFinite(boundary)
    ? Date.parse(bucket.from) < boundary
    : Date.parse(bucket.to) < first);
  const times = buckets.map(bucket => Math.min(Date.parse(bucket.to), boundary));
  const down = buckets.map(bucket => kind === 'connections'
    ? bucket.active_connections_avg
    : bucketRate(bucket.download_bytes, bucket));
  const up = buckets.map(bucket => bucketRate(bucket.upload_bytes, bucket));
  if (live.length) {
    const limit = gapLimit(live.map(point => point.at));
    for (const point of live) {
      const previous = times.at(-1);
      // Null samples preserve reconnects, restarts and unavailable metrics.
      if (previous !== undefined && point.at - previous > limit) {
        times.push((previous + point.at) / 2);
        down.push(null);
      }
      times.push(point.at);
      down.push(point.connections);
    }
  }
  const unit = kind === 'traffic' ? scaleTrafficRates(down, up) : null;
  return {
    data: kind === 'traffic' ? [times, down, up] : [times, down],
    unit,
    from: from ?? times[0],
    to: history ? Math.max(Date.parse(history.to), times.at(-1) ?? 0) : times.at(-1),
  };
}

/**
 * Picks the unit the rates read best in and scales them to it. The peak comes
 * from the values from first on, the ones in view, so a reading before the
 * window does not choose the unit.
 */
function scaleTrafficRates(down: (number | null)[], up: (number | null)[], first = 0): string {
  const units = ['B/s', 'KB/s', 'MB/s', 'GB/s', 'TB/s', 'PB/s'];
  let peak = 0;
  for (const series of [down, up]) {
    for (const value of series.slice(first)) peak = Math.max(peak, value ?? 0);
  }
  let index = 0;
  while (peak >= 1024 && index < units.length - 1) {
    peak /= 1024;
    index++;
  }
  const divisor = 1024 ** index;
  for (const series of [down, up]) {
    for (let i = 0; i < series.length; i++) {
      if (series[i] !== null) series[i] = series[i]! / divisor;
    }
  }
  return units[index]!;
}

/**
 * Whether a time label centred at position, in pixels from the plot's left
 * edge, fits between the vertical axis's labels and the chart's right edge.
 * A label that would not is left out, so the first tick never runs into the
 * vertical axis's zero. Live charts keep every label and clip them at the
 * plot's edges instead, so a label slides out as the window steps.
 */
export function timeLabelFits(label: string, position: number, width: number, endPadding: number): boolean {
  // The axis sets 11px numerals, about six and a half pixels a character.
  const half = label.length * 3.25 + 2;
  return position >= half && position <= width + endPadding - half;
}
