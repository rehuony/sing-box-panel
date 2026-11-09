import type { CSSProperties, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { Check, Copy } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Fragment, useEffect, useState } from 'react';
import { Spinner } from '@shared/components/ui/spinner';
import { formatBytes, formatDuration, formatRate, formatUptime } from '@shared/lib/telemetry-format';

import type { ConsoleStatus, RuntimeReading } from '@/api/generated';

import type { TrafficRates } from './status-format';

import { connectionTone, panelAddress, percentOf } from './status-format';

/** How long a copy action shows that it copied. */
const COPIED_MS = 1_500;
/** The readout's rows, so the loading state holds their height. */
const ROWS = 12;

const coreTone: Record<RuntimeReading['state'], string> = {
  inspection_unavailable: 'text-[var(--color-warning-text)]',
  running: 'text-[var(--color-success-text)]',
  stale: 'text-[var(--color-warning-text)]',
  stopped: 'text-[var(--color-terminal-muted)]',
};

/**
 * Parts of one value, set apart by a faint dot. A part moves to the next line
 * whole, after its dot, and breaks inside only when it is longer than a line.
 */
function Parts({ children }: { children: ReactNode[] }) {
  const parts = children.filter(part => part !== null && part !== undefined && part !== false && part !== '');
  return (
    <span className='min-w-0'>
      {parts.map((part, index) => (
        // The parts of a value keep their order; their position is their identity.
        // eslint-disable-next-line react/no-array-index-key
        <Fragment key={index}>
          {index > 0 ? ' ' : null}
          <span className='inline-block max-w-full wrap-anywhere'>
            {part}
            {index < parts.length - 1 ? <span aria-hidden='true' className='text-[var(--color-terminal-muted)]'>{'\u00A0·'}</span> : null}
          </span>
        </Fragment>
      ))}
    </span>
  );
}

/** An upload and a download value, each after its arrow. */
function UpDown({ download, upload }: { download: string; upload: string }) {
  const { t } = useTranslation();
  return (
    <span className='inline-flex gap-x-3'>
      <span>
        <span aria-hidden='true'>↑ </span>
        <span className='sr-only'>{t('terminal.value.upload')}</span>
        {upload}
      </span>
      <span>
        <span aria-hidden='true'>↓ </span>
        <span className='sr-only'>{t('terminal.value.download')}</span>
        {download}
      </span>
    </span>
  );
}

function Row({ children, label }: { children: ReactNode; label: string }) {
  return (
    <div className='contents'>
      <dt className='text-[var(--color-terminal-muted)]'>{label}</dt>
      <dd className='flex min-h-5 min-w-0 items-center'>{children}</dd>
    </div>
  );
}

function Unavailable() {
  const { t } = useTranslation();
  return <span className='text-[var(--color-terminal-muted)]'>{t('common.unavailable')}</span>;
}

function CopyButton({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(setCopied, COPIED_MS, false);
    return () => window.clearTimeout(timer);
  }, [copied]);
  return (
    <Button
      aria-label={copied ? t('terminal.copied') : t('terminal.copy', { label })}
      // The button stays a small mark in the line; its target reaches past it.
      className={cn(
        'relative size-5 min-h-5 rounded-[var(--radius-control)] text-[var(--color-terminal-muted)] after:absolute after:-inset-2.5',
        'hover:bg-[var(--color-terminal-selection)] hover:text-[var(--color-terminal-text)]',
      )}
      onClick={() => {
        void navigator.clipboard?.writeText(value).then(() => setCopied(true), () => undefined);
      }}
      size='icon-xs'
      variant='ghost'
    >
      {copied ? <Check aria-hidden='true' className='size-3.5 text-[var(--color-success)]' /> : <Copy aria-hidden='true' className='size-3.5' />}
    </Button>
  );
}

function Copyable({ label, value }: { label: string; value: string }) {
  return (
    <span className='inline-flex max-w-full min-w-0 items-center gap-1 align-middle'>
      <span className='truncate' title={value}>{value}</span>
      <CopyButton label={label} value={value} />
    </span>
  );
}

/**
 * A share drawn as ten cells, filled to the nearest cell and toned as it nears
 * full. Its number beside it carries the value, so a phone leaves it out.
 */
