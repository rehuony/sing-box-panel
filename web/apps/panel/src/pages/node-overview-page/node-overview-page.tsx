import { cn } from '@shared/lib/utils';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useState } from 'react';
import { Spinner } from '@shared/components/ui/spinner';
import { formatDuration } from '@shared/lib/telemetry-format';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

import type { RuntimeHistoryPage, RuntimeTransitionState } from '@/api/api-client';

import { nodePath } from '@/utils/node-path';
import { percentOf } from '@/utils/fleet-summary';
import { ErrorNotice } from '@/components/error-notice';
import { useCurrentNode } from '@/stores/current-node.store';
import { KeyFigure, KeyFigures } from '@/components/key-figures';
import { PanelLogLevelText } from '@/components/panel-log-level';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';
import { useDashboardSubscription, useOptionalSharedTelemetry } from '@/components/node-workspace';

import { TrendChart } from './trend-chart';
import { buildRuntimeSlots, buildRuntimeTimeline } from './runtime-timeline';
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
const timelineStates = ['running', 'failed', 'stopped', 'unknown'] as const satisfies readonly RuntimeTransitionState[];
/** Where each mark of the axis sits under the bar's quarters; on a narrow card only the ends and the middle. */
const axisMark: Record<number, string> = {
  24: 'col-start-1 row-start-1 w-max whitespace-nowrap',
  18: 'col-start-2 row-start-1 w-max -translate-x-1/2 whitespace-nowrap @max-[26rem]:hidden',
  12: 'col-start-3 row-start-1 w-max -translate-x-1/2 whitespace-nowrap @max-[26rem]:col-start-2',
  6: 'col-start-4 row-start-1 w-max -translate-x-1/2 whitespace-nowrap @max-[26rem]:hidden',
  0: 'col-start-4 row-start-1 w-max justify-self-end whitespace-nowrap @max-[26rem]:col-start-2',
};
const stateSwatch: Record<RuntimeTransitionState, string> = {
  running: 'bg-[var(--color-success)]',
  failed: 'bg-[var(--color-danger)]',
  stopped: 'bg-muted-foreground',
  // The unknown state's fine hatch, as in a cell meter, so a gap in the record never reads as a state.
  unknown: 'bg-[repeating-linear-gradient(135deg,var(--color-border-strong)_0_1px,var(--color-surface-raised)_0_3px)]',
};
const titleBar = 'flex h-11 flex-none items-center gap-2 border-b ps-4 pe-2';
const card = 'relative flex min-w-0 flex-col rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]';

