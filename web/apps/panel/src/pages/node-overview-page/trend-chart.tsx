import UPlot from 'uplot';
import { useTranslation } from 'react-i18next';
import { useEffect, useEffectEvent, useId, useLayoutEffect, useRef, useState } from 'react';

import type { TrendChartData } from './trend-chart-data';

import { LIVE_WINDOW, timeLabelFits } from './trend-chart-data';
import 'uplot/dist/uPlot.min.css';

/** Steps between the connection axis's labels: whole connections only. */
const connectionSteps = Array.from({ length: 10 }, (_, power) => [1, 2, 5].map(step => step * 10 ** power)).flat();

/**
 * How far the node's clock runs ahead of this one, estimated from its
 * samples: the largest lead seen, which the fastest delivery comes closest
 * to. A sample more than five seconds behind the estimate, as after the
 * node's clock steps back, starts it over.
 */
function nodeClockLead(lead: number, newest: number | undefined): number {
  if (newest === undefined) return lead;
  const estimate = newest - Date.now();
  return !Number.isFinite(lead) || estimate > lead || estimate < lead - 5000 ? estimate : lead;
}

export function TrendChart({
  chart,
  kind,
  live = false,
}: {
  chart: TrendChartData;
  kind: 'traffic' | 'connections';
  live?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const hostRef = useRef<HTMLDivElement>(null);
  const tooltipRef = useRef<HTMLOutputElement>(null);
  const plotRef = useRef<UPlot | null>(null);
  const [focused, setFocused] = useState<{ index: number; left: number; top: number } | null>(null);
  const descriptionId = useId();
  const { data, from, to, unit } = chart;
  const frameRef = useRef<number | null>(null);
  const pendingFocusRef = useRef<typeof focused>(null);
  // A live chart's window: the node's clock lead and the window's end, which only moves on.
  const liveRef = useRef({ lead: Number.NEGATIVE_INFINITY, end: Number.NEGATIVE_INFINITY });
  /**
   * A live chart shows the LIVE_WINDOW before the next whole second of the
   * node's clock, so the window keeps its length and steps one second at a
   * time on each tick, and the newest sample is always inside it.
   */
  function liveScale(plot: UPlot, always = false) {
    const state = liveRef.current;
    if (!Number.isFinite(state.lead)) return;
    const end = Math.max(state.end, Math.ceil((Date.now() + state.lead) / 1000) * 1000);
    if (always || end !== state.end) plot.setScale('x', { min: end - LIVE_WINDOW, max: end });
    state.end = end;
  }
  const updatePlot = useEffectEvent(() => {
    const plot = plotRef.current;
    if (!plot) return;
    plot.batch(() => {
      plot.setData(data);
      if (live) {
        const state = liveRef.current;
        const lead = nodeClockLead(state.lead, data[0].at(-1));
        // A clock that stepped back moves the window back with it once.
        if (lead < state.lead) state.end = Number.NEGATIVE_INFINITY;
        state.lead = lead;
        liveScale(plot, true);
      } else if (from !== undefined && to !== undefined) {
        plot.setScale('x', { min: from, max: to });
      }
    });
  });
  const stepLive = useEffectEvent(() => {
    if (plotRef.current) liveScale(plotRef.current);
  });
  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const time = new Intl.DateTimeFormat(i18n.language, {
      hour: '2-digit', minute: '2-digit', ...(live ? { second: '2-digit' } as const : {}),
    });
    function create() {
      plotRef.current?.destroy();
      plotRef.current = null;
      const style = getComputedStyle(document.documentElement);
      const color = (name: string) => style.getPropertyValue(name).trim();
      // Monotone cubic curves follow the samples without overshooting them,
      // so a rate never dips below zero between two.
      const smooth = UPlot.paths.spline?.();
      const endPadding = 8;
      const width = Math.floor(host!.clientWidth);
      const height = Math.floor(host!.clientHeight);
      if (width < 160 || height < 120) return;
      plotRef.current = new UPlot(
        {
          width,
          height,
          ms: 1,
          padding: [8, endPadding, 0, 0],
          legend: { show: false },
          cursor: { drag: { x: false, y: false }, points: { size: 6 } },
          scales: {
            x: { time: true, auto: false },
            y: { range: (_plot, _min, max) => UPlot.rangeNum(0, Math.max(1, max ?? 1), {
              min: { hard: 0 }, max: { pad: 0.08 },
            }) },
          },
          axes: [
            {
              size: 20,
              gap: 4,
              space: 85,
              stroke: color('--muted-foreground'),
              grid: { show: false },
              ticks: { show: false },
              font: '11px sans-serif',
              // A live chart keeps every label and clips it at the plot's edges, so labels slide out as it steps.
              values: (plot, splits) => splits.map((value) => {
                const label = time.format(new Date(value));
                return live || timeLabelFits(label, plot.valToPos(value, 'x'), plot.over.clientWidth, endPadding) ? label : null;
              }),
            },
            {
              size: 44,
              gap: 4,
              space: 55,
              ...(kind === 'connections' ? { incrs: connectionSteps } : {}),
              stroke: color('--muted-foreground'),
              ticks: { show: false },
              grid: { stroke: color('--border'), width: 1 },
              font: '11px sans-serif',
              values: (_plot, splits) =>
                splits.map((value) =>
                  new Intl.NumberFormat(i18n.language, {
                    maximumFractionDigits: 3,
                    notation: 'compact',
                  }).format(value),
                ),
            },
          ],
          series: [
            { label: 'Time' },
            {
              label: t(
                kind === 'traffic' ? 'dashboard.trend.download' : 'dashboard.trend.connections',
              ),
              stroke: color('--chart-1'),
              // Keep the grid visible through the fill instead of tinting it with an opaque surface color.
              fill: `color-mix(in srgb, ${color('--chart-1')} 8%, transparent)`,
              width: 2,
              paths: smooth,
              spanGaps: false,
              points: { show: live, size: 3 },
            },
            ...(kind === 'traffic'
              ? [
                  {
                    label: t('dashboard.trend.upload'),
                    stroke: color('--chart-2'),
                    width: 1.7,
                    paths: smooth,
                    spanGaps: false,
                    points: { show: live, size: 3 },
                  },
                ]
              : []),
          ],
          hooks: {
            // The time labels' row outside the plot is cleared from just below the vertical axis's zero.
            drawAxes: live
              ? [(plot) => {
                  const { ctx } = plot;
                  const ratio = window.devicePixelRatio || 1;
                  const bottom = plot.bbox.top + plot.bbox.height;
                  const right = plot.bbox.left + plot.bbox.width;
                  ctx.clearRect(0, bottom + 5 * ratio, plot.bbox.left, ctx.canvas.height);
                  ctx.clearRect(right, bottom + 1, ctx.canvas.width - right, ctx.canvas.height);
                }]
              : [],
            setCursor: [(plot) => {
              const { idx, left = -1, top = -1 } = plot.cursor;
              pendingFocusRef.current = idx == null || left < 0 || top < 0
                ? null
                : {
                    index: idx,
                    // uPlot cursor coordinates exclude the axes around its plotting area.
                    left: plot.over.offsetLeft + left,
                    top: plot.over.offsetTop + top,
                  };
              frameRef.current ??= requestAnimationFrame(() => {
                frameRef.current = null;
                setFocused(pendingFocusRef.current);
              });
            }],
          },
        },
        kind === 'traffic' ? [[], [], []] : [[], []],
        host!,
      );
      updatePlot();
    }
    create();
    const resize = new ResizeObserver(() => {
      const width = host.clientWidth;
      const height = host.clientHeight;
      if (width < 160 || height < 120) return;
      if (plotRef.current) plotRef.current.setSize({ width, height });
      else create();
    });
    resize.observe(host);
    const theme = new MutationObserver(create);
    theme.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['style', 'class', 'data-theme'],
    });
    return () => {
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
      resize.disconnect();
      theme.disconnect();
      plotRef.current?.destroy();
      plotRef.current = null;
    };
  }, [kind, live, i18n.language, t]);
  useEffect(() => {
    updatePlot();
  }, [data, from, to]);
  // A live window steps on each whole second of the node's clock.
  useEffect(() => {
    if (!live) return;
    let timer = 0;
    const tick = () => {
      stepLive();
      const lead = liveRef.current.lead;
      const clock = Date.now() + (Number.isFinite(lead) ? lead : 0);
      timer = window.setTimeout(tick, Math.ceil(clock / 1000) * 1000 - clock + 5);
    };
    tick();
    return () => window.clearTimeout(timer);
  }, [live]);
  const bucket = focused === null ? undefined : data[0][focused.index];
  useLayoutEffect(() => {
    const host = hostRef.current;
    const tooltip = tooltipRef.current;
    if (!focused || !bucket || !host || !tooltip) return;
    const gap = 14;
    const inset = 8;
    const width = tooltip.offsetWidth;
    const height = tooltip.offsetHeight;
    const left = focused.left + gap + width > host.clientWidth - inset
      ? focused.left - gap - width
      : focused.left + gap;
    const top = focused.top + gap + height > host.clientHeight - inset
      ? focused.top - gap - height
      : focused.top + gap;
    tooltip.style.left = `${Math.max(inset, Math.min(left, host.clientWidth - width - inset))}px`;
    tooltip.style.top = `${Math.max(inset, Math.min(top, host.clientHeight - height - inset))}px`;
  }, [focused, bucket, data, unit, i18n.language]);
  const format = new Intl.NumberFormat(i18n.language, kind === 'traffic'
    ? { minimumSignificantDigits: 2, maximumSignificantDigits: 3 }
    : { maximumFractionDigits: 1 });
  const metrics = kind === 'traffic' ? ['download', 'upload'] as const : ['connections'] as const;
  function clearCursor() {
    plotRef.current?.setCursor({ left: -10, top: -10 });
    if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
    frameRef.current = null;
    pendingFocusRef.current = null;
    setFocused(null);
  }
  return (
    <div
      className='relative min-h-40 flex-1 outline-none @max-[650px]:min-h-48'
      role='figure'
      aria-label={t(`dashboard.trend.${kind}`)}
      aria-describedby={descriptionId}
      tabIndex={0}
      onBlur={clearCursor}
      onMouseLeave={clearCursor}
      onKeyDown={(event) => {
        if (event.key === 'Escape') {
          clearCursor();
          return;
        }
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
        event.preventDefault();
        const plot = plotRef.current;
        if (!plot || !data[0].length) return;
        const direction = event.key === 'ArrowRight' ? 1 : -1;
        const index = Math.max(0, Math.min(data[0].length - 1,
          (focused?.index ?? (direction === 1 ? -1 : data[0].length)) + direction));
        const value = data[1]?.[index];
        plot.setCursor({
          left: plot.valToPos(data[0][index]!, 'x'),
          top: value == null ? plot.over.clientHeight / 2 : plot.valToPos(value, 'y'),
        });
      }}
    >
      <div ref={hostRef} className='absolute inset-0' />
      <output ref={tooltipRef} className={bucket ? 'pointer-events-none absolute top-0 left-0 z-1 grid w-max max-w-[calc(100%-16px)] gap-1.5 rounded-[var(--radius-card)] border bg-popover px-3 py-2.5 text-xs text-foreground shadow-[var(--shadow-popover)]' : 'sr-only'} id={descriptionId}>
        {bucket && focused
          ? metrics.map((metric, index) => {
              const value = data[index + 1]?.[focused.index];
              return (
                <span className='flex items-baseline justify-between gap-5' key={metric}>
                  <span className='flex items-center gap-1.5 text-muted-foreground'>
                    <i className={`size-1.5 flex-none rounded-full ${metric === 'upload' ? 'bg-chart-2' : 'bg-chart-1'}`} aria-hidden='true' />
                    {t(`dashboard.trend.${metric}`)}
                  </span>
                  <span className='font-semibold whitespace-nowrap tabular-nums'>
                    {value == null ? '—' : format.format(value)}
                    {' '}
                    <small className='text-2xs font-normal text-muted-foreground'>{unit ?? t('dashboard.metric.countUnit')}</small>
                  </span>
                </span>
              );
            })
          : t('dashboard.chart.arrows')}
      </output>
    </div>
  );
}
