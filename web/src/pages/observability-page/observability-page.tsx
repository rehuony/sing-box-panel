import type { ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';

import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Activity,
  AlertTriangle,
  ArrowDown,
  ArrowUp,
  Clock3,
  Database,
  Gauge,
  RefreshCw,
} from 'lucide-react';

import type {
  MetricsSnapshot,
  TrafficPeriod,
  TrafficPeriodCursor,
  TrafficPeriodFilter,
  TrafficPeriodPage,
} from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';

import { LogsPanel } from './logs-panel';
import './observability-page.css';

type EvidenceTone = 'danger' | 'info' | 'neutral' | 'success' | 'warning';
type ObservabilityTab = 'summary' | 'traffic' | 'logs';

interface EvidenceCardProps {
  label: string;
  detail: string;
  icon: LucideIcon;
  children: ReactNode;
  tone?: EvidenceTone;
}

interface TrafficFilterDraft {
  to: string;
  from: string;
  bundle: string;
  appliedRevision: number;
}

function formatBytes(value: number, locale: string): string {
  if (value < 10_240) return `${new Intl.NumberFormat(locale).format(value)} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let amount = value / 1_024;
  let unit = units[0];
  for (let index = 1; index < units.length && amount >= 1_024; index += 1) {
    amount /= 1_024;
    unit = units[index];
  }
  return `${new Intl.NumberFormat(locale, {
    maximumFractionDigits: amount >= 10 ? 0 : 1,
  }).format(amount)} ${unit}`;
}

function localDateTimeValue(value: string | null): string {
  if (value === null || value === '') return '';
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return '';
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.valueOf() - offset).toISOString().slice(0, 16);
}

function isoDateTimeValue(value: string): string | undefined {
  if (value === '') return undefined;
  const timestamp = new Date(value);
  return Number.isNaN(timestamp.valueOf()) ? undefined : timestamp.toISOString();
}

function EvidenceCard({
  children,
  detail,
  icon: Icon,
  label,
  tone = 'neutral',
}: EvidenceCardProps) {
  return (
    <article className={`evidence-card evidence-card--${tone}`}>
      <div className='evidence-card__heading'>
        <span className='evidence-card__icon'><Icon aria-hidden='true' size={18} strokeWidth={1.8} /></span>
        <span>{label}</span>
      </div>
      <div className='evidence-card__value'>{children}</div>
      <p>{detail}</p>
    </article>
  );
}

export function ObservabilityPage() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedTab = searchParams.get('tab');
  const tab: ObservabilityTab = requestedTab === 'logs' || requestedTab === 'traffic'
    ? requestedTab
    : 'summary';
  const appliedBundle = searchParams.get('activation_bundle_id') ?? '';
  const appliedFrom = searchParams.get('from') ?? '';
  const appliedTo = searchParams.get('to') ?? '';
  const appliedTrafficFingerprint = [appliedBundle, appliedFrom, appliedTo].join('\u0000');
  const appliedTrafficStateRef = useRef({ fingerprint: appliedTrafficFingerprint, revision: 0 });
  if (appliedTrafficStateRef.current.fingerprint !== appliedTrafficFingerprint) {
    appliedTrafficStateRef.current = {
      fingerprint: appliedTrafficFingerprint,
      revision: appliedTrafficStateRef.current.revision + 1,
    };
  }
  const appliedTrafficRevision = appliedTrafficStateRef.current.revision;
  const trafficFilter = useMemo<TrafficPeriodFilter>(() => ({
    activationBundleID: appliedBundle || undefined,
    from: appliedFrom || undefined,
    to: appliedTo || undefined,
  }), [appliedBundle, appliedFrom, appliedTo]);
  const [metrics, setMetrics] = useState<MetricsSnapshot | null>(null);
  const [traffic, setTraffic] = useState<MetricsSnapshot | null>(null);
  const [periodPage, setPeriodPage] = useState<TrafficPeriodPage | null>(null);
  const [selectedPeriod, setSelectedPeriod] = useState<TrafficPeriod | null>(null);
  const [trafficFilterDraft, setTrafficFilterDraft] = useState<TrafficFilterDraft>(() => ({
    appliedRevision: appliedTrafficRevision,
    bundle: appliedBundle,
    from: localDateTimeValue(appliedFrom),
    to: localDateTimeValue(appliedTo),
  }));
  const trafficDraftMatchesURL = trafficFilterDraft.appliedRevision === appliedTrafficRevision;
  const bundleDraft = trafficDraftMatchesURL ? trafficFilterDraft.bundle : appliedBundle;
  const fromDraft = trafficDraftMatchesURL ? trafficFilterDraft.from : localDateTimeValue(appliedFrom);
  const toDraft = trafficDraftMatchesURL ? trafficFilterDraft.to : localDateTimeValue(appliedTo);
  const [metricsError, setMetricsError] = useState<unknown>(null);
  const [trafficError, setTrafficError] = useState<unknown>(null);
  const [periodsLoadError, setPeriodsLoadError] = useState<unknown>(null);
  const [periodDetailError, setPeriodDetailError] = useState('');
  const [refreshKey, setRefreshKey] = useState(0);
  const [metricsLoading, setMetricsLoading] = useState(true);
  const [trafficLoading, setTrafficLoading] = useState(true);
  const [periodsLoading, setPeriodsLoading] = useState(true);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [periodDetailLoading, setPeriodDetailLoading] = useState(false);
  const summaryGenerationRef = useRef(0);
  const periodListGenerationRef = useRef(0);
  const loadingOlderRef = useRef(false);
  const periodDetailGenerationRef = useRef(0);

  const loadSummary = useCallback(async (signal?: AbortSignal) => {
    const generation = ++summaryGenerationRef.current;
    setMetricsLoading(true);
    setTrafficLoading(true);
    setMetricsError(null);
    setTrafficError(null);
    const [metricsResult, trafficResult] = await Promise.allSettled([
      client.getMetrics(signal), client.getTrafficStatus(signal),
    ]);
    if (signal?.aborted || generation !== summaryGenerationRef.current) return;
    if (metricsResult.status === 'fulfilled') {
      setMetrics(metricsResult.value);
    } else {
      setMetrics(null);
      setMetricsError(metricsResult.reason);
    }
    if (trafficResult.status === 'fulfilled') {
      setTraffic(trafficResult.value);
    } else {
      setTraffic(null);
      setTrafficError(trafficResult.reason);
    }
    setMetricsLoading(false);
    setTrafficLoading(false);
  }, [client]);

  const loadPeriods = useCallback(async (
    signal?: AbortSignal,
    cursor?: TrafficPeriodCursor,
    append = false,
  ) => {
    if (append && loadingOlderRef.current) return;
    const generation = ++periodListGenerationRef.current;
    if (append) {
      loadingOlderRef.current = true;
      setLoadingOlder(true);
    } else {
      loadingOlderRef.current = false;
      setLoadingOlder(false);
      setPeriodsLoading(true);
      setPeriodPage(null);
    }
    setPeriodsLoadError(null);
    try {
      const nextPage = await client.listTrafficPeriods({
        ...trafficFilter,
        beforeID: cursor?.id,
        beforeTime: cursor?.period_start,
        limit: 30,
      }, signal);
      if (signal?.aborted || generation !== periodListGenerationRef.current) return;
      setPeriodPage((current) => append && current !== null
        ? { items: [...current.items, ...nextPage.items], next: nextPage.next }
        : nextPage);
    } catch (loadError) {
      if (!signal?.aborted && generation === periodListGenerationRef.current) {
        setPeriodsLoadError(loadError);
      }
    } finally {
      if (!signal?.aborted && generation === periodListGenerationRef.current) {
        loadingOlderRef.current = false;
        setPeriodsLoading(false);
        setLoadingOlder(false);
      }
    }
  }, [client, trafficFilter]);

  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([loadSummary(controller.signal), loadPeriods(controller.signal)]);
    return () => controller.abort();
  }, [loadPeriods, loadSummary, refreshKey]);

  async function inspectPeriod(period: TrafficPeriod) {
    const generation = ++periodDetailGenerationRef.current;
    setSelectedPeriod(period);
    setPeriodDetailError('');
    setPeriodDetailLoading(true);
    try {
      const detail = await client.getTrafficPeriod(period.id);
      if (generation === periodDetailGenerationRef.current) setSelectedPeriod(detail);
    } catch (inspectError) {
      if (generation === periodDetailGenerationRef.current) {
        setPeriodDetailError(describeRequestError(inspectError));
      }
    } finally {
      if (generation === periodDetailGenerationRef.current) setPeriodDetailLoading(false);
    }
  }

  function closePeriodDetail() {
    periodDetailGenerationRef.current += 1;
    setSelectedPeriod(null);
    setPeriodDetailError('');
    setPeriodDetailLoading(false);
  }

  function updateTrafficFilterDraft(values: Partial<Pick<TrafficFilterDraft, 'bundle' | 'from' | 'to'>>) {
    setTrafficFilterDraft({
      appliedRevision: appliedTrafficRevision,
      bundle: bundleDraft,
      from: fromDraft,
      to: toDraft,
      ...values,
    });
  }

  function applyTrafficFilter() {
    const from = isoDateTimeValue(fromDraft);
    const to = isoDateTimeValue(toDraft);
    closePeriodDetail();
    setPeriodPage(null);
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.set('tab', tab);
      if (bundleDraft.trim() === '') next.delete('activation_bundle_id');
      else next.set('activation_bundle_id', bundleDraft.trim());
      if (from === undefined) next.delete('from');
      else next.set('from', from);
      if (to === undefined) next.delete('to');
      else next.set('to', to);
      return next;
    }, { replace: true });
  }

  function formatTime(value: string): string {
    return new Intl.DateTimeFormat(locale, {
      dateStyle: 'medium',
      timeStyle: 'medium',
    }).format(new Date(value));
  }

  const unavailableTitle = metrics?.reason_code === 'not_applied'
    ? t('observability.availability.notApplied', { defaultValue: 'No bundle is applied' })
    : metrics?.reason_code === 'process_only'
      ? t('observability.availability.processOnly', { defaultValue: 'Process-only monitoring' })
      : metrics?.reason_code === 'no_collector_sample'
        ? t('observability.availability.noSample', { defaultValue: 'No collector sample yet' })
        : metrics?.reason_code === 'stale_collector_sample'
          ? t('observability.availability.stale', { defaultValue: 'Collector sample is stale' })
          : t('observability.availability.unavailable', { defaultValue: 'Metrics unavailable' });
  const unavailableDetail = t('observability.availability.detail', {
    defaultValue: 'Unavailable evidence is not displayed as zero.',
  });
  const current = metrics?.traffic_available ? metrics.current_traffic_period : undefined;
  const sample = metrics?.available ? metrics.latest_sample : undefined;
  const loading = metricsLoading || trafficLoading || periodsLoading || loadingOlder;

  return (
    <div className='observability-page page-stack'>
      <header className='observability-titlebar'>
        <div>
          <span>{t('observability.eyebrow', { defaultValue: 'Observability' })}</span>
          <h1>{t('observability.title', { defaultValue: 'Runtime evidence' })}</h1>
        </div>
        <button
          className='button button--secondary'
          disabled={loading}
          onClick={() => setRefreshKey((key) => key + 1)}
          type='button'
        >
          <RefreshCw aria-hidden='true' size={16} />
          {loading
            ? t('observability.action.refreshing', { defaultValue: 'Refreshing…' })
            : t('observability.action.refresh', { defaultValue: 'Refresh' })}
        </button>
      </header>

      <Tabs
        className='observability-tabs'
        onValueChange={(value) => {
          const nextTab = value as ObservabilityTab;
          setSearchParams((currentParams) => {
            const next = new URLSearchParams(currentParams);
            next.set('tab', nextTab);
            return next;
          }, { replace: true });
        }}
        value={tab}
      >
        <TabsList aria-label={t('observability.tabs.label', { defaultValue: 'Observability sections' })}>
          <TabsTrigger value='summary'>{t('observability.tabs.summary', { defaultValue: 'Summary' })}</TabsTrigger>
          <TabsTrigger value='traffic'>{t('observability.tabs.traffic', { defaultValue: 'Traffic' })}</TabsTrigger>
          <TabsTrigger value='logs'>{t('observability.tabs.logs', { defaultValue: 'Logs' })}</TabsTrigger>
        </TabsList>

        <TabsContent value='summary'>
          <section className='observability-tab-card' aria-labelledby='evidence-summary-title'>
            <div className='observability-panel__heading'>
              <div><h2 id='evidence-summary-title'>{t('observability.summary.title', { defaultValue: 'Evidence summary' })}</h2></div>
              <span className='observability-panel__count'>
                {metrics === null
                  ? t('observability.summary.unavailable', { defaultValue: 'Snapshot unavailable' })
                  : t('observability.summary.checked', { defaultValue: 'Checked {{time}}', time: formatTime(metrics.collected_at) })}
              </span>
            </div>
            {metricsError === null ? null : <ErrorNotice error={metricsError} title={t('observability.error.metrics', { defaultValue: 'Collector snapshot is unavailable' })} />}
            {trafficError === null ? null : <ErrorNotice error={trafficError} title={t('observability.error.traffic', { defaultValue: 'Traffic status is unavailable' })} />}
            <div className='evidence-card-grid' aria-busy={metricsLoading}>
              <EvidenceCard
                detail={metrics?.available
                  ? t('observability.summary.bound', { defaultValue: 'Bound to {{bundle}}.', bundle: metrics.applied_bundle_id ?? '—' })
                  : unavailableDetail}
                icon={metrics?.available ? Activity : AlertTriangle}
                label={t('observability.summary.collector', { defaultValue: 'Collector state' })}
                tone={metrics?.available ? 'success' : 'warning'}
              >
                <span className='evidence-card__status'>
                  {metrics?.available
                    ? t('observability.summary.available', { defaultValue: 'Available' })
                    : unavailableTitle}
                </span>
              </EvidenceCard>
              <EvidenceCard
                detail={current
                  ? t('observability.summary.persisted', { defaultValue: 'Persisted current-period totals.' })
                  : unavailableDetail}
                icon={Database}
                label={t('observability.summary.transfer', { defaultValue: 'Current transfer' })}
                tone={current ? 'info' : 'neutral'}
              >
                {current
                  ? (
                      <span className='evidence-card__pair'>
                        <span>
                          <ArrowDown aria-hidden='true' size={14} />
                          {formatBytes(current.inbound_bytes, locale)}
                        </span>
                        <span>
                          <ArrowUp aria-hidden='true' size={14} />
                          {formatBytes(current.outbound_bytes, locale)}
                        </span>
                      </span>
                    )
                  : <span>{t('observability.value.notReported', { defaultValue: 'Not reported' })}</span>}
              </EvidenceCard>
              <EvidenceCard
                detail={sample
                  ? t('observability.summary.accepted', { defaultValue: 'Accepted {{time}}.', time: formatTime(sample.sampled_at) })
                  : unavailableDetail}
                icon={Gauge}
                label={t('observability.summary.sample', { defaultValue: 'Runtime sample' })}
                tone={sample ? 'info' : 'neutral'}
              >
                {sample
                  ? (
                      <span className='evidence-card__pair'>
                        <span>
                          {formatBytes(sample.memory_bytes, locale)}
                          {' '}
                          {t('observability.unit.memory', { defaultValue: 'memory' })}
                        </span>
                        <span>
                          {new Intl.NumberFormat(locale).format(sample.active_connections)}
                          {' '}
                          {t('observability.unit.connections', { defaultValue: 'connections' })}
                        </span>
                      </span>
                    )
                  : <span>{t('observability.value.notReported', { defaultValue: 'Not reported' })}</span>}
              </EvidenceCard>
              <EvidenceCard
                detail={metrics?.available
                  ? t('observability.summary.tier', {
                      defaultValue: '{{tier}} monitoring tier.',
                      tier: metrics.monitoring_tier === undefined
                        ? '—'
                        : t(`observability.monitoring.${metrics.monitoring_tier}`, { defaultValue: metrics.monitoring_tier }),
                    })
                  : unavailableDetail}
                icon={Clock3}
                label={t('observability.summary.quota', { defaultValue: 'Quota state' })}
                tone={metrics?.quota_exceeded ? 'danger' : 'neutral'}
              >
                {metrics?.available
                  ? metrics.quota_bytes === undefined
                    ? <span>{t('observability.value.unlimited', { defaultValue: 'Unlimited' })}</span>
                    : (
                        <span>
                          {metrics.quota_exceeded ? t('observability.value.exceeded', { defaultValue: 'Exceeded' }) : t('observability.value.withinLimit', { defaultValue: 'Within limit' })}
                          {' '}
                          ·
                          {' '}
                          {formatBytes(metrics.quota_bytes, locale)}
                        </span>
                      )
                  : <span>{t('observability.value.notEvaluated', { defaultValue: 'Not evaluated' })}</span>}
              </EvidenceCard>
            </div>
          </section>
        </TabsContent>

        <TabsContent value='traffic'>
          <section className='observability-panel' aria-labelledby='traffic-periods-title'>
            <div className='observability-panel__heading'>
              <div>
                <h2 id='traffic-periods-title'>{t('observability.traffic.title', { defaultValue: 'Traffic periods' })}</h2>
                <p>{t('observability.traffic.description', { defaultValue: 'Persisted counters with exact bundle evidence.' })}</p>
              </div>
              <span className='observability-panel__count'>
                {t('observability.traffic.loaded', {
                  defaultValue: '{{count}} loaded · {{tier}} tier',
                  count: new Intl.NumberFormat(locale).format(periodPage?.items.length ?? 0),
                  tier: traffic?.monitoring_tier === undefined
                    ? '—'
                    : t(`observability.monitoring.${traffic.monitoring_tier}`, { defaultValue: traffic.monitoring_tier }),
                })}
              </span>
            </div>
            <div className='traffic-filters' role='group' aria-label={t('observability.traffic.filters', { defaultValue: 'Traffic filters' })}>
              <label>
                <span>{t('observability.filter.bundle', { defaultValue: 'Bundle' })}</span>
                <input onChange={(event) => updateTrafficFilterDraft({ bundle: event.target.value })} placeholder={t('observability.filter.allBundles', { defaultValue: 'All bundles' })} type='search' value={bundleDraft} />
              </label>
              <label>
                <span>{t('observability.filter.from', { defaultValue: 'From' })}</span>
                <input onChange={(event) => updateTrafficFilterDraft({ from: event.target.value })} type='datetime-local' value={fromDraft} />
              </label>
              <label>
                <span>{t('observability.filter.to', { defaultValue: 'To' })}</span>
                <input onChange={(event) => updateTrafficFilterDraft({ to: event.target.value })} type='datetime-local' value={toDraft} />
              </label>
              <button className='button button--secondary button--small' onClick={applyTrafficFilter} type='button'>{t('observability.filter.apply', { defaultValue: 'Apply filters' })}</button>
            </div>
            {periodsLoadError === null ? null : <ErrorNotice error={periodsLoadError} title={t('observability.error.periods', { defaultValue: 'Traffic history is unavailable' })} />}
            <div className='observability-list-shell'>
              <div className='observability-master'>
                {periodsLoading && periodPage === null
                  ? (
                      <div className='observability-skeleton' aria-busy='true'>
                        <span />
                        <span />
                        <span />
                        <p>{t('observability.traffic.loading', { defaultValue: 'Loading persisted periods…' })}</p>
                      </div>
                    )
                  : null}
                {periodPage?.items.length === 0
                  ? (
                      <div className='empty-state'>
                        <strong>{t('observability.traffic.empty', { defaultValue: 'No persisted periods.' })}</strong>
                        <p>{t('observability.traffic.emptyDetail', { defaultValue: 'Missing evidence is not zero traffic.' })}</p>
                      </div>
                    )
                  : null}
                <ol className='traffic-period-list'>
                  {periodPage?.items.map((period) => {
                    const selected = selectedPeriod?.id === period.id;
                    return (
                      <li data-selected={selected || undefined} key={period.id}>
                        <button
                          aria-expanded={selected}
                          aria-haspopup='dialog'
                          onClick={() => void inspectPeriod(period)}
                          type='button'
                        >
                          <span className='traffic-period-list__time'>
                            <strong>{formatTime(period.period_start)}</strong>
                            <small>{t('observability.traffic.through', { defaultValue: 'Through {{time}}', time: formatTime(period.period_end) })}</small>
                          </span>
                          <span className='traffic-period-list__totals'>
                            <span>
                              <ArrowDown aria-hidden='true' size={14} />
                              {formatBytes(period.inbound_bytes, locale)}
                            </span>
                            <span>
                              <ArrowUp aria-hidden='true' size={14} />
                              {formatBytes(period.outbound_bytes, locale)}
                            </span>
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ol>
                {periodPage?.next === undefined ? null : <button className='button button--quiet traffic-load-more' disabled={loadingOlder} onClick={() => void loadPeriods(undefined, periodPage.next, true)} type='button'>{loadingOlder ? t('observability.traffic.loadingOlder', { defaultValue: 'Loading…' }) : t('observability.traffic.loadOlder', { defaultValue: 'Load older' })}</button>}
              </div>
            </div>

            <Sheet
              onOpenChange={(open) => {
                if (!open) closePeriodDetail();
              }}
              open={selectedPeriod !== null}
            >
              <SheetContent className='observability-detail-sheet' side='right'>
                <SheetHeader className='observability-detail-sheet__header'>
                  <SheetTitle>
                    {selectedPeriod?.id ?? t('observability.traffic.detailLabel', { defaultValue: 'Traffic period detail' })}
                  </SheetTitle>
                  <SheetDescription>
                    {selectedPeriod?.activation_bundle_id ?? t('observability.value.noBundle', { defaultValue: 'No bundle ID' })}
                  </SheetDescription>
                </SheetHeader>
                <div
                  aria-busy={periodDetailLoading}
                  aria-live='polite'
                  className='observability-detail-sheet__body'
                >
                  {periodDetailLoading
                    ? <p className='observability-detail-sheet__status' role='status'>{t('observability.traffic.loadingDetail', { defaultValue: 'Loading complete period…' })}</p>
                    : null}
                  {periodDetailError === ''
                    ? null
                    : (
                        <div className='form-error' role='alert'>
                          <strong>{t('observability.error.periodDetail', { defaultValue: 'Period detail failed' })}</strong>
                          <span>{periodDetailError}</span>
                        </div>
                      )}
                  {selectedPeriod === null
                    ? null
                    : (
                        <div className='observability-detail' key={selectedPeriod.id}>
                          <dl>
                            <div>
                              <dt>{t('observability.traffic.inbound', { defaultValue: 'Inbound bytes' })}</dt>
                              <dd>{new Intl.NumberFormat(locale).format(selectedPeriod.inbound_bytes)}</dd>
                            </div>
                            <div>
                              <dt>{t('observability.traffic.outbound', { defaultValue: 'Outbound bytes' })}</dt>
                              <dd>{new Intl.NumberFormat(locale).format(selectedPeriod.outbound_bytes)}</dd>
                            </div>
                            <div>
                              <dt>{t('observability.filter.from', { defaultValue: 'From' })}</dt>
                              <dd>{formatTime(selectedPeriod.period_start)}</dd>
                            </div>
                            <div>
                              <dt>{t('observability.filter.to', { defaultValue: 'To' })}</dt>
                              <dd>{formatTime(selectedPeriod.period_end)}</dd>
                            </div>
                          </dl>
                          <details>
                            <summary>{t('observability.traffic.counters', { defaultValue: 'Counter breakdown' })}</summary>
                            <pre>{JSON.stringify(selectedPeriod.counters, null, 2)}</pre>
                          </details>
                        </div>
                      )}
                </div>
              </SheetContent>
            </Sheet>
          </section>
        </TabsContent>

        <TabsContent value='logs'>
          <LogsPanel refreshKey={refreshKey} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