export function NodeOverviewPage() {
  const { t } = useTranslation();
  useDashboardSubscription();
  return (
    // The overview fills at least the window; the charts take the room the other cards leave.
    <div className='@container flex min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)]'>
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
      className={`${card} flex-1 gap-0`}
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
      <TabsContent className='relative grid flex-1 grid-rows-[3fr_2fr] gap-6 px-2 pt-3 pb-2 @min-[100rem]:grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)] @min-[100rem]:grid-rows-[1fr] @min-[100rem]:gap-4' value={range}>
        {/* The charts share the card's height, traffic three parts to two, each above a legible floor. */}
        <div className='flex min-h-60 min-w-0 flex-col gap-1'>
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
        <div className='flex min-h-44 min-w-0 flex-col gap-1 @min-[100rem]:min-h-60'>
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

function useDurationUnits() {
  const { t } = useTranslation();
  return { day: t('telemetry.unit.day'), hour: t('telemetry.unit.hour'), minute: t('telemetry.unit.minute'), second: t('telemetry.unit.second') };
}

type StateTotals = Record<RuntimeTransitionState, number>;

/** How long each runtime state held within the window, from its exact segments. */
function stateTotals(history: RuntimeHistoryPage | null, from: number, to: number): StateTotals {
  const totals: StateTotals = { failed: 0, running: 0, stopped: 0, unknown: 0 };
  for (const segment of buildRuntimeTimeline(history, from, to)) {
    totals[segment.state] += Math.max(0, Math.min(to, segment.end) - Math.max(from, segment.start));
  }
  return totals;
}

/** The timeline's legend: each state's swatch, name and, where there is room, how long it held. */
function TimelineLegend({ className, durationClassName, totals }: {
  totals: StateTotals | null;
  className?: string;
  durationClassName?: string;
}) {
  const { t } = useTranslation();
  const units = useDurationUnits();
  return (
    <ul className={cn('m-0 flex list-none gap-x-3 gap-y-1 p-0 font-[family-name:var(--font-data)] text-xs whitespace-nowrap text-muted-foreground', className)}>
      {timelineStates.map(state => (
        <li className='flex items-center gap-1.5' key={state}>
          <i aria-hidden='true' className={cn('h-3 w-2 flex-none rounded-[2px]', stateSwatch[state])} />
          {t(`dashboard.timeline.state.${state}`)}
          {totals === null
            ? null
            : <span className={cn('text-[var(--color-text-secondary)] tabular-nums', durationClassName)}>{formatDuration(totals[state], units)}</span>}
        </li>
      ))}
    </ul>
  );
}

/**
 * The runtime of the last day as a bar of 48 cells between brackets, the
 * legend in the title bar carrying each state's total, and the activity
 * beside it as log lines. Both cards are compact, so the charts above take
 * the room.
 */
function DashboardHistory() {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const current = useOptionalSharedTelemetry(s => s.dashboardSnapshot);
  const node = useCurrentNode();
  const logs = `${nodePath(node.id, 'logs')}#logs-events`;
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
  const history = current?.runtime_24h ?? null;
  const slots = useMemo(() => buildRuntimeSlots(history, start, end), [history, start, end]);
  const totals = useMemo(() => current ? stateTotals(history, start, end) : null, [current, history, start, end]);
  const bar = 'flex h-11 flex-none items-center gap-3 border-b px-4';
  return (
    <div className='grid flex-none grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)] gap-3 @max-[650px]:grid-cols-1'>
      <section
        className={`${card} @container`}
        aria-label={t('dashboard.timeline.title')}
        aria-busy={current === null}
      >
        <div className={bar}>
          <h2 className='min-w-0 flex-1 truncate text-sm font-medium'>{t('dashboard.timeline.title')}</h2>
          <TimelineLegend className='flex-none @max-[26rem]:hidden' durationClassName='@max-[40rem]:hidden' totals={totals} />
        </div>
        <div className='flex flex-1 flex-col justify-center gap-1.5 px-4 py-3 font-[family-name:var(--font-data)]'>
          {/* Short of room in the title bar, the legend takes a line of its own. */}
          <TimelineLegend className='hidden flex-wrap @max-[26rem]:flex' totals={totals} />
          <div className='grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-1.5 gap-y-1 text-compact text-[var(--color-text-faint)]'>
            <span aria-hidden='true'>[</span>
            <div className='grid h-4 grid-cols-[repeat(48,minmax(0,1fr))] gap-0.5' role='group' aria-label={t('dashboard.timeline.ariaLabel')}>
              {slots.map((slot) => {
                const from = formatLogTime(new Date(slot.start).toISOString(), locale);
                const to = formatLogTime(new Date(slot.end).toISOString(), locale);
                const label = `${from} – ${to} · ${t(`dashboard.timeline.state.${slot.state}`)}`;
                return (
                  <Tooltip key={slot.id}>
                    <TooltipTrigger
                      className={cn('min-w-0 rounded-[2px] border-0 p-0 hover:shadow-[inset_0_0_0_1px_var(--foreground)]', stateSwatch[slot.state])}
                      data-state={slot.state}
                      // Forty-eight cells are no Tab stops: the legend gives the day's
                      // totals, and each cell names itself to assistive technology.
                      tabIndex={-1}
                      aria-label={label}
                      disabled={current === null}
                    />
                    <TooltipContent role='tooltip'>{label}</TooltipContent>
                  </Tooltip>
                );
              })}
            </div>
            <span aria-hidden='true'>]</span>
            {/* The axis counts back from now, so its marks read the same whenever the window is opened. */}
            <div className='col-start-2 grid grid-cols-4 text-2xs tabular-nums @max-[26rem]:grid-cols-2'>
              {[24, 18, 12, 6, 0].map(hours => (
                <time
                  className={axisMark[hours]}
                  dateTime={new Date(end - (hours * 3_600_000)).toISOString()}
                  key={hours}
                >
                  {hours === 0 ? t('dashboard.timeline.now') : `-${hours}h`}
                </time>
              ))}
            </div>
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
      </section>
      <section
        className={`${card} @container`}
        aria-label={t('dashboard.activity.title')}
        aria-busy={current === null}
      >
        <div className={bar}>
          <h2 className='min-w-0 flex-1 truncate text-sm font-medium'>{t('dashboard.activity.title')}</h2>
          <Link className='font-[family-name:var(--font-data)] text-xs text-muted-foreground hover:text-foreground' to={logs}>
            {t('dashboard.activity.viewAll')}
          </Link>
        </div>
        {/* Two log lines' height whatever the activity holds, so the row keeps its size while it loads. */}
        <div className='relative min-h-[calc(2*1.75rem+0.75rem)] flex-1 px-2 py-1.5'>
          <ol className='m-0 grid list-none grid-cols-[max-content_max-content_minmax(0,1fr)] p-0 font-[family-name:var(--font-data)] text-compact'>
            {current?.activity.items.map((entry) => {
              const title = logTitle(entry, t);
              const fullTime = formatLogTime(entry.time, locale);
              return (
                <li className='col-span-3 grid grid-cols-subgrid' key={entry.id}>
                  <Tooltip>
                    <TooltipTrigger render={<Link className='col-span-3 grid min-h-7 grid-cols-subgrid items-baseline gap-x-3 rounded-[var(--radius-control)] px-2 py-1 transition-colors duration-[var(--dur-micro)] ease-out hover:bg-[var(--color-hover)] focus-visible:bg-[var(--color-hover)] motion-reduce:transition-none' to={logs} />}>
                      <time className='whitespace-nowrap text-[var(--color-text-faint)] tabular-nums' dateTime={entry.time} aria-label={fullTime}>
                        {formatActivityTime(entry.time, locale, today)}
                      </time>
                      <PanelLogLevelText level={entry.level} />
                      <span className='min-w-0 truncate font-[family-name:var(--font-body)]'>{title}</span>
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
