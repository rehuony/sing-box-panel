import type { KeyboardEvent as ReactKeyboardEvent } from 'react';

import UPlot from 'uplot';
import { Activity } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';

import type { MetricsHistoryBucket } from '@/api/api-client';

import 'uplot/dist/uPlot.min.css';

type TrendKind = 'traffic' | 'connections' | 'memory';

type PlotDomain = readonly [minimum: number, maximum: number];

interface RangeControl {
  key: string;
  label: string;
}

interface HistoryUPlotProps {
  to: number;
  from: number;
  range: string;
  locale: string;
  trend: TrendKind;
  focusedIndex: number;
  bucketSeconds: number;
  ranges: RangeControl[];
  buckets: MetricsHistoryBucket[];
  onRangeChange: (range: string) => void;
  valueFormatter: (value: number) => string;
  onFocusedIndexChange: (index: number) => void;
  onInspect: (bucket: MetricsHistoryBucket) => void;
}

const MINIMUM_CHART_WIDTH = 240;
const KEYBOARD_ZOOM_IN = 0.8;
const KEYBOARD_ZOOM_OUT = 1.25;
const KEYBOARD_PAN_FRACTION = 0.12;

function normalizeDomain(domain: PlotDomain): PlotDomain {
  const [minimum, maximum] = domain;
  if (Number.isFinite(minimum) && Number.isFinite(maximum) && maximum > minimum) {
    return domain;
  }
  const now = Date.now();
  return [now - 3_600_000, now];
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}

function zoomPlotDomain(
  domain: PlotDomain,
  bounds: PlotDomain,
  anchorRatio: number,
  factor: number,
  minimumSpan: number,
): PlotDomain {
  const [boundMinimum, boundMaximum] = normalizeDomain(bounds);
  const boundSpan = boundMaximum - boundMinimum;
  const domainMinimum = clamp(domain[0], boundMinimum, boundMaximum);
  const domainMaximum = clamp(domain[1], boundMinimum, boundMaximum);
  const currentSpan = Math.max(minimumSpan, domainMaximum - domainMinimum);
  const nextSpan = clamp(currentSpan * factor, Math.min(minimumSpan, boundSpan), boundSpan);
  const anchor = clamp(anchorRatio, 0, 1);
  const anchorValue = domainMinimum + currentSpan * anchor;
  let nextMinimum = anchorValue - nextSpan * anchor;
  let nextMaximum = nextMinimum + nextSpan;

  if (nextMinimum < boundMinimum) {
    nextMinimum = boundMinimum;
    nextMaximum = boundMinimum + nextSpan;
  }
  if (nextMaximum > boundMaximum) {
    nextMaximum = boundMaximum;
    nextMinimum = boundMaximum - nextSpan;
  }

  return [nextMinimum, nextMaximum];
}

function panPlotDomain(
  domain: PlotDomain,
  bounds: PlotDomain,
  fraction: number,
): PlotDomain {
  const [boundMinimum, boundMaximum] = normalizeDomain(bounds);
  const span = Math.min(domain[1] - domain[0], boundMaximum - boundMinimum);
  if (span <= 0) return [boundMinimum, boundMaximum];

  const requestedOffset = span * fraction;
  const nextMinimum = clamp(domain[0] + requestedOffset, boundMinimum, boundMaximum - span);
  return [nextMinimum, nextMinimum + span];
}

function cssColor(property: string, fallback: string): string {
  const value = window.getComputedStyle(document.documentElement).getPropertyValue(property).trim();
  return value || fallback;
}

