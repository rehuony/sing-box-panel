import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useState } from 'react';
import { Spinner } from '@shared/components/ui/spinner';
import { Cpu, HardDrive, MemoryStick, Radio } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

import { ErrorNotice } from '@/components/error-notice';
import { PanelLogLevel } from '@/components/panel-log-level';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';
import { useDashboardSubscription, useOptionalSharedTelemetry } from '@/components/node-workspace';

import { TrendChart } from './trend-chart';
import { buildRuntimeSlots } from './runtime-timeline';
import { liveTrafficChartData, trendChartData } from './trend-chart-data';

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
  0: 'col-start-1 row-start-1 flex w-max gap-1 whitespace-nowrap @max-[400px]:flex-col @max-[400px]:gap-0',
  6: 'col-start-2 row-start-1 w-max -translate-x-1/2 whitespace-nowrap @max-[650px]:hidden',
  12: 'col-start-3 row-start-1 w-max -translate-x-1/2 whitespace-nowrap @max-[650px]:col-start-2',
  18: 'col-start-4 row-start-1 w-max -translate-x-1/2 whitespace-nowrap @max-[650px]:hidden',
  24: 'col-start-4 row-start-1 flex w-max gap-1 justify-self-end whitespace-nowrap @max-[650px]:col-start-2 @max-[400px]:flex-col @max-[400px]:items-end @max-[400px]:gap-0',
};
const legendSwatch = {
  running: 'bg-[var(--color-success)]',
  failed: 'bg-[var(--color-danger)]',
  stopped: 'bg-muted-foreground',
  unknown: 'bg-[repeating-linear-gradient(135deg,var(--color-surface-raised)_0_4px,var(--color-border-strong)_4px_6px)]',
} as const;

