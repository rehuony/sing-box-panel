import { useCallback, useEffect, useState } from 'react';

import type { MetricsSnapshot, TrafficPeriod } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { PageHeading } from '@/components/page-heading';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

import { LogsPanel } from './logs-panel';
import './observability-page.css';

function formatBytes(value: number): string {
  return `${new Intl.NumberFormat(undefined, {
    notation: value >= 1_000_000 ? 'compact' : 'standard',
    maximumFractionDigits: 1,
  }).format(value)} B`;
}

function formatTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(new Date(value));
}

const unavailableReasons: Record<string, { title: string; detail: string }> = {
  not_applied: {
    title: 'No bundle is applied',
    detail: 'Apply a checked startup artifact before requesting collector data.',
  },
  process_only: {
    title: 'Process-only monitoring',
    detail: 'This bundle intentionally exposes process health without traffic counters.',
  },
  no_collector_sample: {
    title: 'No collector sample yet',
    detail: 'The applied bundle supports metrics, but no current period has been persisted.',
  },
  stale_collector_sample: {
    title: 'Collector sample is stale',
    detail: 'No successful sample for the current applied process was recorded in the last 30 seconds.',
  },
  monitoring_unavailable: {
    title: 'Monitoring tier unavailable',
    detail: 'Full monitoring is not implemented and no metrics are fabricated.',
  },
};

