import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { Clock3, FileClock, ListChecks, RefreshCw } from 'lucide-react';

import type {
  MetricsHistory,
  MetricsHistoryBucket,
  RuntimeHistoryPage,
  RuntimeStatus,
  RuntimeTransitionState,
  Task,
  TaskPage,
} from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { useControlPlane } from '@/stores/control-plane.store';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useOptionalSharedTelemetry } from '@/components/app-shell/telemetry-context';
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table';

import { HistoryUPlot } from './history-uplot';
import { buildRuntimeTimeline } from './runtime-timeline';
import './dashboard-page.css';

type TrendKind = 'traffic' | 'connections' | 'memory';
type RangeKey = '1h' | '6h' | '24h' | '7d' | '30d' | '90d';
const rangeOptions: Record<RangeKey, { milliseconds: number; bucketSeconds: number }> = {
  '1h': { milliseconds: 3_600_000, bucketSeconds: 60 },
  '6h': { milliseconds: 21_600_000, bucketSeconds: 300 },
  '24h': { milliseconds: 86_400_000, bucketSeconds: 900 },
  '7d': { milliseconds: 604_800_000, bucketSeconds: 3_600 },
  '30d': { milliseconds: 2_592_000_000, bucketSeconds: 21_600 },
  '90d': { milliseconds: 7_776_000_000, bucketSeconds: 21_600 },
};

