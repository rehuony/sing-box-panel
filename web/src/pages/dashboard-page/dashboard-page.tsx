import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useState } from 'react';
import { Cpu, HardDrive, MemoryStick, Radio } from 'lucide-react';

import { Spinner } from '@/components/ui/spinner';
import { ErrorNotice } from '@/components/error-notice';
import { PanelLogLevel } from '@/components/panel-log-level';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useDashboardSubscription, useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';

import { TrendChart } from './trend-chart';
import { buildRuntimeSlots } from './runtime-timeline';
import { liveTrafficChartData, trendChartData } from './trend-chart-data';
import './dashboard-page.css';

const emptyTail: import('@/components/app-shell/use-telemetry').LivePoint[] = [];

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
export function DashboardPage() {
  const { t } = useTranslation();
  useDashboardSubscription();
  return (
    <div className='dashboard-page panel-page'>
      <h1 className='sr-only'>{t('nav.dashboard')}</h1>
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
      {liveStale && updatedAt ? <small role='status'>{t('dashboard.state.liveStale', { time: new Date(updatedAt).toLocaleTimeString() })}</small> : null}
      {stale && snapshot ? <small role='status'>{t('dashboard.state.historyStale', { time: new Date(snapshot.collected_at).toLocaleTimeString() })}</small> : null}
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
    <div className='dashboard-metrics'>
      {metrics.map(({ key, icon: Icon, value, detail }) => (
        <section key={key} className='dashboard-metric'>
          <div>
            <span>{t(`dashboard.metric.${key}`)}</span>
            <Icon size={18} />
          </div>
          <strong>{value}</strong>
          <small>{detail}</small>
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
    <div className='dashboard-charts'>
      <Tabs
        className='dashboard-card dashboard-traffic'
        render={<section />}
        value={range}
        onValueChange={(value) => setRange(value as typeof range)}
      >
        <header>
          <h2>
            {t('dashboard.trend.traffic')}
            （
            {traffic.unit}
            ）
          </h2>
          <TabsList className='dashboard-range' aria-label={t('dashboard.range.label')}>
            {(['live', '1h', '24h'] as const).map((value) => (
              <TabsTrigger key={value} value={value}>
                {t(`dashboard.range.option.${value}`)}
              </TabsTrigger>
            ))}
          </TabsList>
        </header>
        {(['live', '1h', '24h'] as const).map((value) => (
          <TabsContent key={value} value={value} className='dashboard-traffic__chart'>
            <TrendChart chart={traffic} kind='traffic' live={range === 'live'} />
            {(range === 'live' ? metrics === null : current === null)
              ? (
                  <div className='dashboard-stream-loading'>
                    <Spinner />
                    <span>{t(range === 'live' ? 'dashboard.state.liveLoading' : 'dashboard.state.historyLoading')}</span>
                  </div>
                )
              : null}
          </TabsContent>
        ))}
      </Tabs>
      <section className='dashboard-card'>
        <header>
          <h2>
            {t('dashboard.trend.connections')}
            （
            {t('dashboard.metric.countUnit')}
            ）
          </h2>
          <small>{t('dashboard.metric.lastHour')}</small>
        </header>
        <TrendChart
          chart={connections}
          kind='connections'
        />
        {current === null
          ? (
              <div className='dashboard-stream-loading'>
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
    <div className='dashboard-bottom'>
      <section
        className='dashboard-card runtime-timeline-card'
        aria-label={t('dashboard.timeline.title')}
        aria-busy={current === null}
      >
        <header>
          <h2>{t('dashboard.timeline.title')}</h2>
          <small>{t('dashboard.metric.lastDay')}</small>
        </header>
        <div className='runtime-timeline__plot'>
          <div className='runtime-timeline' role='group' aria-label={t('dashboard.timeline.ariaLabel')}>
            {slots.map((slot) => {
              const from = formatLogTime(new Date(slot.start).toISOString(), locale);
              const to = formatLogTime(new Date(slot.end).toISOString(), locale);
              const label = `${from} – ${to} · ${t(`dashboard.timeline.state.${slot.state}`)}`;
              return (
                <Tooltip key={slot.id}>
                  <TooltipTrigger
                    className='runtime-timeline__slot'
                    data-state={slot.state}
                    aria-label={label}
                    disabled={current === null}
                  />
                  <TooltipContent role='tooltip'>{label}</TooltipContent>
                </Tooltip>
              );
            })}
          </div>
          <div className='runtime-timeline__axis'>
            {[0, 6, 12, 18, 24].map((hour) => {
              const at = new Date(start + hour * 3_600_000);
              const endpoint = hour === 0 || hour === 24;
              return (
                <time
                  key={hour}
                  dateTime={at.toISOString()}
                  data-endpoint={endpoint ? '' : undefined}
                  data-intermediate={hour === 6 || hour === 18 ? '' : undefined}
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
              <div className='dashboard-stream-loading dashboard-stream-loading--compact'>
                <Spinner />
                <span>{t('dashboard.state.runtimeLoading')}</span>
              </div>
            )
          : null}
        <div className='runtime-timeline__legend'>
          {(['running', 'failed', 'stopped', 'unknown'] as const).map((state) => (
            <span key={state} data-state={state}>
              <i aria-hidden />
              {t(`dashboard.timeline.state.${state}`)}
            </span>
          ))}
        </div>
      </section>
      <section
        className='dashboard-card dashboard-activity-card'
        aria-label={t('dashboard.activity.title')}
        aria-busy={current === null}
      >
        <header>
          <h2>{t('dashboard.activity.title')}</h2>
          <Link to='/observability#logs-panel'>{t('dashboard.activity.viewAll')}</Link>
        </header>
        <div className='dashboard-activity-body'>
          <ol className='dashboard-activity'>
            {current?.activity.items.map((entry) => {
              const title = logTitle(entry, t);
              const fullTime = formatLogTime(entry.time, locale);
              return (
                <li key={entry.id}>
                  <Tooltip>
                    <TooltipTrigger render={<Link className='dashboard-activity__link' to='/observability#logs-panel' />}>
                      <span className='dashboard-activity__level'><PanelLogLevel level={entry.level} /></span>
                      <span className='dashboard-activity__title'>{title}</span>
                      <time dateTime={entry.time} aria-label={fullTime}>
                        {formatActivityTime(entry.time, locale, today)}
                      </time>
                    </TooltipTrigger>
                    <TooltipContent role='tooltip'>
                      <span className='dashboard-activity__details'>
                        <span>{title}</span>
                        <span>{fullTime}</span>
                      </span>
                    </TooltipContent>
                  </Tooltip>
                </li>
              );
            })}
          </ol>
          {current?.activity.items.length === 0 ? <small className='dashboard-activity__empty'>{t('dashboard.activity.empty')}</small> : null}
        </div>
        {current === null
          ? (
              <div className='dashboard-stream-loading dashboard-stream-loading--compact'>
                <Spinner />
                <span>{t('dashboard.state.activityLoading')}</span>
              </div>
            )
          : null}
      </section>
    </div>
  );
}