function Meter({ percent }: { percent: number | null }) {
  if (percent === null) return null;
  const cells = percent < 0.5 ? 0 : Math.max(1, Math.round(percent / 10));
  const tone = percent >= 95 ? 'danger' : percent >= 80 ? 'warning' : undefined;
  return (
    <span
      aria-hidden='true'
      className='status-meter max-sm:hidden'
      data-tone={tone}
      style={{ '--meter-fill': `${cells * 10}%` } as CSSProperties}
    />
  );
}

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
    const address = enrollment ? panelAddress(enrollment) : null;
    const memory = host ? percentOf(host.memory_used, host.memory_total) : null;
    const disk = host ? percentOf(host.disk_used, host.disk_total) : null;
    const retry = connection.retry_at === undefined ? null : Date.parse(connection.retry_at) - now;
    body = (
      <dl className='grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4'>
        <Row label={t('terminal.row.link')}>
          <Parts>
            {[
              <span className={connectionTone[connection.state].text} key='state'>{t(`connection.state.${connection.state}`)}</span>,
              retry === null
                ? null
                : (
                    <span className='text-[var(--color-terminal-muted)]' key='retry'>
                      {retry > 0 ? t('terminal.value.retryIn', { duration: formatDuration(retry, duration) }) : t('terminal.value.retrying')}
                    </span>
                  ),
            ]}
          </Parts>
        </Row>
        <Row label={t('terminal.row.panel')}>{address ? <Copyable label={t('terminal.row.panel')} value={address} /> : <Unavailable />}</Row>
        <Row label={t('terminal.row.nodeId')}>{enrollment ? <Copyable label={t('terminal.row.nodeId')} value={enrollment.node_id} /> : <Unavailable />}</Row>
        <Row label={t('terminal.row.trust')}>{enrollment ? t(`terminal.value.trust.${enrollment.trust}`) : <Unavailable />}</Row>
        <Row label={t('terminal.row.core')}>
          {runtime
            ? (
                <Parts>
                  {[
                    <span className={coreTone[runtime.state]} key='state'>{t(`terminal.value.core.${runtime.state}`)}</span>,
                    runtime.state === 'running'
                      ? runtime.running_version
                      : runtime.enabled_version ?? <span className='text-[var(--color-terminal-muted)]' key='none'>{t('terminal.value.noCore')}</span>,
                    runtime.started_at ? t('terminal.value.uptime', { duration: formatUptime(runtime.started_at, now, duration) }) : null,
                    runtime.pid === undefined ? null : t('terminal.value.pid', { pid: runtime.pid }),
                  ]}
                </Parts>
              )
            : <Unavailable />}
        </Row>
        <Row label={t('terminal.row.traffic')}>
          {live || idle
            ? (
                <Parts>
                  {[
                    <UpDown
                      download={formatRate(live ? rates?.download ?? null : 0, locale)}
                      key='rates'
                      upload={formatRate(live ? rates?.upload ?? null : 0, locale)}
                    />,
                    t('terminal.value.connections', { count: live?.connections ?? 0 }),
                  ]}
                </Parts>
              )
            : (
                <Parts>
                  {[
                    <Unavailable key='rates' />,
                    runtime?.state === 'running'
                      ? <span className='text-[var(--color-terminal-muted)]' key='why'>{t('terminal.value.noCounters')}</span>
                      : null,
                  ]}
                </Parts>
              )}
        </Row>
        <Row label={t('terminal.row.total')}>
          {traffic
            ? (
                <UpDown
                  download={formatBytes(traffic.download_bytes, locale)}
                  upload={formatBytes(traffic.upload_bytes, locale)}
                />
              )
            : <Unavailable />}
        </Row>
        <Row label={t('terminal.row.cpu')}>
          {host
            ? (
                <span className='flex min-w-0 items-center gap-x-2'>
                  <Meter percent={host.cpu_percent} />
                  <Parts>
                    {[
                      host.cpu_percent === null ? <Unavailable key='cpu' /> : percent.format(host.cpu_percent / 100),
                      host.load_one === null ? null : t('terminal.value.load', { load: load.format(host.load_one) }),
                      t('terminal.value.cores', { count: host.cpu_count }),
                    ]}
                  </Parts>
                </span>
              )
            : <Unavailable />}
        </Row>
        <Row label={t('terminal.row.memory')}>
          {host && host.memory_total !== null
            ? (
                <span className='flex min-w-0 items-center gap-x-2'>
                  <Meter percent={memory} />
                  <span>{`${formatBytes(host.memory_used, locale)} / ${formatBytes(host.memory_total, locale)}`}</span>
                </span>
              )
            : <Unavailable />}
        </Row>
        <Row label={t('terminal.row.disk')}>
          {host && host.disk_total !== null
            ? (
                <span className='flex min-w-0 items-center gap-x-2'>
                  <Meter percent={disk} />
                  <span>{`${formatBytes(host.disk_used, locale)} / ${formatBytes(host.disk_total, locale)}`}</span>
                </span>
              )
            : <Unavailable />}
        </Row>
        <Row label={t('terminal.row.host')}>
          <Parts>
            {[
              `${node.os}/${node.arch}`,
              // An address not yet detected is left out rather than marked.
              node.public_ip ? <Copyable key='ip' label={t('terminal.row.publicIp')} value={node.public_ip} /> : null,
            ]}
          </Parts>
        </Row>
        <Row label={t('terminal.row.agent')}>
          <Parts>{[node.version, t('terminal.value.protocol', { protocol: node.protocol })]}</Parts>
        </Row>
      </dl>
    );
  }

  // Its values change on every refresh; only new output is announced.
  return <div aria-live='off' className={cn('transition-opacity duration-200', stale && 'opacity-60')}>{body}</div>;
}