function gapBackgroundPlugin(buckets: MetricsHistoryBucket[]): UPlot.Plugin {
  return {
    hooks: {
      drawClear: [
        (plot) => {
          const incomplete = buckets.filter((bucket) => bucket.coverage !== 'complete');
          if (incomplete.length === 0) return;

          const ratio = UPlot.pxRatio;
          const { ctx, bbox } = plot;
          const missingColor = cssColor('--color-border-strong', '#a9a6b4');
          const partialColor = cssColor('--color-brand-orange', '#d97706');
          ctx.save();
          ctx.beginPath();
          ctx.rect(bbox.left, bbox.top, bbox.width, bbox.height);
          ctx.clip();

          for (const bucket of incomplete) {
            const start = Math.max(bbox.left, plot.valToPos(Date.parse(bucket.from), 'x', true));
            const end = Math.min(bbox.left + bbox.width, plot.valToPos(Date.parse(bucket.to), 'x', true));
            if (end <= start) continue;

            const color = bucket.coverage === 'missing' ? missingColor : partialColor;
            ctx.fillStyle = color;
            ctx.globalAlpha = bucket.coverage === 'missing' ? 0.1 : 0.055;
            ctx.fillRect(start, bbox.top, end - start, bbox.height);

            if (bucket.coverage !== 'missing') continue;
            ctx.globalAlpha = 0.22;
            ctx.strokeStyle = color;
            ctx.lineWidth = ratio;
            const spacing = 9 * ratio;
            ctx.beginPath();
            for (let x = start - bbox.height; x < end; x += spacing) {
              ctx.moveTo(x, bbox.top + bbox.height);
              ctx.lineTo(x + bbox.height, bbox.top);
            }
            ctx.stroke();
          }

          ctx.restore();
        },
      ],
    },
  };
}

function normalizeWheelDelta(event: WheelEvent, viewportSize: number): number {
  if (event.deltaMode === WheelEvent.DOM_DELTA_LINE) return event.deltaY * 16;
  if (event.deltaMode === WheelEvent.DOM_DELTA_PAGE) return event.deltaY * viewportSize;
  return event.deltaY;
}

