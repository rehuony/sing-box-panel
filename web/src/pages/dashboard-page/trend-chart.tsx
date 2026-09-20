import UPlot from 'uplot';
import { useTranslation } from 'react-i18next';
import { useEffect, useEffectEvent, useId, useMemo, useRef, useState } from 'react';

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
  const plotRef = useRef<UPlot | null>(null);
  const [focused, setFocused] = useState<number | null>(null);
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
              fill: color('--color-primary-soft'),
              width: 2,
              spanGaps: false,
              points: { show: false },
            },
            ...(kind === 'traffic'
              ? [
                  {
                    label: t('dashboard.trend.upload'),
                    stroke: '#2895A6',
                    width: 1.7,
                    spanGaps: false,
                    points: { show: false },
                  },
                ]
              : []),
          ],
          hooks: { setCursor: [(plot) => setFocused(plot.cursor.idx ?? null)] },
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
  const bucket = focused === null ? undefined : history?.buckets[focused];
  const values = !bucket
    ? ''
    : kind === 'connections'
      ? `${bucket.active_connections_avg ?? '—'}`
      : `${t('dashboard.trend.download')} ${bucket.download_bytes === null ? '—' : (bucket.download_bytes / ((Date.parse(bucket.to) - Date.parse(bucket.from)) / 1000) / 1024).toFixed(1)} · ${t('dashboard.trend.upload')} ${bucket.upload_bytes === null ? '—' : (bucket.upload_bytes / ((Date.parse(bucket.to) - Date.parse(bucket.from)) / 1000) / 1024).toFixed(1)} KB/s`;
  return (
    <div
      className='trend-chart'
      role='figure'
      aria-label={t(`dashboard.trend.${kind}`)}
      aria-describedby={descriptionId}
      tabIndex={0}
      onBlur={() => setFocused(null)}
      onMouseLeave={() => setFocused(null)}
      onKeyDown={(event) => {
        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return;
        event.preventDefault();
        setFocused((value) =>
          Math.max(
            0,
            Math.min(
              (history?.buckets.length ?? 1) - 1,
              (value ?? 0) + (event.key === 'ArrowRight' ? 1 : -1),
            ),
          ),
        );
      }}
    >
      <div ref={hostRef} className='trend-chart__plot' />
      {(!history
        || history.buckets.every((item) =>
          kind === 'connections'
            ? item.active_connections_avg === null
            : item.upload_bytes === null && item.download_bytes === null,
        )) && <span className='trend-chart__empty'>{t('dashboard.empty.title')}</span>}
      <output className={bucket ? 'trend-chart__tooltip' : 'sr-only'} id={descriptionId}>
        {bucket
          ? `${new Date(bucket.to).toLocaleTimeString(i18n.language)} · ${values}`
          : t('dashboard.chart.arrows')}
      </output>
    </div>
  );
}