export function ObservabilityPage() {
  const client = useApiClient();
  const [metrics, setMetrics] = useState<MetricsSnapshot | null>(null);
  const [traffic, setTraffic] = useState<MetricsSnapshot | null>(null);
  const [periods, setPeriods] = useState<TrafficPeriod[] | null>(null);
  const [selectedPeriod, setSelectedPeriod] = useState<TrafficPeriod | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [periodError, setPeriodError] = useState('');
  const [refreshKey, setRefreshKey] = useState(0);
  const [loading, setLoading] = useState(false);

  const load = useCallback(async (signal?: AbortSignal) => {
    setLoading(true);
    setError(null);
    try {
      const [nextMetrics, nextTraffic, nextPeriods] = await Promise.all([
        client.getMetrics(signal),
        client.getTrafficStatus(signal),
        client.listTrafficPeriods({ limit: 20 }, signal),
      ]);
      if (!signal?.aborted) {
        setMetrics(nextMetrics);
        setTraffic(nextTraffic);
        setPeriods(nextPeriods.items);
      }
    } catch (loadError) {
      if (!signal?.aborted) setError(loadError);
    } finally {
      if (!signal?.aborted) setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load, refreshKey]);

  async function inspectPeriod(period: TrafficPeriod) {
    setPeriodError('');
    try {
      setSelectedPeriod(await client.getTrafficPeriod(period.id));
    } catch (inspectError) {
      setPeriodError(describeRequestError(inspectError));
    }
  }

  const availability = metrics?.available
    ? null
    : unavailableReasons[metrics?.reason_code ?? ''] ?? {
      title: 'Metrics unavailable',
      detail: metrics?.reason_code
        ? `The server reported “${metrics.reason_code}” and no counters.`
        : 'The server did not return an availability reason.',
    };
  const current = metrics?.current_traffic_period;

  return (
    <div className='page-stack'>
      <PageHeading
        action={<button className='button button--secondary' disabled={loading} onClick={() => setRefreshKey((key) => key + 1)} type='button'>{loading ? 'Refreshing…' : 'Refresh evidence'}</button>}
        eyebrow='Evidence / persisted samples'
        summary='Health, durable events and traffic are shown only when the server has actual evidence for the exact applied bundle. Missing data stays missing.'
        title='No invented zeroes.'
      />

      {error === null
        ? null
        : (
            <ErrorNotice error={error} title='Observability HTTP endpoints are unavailable' />
          )}

      <section className='instrument-strip' aria-labelledby='metrics-title'>
        <div className='instrument-strip__identity'>
          <p className='eyebrow'>Live collector contract</p>
          <h2 id='metrics-title'>Metrics availability</h2>
          <span className={`state-label ${metrics?.available ? 'state-label--success' : 'state-label--warning'}`}>
            <span aria-hidden='true' />
            {metrics?.available ? 'Available' : 'Unavailable'}
          </span>
        </div>
        {metrics === null
          ? <div className='inline-loading' aria-busy='true'>Reading collector evidence…</div>
          : metrics.available && current
            ? (
                <>
                  <div className='instrument-reading'>
                    <span>Inbound</span>
                    <strong>{formatBytes(current.inbound_bytes)}</strong>
                    <small>Measured in current period</small>
                  </div>
                  <div className='instrument-reading'>
                    <span>Outbound</span>
                    <strong>{formatBytes(current.outbound_bytes)}</strong>
                    <small>Measured in current period</small>
                  </div>
                  <div className='instrument-reading'>
                    <span>Memory / connections</span>
                    <strong>
                      {metrics.latest_sample
                        ? `${formatBytes(metrics.latest_sample.memory_bytes)} / ${metrics.latest_sample.active_connections}`
                        : 'Sample unavailable'}
                    </strong>
                    <small>
                      {metrics.quota_bytes
                        ? `${metrics.quota_exceeded ? 'Quota reached' : 'Within quota'} · ${formatBytes(metrics.quota_bytes)}`
                        : 'Unlimited quota'}
                    </small>
                  </div>
                </>
              )
            : (
                <div className='availability-gap'>
                  <strong>{availability?.title}</strong>
                  <p>{availability?.detail}</p>
                  <code>{metrics?.reason_code ?? 'response_unavailable'}</code>
                </div>
              )}
      </section>

      <div className='observability-layout'>
        <LogsPanel refreshKey={refreshKey} />

        <section className='traffic-panel' aria-labelledby='traffic-periods-title'>
          <div className='section-heading'>
            <div>
              <p className='eyebrow'>Recorded intervals</p>
              <h2 id='traffic-periods-title'>Traffic periods</h2>
            </div>
            <span>
              {traffic?.monitoring_tier ?? 'unavailable'}
              {' '}
              tier
            </span>
          </div>
          {periodError === ''
            ? null
            : (
                <div className='form-error' role='alert'>
                  <strong>Period detail failed</strong>
                  <span>{periodError}</span>
                </div>
              )}
          {periods === null ? <div className='inline-loading' aria-busy='true'>Loading persisted periods…</div> : null}
          {periods?.length === 0
            ? (
                <div className='empty-state'>
                  <strong>No persisted periods.</strong>
                  <p>This is not zero traffic. No collector sample is available for the selected history.</p>
                </div>
              )
            : null}
          {periods && periods.length > 0
            ? (
                <ol className='traffic-period-list'>
                  {periods.map((period) => (
                    <li key={period.id}>
                      <button aria-pressed={selectedPeriod?.id === period.id} onClick={() => void inspectPeriod(period)} type='button'>
                        <span>
                          <strong>{formatTime(period.period_start)}</strong>
                          <small>
                            until
                            {formatTime(period.period_end)}
                          </small>
                        </span>
                        <span>
                          <b>
                            ↓
                            {formatBytes(period.inbound_bytes)}
                          </b>
                          <b>
                            ↑
                            {formatBytes(period.outbound_bytes)}
                          </b>
                        </span>
                      </button>
                    </li>
                  ))}
                </ol>
              )
            : null}
          {selectedPeriod === null
            ? null
            : (
                <div className='period-detail' aria-live='polite'>
                  <div className='section-heading'>
                    <div>
                      <p className='eyebrow'>Exact persisted record</p>
                      <h3>{selectedPeriod.id}</h3>
                    </div>
                    <span>{selectedPeriod.activation_bundle_id ?? 'No bundle ID'}</span>
                  </div>
                  <dl>
                    <div>
                      <dt>Inbound bytes</dt>
                      <dd>{selectedPeriod.inbound_bytes.toLocaleString()}</dd>
                    </div>
                    <div>
                      <dt>Outbound bytes</dt>
                      <dd>{selectedPeriod.outbound_bytes.toLocaleString()}</dd>
                    </div>
                    <div>
                      <dt>Created</dt>
                      <dd>{formatTime(selectedPeriod.created_at)}</dd>
                    </div>
                  </dl>
                  <details>
                    <summary>Counter breakdown</summary>
                    <pre>{JSON.stringify(selectedPeriod.counters, null, 2)}</pre>
                  </details>
                </div>
              )}
        </section>
      </div>
    </div>
  );
}
