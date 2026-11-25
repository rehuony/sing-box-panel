import type { ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@shared/components/ui/spinner';
import { formatBytes, formatDuration, formatRate, formatUptime } from '@shared/lib/telemetry-format';
import {
  ReadoutCopyable,
  ReadoutMeter,
  ReadoutParts,
  ReadoutRow,
  ReadoutUnavailable,
  ReadoutUpDown,
} from '@shared/components/terminal';

import type { ConsoleStatus, RuntimeReading } from '@/api/generated';

import type { TrafficRates } from './status-format';

import { connectionTone, percentOf } from './status-format';

/** The readout's rows, so the loading state holds their height. */
const ROWS = 12;

const coreTone: Record<RuntimeReading['state'], string> = {
  inspection_unavailable: 'text-[var(--color-warning-text)]',
  running: 'text-[var(--color-success-text)]',
  stale: 'text-[var(--color-warning-text)]',
  stopped: 'text-muted-foreground',
};

interface StatusReadoutProps {
  /** The time the readout's durations count to. */
  now: number;
  /** The node stopped answering; the readout keeps the last status, dimmed. */
  stale: boolean;
  rates: TrafficRates | null;
  status: ConsoleStatus | null;
}

/** The output of the status command: the node's link, core, traffic and host. */
export function StatusReadout({ now, rates, stale, status }: StatusReadoutProps) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? 'en';
  const duration = {
    day: t('terminal.duration.day'),
    hour: t('terminal.duration.hour'),
    minute: t('terminal.duration.minute'),
    second: t('terminal.duration.second'),
  };
  const percent = new Intl.NumberFormat(locale, { maximumFractionDigits: 0, style: 'percent' });
  const load = new Intl.NumberFormat(locale, { maximumFractionDigits: 2, minimumFractionDigits: 2 });

  let body: ReactNode;
  if (status === null) {
    body = (
      <div aria-busy='true' className='flex items-start pt-0.5' style={{ minHeight: `${ROWS * 1.25}rem` }}>
        <Spinner className='size-3.5' />
      </div>
    );
  } else {
    const { connection, enrollment, host, node, runtime, traffic } = status;
    const live = traffic?.live ?? null;
    // Without a running core nothing flows, so its rates are a known zero rather than unknown.
    const idle = live === null && runtime?.state === 'stopped';
    const address = enrollment ? enrollment.panel_url : null;
    const memory = host ? percentOf(host.memory_used, host.memory_total) : null;
    const disk = host ? percentOf(host.disk_used, host.disk_total) : null;
    const retry = connection.retry_at === undefined ? null : Date.parse(connection.retry_at) - now;
    body = (
      <dl className='grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4'>
        <ReadoutRow label={t('terminal.row.link')}>
          <ReadoutParts>
            {[
              <span className={connectionTone[connection.state].text} key='state'>{t(`connection.state.${connection.state}`)}</span>,
              retry === null
                ? null
                : (
                    <span className='text-muted-foreground' key='retry'>
                      {retry > 0 ? t('terminal.value.retryIn', { duration: formatDuration(retry, duration) }) : t('terminal.value.retrying')}
                    </span>
                  ),
            ]}
          </ReadoutParts>
        </ReadoutRow>
        <ReadoutRow label={t('terminal.row.panel')}>{address ? <ReadoutCopyable label={t('terminal.row.panel')} value={address} /> : <ReadoutUnavailable />}</ReadoutRow>
        <ReadoutRow label={t('terminal.row.nodeId')}>{enrollment ? <ReadoutCopyable label={t('terminal.row.nodeId')} value={enrollment.node_id} /> : <ReadoutUnavailable />}</ReadoutRow>
        <ReadoutRow label={t('terminal.row.core')}>
          {runtime
            ? (
                <ReadoutParts>
                  {[
                    <span className={coreTone[runtime.state]} key='state'>{t(`terminal.value.core.${runtime.state}`)}</span>,
                    runtime.state === 'running'
                      ? runtime.running_version
                      : runtime.enabled_version ?? <span className='text-muted-foreground' key='none'>{t('terminal.value.noCore')}</span>,
                    runtime.started_at ? t('terminal.value.uptime', { duration: formatUptime(runtime.started_at, now, duration) }) : null,
                    runtime.pid === undefined ? null : t('terminal.value.pid', { pid: runtime.pid }),
                  ]}
                </ReadoutParts>
              )
            : <ReadoutUnavailable />}
        </ReadoutRow>
        <ReadoutRow label={t('terminal.row.traffic')}>
          {live || idle
            ? (
                <ReadoutParts>
                  {[
                    <ReadoutUpDown
                      download={formatRate(live ? rates?.download ?? null : 0, locale)}
                      key='rates'
                      upload={formatRate(live ? rates?.upload ?? null : 0, locale)}
                    />,
                    t('terminal.value.connections', { count: live?.connections ?? 0 }),
                  ]}
                </ReadoutParts>
              )
            : (
                <ReadoutParts>
                  {[
                    <ReadoutUnavailable key='rates' />,
                    runtime?.state === 'running'
                      ? <span className='text-muted-foreground' key='why'>{t('terminal.value.noCounters')}</span>
                      : null,
                  ]}
                </ReadoutParts>
              )}
        </ReadoutRow>
        <ReadoutRow label={t('terminal.row.total')}>
          {traffic
            ? (
                <ReadoutUpDown
                  download={formatBytes(traffic.download_bytes, locale)}
                  upload={formatBytes(traffic.upload_bytes, locale)}
                />
              )
            : <ReadoutUnavailable />}
        </ReadoutRow>
        <ReadoutRow label={t('terminal.row.cpu')}>
          {host
            ? (
                <span className='flex min-w-0 items-center gap-x-2'>
                  <ReadoutMeter percent={host.cpu_percent} />
                  <ReadoutParts>
                    {[
                      host.cpu_percent === null ? <ReadoutUnavailable key='cpu' /> : percent.format(host.cpu_percent / 100),
                      host.load_one === null ? null : t('terminal.value.load', { load: load.format(host.load_one) }),
                      t('terminal.value.cores', { count: host.cpu_count }),
                    ]}
                  </ReadoutParts>
                </span>
              )
            : <ReadoutUnavailable />}
        </ReadoutRow>
        <ReadoutRow label={t('terminal.row.memory')}>
          {host && host.memory_total !== null
            ? (
                <span className='flex min-w-0 items-center gap-x-2'>
                  <ReadoutMeter percent={memory} />
                  <span>{`${formatBytes(host.memory_used, locale)} / ${formatBytes(host.memory_total, locale)}`}</span>
                </span>
              )
            : <ReadoutUnavailable />}
        </ReadoutRow>
        <ReadoutRow label={t('terminal.row.disk')}>
          {host && host.disk_total !== null
            ? (
                <span className='flex min-w-0 items-center gap-x-2'>
                  <ReadoutMeter percent={disk} />
                  <span>{`${formatBytes(host.disk_used, locale)} / ${formatBytes(host.disk_total, locale)}`}</span>
                </span>
              )
            : <ReadoutUnavailable />}
        </ReadoutRow>
        <ReadoutRow label={t('terminal.row.host')}>
          <ReadoutParts>
            {[
              `${node.os}/${node.arch}`,
              // An address not yet detected is left out rather than marked.
              node.public_ip ? <ReadoutCopyable key='ip' label={t('terminal.row.publicIp')} value={node.public_ip} /> : null,
            ]}
          </ReadoutParts>
        </ReadoutRow>
        <ReadoutRow label={t('terminal.row.agent')}>
          <ReadoutParts>{[node.version, t('terminal.value.protocol', { protocol: node.protocol })]}</ReadoutParts>
        </ReadoutRow>
      </dl>
    );
  }

  // Its values change on every refresh; only new output is announced.
  return <div aria-live='off' className={cn('transition-opacity duration-200', stale && 'opacity-60')}>{body}</div>;
}
