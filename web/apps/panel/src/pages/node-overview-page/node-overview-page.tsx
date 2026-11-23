import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useState } from 'react';
import { Spinner } from '@shared/components/ui/spinner';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

import { percentOf } from '@/utils/fleet-summary';
import { ErrorNotice } from '@/components/error-notice';
import { PanelLogLevel } from '@/components/panel-log-level';
import { useCurrentNode } from '@/stores/current-node.store';
import { KeyFigure, KeyFigures } from '@/components/key-figures';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';
import { useDashboardSubscription, useOptionalSharedTelemetry } from '@/components/node-workspace';

import { TrendChart } from './trend-chart';
import { buildRuntimeSlots } from './runtime-timeline';
import { liveConnectionsChartData, liveTrafficChartData, trendChartData } from './trend-chart-data';

const emptyTail: import('@/components/node-workspace').LivePoint[] = [];

function bytes(value: number | null | undefined, locale: string): string {
  if (value == null) return '—';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let amount = value;
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024;
    index++;
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(amount)} ${units[index]}`;
}
function percent(used: number | null | undefined, total: number | null | undefined): string {
  return used == null || !total ? '—' : `${((100 * used) / total).toFixed(1)}%`;
}
function gibibytes(value: number | undefined, locale: string): string {
  if (value === undefined) return '—';
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 2 }).format(value / 2 ** 30);
}
// Axis labels at 0, 6, 12, 18 and 24 hours; narrow cards keep the ends and the middle.
const axisTime: Record<number, string> = {
  0: 'col-start-1 row-start-1 flex w-max gap-1 whitespace-nowrap',
  6: 'col-start-2 row-start-1 w-max -translate-x-1/2 whitespace-nowrap @max-[650px]:hidden',
  12: 'col-start-3 row-start-1 w-max -translate-x-1/2 whitespace-nowrap @max-[650px]:col-start-2',
  18: 'col-start-4 row-start-1 w-max -translate-x-1/2 whitespace-nowrap @max-[650px]:hidden',
  24: 'col-start-4 row-start-1 flex w-max gap-1 justify-self-end whitespace-nowrap @max-[650px]:col-start-2',
};
const legendSwatch = {
  running: 'bg-[var(--color-success)]',
  failed: 'bg-[var(--color-danger)]',
  stopped: 'bg-muted-foreground',
  // The unknown state's fine hatch, as in a cell meter, so a gap in the record never reads as a state.
  unknown: 'bg-[repeating-linear-gradient(135deg,var(--color-border-strong)_0_1px,var(--color-surface-raised)_0_3px)]',
} as const;
/** A card's title bar, as on settings cards: its title, then what it says or offers at the end, over a hairline. */
const titleBar = 'flex h-11 flex-none items-center gap-2 border-b ps-4 pe-2';
const card = 'relative flex min-w-0 flex-col rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]';

export function NodeOverviewPage() {
  const { t } = useTranslation();
  useDashboardSubscription();
  return (
    <div className='@container flex min-w-0 flex-none flex-col gap-4'>
      <h1 className='sr-only'>{t('node.tabs.overview')}</h1>
      <DashboardStatus />
      <DashboardMetrics />
      <DashboardCharts />
      <DashboardHistory />
    </div>
  );
}

function DashboardStatus() {
  const { t } = useTranslation();
  const trafficError = useOptionalSharedTelemetry(s => s.trafficError);
  const historyError = useOptionalSharedTelemetry(s => s.dashboardError);
  return (
    <>
      {trafficError ? <ErrorNotice error={trafficError} title={t('dashboard.error.metrics')} /> : null}
      {historyError ? <ErrorNotice error={historyError} title={t('dashboard.error.history')} /> : null}
    </>
  );
}

/** The node's key figures: host load with its meters, and this period's traffic against the node's quota. */
function DashboardMetrics() {
  const { t, i18n } = useTranslation();
  const snapshot = useOptionalSharedTelemetry(s => s.snapshot);
  const host = snapshot?.host;
  const traffic = snapshot?.current_traffic_period;
  const used = traffic ? traffic.inbound_bytes + traffic.outbound_bytes : undefined;
  const quota = snapshot?.quota_bytes;
  const locale = i18n.language;
  let usageTone: 'danger' | 'warning' | undefined;
  if (snapshot?.quota_exceeded) usageTone = 'danger';
  else if (quota && used !== undefined && (quota - used) / quota <= 0.1) usageTone = 'warning';
  return (
    <KeyFigures label={t('dashboard.metric.label')}>
      <KeyFigure
        label={t('dashboard.metric.cpu')}
        meta={host ? t('dashboard.metric.cpuDetail', { count: host.cpu_count, load: host.load_one ?? '—' }) : '—'}
        meter={host?.cpu_percent}
      >
        {host?.cpu_percent == null ? '—' : `${host.cpu_percent.toFixed(1)}%`}
      </KeyFigure>
      <KeyFigure
        label={t('dashboard.metric.memory')}
        meta={`${bytes(host?.memory_used, locale)} / ${bytes(host?.memory_total, locale)}`}
        meter={percentOf(host?.memory_used, host?.memory_total) ?? undefined}
      >
        {percent(host?.memory_used, host?.memory_total)}
      </KeyFigure>
      <KeyFigure
        label={t('dashboard.metric.disk')}
        meta={`${bytes(host?.disk_used, locale)} / ${bytes(host?.disk_total, locale)}`}
        meter={percentOf(host?.disk_used, host?.disk_total) ?? undefined}
      >
        {percent(host?.disk_used, host?.disk_total)}
      </KeyFigure>
      <KeyFigure
        label={t('dashboard.metric.transfer')}
        meta={quota
          ? t('dashboard.metric.quota', { used: percent(used, quota), total: bytes(quota, locale) })
          : `${gibibytes(used, locale)} / ∞ GiB`}
        tone={usageTone}
      >
        {used === undefined ? t('dashboard.metric.usageUnknown') : bytes(used, locale)}
      </KeyFigure>
    </KeyFigures>
  );
}

const ranges = ['live', '1h', '24h'] as const;

/** Traffic and connections over one time range, the live one by default. */
function DashboardCharts() {
  const { t } = useTranslation();
  const current = useOptionalSharedTelemetry(s => s.dashboardSnapshot);
  const metrics = useOptionalSharedTelemetry(s => s.snapshot);
  const tail = useOptionalSharedTelemetry(s => s.liveTail) ?? emptyTail;
  const liveStale = useOptionalSharedTelemetry(s => s.liveStale);
  const historyStale = useOptionalSharedTelemetry(s => s.dashboardStale);
  const [range, setRange] = useState<typeof ranges[number]>('live');
  const history = current ? range === '1h' ? current.history_1h : current.history_24h : null;
  const watermark = current?.persisted_through;
  const traffic = useMemo(() => range === 'live'
    ? liveTrafficChartData(tail)
    : trendChartData(history, 'traffic', emptyTail), [range, history, tail]);
  const connections = useMemo(() => range === 'live'
    ? liveConnectionsChartData(tail)
    : trendChartData(history, 'connections', tail, watermark), [range, history, tail, watermark]);
  const live = range === 'live';
  const loading = live ? metrics === null : current === null;
  // When the stream or the history falls behind, the title bar says since when, so nothing below moves.
  const staleAt = live
    ? liveStale && metrics ? metrics.collected_at : undefined
    : historyStale && current ? current.collected_at : undefined;
  return (
    <Tabs
      className={`${card} flex-none gap-0`}
      render={<section aria-labelledby='dashboard-trend-title' />}
      value={range}
      onValueChange={(value) => setRange(value as typeof range)}
    >
      <div className={titleBar}>
        <h2 className='min-w-0 flex-1 truncate text-sm font-medium' id='dashboard-trend-title'>{t('dashboard.trend.title')}</h2>
        {staleAt
          ? (
              <span className='truncate text-xs text-[var(--color-warning-text)] max-sm:hidden' role='status'>
                {t(live ? 'dashboard.state.liveStale' : 'dashboard.state.historyStale', { time: new Date(staleAt).toLocaleTimeString() })}
              </span>
            )
          : null}
        <TabsList aria-label={t('dashboard.range.label')} className='flex-none' variant='pill'>
          {ranges.map((value) => (
            <TabsTrigger key={value} value={value}>
              {t(`dashboard.range.option.${value}`)}
            </TabsTrigger>
          ))}
        </TabsList>
      </div>
      {/* Traffic leads and connections follow below it; only a very wide window has room for them side by side. */}
      <TabsContent className='relative grid gap-6 px-2 pt-3 pb-2 @min-[100rem]:grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)] @min-[100rem]:gap-4' value={range}>
        {/* Each chart grows with the window above a floor that keeps its axes legible. */}
        <div className='flex h-[max(15rem,30dvh)] min-w-0 flex-col gap-1'>
          <div className='mx-3 flex h-5 items-center gap-2 text-xs text-muted-foreground'>
            <h3 className='m-0 flex items-baseline gap-2 text-xs font-medium text-foreground'>
              {t('dashboard.trend.traffic')}
              {' '}
              <span className='font-normal text-muted-foreground tabular-nums'>{traffic.unit}</span>
            </h3>
            <span className='ms-auto flex items-center gap-3'>
              {(['download', 'upload'] as const).map(metric => (
                <span className='flex items-center gap-1.5' key={metric}>
                  <i aria-hidden='true' className={`h-0.5 w-3 rounded-full ${metric === 'upload' ? 'bg-chart-2' : 'bg-chart-1'}`} />
                  {t(`dashboard.trend.${metric}`)}
                </span>
              ))}
            </span>
          </div>
          <TrendChart chart={traffic} kind='traffic' live={live} />
        </div>
        <div className='flex h-[max(11rem,20dvh)] min-w-0 flex-col gap-1 @min-[100rem]:h-[max(15rem,30dvh)]'>
          <div className='mx-3 flex h-5 items-center gap-2 text-xs text-muted-foreground'>
            <h3 className='m-0 text-xs font-medium text-foreground'>{t('dashboard.trend.connections')}</h3>
          </div>
          <TrendChart chart={connections} kind='connections' live={live} />
        </div>
        {loading
          ? (
              <div className='absolute inset-0 z-1 grid place-content-center justify-items-center gap-2.5 bg-[color-mix(in_srgb,var(--color-surface)_84%,transparent)] text-xs text-muted-foreground'>
                <Spinner />
                <span>{t(live ? 'dashboard.state.liveLoading' : 'dashboard.state.historyLoading')}</span>
              </div>
            )
          : null}
      </TabsContent>
    </Tabs>
  );
}

function formatActivityTime(value: string, locale: string, today: Date): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const sameYear = date.getFullYear() === today.getFullYear();
  const sameDay = sameYear && date.getMonth() === today.getMonth() && date.getDate() === today.getDate();
  return new Intl.DateTimeFormat(locale, {
    ...(!sameYear ? { year: 'numeric' } as const : {}),
    ...(!sameDay ? { month: '2-digit', day: '2-digit' } as const : {}),
    hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  }).format(date);
}

function DashboardHistory() {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const current = useOptionalSharedTelemetry(s => s.dashboardSnapshot);
  const node = useCurrentNode();
  const logs = `/logs?node=${encodeURIComponent(node.id)}`;
  const [initialEnd] = useState(() => Date.now());
  const [today, setToday] = useState(() => new Date(initialEnd));
  const end = current ? Date.parse(current.collected_at) : initialEnd;
  const start = end - 86_400_000;
  useEffect(() => {
    const tomorrow = new Date(today);
    tomorrow.setHours(24, 0, 0, 0);
    const timer = setTimeout(() => setToday(new Date()), Math.max(0, tomorrow.getTime() - Date.now()));
    return () => clearTimeout(timer);
  }, [today]);
  const slots = useMemo(
    () => buildRuntimeSlots(current?.runtime_24h ?? null, start, end), [current?.runtime_24h, start, end],
  );
  const timeFormat = new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
  const endpointFormat = new Intl.DateTimeFormat(locale, {
    ...(new Date(start).getFullYear() !== new Date(end).getFullYear() ? { year: 'numeric' } as const : {}),
    month: '2-digit', day: '2-digit',
  });
  return (
    <div className='grid min-h-45 flex-none grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)] gap-4 @max-[650px]:grid-cols-1'>
      <section
        className={`${card} @container min-h-45`}
        aria-label={t('dashboard.timeline.title')}
        aria-busy={current === null}
      >
        <div className={`${titleBar} pe-4`}>
          <h2 className='min-w-0 flex-1 truncate text-sm font-medium'>{t('dashboard.timeline.title')}</h2>
          <small className='text-xs text-muted-foreground'>{t('dashboard.metric.lastDay')}</small>
        </div>
        <div className='grid gap-2 px-4 pt-4'>
          <div className='grid h-6 grid-cols-[repeat(48,minmax(0,1fr))] gap-0.5' role='group' aria-label={t('dashboard.timeline.ariaLabel')}>
            {slots.map((slot) => {
              const from = formatLogTime(new Date(slot.start).toISOString(), locale);
              const to = formatLogTime(new Date(slot.end).toISOString(), locale);
              const label = `${from} – ${to} · ${t(`dashboard.timeline.state.${slot.state}`)}`;
              return (
                <Tooltip key={slot.id}>
                  <TooltipTrigger
                    className='min-w-0 rounded-[2px] border-0 bg-muted-foreground p-0 outline-offset-3 hover:shadow-[inset_0_0_0_1px_var(--foreground)] focus-visible:shadow-[inset_0_0_0_1px_var(--foreground)] focus-visible:outline-2 focus-visible:outline-ring data-[state=failed]:bg-[var(--color-danger)] data-[state=running]:bg-[var(--color-success)] data-[state=unknown]:bg-[repeating-linear-gradient(135deg,var(--color-border-strong)_0_1px,var(--color-surface-raised)_0_3px)]'
                    data-state={slot.state}
                    aria-label={label}
                    disabled={current === null}
                  />
                  <TooltipContent role='tooltip'>{label}</TooltipContent>
                </Tooltip>
              );
            })}
          </div>
          <div className='grid grid-cols-4 items-start text-xs text-muted-foreground tabular-nums @max-[650px]:grid-cols-2'>
            {[0, 6, 12, 18, 24].map((hour) => {
              const at = new Date(start + hour * 3_600_000);
              const endpoint = hour === 0 || hour === 24;
              return (
                <time
                  className={axisTime[hour]}
                  key={hour}
                  dateTime={at.toISOString()}
                >
                  {endpoint ? <span>{endpointFormat.format(at)}</span> : null}
                  {endpoint ? ' ' : null}
                  <span>{timeFormat.format(at)}</span>
                </time>
              );
            })}
          </div>
        </div>
        {current === null
          ? (
              <div className='absolute inset-x-3 top-12 bottom-3 z-1 grid place-content-center justify-items-center gap-2.5 bg-[color-mix(in_srgb,var(--color-surface)_84%,transparent)] text-xs text-muted-foreground'>
                <Spinner />
                <span>{t('dashboard.state.runtimeLoading')}</span>
              </div>
            )
          : null}
        <div className='flex flex-wrap gap-4 px-4 pt-3 pb-4'>
          {(['running', 'failed', 'stopped', 'unknown'] as const).map((state) => (
            <span className='flex items-center gap-1.5 text-xs text-muted-foreground' key={state}>
              <i aria-hidden className={`size-2.5 rounded-[2px] ${legendSwatch[state]}`} />
              {t(`dashboard.timeline.state.${state}`)}
            </span>
          ))}
        </div>
      </section>
      <section
        className={`${card} @container min-h-45`}
        aria-label={t('dashboard.activity.title')}
        aria-busy={current === null}
      >
        <div className={`${titleBar} pe-4`}>
          <h2 className='min-w-0 flex-1 truncate text-sm font-medium'>{t('dashboard.activity.title')}</h2>
          <Link className='text-xs text-muted-foreground hover:text-foreground' to={logs}>{t('dashboard.activity.viewAll')}</Link>
        </div>
        <div className='relative min-h-22 flex-1 px-2 py-1'>
          <ol className='m-0 list-none p-0 [&>li+li]:border-t'>
            {current?.activity.items.map((entry) => {
              const title = logTitle(entry, t);
              const fullTime = formatLogTime(entry.time, locale);
              return (
                <li key={entry.id}>
                  <Tooltip>
                    <TooltipTrigger render={<Link className='grid min-h-11 grid-cols-[54px_minmax(0,1fr)_max-content] items-center gap-3 rounded-[var(--radius-control)] p-2 text-compact transition-colors duration-[var(--dur-micro)] ease-out hover:bg-[var(--color-hover)] focus-visible:bg-[var(--color-hover)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring motion-reduce:transition-none @max-[650px]:gap-2 @max-[650px]:px-0' to={logs} />}>
                      <span className='flex justify-center'><PanelLogLevel level={entry.level} /></span>
                      <span className='min-w-0 truncate'>{title}</span>
                      <time className='text-end text-xs whitespace-nowrap text-muted-foreground tabular-nums' dateTime={entry.time} aria-label={fullTime}>
                        {formatActivityTime(entry.time, locale, today)}
                      </time>
                    </TooltipTrigger>
                    <TooltipContent role='tooltip'>
                      <span className='grid gap-1 wrap-anywhere'>
                        <span>{title}</span>
                        <span>{fullTime}</span>
                      </span>
                    </TooltipContent>
                  </Tooltip>
                </li>
              );
            })}
          </ol>
          {current?.activity.items.length === 0 ? <small className='absolute inset-0 grid place-content-center text-xs text-muted-foreground'>{t('dashboard.activity.empty')}</small> : null}
        </div>
        {current === null
          ? (
              <div className='absolute inset-x-3 top-12 bottom-3 z-1 grid place-content-center justify-items-center gap-2.5 bg-[color-mix(in_srgb,var(--color-surface)_84%,transparent)] text-xs text-muted-foreground'>
                <Spinner />
                <span>{t('dashboard.state.activityLoading')}</span>
              </div>
            )
          : null}
      </section>
    </div>
  );
}