function formatBytes(value: number, locale: string): string {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let amount = value;
  let unit = units[0];
  for (let index = 1; index < units.length && amount >= 1_024; index += 1) {
    amount /= 1_024;
    unit = units[index];
  }
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: amount >= 10 ? 0 : 1 }).format(amount)} ${unit}`;
}

function formatDuration(
  milliseconds: number,
  locale: string,
  units: { day: string; hour: string; minute: string },
): string {
  const minutes = Math.max(0, Math.round(milliseconds / 60_000));
  if (minutes < 60) return `${new Intl.NumberFormat(locale).format(minutes)}${units.minute}`;
  const hours = minutes / 60;
  if (hours < 24) return `${new Intl.NumberFormat(locale, { maximumFractionDigits: hours >= 10 ? 0 : 1 }).format(hours)}${units.hour}`;
  const days = hours / 24;
  return `${new Intl.NumberFormat(locale, { maximumFractionDigits: days >= 10 ? 0 : 1 }).format(days)}${units.day}`;
}

function taskStatusLabel(task: Task): string {
  if (task.cancel_requested && (task.status === 'queued' || task.status === 'running')) return 'canceling';
  return task.status;
}

export function DashboardPage() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const navigate = useNavigate();
  const client = useApiClient();
  const sharedTelemetry = useOptionalSharedTelemetry();
  const hasSharedTelemetry = sharedTelemetry !== null;
  const controlPlane = useControlPlane();
  const [range, setRange] = useState<RangeKey>('24h');
  const [trend, setTrend] = useState<TrendKind>('traffic');
  const [bundleDraft, setBundleDraft] = useState('');
  const [bundleFilter, setBundleFilter] = useState('');
  const [windowEnd, setWindowEnd] = useState(() => Date.now());
  const [loadedHistory, setLoadedHistory] = useState<MetricsHistory | null>(null);
  const [loadedHistoryKey, setLoadedHistoryKey] = useState<string | null>(null);
  const [loadedRuntimeHistory, setLoadedRuntimeHistory] = useState<RuntimeHistoryPage | null>(null);
  const [loadedRuntimeHistoryKey, setLoadedRuntimeHistoryKey] = useState<string | null>(null);
  const [runtimeStatus, setRuntimeStatus] = useState<RuntimeStatus | null>(null);
  const [taskPage, setTaskPage] = useState<TaskPage | null>(null);
  const [loadedHistoryError, setLoadedHistoryError] = useState<unknown>(null);
  const [loadedRuntimeError, setLoadedRuntimeError] = useState<unknown>(null);
  const [loadedRuntimeStatusError, setLoadedRuntimeStatusError] = useState<unknown>(null);
  const [loadedTaskError, setLoadedTaskError] = useState<unknown>(null);
  const [loadedSnapshotKey, setLoadedSnapshotKey] = useState<string | null>(null);
  const [snapshotRevision, setSnapshotRevision] = useState(0);
  const [focusIndex, setFocusIndex] = useState(0);
  const selectedRange = rangeOptions[range];
  const windowStart = windowEnd - selectedRange.milliseconds;
  const historyQueryKey = `${range}:${windowEnd}:${bundleFilter.trim()}`;
  const runtimeQueryKey = `${range}:${windowEnd}`;
  const snapshotQueryKey = `${hasSharedTelemetry}:${snapshotRevision}`;
  const history = loadedHistoryKey === historyQueryKey ? loadedHistory : null;
  const runtimeHistory = loadedRuntimeHistoryKey === runtimeQueryKey ? loadedRuntimeHistory : null;
  const currentTaskPage = loadedSnapshotKey === snapshotQueryKey ? taskPage : null;
  const currentRuntimeStatus = loadedSnapshotKey === snapshotQueryKey ? runtimeStatus : null;
  const historyError = loadedHistoryKey === historyQueryKey ? loadedHistoryError : null;
  const runtimeError = loadedRuntimeHistoryKey === runtimeQueryKey ? loadedRuntimeError : null;
  const runtimeStatusError = loadedSnapshotKey === snapshotQueryKey ? loadedRuntimeStatusError : null;
  const taskError = loadedSnapshotKey === snapshotQueryKey ? loadedTaskError : null;
  const historyStale = loadedHistory !== null && loadedHistoryKey !== historyQueryKey;
  const runtimeStale = loadedRuntimeHistory !== null && loadedRuntimeHistoryKey !== runtimeQueryKey;
  const snapshotStale = loadedSnapshotKey !== null && loadedSnapshotKey !== snapshotQueryKey;
  const historyLoading = loadedHistoryKey !== historyQueryKey;
  const runtimeLoading = loadedRuntimeHistoryKey !== runtimeQueryKey;
  const snapshotLoading = loadedSnapshotKey !== snapshotQueryKey;
  const loading = historyLoading || runtimeLoading || snapshotLoading;

  useEffect(() => {
    const controller = new AbortController();
    const from = new Date(windowEnd - selectedRange.milliseconds).toISOString();
    const to = new Date(windowEnd).toISOString();
    void client.getMetricsHistory({
      from,
      to,
      bucketSeconds: selectedRange.bucketSeconds,
      activationBundleID: bundleFilter.trim() || undefined,
    }, controller.signal).then((result) => {
      if (controller.signal.aborted) return;
      setLoadedHistory(result);
      setLoadedHistoryError(null);
      setLoadedHistoryKey(historyQueryKey);
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      setLoadedHistory(null);
      setLoadedHistoryError(error);
      setLoadedHistoryKey(historyQueryKey);
    });
    return () => controller.abort();
  }, [bundleFilter, client, historyQueryKey, selectedRange.bucketSeconds, selectedRange.milliseconds, windowEnd]);

  useEffect(() => {
    const controller = new AbortController();
    const from = new Date(windowEnd - selectedRange.milliseconds).toISOString();
    const to = new Date(windowEnd).toISOString();
    void (async () => {
      const firstPage = await client.getRuntimeHistory({ from, to, limit: 512 }, controller.signal);
      const items = [...firstPage.items];
      let cursor = firstPage.next;
      while (cursor !== undefined && items.length < 4_096 && !controller.signal.aborted) {
        const nextPage = await client.getRuntimeHistory({
          beforeID: cursor.id,
          beforeTime: cursor.occurred_at,
          from,
          to,
          limit: 512,
        }, controller.signal);
        items.push(...nextPage.items);
        cursor = nextPage.next;
      }
      return { ...firstPage, items, next: cursor };
    })().then((result) => {
      if (controller.signal.aborted) return;
      setLoadedRuntimeHistory(result);
      setLoadedRuntimeError(null);
      setLoadedRuntimeHistoryKey(runtimeQueryKey);
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      setLoadedRuntimeHistory(null);
      setLoadedRuntimeError(error);
      setLoadedRuntimeHistoryKey(runtimeQueryKey);
    });
    return () => controller.abort();
  }, [client, runtimeQueryKey, selectedRange.milliseconds, windowEnd]);

  useEffect(() => {
    const controller = new AbortController();
    void Promise.allSettled([
      hasSharedTelemetry ? Promise.resolve(null) : client.getRuntimeStatus(controller.signal),
      client.listTasks({ limit: 5 }, controller.signal),
    ]).then(([statusResult, tasksResult]) => {
      if (controller.signal.aborted) return;
      if (!hasSharedTelemetry) {
        if (statusResult.status === 'fulfilled') {
          setRuntimeStatus(statusResult.value);
          setLoadedRuntimeStatusError(null);
        } else {
          setRuntimeStatus(null);
          setLoadedRuntimeStatusError(statusResult.reason);
        }
      } else {
        setRuntimeStatus(null);
        setLoadedRuntimeStatusError(null);
      }
      if (tasksResult.status === 'fulfilled') {
        setTaskPage(tasksResult.value);
        setLoadedTaskError(null);
      } else {
        setTaskPage(null);
        setLoadedTaskError(tasksResult.reason);
      }
      setLoadedSnapshotKey(snapshotQueryKey);
    });
    return () => controller.abort();
  }, [client, hasSharedTelemetry, snapshotQueryKey]);

  const coverage = history === null || history.buckets.length === 0
    ? null
    : history.buckets.filter((bucket) => bucket.coverage === 'complete').length / history.buckets.length;
  const trafficBuckets = history?.buckets.filter((bucket) =>
    bucket.upload_bytes !== null || bucket.download_bytes !== null) ?? [];
  const trafficTotal = trafficBuckets.length === 0
    ? null
    : trafficBuckets.reduce((sum, bucket) =>
        sum + (bucket.upload_bytes ?? 0) + (bucket.download_bytes ?? 0), 0);
  const averageValues = history?.buckets.map((bucket) =>
    trend === 'connections' ? bucket.active_connections_avg : bucket.memory_bytes_avg)
    .filter((value): value is number => value !== null) ?? [];
  const peakValues = history?.buckets.map((bucket) =>
    trend === 'connections' ? bucket.active_connections_peak : bucket.memory_bytes_peak)
    .filter((value): value is number => value !== null) ?? [];
  const average = averageValues.length === 0
    ? null
    : averageValues.reduce((sum, value) => sum + value, 0) / averageValues.length;
  const peak = peakValues.length === 0 ? null : Math.max(...peakValues);
  const timeline = useMemo(
    () => buildRuntimeTimeline(runtimeHistory, windowStart, windowEnd),
    [runtimeHistory, windowEnd, windowStart],
  );
  const durations = timeline.reduce<Record<RuntimeTransitionState, number>>((totals, segment) => {
    totals[segment.state] += segment.end - segment.start;
    return totals;
  }, { running: 0, stopped: 0, failed: 0, unknown: 0 });
  const observedDuration = durations.running + durations.stopped + durations.failed;
  const availability = observedDuration === 0 ? null : durations.running / observedDuration;
  const resolvedRuntimeStatus = sharedTelemetry?.runtimeStatus ?? currentRuntimeStatus;
  const resolvedRuntimeStatusError = sharedTelemetry?.runtimeError ?? runtimeStatusError;

  function inspectBucket(bucket: MetricsHistoryBucket) {
    const params = new URLSearchParams({ from: bucket.from, to: bucket.to, tab: 'logs' });
    if (bundleFilter.trim() !== '') params.set('activation_bundle_id', bundleFilter.trim());
    navigate(`/observability?${params.toString()}`);
  }

  const valueFormatter = useCallback((value: number) => trend === 'memory' || trend === 'traffic'
    ? formatBytes(value, locale)
    : new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(value), [locale, trend]);

  if (controlPlane.status !== 'ready') return null;
  const { context } = controlPlane;
  const durationUnits = {
    day: t('telemetry.unit.day'),
    hour: t('telemetry.unit.hour'),
    minute: t('telemetry.unit.minute'),
  };
  return (
    <div className='dashboard-page page-stack'>
      <header className='dashboard-titlebar'>
        <div>
          <span>{t('dashboard.eyebrow', { defaultValue: 'Overview' })}</span>
          <h1>{t('dashboard.title', { defaultValue: 'Runtime overview' })}</h1>
        </div>
        <button
          className='button button--secondary'
          disabled={loading}
          onClick={() => {
            setWindowEnd(Date.now());
            setSnapshotRevision((revision) => revision + 1);
            void controlPlane.refresh();
          }}
          type='button'
        >
          <RefreshCw aria-hidden='true' size={16} />
          {loading
            ? t('dashboard.action.refreshing', { defaultValue: 'Refreshing…' })
            : t('dashboard.action.refresh', { defaultValue: 'Refresh' })}
        </button>
      </header>

      <section className='dashboard-trend-card' aria-labelledby='dashboard-trend-title'>
        <div className='dashboard-trend-card__toolbar'>
          <div>
            <h2 id='dashboard-trend-title'>{t('dashboard.trend.title', { defaultValue: 'History' })}</h2>
            <p>{t('dashboard.trend.description', { defaultValue: 'Persisted samples; gaps remain gaps.' })}</p>
          </div>
          <Tabs onValueChange={(value) => setTrend(value as TrendKind)} value={trend}>
            <TabsList aria-label={t('dashboard.trend.metricLabel', { defaultValue: 'Trend metric' })}>
              <TabsTrigger value='traffic'>{t('dashboard.trend.traffic', { defaultValue: 'Traffic' })}</TabsTrigger>
              <TabsTrigger value='connections'>{t('dashboard.trend.connections', { defaultValue: 'Connections' })}</TabsTrigger>
              <TabsTrigger value='memory'>{t('dashboard.trend.memory', { defaultValue: 'Memory' })}</TabsTrigger>
            </TabsList>
          </Tabs>
        </div>

        <form
          className='dashboard-filterbar'
          onSubmit={(event) => {
            event.preventDefault();
            setBundleFilter(bundleDraft.trim());
          }}
        >
          <label className='dashboard-bundle-filter'>
            <span>{t('dashboard.bundle.label', { defaultValue: 'Bundle' })}</span>
            <input
              onChange={(event) => setBundleDraft(event.target.value)}
              placeholder={t('dashboard.bundle.placeholder', { defaultValue: 'All bundles' })}
              type='search'
              value={bundleDraft}
            />
          </label>
          <button className='button button--secondary' disabled={historyLoading} type='submit'>
            {t('dashboard.bundle.apply', { defaultValue: 'Apply filter' })}
          </button>
        </form>

        {historyStale
          ? (
              <p className='notice notice--warning' role='status'>
                {t('dashboard.state.historyStale', { defaultValue: 'History filters changed. Waiting for matching samples.' })}
              </p>
            )
          : historyLoading && history === null
            ? <p className='notice' role='status'>{t('dashboard.state.historyLoading', { defaultValue: 'Loading matching history…' })}</p>
            : null}
        {historyError === null
          ? null
          : (
              <ErrorNotice error={historyError} title={t('dashboard.error.history', { defaultValue: 'History is unavailable' })} />
            )}

        <div className='dashboard-summary' aria-busy={historyLoading || runtimeLoading} aria-label={t('dashboard.summary.label', { defaultValue: 'Range summary' })}>
          <div>
            <span>
              {trend === 'traffic'
                ? t('dashboard.summary.total', { defaultValue: 'Range total' })
                : t('dashboard.summary.average', { defaultValue: 'Average' })}
            </span>
            <strong>
              {trend === 'traffic'
                ? trafficTotal === null ? '—' : formatBytes(trafficTotal, locale)
                : average === null ? '—' : valueFormatter(average)}
            </strong>
          </div>
          <div>
            <span>{t('dashboard.summary.peak', { defaultValue: 'Peak' })}</span>
            <strong>
              {trend === 'traffic'
                ? trafficBuckets.length === 0
                  ? '—'
                  : formatBytes(Math.max(...trafficBuckets.map((bucket) =>
                      (bucket.upload_bytes ?? 0) + (bucket.download_bytes ?? 0))), locale)
                : peak === null ? '—' : valueFormatter(peak)}
            </strong>
          </div>
          <div>
            <span>{t('dashboard.summary.coverage', { defaultValue: 'Complete coverage' })}</span>
            <strong>{coverage === null ? '—' : new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 0 }).format(coverage)}</strong>
          </div>
          <div>
            <span>{t('dashboard.summary.observed', { defaultValue: 'Observed runtime' })}</span>
            <strong>{runtimeHistory === null ? '—' : formatDuration(observedDuration, locale, durationUnits)}</strong>
          </div>
        </div>

        <HistoryUPlot
          bucketSeconds={history?.bucket_seconds ?? selectedRange.bucketSeconds}
          buckets={history?.buckets ?? []}
          focusedIndex={focusIndex}
          from={windowStart}
          key={`${windowStart}:${windowEnd}`}
          locale={locale}
          onFocusedIndexChange={setFocusIndex}
          onInspect={inspectBucket}
          onRangeChange={(value) => {
            setRange(value as RangeKey);
            setWindowEnd(Date.now());
          }}
          range={range}
          ranges={(Object.keys(rangeOptions) as RangeKey[]).map((option) => ({
            key: option,
            label: t(`dashboard.range.option.${option}`, { defaultValue: option }),
          }))}
          to={windowEnd}
          trend={trend}
          valueFormatter={valueFormatter}
        />

        <details className='dashboard-data-table'>
          <summary>{t('dashboard.table.title', { defaultValue: 'Accessible data table' })}</summary>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('dashboard.table.time', { defaultValue: 'Time' })}</TableHead>
                <TableHead>{t('dashboard.table.coverage', { defaultValue: 'Coverage' })}</TableHead>
                <TableHead>{t('dashboard.trend.upload', { defaultValue: 'Upload' })}</TableHead>
                <TableHead>{t('dashboard.trend.download', { defaultValue: 'Download' })}</TableHead>
                <TableHead>{t('dashboard.trend.connections', { defaultValue: 'Connections' })}</TableHead>
                <TableHead>{t('dashboard.trend.memory', { defaultValue: 'Memory' })}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {history?.buckets.map((bucket) => (
                <TableRow key={bucket.from}>
                  <TableCell><button className='dashboard-table-link' onClick={() => inspectBucket(bucket)} type='button'>{new Intl.DateTimeFormat(locale, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(bucket.from))}</button></TableCell>
                  <TableCell>{t(`dashboard.coverage.${bucket.coverage}`, { defaultValue: bucket.coverage })}</TableCell>
                  <TableCell>{bucket.upload_bytes === null ? '—' : formatBytes(bucket.upload_bytes, locale)}</TableCell>
                  <TableCell>{bucket.download_bytes === null ? '—' : formatBytes(bucket.download_bytes, locale)}</TableCell>
                  <TableCell>{bucket.active_connections_avg === null ? '—' : new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(bucket.active_connections_avg)}</TableCell>
                  <TableCell>{bucket.memory_bytes_avg === null ? '—' : formatBytes(bucket.memory_bytes_avg, locale)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </details>
      </section>

      <section className='runtime-timeline-card' aria-labelledby='runtime-timeline-title'>
        <div className='runtime-timeline-card__heading'>
          <div>
            <Clock3 aria-hidden='true' size={18} />
            <h2 id='runtime-timeline-title'>{t('dashboard.timeline.title', { defaultValue: 'Runtime timeline' })}</h2>
          </div>
          <span>
            {availability === null
              ? t('dashboard.timeline.unavailable', { defaultValue: 'Availability unavailable' })
              : t('dashboard.timeline.availability', { defaultValue: '{{value}} availability', value: new Intl.NumberFormat(locale, { style: 'percent', maximumFractionDigits: 1 }).format(availability) })}
          </span>
        </div>
        {runtimeStale
          ? (
              <p className='notice notice--warning' role='status'>
                {t('dashboard.state.runtimeStale', { defaultValue: 'The time range changed. Waiting for matching runtime history.' })}
              </p>
            )
          : runtimeLoading && runtimeHistory === null
            ? <p className='notice' role='status'>{t('dashboard.state.runtimeLoading', { defaultValue: 'Loading runtime history…' })}</p>
            : null}
        {runtimeError === null ? null : <ErrorNotice error={runtimeError} title={t('dashboard.error.timeline', { defaultValue: 'Runtime history is unavailable' })} />}
        {runtimeHistory?.next === undefined
          ? null
          : (
              <p className='notice notice--warning' role='status'>
                {t('dashboard.timeline.truncated', { defaultValue: 'Runtime history reached the 4096-transition display limit. The oldest interval is shown as unknown.' })}
              </p>
            )}
        <div className='runtime-timeline' role='img' aria-label={t('dashboard.timeline.ariaLabel', { defaultValue: 'Observed runtime state across the selected range' })}>
          {timeline.map((segment) => <span data-state={segment.state} key={segment.id} style={{ flexGrow: segment.end - segment.start }} title={`${t(`dashboard.timeline.state.${segment.state}`, { defaultValue: segment.state })} · ${t(`dashboard.timeline.reason.${segment.reason}`, { defaultValue: segment.reason })} · ${formatDuration(segment.end - segment.start, locale, durationUnits)}`} />)}
        </div>
        <div className='runtime-timeline__legend'>
          {(['running', 'stopped', 'failed', 'unknown'] as RuntimeTransitionState[]).map((state) => (
            <span data-state={state} key={state}>
              <i aria-hidden='true' />
              {t(`dashboard.timeline.state.${state}`, { defaultValue: state })}
              {' '}
              ·
              {formatDuration(durations[state], locale, durationUnits)}
            </span>
          ))}
        </div>
        <p>{t('dashboard.timeline.unknownNote', { defaultValue: 'Unknown intervals are excluded from availability.' })}</p>
      </section>

      <div className='dashboard-lower-grid'>
        <section className='dashboard-compact-card' aria-busy={!hasSharedTelemetry && snapshotLoading} aria-labelledby='runtime-evidence-title'>
          <div className='dashboard-compact-card__heading'>
            <FileClock aria-hidden='true' size={18} />
            <h2 id='runtime-evidence-title'>{t('dashboard.evidence.title', { defaultValue: 'Runtime evidence' })}</h2>
          </div>
          {!hasSharedTelemetry && snapshotStale
            ? <p className='notice notice--warning' role='status'>{t('dashboard.state.snapshotStale', { defaultValue: 'Runtime evidence is refreshing; the previous snapshot is hidden.' })}</p>
            : null}
          <dl className='runtime-evidence-list'>
            <div>
              <dt>{t('dashboard.evidence.bundle', { defaultValue: 'Applied bundle' })}</dt>
              <dd>{resolvedRuntimeStatus?.applied_bundle_id ?? '—'}</dd>
            </div>
            <div>
              <dt>{t('dashboard.evidence.revision', { defaultValue: 'Canonical revision' })}</dt>
              <dd>
                #
                {context.canonical.revision}
              </dd>
            </div>
            <div>
              <dt>{t('dashboard.evidence.saved', { defaultValue: 'Saved' })}</dt>
              <dd>{new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(context.canonical.savedAt))}</dd>
            </div>
            <div>
              <dt>{t('dashboard.evidence.intent', { defaultValue: 'Desired state' })}</dt>
              <dd>{resolvedRuntimeStatus === null ? '—' : resolvedRuntimeStatus.desired_running ? t('dashboard.timeline.state.running', { defaultValue: 'running' }) : t('dashboard.timeline.state.stopped', { defaultValue: 'stopped' })}</dd>
            </div>
          </dl>
          {resolvedRuntimeStatusError === null
            ? null
            : <ErrorNotice error={resolvedRuntimeStatusError} title={t('dashboard.error.runtimeStatus', { defaultValue: 'Runtime status is unavailable' })} />}
        </section>

        <section className='dashboard-compact-card' aria-busy={snapshotLoading} aria-labelledby='recent-tasks-title'>
          <div className='dashboard-compact-card__heading'>
            <ListChecks aria-hidden='true' size={18} />
            <h2 id='recent-tasks-title'>{t('dashboard.tasks.title', { defaultValue: 'Recent tasks' })}</h2>
            <Link to='/tasks'>{t('dashboard.tasks.viewAll', { defaultValue: 'View all' })}</Link>
          </div>
          {snapshotStale && taskPage !== null
            ? <p className='notice notice--warning' role='status'>{t('dashboard.state.tasksStale', { defaultValue: 'Recent tasks are refreshing; the previous snapshot is hidden.' })}</p>
            : snapshotLoading
              ? <p className='dashboard-compact-empty' role='status'>{t('dashboard.state.tasksLoading', { defaultValue: 'Loading recent tasks…' })}</p>
              : null}
          {taskError === null ? null : <ErrorNotice error={taskError} title={t('dashboard.error.tasks', { defaultValue: 'Tasks are unavailable' })} />}
          {currentTaskPage?.items.length === 0 ? <p className='dashboard-compact-empty'>{t('dashboard.tasks.empty', { defaultValue: 'No task records.' })}</p> : null}
          <ul className='dashboard-task-list'>
            {currentTaskPage?.items.map((task) => (
              <li key={task.id}>
                <span className={`task-mark task-mark--${task.status}`} />
                <strong>{task.kind}</strong>
                <span>
                  {t(`tasks.lane.${task.lane}`, { defaultValue: task.lane })}
                  {' '}
                  ·
                  {' '}
                  {t(`dashboard.tasks.status.${taskStatusLabel(task)}`, {
                    defaultValue: taskStatusLabel(task),
                  })}
                </span>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}
