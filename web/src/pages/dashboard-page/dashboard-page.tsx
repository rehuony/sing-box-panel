import { Link } from 'react-router-dom';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Cpu, HardDrive, MemoryStick, Radio } from 'lucide-react';

import { Spinner } from '@/components/ui/spinner';
import { ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useDashboardSubscription, useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';

import { TrendChart } from './trend-chart';
import { trendChartData } from './trend-chart-data';
import { buildRuntimeSlots } from './runtime-timeline';
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
  const tail = useOptionalSharedTelemetry(s => s.liveTail) ?? emptyTail;
  const [range, setRange] = useState<'1h' | '24h'>('1h');
  const trafficHistory = current ? range === '1h' ? current.history_1h : current.history_24h : null;
  const watermark = current?.persisted_through;
  const traffic = useMemo(() => trendChartData(trafficHistory, 'traffic', tail, watermark), [trafficHistory, tail, watermark]);
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
            {(['1h', '24h'] as const).map((value) => (
              <TabsTrigger key={value} value={value}>
                {t(`dashboard.range.option.${value}`)}
              </TabsTrigger>
            ))}
          </TabsList>
        </header>
        {(['1h', '24h'] as const).map((value) => (
          <TabsContent key={value} value={value} className='dashboard-traffic__chart'>
            <TrendChart chart={traffic} kind='traffic' />
            {current === null
              ? (
                  <div className='dashboard-stream-loading'>
                    <Spinner />
                    <span>{t('dashboard.state.historyLoading')}</span>
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

function DashboardHistory() {
  const { t, i18n } = useTranslation();
  const locale = i18n.language;
  const current = useOptionalSharedTelemetry(s => s.dashboardSnapshot);
  const [initialEnd] = useState(() => Date.now());
  const end = current ? Date.parse(current.collected_at) : initialEnd;
  const slots = useMemo(
    () => buildRuntimeSlots(current?.runtime_24h ?? null, end - 86_400_000, end), [current?.runtime_24h, end],
  );
  return (
    <div className='dashboard-bottom'>
      <section className='dashboard-card runtime-timeline-card'>
        <header>
          <h2>{t('dashboard.timeline.title')}</h2>
          <small>{t('dashboard.metric.lastDay')}</small>
        </header>
        <div className='runtime-timeline' aria-label={t('dashboard.timeline.ariaLabel')}>
          {slots.map((slot) => (
            <span
              key={slot.id}
              data-state={slot.state}
              tabIndex={0}
              title={`${new Date(slot.start).toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })}–${new Date(slot.end).toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })} · ${t(`dashboard.timeline.state.${slot.state}`)}`}
            />
          ))}
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
              <i />
              {t(`dashboard.timeline.state.${state}`)}
            </span>
          ))}
        </div>
      </section>
      <section className='dashboard-card'>
        <header>
          <h2>{t('dashboard.activity.title')}</h2>
          <Link to='/observability?tab=panel'>{t('dashboard.activity.viewAll')}</Link>
        </header>
        <ol className='dashboard-activity'>
          {current?.activity.items.map((entry) => (
            <li key={entry.id}>
              <Link to='/observability#logs-panel'>{entry.message}</Link>
              <time dateTime={entry.time}>
                {new Date(entry.time).toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })}
              </time>
              <span data-status={entry.level}>{entry.level.toUpperCase()}</span>
            </li>
          ))}
        </ol>
        {current?.activity.items.length === 0 ? <small>{t('dashboard.activity.empty')}</small> : null}
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
