import UPlot from 'uplot';
import { useTranslation } from 'react-i18next';
import { useEffect, useEffectEvent, useId, useLayoutEffect, useMemo, useRef, useState } from 'react';

import type { MetricsHistory } from '@/api/api-client';
import 'uplot/dist/uPlot.min.css';

export function TrendChart({
  history,
  kind,
}: {
  history: MetricsHistory | null;
  kind: 'traffic' | 'connections';
}) {
  const { t, i18n } = useTranslation();
  const hostRef = useRef<HTMLDivElement>(null);
  const tooltipRef = useRef<HTMLOutputElement>(null);
  const plotRef = useRef<UPlot | null>(null);
  const [focused, setFocused] = useState<{ index: number; left: number; top: number } | null>(null);
  const descriptionId = useId();
  const data = useMemo<UPlot.AlignedData>(() => {
    const buckets = history?.buckets ?? [];
    // A bucket's aggregate becomes available at its end, including the newest point at the right edge.
    const times = buckets.map((bucket) => Date.parse(bucket.to));
    return kind === 'traffic'
      ? [
          times,
          buckets.map((bucket) =>
            bucket.download_bytes === null
              ? null
              : bucket.download_bytes
                / ((Date.parse(bucket.to) - Date.parse(bucket.from)) / 1000)
                / 1024,
          ),
          buckets.map((bucket) =>
            bucket.upload_bytes === null
              ? null
              : bucket.upload_bytes
                / ((Date.parse(bucket.to) - Date.parse(bucket.from)) / 1000)
                / 1024,
          ),
        ]
      : [times, buckets.map((bucket) => bucket.active_connections_avg)];
  }, [history, kind]);
  const updatePlot = useEffectEvent(() => {
    const plot = plotRef.current;
    if (!plot) return;
    plot.batch(() => {
      plot.setData(data);
      if (history) {
        plot.setScale('x', { min: Date.parse(history.from), max: Date.parse(history.to) });
      }
    });
  });
  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const time = new Intl.DateTimeFormat(i18n.language, { hour: '2-digit', minute: '2-digit' });
    function create() {
      plotRef.current?.destroy();
      plotRef.current = null;
      const style = getComputedStyle(document.documentElement);
      const color = (name: string) => style.getPropertyValue(name).trim();
      const width = Math.floor(host!.clientWidth);
      const height = Math.floor(host!.clientHeight);
      if (width < 160 || height < 120) return;
      plotRef.current = new UPlot(
        {
          width,
          height,
          ms: 1,
          padding: [8, 8, 0, 0],
          legend: { show: false },
          cursor: { drag: { x: false, y: false }, points: { size: 6 } },
          scales: {
            x: { time: true, auto: false },
            y: { range: (_plot, _min, max) => [0, Math.max(1, max ?? 1) * 1.08] },
          },
          axes: [
            {
              size: 20,
              gap: 4,
              space: 85,
              stroke: color('--color-text-muted'),
              grid: { show: false },
              ticks: { show: false },
              font: '11px sans-serif',
              values: (_plot, splits) => splits.map((value) => time.format(new Date(value))),
            },
            {
              size: 32,
              gap: 4,
              space: 55,
              stroke: color('--color-text-muted'),
              ticks: { show: false },
              grid: { stroke: color('--color-border'), width: 1 },
              font: '11px sans-serif',
              values: (_plot, splits) =>
                splits.map((value) =>
                  new Intl.NumberFormat(i18n.language, {
                    maximumFractionDigits: 1,
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
              stroke: color('--color-primary'),
              // Keep the grid visible through the fill instead of tinting it with an opaque surface color.
              fill: `color-mix(in srgb, ${color('--color-primary')} 8%, transparent)`,
              width: 2,
              spanGaps: false,
              points: { show: false },
            },
            ...(kind === 'traffic'
              ? [
                  {
                    label: t('dashboard.trend.upload'),
                    stroke: getComputedStyle(host!).getPropertyValue('--trend-upload').trim(),
                    width: 1.7,
                    spanGaps: false,
                    points: { show: false },
                  },
                ]
              : []),
          ],
          hooks: {
            setCursor: [(plot) => {
              const { idx, left = -1, top = -1 } = plot.cursor;
              setFocused(idx == null || left < 0 || top < 0
                ? null
                : {
                    index: idx,
                    // uPlot cursor coordinates exclude the axes around its plotting area.
                    left: plot.over.offsetLeft + left,
                    top: plot.over.offsetTop + top,
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
      resize.disconnect();
      theme.disconnect();
      plotRef.current?.destroy();
      plotRef.current = null;
    };
  }, [kind, i18n.language, t]);
  useEffect(() => {
    updatePlot();
  }, [data, history]);
  const bucket = focused === null ? undefined : history?.buckets[focused.index];
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
  }, [focused, bucket]);
  const format = new Intl.NumberFormat(i18n.language, {
    minimumFractionDigits: kind === 'traffic' ? 1 : 0,
    maximumFractionDigits: 1,
  });
  const metrics = kind === 'traffic' ? ['download', 'upload'] as const : ['connections'] as const;
  function clearCursor() {
    plotRef.current?.setCursor({ left: -10, top: -10 });
    setFocused(null);
  }
  return (
    <div
      className='trend-chart'
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
      <div ref={hostRef} className='trend-chart__plot' />
      {(!history
        || history.buckets.every((item) =>
          kind === 'connections'
            ? item.active_connections_avg === null
            : item.upload_bytes === null && item.download_bytes === null,
        )) && <span className='trend-chart__empty'>{t('dashboard.empty.title')}</span>}
      <output ref={tooltipRef} className={bucket ? 'trend-chart__tooltip' : 'sr-only'} id={descriptionId}>
        {bucket && focused
          ? metrics.map((metric, index) => {
              const value = data[index + 1]?.[focused.index];
              return (
                <span className='trend-chart__tooltip-row' key={metric}>
                  <span className='trend-chart__tooltip-label'>
                    <i data-series={metric} aria-hidden='true' />
                    {t(`dashboard.trend.${metric}`)}
                  </span>
                  <span className='trend-chart__tooltip-value'>
                    {value == null ? '—' : format.format(value)}
                    {' '}
                    <small>{kind === 'traffic' ? 'KB/s' : t('dashboard.metric.countUnit')}</small>
                  </span>
                </span>
              );
            })
          : t('dashboard.chart.arrows')}
      </output>
    </div>
  );
}
