import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useState } from 'react';
import { Cpu, HardDrive, MemoryStick, Radio } from 'lucide-react';

import type {
  MetricsHistory,
  MetricsSnapshot,
  PanelLogPage,
  RuntimeHistoryPage,
} from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';

import { TrendChart } from './trend-chart';
import { buildRuntimeSlots } from './runtime-timeline';
import './dashboard-page.css';

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
export function DashboardPage() {
  const { t, i18n } = useTranslation();
  const client = useApiClient();
  const telemetry = useOptionalSharedTelemetry();
  const [range, setRange] = useState<'1h' | '24h'>('1h');
  const [tick, setTick] = useState(() => Math.floor(Date.now() / 30_000));
  const [data, setData] = useState<{
    range: '1h' | '24h';
    traffic: MetricsHistory | null;
    connections: MetricsHistory | null;
    runtime: RuntimeHistoryPage | null;
    activity: PanelLogPage | null;
    metrics: MetricsSnapshot | null;
    errors: unknown[];
  } | null>(null);
  useEffect(() => {
    const timer = setInterval(() => setTick(Math.floor(Date.now() / 30_000)), 30_000);
    return () => clearInterval(timer);
  }, []);
  const streamTick = telemetry?.snapshot
    ? Math.floor(Date.parse(telemetry.snapshot.collected_at) / 30_000)
    : 0;
  const end = Math.max(tick, streamTick) * 30_000;
  const shared = telemetry !== null;
  useEffect(() => {
    const controller = new AbortController();
    const to = new Date(end).toISOString();
    const requestHistory = (hours: number) =>
      client.getMetricsHistory(
        {
          from: new Date(end - hours * 3_600_000).toISOString(),
          to,
          bucketSeconds: hours === 1 ? 60 : 300,
        },
        controller.signal,
      );
    const traffic = requestHistory(range === '1h' ? 1 : 24);
    void Promise.allSettled([
      traffic,
      range === '1h' ? traffic : requestHistory(1),
      (async () => {
        const filter = { from: new Date(end - 86_400_000).toISOString(), to, limit: 200 };
        const page = await client.getRuntimeHistory(filter, controller.signal);
        const items = [...page.items];
        let next = page.next;
        while (next && items.length < 4096 && !controller.signal.aborted) {
          const older = await client.getRuntimeHistory(
            { ...filter, beforeID: next.id, beforeTime: next.occurred_at },
            controller.signal,
          );
          items.push(...older.items);
          next = older.next;
        }
        return { ...page, items, next };
      })(),
      client.listPanelLogs({ limit: 2 }, controller.signal),
      shared ? Promise.resolve(null) : client.getMetrics(controller.signal),
    ]).then(([traffic, connections, runtime, activity, metrics]) => {
      if (controller.signal.aborted) return;
      setData({
        range,
        traffic: traffic.status === 'fulfilled' ? traffic.value : null,
        connections: connections.status === 'fulfilled' ? connections.value : null,
        runtime: runtime.status === 'fulfilled' ? runtime.value : null,
        activity: activity.status === 'fulfilled' ? activity.value : null,
        metrics: metrics.status === 'fulfilled' ? metrics.value : null,
        errors: [traffic, connections, runtime, activity, metrics].flatMap((result) =>
          result.status === 'rejected' ? [result.reason] : [],
        ),
      });
    });
    return () => controller.abort();
  }, [client, end, range, shared]);
  const current = data?.range === range ? data : null;
  const snapshot = telemetry?.snapshot ?? current?.metrics;
  const host = snapshot?.host;
  const traffic = snapshot?.traffic_available ? snapshot.current_traffic_period : undefined;
  const used = traffic ? traffic.inbound_bytes + traffic.outbound_bytes : undefined;
  const slots = useMemo(
    () => buildRuntimeSlots(current?.runtime ?? null, end - 86_400_000, end),
    [current?.runtime, end],
  );
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
      value: bytes(used, locale),
      detail: snapshot?.quota_bytes
        ? t('dashboard.metric.quota', {
            used: percent(used, snapshot.quota_bytes),
            total: bytes(snapshot.quota_bytes, locale),
          })
        : t('dashboard.metric.noQuota'),
    },
  ];
  return (
    <div className='dashboard-page panel-page'>
      <h1 className='sr-only'>{t('nav.dashboard')}</h1>
      {current?.errors.length
        ? (
            <ErrorNotice error={current.errors[0]} title={t('dashboard.error.history')} />
          )
        : null}
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
              （KB/s）
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
              <TrendChart history={current?.traffic ?? null} kind='traffic' />
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
          <TrendChart history={current?.connections ?? null} kind='connections' />
        </section>
      </div>
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
            {current?.activity?.items.map((entry) => (
              <li key={entry.id}>
                <Link to='/observability#logs-panel'>{entry.message}</Link>
                <time dateTime={entry.time}>
                  {new Date(entry.time).toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit' })}
                </time>
                <span data-status={entry.level}>{entry.level.toUpperCase()}</span>
              </li>
            ))}
          </ol>
          {current?.activity?.items.length === 0 && <small>{t('dashboard.activity.empty')}</small>}
        </section>
      </div>
    </div>
  );
}