export function HistoryUPlot({
  bucketSeconds,
  buckets,
  focusedIndex,
  from,
  locale,
  onFocusedIndexChange,
  onInspect,
  onRangeChange,
  range,
  ranges,
  to,
  trend,
  valueFormatter,
}: HistoryUPlotProps) {
  const { t } = useTranslation();
  const shellRef = useRef<HTMLDivElement>(null);
  const plotHostRef = useRef<HTMLDivElement>(null);
  const plotRef = useRef<UPlot | null>(null);
  const onFocusedIndexChangeRef = useRef(onFocusedIndexChange);
  const onInspectRef = useRef(onInspect);
  const fullDomain = useMemo<PlotDomain>(() => normalizeDomain([from, to]), [from, to]);
  const [visibleDomain, setVisibleDomain] = useState<PlotDomain>(fullDomain);
  const visibleDomainRef = useRef(visibleDomain);
  const minimumSpan = Math.min(
    fullDomain[1] - fullDomain[0],
    Math.max(bucketSeconds * 2_000, 60_000),
  );

  onFocusedIndexChangeRef.current = onFocusedIndexChange;
  onInspectRef.current = onInspect;
  visibleDomainRef.current = visibleDomain;

  const plotData = useMemo<UPlot.AlignedData>(() => {
    const time = buckets.map((bucket) => Date.parse(bucket.from));
    if (trend === 'traffic') {
      return [
        time,
        buckets.map((bucket) => bucket.download_bytes),
        buckets.map((bucket) => bucket.upload_bytes),
      ];
    }
    if (trend === 'connections') {
      return [
        time,
        buckets.map((bucket) => bucket.active_connections_avg),
        buckets.map((bucket) => bucket.active_connections_peak),
      ];
    }
    return [
      time,
      buckets.map((bucket) => bucket.memory_bytes_avg),
      buckets.map((bucket) => bucket.memory_bytes_peak),
    ];
  }, [buckets, trend]);

  const safeFocusedIndex = Math.min(focusedIndex, Math.max(0, buckets.length - 1));
  const focusedBucket = buckets[safeFocusedIndex];
  const focusedSeries = trend === 'traffic'
    ? [
        { label: t('dashboard.trend.download'), value: focusedBucket?.download_bytes },
        { label: t('dashboard.trend.upload'), value: focusedBucket?.upload_bytes },
      ]
    : [
        { label: t('dashboard.summary.average'), value: trend === 'connections' ? focusedBucket?.active_connections_avg : focusedBucket?.memory_bytes_avg },
        { label: t('dashboard.summary.peak'), value: trend === 'connections' ? focusedBucket?.active_connections_peak : focusedBucket?.memory_bytes_peak },
      ];
  const focusedTime = focusedBucket === undefined
    ? null
    : new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(focusedBucket.from));
  const visibleRangeLabel = new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeStyle: 'short',
  });

  useEffect(() => {
    const plot = plotRef.current;
    if (plot === null) return;
    plot.setScale('x', { min: visibleDomain[0], max: visibleDomain[1] });
  }, [visibleDomain]);

  useEffect(() => {
    const shell = shellRef.current;
    if (shell === null) return;

    const handleWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.shiftKey) return;
      event.preventDefault();
      const rect = plotRef.current?.over.getBoundingClientRect() ?? shell.getBoundingClientRect();
      const viewportWidth = Math.max(rect.width, 320);

      if (event.ctrlKey) {
        const anchorRatio = rect.width <= 0 ? 0.5 : (event.clientX - rect.left) / rect.width;
        const delta = normalizeWheelDelta(event, viewportWidth);
        const factor = clamp(Math.exp(delta * 0.002), 0.5, 2);
        setVisibleDomain((current) => zoomPlotDomain(
          current,
          fullDomain,
          anchorRatio,
          factor,
          minimumSpan,
        ));
        return;
      }

      const rawDelta = Math.abs(event.deltaX) > Math.abs(event.deltaY)
        ? event.deltaX
        : event.deltaY;
      const delta = event.deltaMode === WheelEvent.DOM_DELTA_LINE
        ? rawDelta * 16
        : event.deltaMode === WheelEvent.DOM_DELTA_PAGE
          ? rawDelta * viewportWidth
          : rawDelta;
      setVisibleDomain((current) => panPlotDomain(current, fullDomain, delta / viewportWidth));
    };

    shell.addEventListener('wheel', handleWheel, { passive: false });
    return () => shell.removeEventListener('wheel', handleWheel);
  }, [fullDomain, minimumSpan]);

  useEffect(() => {
    const host = plotHostRef.current;
    if (host === null || buckets.length === 0) return;
    let plot: UPlot | null = null;
    let clickHandler: (() => void) | null = null;
    let animationFrame: number | null = null;
    let lastWidth = 0;
    let lastHeight = 0;

    const createOrResizePlot = () => {
      const width = Math.floor(host.clientWidth);
      const height = Math.floor(host.clientHeight);
      if (width < MINIMUM_CHART_WIDTH || height < 120) return;
      if (width === lastWidth && height === lastHeight) return;
      lastWidth = width;
      lastHeight = height;
      if (plot !== null) {
        plot.setSize({ width, height });
        return;
      }

      const primary = cssColor('--color-primary', '#6554d9');
      const orange = cssColor('--color-brand-orange', '#d97706');
      const primaryFill = cssColor('--color-primary-soft', '#eeebff');
      const orangeFill = cssColor('--color-warning-soft', '#fff4dc');
      const rule = cssColor('--color-border', '#dedce6');
      const muted = cssColor('--color-text-muted', '#6d6878');
      const firstLabel = trend === 'traffic'
        ? t('dashboard.trend.download')
        : t('dashboard.summary.average');
      const secondLabel = trend === 'traffic'
        ? t('dashboard.trend.upload')
        : t('dashboard.summary.peak');
      const timeFormatter = new Intl.DateTimeFormat(locale, fullDomain[1] - fullDomain[0] <= 86_400_000
        ? { hour: '2-digit', minute: '2-digit' }
        : { month: 'short', day: 'numeric' });
      const numberFormatter = new Intl.NumberFormat(locale, { maximumFractionDigits: 1, notation: 'compact' });

      plot = new UPlot({
        width,
        height,
        ms: 1,
        padding: [12, 12, 0, 4],
        legend: { show: false },
        cursor: {
          drag: { setScale: false, x: false, y: false },
          points: { size: 7, width: 2 },
        },
        scales: {
          x: { auto: false, range: [visibleDomainRef.current[0], visibleDomainRef.current[1]], time: true },
          y: { auto: true },
        },
        axes: [
          {
            grid: { stroke: rule, width: 1 },
            stroke: muted,
            values: (_plot, splits) => splits.map((value) => timeFormatter.format(new Date(value))),
          },
          {
            grid: { stroke: rule, width: 1 },
            size: 68,
            stroke: muted,
            values: (_plot, splits) => splits.map((value) => trend === 'memory'
              ? valueFormatter(value)
              : numberFormatter.format(value)),
          },
        ],
        series: [
          { label: t('dashboard.table.time') },
          {
            fill: primaryFill,
            label: firstLabel,
            points: { show: false },
            spanGaps: false,
            stroke: primary,
            width: 2,
          },
          {
            dash: trend === 'traffic' ? undefined : [5, 4],
            fill: orangeFill,
            label: secondLabel,
            points: { show: false },
            spanGaps: false,
            stroke: orange,
            width: 1.6,
          },
        ],
        plugins: [gapBackgroundPlugin(buckets)],
        hooks: {
          setCursor: [
            (currentPlot) => {
              if (currentPlot.cursor.idx !== null && currentPlot.cursor.idx !== undefined) {
                onFocusedIndexChangeRef.current(currentPlot.cursor.idx);
              }
            },
          ],
        },
      }, plotData, host);
      plotRef.current = plot;
      clickHandler = () => {
        const index = plot?.cursor.idx;
        if (index === null || index === undefined) return;
        const bucket = buckets[index];
        if (bucket !== undefined) onInspectRef.current(bucket);
      };
      plot.over.addEventListener('click', clickHandler);
    };

    const scheduleResize = () => {
      if (animationFrame !== null) return;
      animationFrame = window.requestAnimationFrame(() => {
        animationFrame = null;
        createOrResizePlot();
      });
    };

    scheduleResize();
    const resizeObserver = typeof ResizeObserver === 'undefined'
      ? null
      : new ResizeObserver(scheduleResize);
    resizeObserver?.observe(host);
    window.addEventListener('resize', scheduleResize);

    return () => {
      resizeObserver?.disconnect();
      window.removeEventListener('resize', scheduleResize);
      if (animationFrame !== null) window.cancelAnimationFrame(animationFrame);
      if (plot !== null && clickHandler !== null) plot.over.removeEventListener('click', clickHandler);
      plot?.destroy();
      if (plotRef.current === plot) plotRef.current = null;
    };
  }, [buckets, fullDomain, locale, plotData, t, trend, valueFormatter]);

  useEffect(() => {
    const plot = plotRef.current;
    const bucket = buckets[safeFocusedIndex];
    if (plot === null || bucket === undefined) return;
    plot.setCursor({ left: plot.valToPos(Date.parse(bucket.from), 'x'), top: 0 }, false);
    plot.setLegend({ idx: safeFocusedIndex }, false);
  }, [buckets, safeFocusedIndex]);

  function handleKeyDown(event: ReactKeyboardEvent<HTMLDivElement>) {
    if (event.target !== event.currentTarget) return;
    if (event.key === '+' || event.key === '=') {
      event.preventDefault();
      setVisibleDomain((current) => zoomPlotDomain(
        current,
        fullDomain,
        0.5,
        KEYBOARD_ZOOM_IN,
        minimumSpan,
      ));
      return;
    }
    if (event.key === '-' || event.key === '_') {
      event.preventDefault();
      setVisibleDomain((current) => zoomPlotDomain(
        current,
        fullDomain,
        0.5,
        KEYBOARD_ZOOM_OUT,
        minimumSpan,
      ));
      return;
    }
    if (event.key === 'Home') {
      event.preventDefault();
      setVisibleDomain(fullDomain);
      return;
    }
    if (event.shiftKey && (event.key === 'ArrowLeft' || event.key === 'ArrowRight')) {
      event.preventDefault();
      const direction = event.key === 'ArrowLeft' ? -1 : 1;
      setVisibleDomain((current) => panPlotDomain(
        current,
        fullDomain,
        direction * KEYBOARD_PAN_FRACTION,
      ));
      return;
    }
    if (buckets.length === 0) return;
    if (event.key === 'ArrowLeft') {
      event.preventDefault();
      onFocusedIndexChange(Math.max(0, safeFocusedIndex - 1));
      return;
    }
    if (event.key === 'ArrowRight') {
      event.preventDefault();
      onFocusedIndexChange(Math.min(buckets.length - 1, safeFocusedIndex + 1));
      return;
    }
    if ((event.key === 'Enter' || event.key === ' ') && focusedBucket !== undefined) {
      event.preventDefault();
      onInspect(focusedBucket);
    }
  }

  const interactionHint = t('dashboard.chart.interactionHint', {
    defaultValue: 'Ctrl + scroll to zoom · Shift + scroll to pan · +/− zoom · Shift + ←/→ pan',
  });

  return (
    <div
      aria-describedby='dashboard-chart-interaction-hint'
      aria-label={t('dashboard.chart.keyboardLabel', {
        defaultValue: 'History chart. Use arrow keys to inspect buckets, plus and minus to zoom, Shift plus arrows to pan, Home to reset, and Enter to open logs.',
      })}
      className='dashboard-chart-shell'
      data-chart-library='uplot'
      data-domain-max={Math.round(visibleDomain[1])}
      data-domain-min={Math.round(visibleDomain[0])}
      onKeyDown={handleKeyDown}
      ref={shellRef}
      role='application'
      tabIndex={0}
    >
      <div className='dashboard-chart-toolbar'>
        <div className='dashboard-chart-focus' aria-live='polite' role='status'>
          {focusedBucket === undefined
            ? <span>{t('dashboard.chart.noBucket', { defaultValue: 'No bucket selected' })}</span>
            : (
                <>
                  <span className='dashboard-chart-focus__identity'>
                    <time dateTime={focusedBucket.from}>{focusedTime}</time>
                    <span data-coverage={focusedBucket.coverage}>
                      {t(`dashboard.coverage.${focusedBucket.coverage}`, { defaultValue: focusedBucket.coverage })}
                    </span>
                  </span>
                  <span className='dashboard-chart-focus__values'>
                    {focusedSeries.map((series) => (
                      <span key={series.label}>
                        {series.label}
                        {' '}
                        <strong>{series.value === null || series.value === undefined ? '—' : valueFormatter(series.value)}</strong>
                      </span>
                    ))}
                  </span>
                </>
              )}
          <span className='sr-only'>
            {t('dashboard.chart.visibleRange', {
              defaultValue: 'Visible range {{from}} to {{to}}',
              from: visibleRangeLabel.format(new Date(visibleDomain[0])),
              to: visibleRangeLabel.format(new Date(visibleDomain[1])),
            })}
          </span>
        </div>
        <div
          aria-label={t('dashboard.range.label', { defaultValue: 'Time range' })}
          className='dashboard-range dashboard-range--inside-chart'
          role='group'
        >
          {ranges.map((option) => (
            <button
              aria-pressed={range === option.key}
              key={option.key}
              onClick={() => onRangeChange(option.key)}
              type='button'
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>

      {buckets.length === 0
        ? (
            <div className='dashboard-empty'>
              <Activity aria-hidden='true' />
              <strong>{t('dashboard.empty.title', { defaultValue: 'No samples in this range' })}</strong>
              <span>{t('dashboard.empty.description', { defaultValue: 'No traffic or runtime value is inferred.' })}</span>
            </div>
          )
        : <div className='dashboard-chart' ref={plotHostRef} />}

      <p className='dashboard-chart-interaction-hint' id='dashboard-chart-interaction-hint'>
        {interactionHint}
      </p>
    </div>
  );
}
