import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { formatBytes } from '@shared/lib/telemetry-format';
import { formatDateTime, formatRelativeTime } from '@shared/lib/format-time';

import type { FleetNode, NodeTrafficUsage } from '@/api/api-client';

import { nodePath } from '@/utils/node-path';
import { NodeStateBadge } from '@/components/node-state-badge';

function UsageMeter({ usage }: { usage: NodeTrafficUsage | undefined }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const period = usage?.current_traffic_period;
  const used = period === undefined ? undefined : period.inbound_bytes + period.outbound_bytes;
  const quota = usage?.quota_bytes;
  const share = used !== undefined && quota ? Math.min(1, used / quota) : 0;
  const fill = usage?.quota_exceeded
    ? 'bg-[var(--color-danger)]'
    : share >= 0.9 ? 'bg-[var(--color-warning)]' : 'bg-primary';
  const label = t('fleet.card.usage');
  const total = quota ? formatBytes(quota, locale) : t('fleet.card.unlimited');
  return (
    <div className='flex flex-col gap-1.5'>
      <div className='flex items-baseline justify-between gap-2 text-xs'>
        <span className='text-muted-foreground'>{label}</span>
        <span className='truncate font-mono tabular-nums'>
          {formatBytes(used, locale)}
          <span className='text-muted-foreground'>{` / ${total}`}</span>
        </span>
      </div>
      <div
        aria-label={label}
        aria-valuemax={quota ?? undefined}
        aria-valuemin={0}
        aria-valuenow={quota ? used : undefined}
        aria-valuetext={`${formatBytes(used, locale)} / ${total}`}
        className='h-1.5 overflow-hidden rounded-full bg-[var(--color-surface-raised)]'
        role='meter'
      >
        <div className={`h-full rounded-full ${fill}`} style={{ width: `${share * 100}%` }} />
      </div>
      {usage?.quota_exceeded ? <span className='text-xs text-[var(--color-danger-text)]'>{t('fleet.card.quotaReached')}</span> : null}
    </div>
  );
}

function NodeStatus({ node }: { node: FleetNode }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  if (node.state === 'pending') {
    return <span className='text-muted-foreground' title={formatDateTime(node.created_at, locale)}>{t('fleet.card.added', { time: formatRelativeTime(node.created_at, locale) })}</span>;
  }
  if (node.state === 'offline') {
    return <span className='text-[var(--color-warning-text)]' title={formatDateTime(node.last_seen_at, locale)}>{t('node.lastSeen', { time: formatRelativeTime(node.last_seen_at, locale) })}</span>;
  }
  const runtime = node.runtime?.observation_state;
  const running = runtime === 'running';
  return (
    <span className='flex items-center gap-1.5'>
      <span aria-hidden='true' className={`size-1.5 rounded-full ${running ? 'bg-[var(--color-success)]' : 'bg-[var(--color-text-faint)]'}`} />
      {t(running ? 'telemetry.runtime.running' : runtime === 'stopped' ? 'telemetry.runtime.stopped' : 'fleet.card.runtimeUnknown')}
    </span>
  );
}

/** One node in the fleet: its identity, state, core version and its own period usage against its own quota. */
export function NodeCard({ node, usage }: { node: FleetNode; usage: NodeTrafficUsage | undefined }) {
  const { t } = useTranslation();
  const version = node.runtime?.enabled_core?.exact_core_version;
  const host = [node.hostname, node.public_ip].filter(Boolean).join(' · ');
  return (
    <article className='relative flex min-w-0 flex-col gap-4 rounded-[var(--radius-card)] border bg-card p-4 transition-colors duration-[var(--dur-short)] ease-out hover:border-[var(--color-border-strong)] has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-ring'>
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0'>
          <h2 className='truncate text-sm font-semibold' title={node.name}>
            <Link className='outline-none after:absolute after:inset-0 after:rounded-[inherit]' to={nodePath(node.id)}>{node.name}</Link>
          </h2>
          <p className='truncate font-mono text-xs text-muted-foreground' title={host || undefined}>
            {host || t('fleet.card.noHost')}
          </p>
        </div>
        <NodeStateBadge state={node.state} />
      </div>
      <dl className='grid grid-cols-2 gap-3 text-xs'>
        <div className='min-w-0'>
          <dt className='text-muted-foreground'>{t('fleet.card.core')}</dt>
          <dd className='truncate font-mono'>{version ? `v${version.replace(/^v/i, '')}` : '—'}</dd>
        </div>
        <div className='min-w-0'>
          <dt className='text-muted-foreground'>{t('fleet.card.status')}</dt>
          <dd className='truncate'><NodeStatus node={node} /></dd>
        </div>
      </dl>
      <UsageMeter usage={usage} />
    </article>
  );
}