export function NodeOverviewPage() {
  const { t } = useTranslation();
  useDashboardSubscription();
  return (
    <div className='@container flex min-w-0 flex-col gap-4'>
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
  const stale = useOptionalSharedTelemetry(s => s.dashboardStale);
  const snapshot = useOptionalSharedTelemetry(s => s.dashboardSnapshot);
  const liveStale = useOptionalSharedTelemetry(s => s.liveStale);
  const updatedAt = useOptionalSharedTelemetry(s => s.snapshot?.collected_at);
  return (
    <>
      {trafficError ? <ErrorNotice error={trafficError} title={t('dashboard.error.metrics')} /> : null}
      {historyError ? <ErrorNotice error={historyError} title={t('dashboard.error.history')} /> : null}
      {liveStale && updatedAt ? <small className='text-xs text-[var(--color-warning-text)]' role='status'>{t('dashboard.state.liveStale', { time: new Date(updatedAt).toLocaleTimeString() })}</small> : null}
      {stale && snapshot ? <small className='text-xs text-[var(--color-warning-text)]' role='status'>{t('dashboard.state.historyStale', { time: new Date(snapshot.collected_at).toLocaleTimeString() })}</small> : null}
    </>
  );
}

function DashboardMetrics() {
  const { t, i18n } = useTranslation();
  const snapshot = useOptionalSharedTelemetry(s => s.snapshot);
  const host = snapshot?.host;
  const traffic = snapshot?.current_traffic_period;
  const used = traffic ? traffic.inbound_bytes + traffic.outbound_bytes : undefined;
  const locale = i18n.language;
  const metrics = [
    {
      key: 'cpu',
      icon: Cpu,
      value: host?.cpu_percent == null ? '—' : `${host.cpu_percent.toFixed(1)}%`,
      detail: host
        ? t('dashboard.metric.cpuDetail', { count: host.cpu_count, load: host.load_one ?? '—' })
        : '—',
    },
    {
      key: 'memory',
      icon: MemoryStick,
      value: percent(host?.memory_used, host?.memory_total),
      detail: `${bytes(host?.memory_used, locale)} / ${bytes(host?.memory_total, locale)}`,
    },
    {
      key: 'disk',
      icon: HardDrive,
      value: percent(host?.disk_used, host?.disk_total),
      detail: `${bytes(host?.disk_used, locale)} / ${bytes(host?.disk_total, locale)}`,
    },
    {
      key: 'transfer',
      icon: Radio,
      value: used === undefined ? t('dashboard.metric.usageUnknown') : bytes(used, locale),
      detail: snapshot?.quota_bytes
        ? t('dashboard.metric.quota', {
            used: percent(used, snapshot.quota_bytes),
            total: bytes(snapshot.quota_bytes, locale),
          })
        : `${gibibytes(used, locale)} / ∞ GiB`,
    },
  ];
  return (
    <div className='grid flex-none grid-cols-4 gap-4 @max-[650px]:grid-cols-2'>
      {metrics.map(({ key, icon: Icon, value, detail }) => (
        <section key={key} className='grid min-w-0 gap-2.5 rounded-[var(--radius-card)] border bg-card p-5 @max-[850px]:p-4'>
          <div className='flex items-center justify-between text-compact text-muted-foreground'>
            <span>{t(`dashboard.metric.${key}`)}</span>
            <Icon size={18} />
          </div>
          <strong className='text-2xl font-semibold tabular-nums'>{value}</strong>
          <small className='truncate text-xs text-muted-foreground'>{detail}</small>
        </section>
      ))}
    </div>
  );
}

function DashboardCharts() {
  const { t } = useTranslation();
  const current = useOptionalSharedTelemetry(s => s.dashboardSnapshot);
  const metrics = useOptionalSharedTelemetry(s => s.snapshot);
  const tail = useOptionalSharedTelemetry(s => s.liveTail) ?? emptyTail;
  const [range, setRange] = useState<'live' | '1h' | '24h'>('live');
  const trafficHistory = current ? range === '1h' ? current.history_1h : current.history_24h : null;
  const watermark = current?.persisted_through;
  const traffic = useMemo(() => range === 'live'
    ? liveTrafficChartData(tail)
    : trendChartData(trafficHistory, 'traffic', emptyTail), [range, trafficHistory, tail]);
  const connections = useMemo(() => trendChartData(current?.history_1h ?? null, 'connections', tail, watermark), [current?.history_1h, tail, watermark]);
  return (
    <div className='grid min-h-min flex-1 grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)] gap-4 @max-[850px]:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)] @max-[650px]:flex-none @max-[650px]:grid-cols-1'>
      <Tabs
        className='relative flex min-w-0 flex-col rounded-[var(--radius-card)] border bg-card gap-2 px-2 pt-4 pb-1'
        render={<section />}
        value={range}
        onValueChange={(value) => setRange(value as typeof range)}
      >
        <header className='mx-3 flex min-h-(--control-height) items-center justify-between gap-2 @max-[850px]:flex-wrap'>
          <h2 className='m-0 text-sm font-semibold'>
            {t('dashboard.trend.traffic')}
            （
            {traffic.unit}
            ）
          </h2>
          <TabsList aria-label={t('dashboard.range.label')}>
            {(['live', '1h', '24h'] as const).map((value) => (
              <TabsTrigger key={value} value={value}>
                {t(`dashboard.range.option.${value}`)}
              </TabsTrigger>
            ))}
          </TabsList>
        </header>
        {(['live', '1h', '24h'] as const).map((value) => (
          <TabsContent key={value} value={value} className='relative flex min-w-0'>
            <TrendChart chart={traffic} kind='traffic' live={range === 'live'} />
            {(range === 'live' ? metrics === null : current === null)
              ? (
                  <div className='absolute inset-0 z-1 grid place-content-center justify-items-center gap-2.5 bg-[color-mix(in_srgb,var(--color-surface)_84%,transparent)] text-xs text-muted-foreground'>
                    <Spinner />
                    <span>{t(range === 'live' ? 'dashboard.state.liveLoading' : 'dashboard.state.historyLoading')}</span>
                  </div>
                )
              : null}
          </TabsContent>
        ))}
      </Tabs>
      <section className='relative flex min-w-0 flex-col rounded-[var(--radius-card)] border bg-card gap-2 px-2 pt-4 pb-1'>
        <header className='mx-3 flex min-h-(--control-height) items-center justify-between gap-2 @max-[850px]:flex-wrap'>
          <h2 className='m-0 text-sm font-semibold'>
            {t('dashboard.trend.connections')}
            （
            {t('dashboard.metric.countUnit')}
            ）
          </h2>
          <small className='text-xs text-muted-foreground'>{t('dashboard.metric.lastHour')}</small>
        </header>
        <TrendChart
          chart={connections}
          kind='connections'
        />
        {current === null
          ? (
              <div className='absolute inset-x-2 top-13 bottom-1 z-1 grid place-content-center justify-items-center gap-2.5 bg-[color-mix(in_srgb,var(--color-surface)_84%,transparent)] text-xs text-muted-foreground'>
                <Spinner />
                <span>{t('dashboard.state.historyLoading')}</span>
              </div>
            )
          : null}
      </section>
    </div>
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
        className='@container relative flex min-w-0 flex-col rounded-[var(--radius-card)] border bg-card min-h-45 gap-3 p-5 @max-[850px]:p-4'
        aria-label={t('dashboard.timeline.title')}
        aria-busy={current === null}
      >
        <header className='flex min-h-8 items-center justify-between gap-2'>
          <h2 className='m-0 text-sm font-semibold'>{t('dashboard.timeline.title')}</h2>
          <small className='text-xs text-muted-foreground'>{t('dashboard.metric.lastDay')}</small>
        </header>
        <div className='grid gap-2'>
          <div className='grid h-6 grid-cols-[repeat(48,minmax(0,1fr))] gap-0.5' role='group' aria-label={t('dashboard.timeline.ariaLabel')}>
            {slots.map((slot) => {
              const from = formatLogTime(new Date(slot.start).toISOString(), locale);
              const to = formatLogTime(new Date(slot.end).toISOString(), locale);
              const label = `${from} – ${to} · ${t(`dashboard.timeline.state.${slot.state}`)}`;
              return (
                <Tooltip key={slot.id}>
                  <TooltipTrigger
                    className='min-w-0 rounded-[2px] border-0 bg-muted-foreground p-0 outline-offset-3 hover:shadow-[inset_0_0_0_1px_var(--foreground)] focus-visible:shadow-[inset_0_0_0_1px_var(--foreground)] focus-visible:outline-2 focus-visible:outline-ring data-[state=failed]:bg-[var(--color-danger)] data-[state=running]:bg-[var(--color-success)] data-[state=unknown]:bg-[repeating-linear-gradient(135deg,var(--color-surface-raised)_0_4px,var(--color-border-strong)_4px_6px)]'
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
              <div className='absolute inset-x-3 top-13 bottom-3 z-1 grid place-content-center justify-items-center gap-2.5 bg-[color-mix(in_srgb,var(--color-surface)_84%,transparent)] text-xs text-muted-foreground'>
                <Spinner />
                <span>{t('dashboard.state.runtimeLoading')}</span>
              </div>
            )
          : null}
        <div className='flex flex-wrap gap-4'>
          {(['running', 'failed', 'stopped', 'unknown'] as const).map((state) => (
            <span className='flex items-center gap-1.5 text-xs text-muted-foreground' key={state}>
              <i aria-hidden className={`size-2.5 rounded-[2px] ${legendSwatch[state]}`} />
              {t(`dashboard.timeline.state.${state}`)}
            </span>
          ))}
        </div>
      </section>
      <section
        className='@container relative flex min-w-0 flex-col rounded-[var(--radius-card)] border bg-card min-h-45 gap-3 p-5 @max-[850px]:p-4'
        aria-label={t('dashboard.activity.title')}
        aria-busy={current === null}
      >
        <header className='flex min-h-8 items-center justify-between gap-2'>
          <h2 className='m-0 text-sm font-semibold'>{t('dashboard.activity.title')}</h2>
          <Link className='text-xs text-muted-foreground hover:text-foreground' to='/logs'>{t('dashboard.activity.viewAll')}</Link>
        </header>
        <div className='relative min-h-22'>
          <ol className='m-0 list-none p-0 [&>li+li]:border-t'>
            {current?.activity.items.map((entry) => {
              const title = logTitle(entry, t);
              const fullTime = formatLogTime(entry.time, locale);
              return (
                <li key={entry.id}>
                  <Tooltip>
                    <TooltipTrigger render={<Link className='grid min-h-11 grid-cols-[54px_minmax(0,1fr)_max-content] items-center gap-3 rounded-[var(--radius-control)] p-2 text-compact transition-colors duration-[var(--dur-micro)] ease-out hover:bg-[var(--color-surface-raised)] focus-visible:bg-[var(--color-surface-raised)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring motion-reduce:transition-none @max-[650px]:gap-2 @max-[650px]:px-0' to='/logs' />}>
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
              <div className='absolute inset-x-3 top-13 bottom-3 z-1 grid place-content-center justify-items-center gap-2.5 bg-[color-mix(in_srgb,var(--color-surface)_84%,transparent)] text-xs text-muted-foreground'>
                <Spinner />
                <span>{t('dashboard.state.activityLoading')}</span>
              </div>
            )
          : null}
      </section>
    </div>
  );
}
