import type UPlot from 'uplot';

import type { MetricsHistory } from '@/api/api-client';
import type { LivePoint } from '@/components/node-workspace';

export interface TrendChartData {
  unit: string | null;
  to: number | undefined;
  data: UPlot.AlignedData;
  from: number | undefined;
}

/** How far back the live charts reach from their newest sample. */
const LIVE_WINDOW = 120_000;
/** The shortest span a live chart shows, so its first samples are not stretched across the width. */
const LIVE_MINIMUM_SPAN = 10_000;

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
 * collection gaps. The span starts at the oldest sample it keeps, so no blank
 * strip leads the lines, whether the page opened moments ago or the window's
 * first sample falls a few seconds after its start.
 */
function liveSeries(tail: LivePoint[], pick: ((point: LivePoint) => number | null)[]) {
  const to = tail.at(-1)?.at;
  const recent = tail.filter(point => to === undefined || point.at >= to - LIVE_WINDOW);
  const from = to === undefined || recent[0] === undefined ? undefined : Math.min(recent[0].at, to - LIVE_MINIMUM_SPAN);
  const limit = gapLimit(recent.map(point => point.at));
  const times: number[] = [];
  const series = pick.map(() => [] as (number | null)[]);
  for (const point of recent) {
    const previous = times.at(-1);
    if (previous !== undefined && point.at - previous > limit) {
      times.push(point.at - 1);
      for (const values of series) values.push(null);
    }
    times.push(point.at);
    pick.forEach((value, index) => series[index]!.push(value(point)));
  }
  return { times, series, from, to };
}

export function liveTrafficChartData(tail: LivePoint[]): TrendChartData {
  const { times, series: [down = [], up = []], from, to } = liveSeries(tail, [
    point => point.downloadBytesPerSecond, point => point.uploadBytesPerSecond,
  ]);
  return { data: [times, down, up], unit: scaleTrafficRates(down, up), from, to };
}

export function liveConnectionsChartData(tail: LivePoint[]): TrendChartData {
  const { times, series: [connections = []], from, to } = liveSeries(tail, [point => point.connections]);
  return { data: [times, connections], unit: null, from, to };
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
  const live = kind === 'connections'
    ? tail.filter(point => point.at > through && (from === undefined || point.at >= from))
    : [];
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
    const limit = gapLimit(live.map(point => point.at));
    for (const point of live) {
      const previous = times.at(-1);
      // Null samples preserve reconnects, restarts and unavailable metrics.
      if (previous !== undefined && point.at - previous > limit) {
        times.push(point.at - 1);
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

function scaleTrafficRates(down: (number | null)[], up: (number | null)[]): string {
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
 * vertical axis's zero.
 */
export function timeLabelFits(label: string, position: number, width: number, endPadding: number): boolean {
  // The axis sets 11px numerals, about six and a half pixels a character.
  const half = label.length * 3.25 + 2;
  return position >= half && position <= width + endPadding - half;
}
