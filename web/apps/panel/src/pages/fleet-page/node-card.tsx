import type { ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';

import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useMutation } from '@tanstack/react-query';
import { Button } from '@shared/components/ui/button';
import { CellMeter } from '@shared/components/cell-meter';
import { ArrowDown, ArrowUp, KeyRound, Network } from 'lucide-react';
import { formatDateTime, formatRelativeTime } from '@shared/lib/format-time';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import { EM_DASH, formatBytes, formatRate, formatUptime } from '@shared/lib/telemetry-format';

import type { NodeEnrollment } from '@/api/api-client';
import type { NodeStatus } from '@/utils/fleet-summary';

import { nodePath } from '@/utils/node-path';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { NodeStateBadge } from '@/components/node-state-badge';
import { NodeEnrollmentDialog } from '@/components/node-enrollment-dialog';
import { attentionItems, hostLoad, loadTone, nodeRates, percentOf } from '@/utils/fleet-summary';

/** Capacity meters span their column in twenty cells of 5% each. */
const METER_CELLS = 20;

type Tone = 'danger' | 'warning';
type Health = Tone | 'neutral' | 'success';

const toneText: Record<Tone, string> = {
  danger: 'text-[var(--color-danger-text)]',
  warning: 'text-[var(--color-warning-text)]',
};

const healthDot: Record<Health, string> = {
  danger: 'bg-[var(--color-danger)]',
  neutral: 'bg-[var(--color-text-faint)]',
  success: 'bg-[var(--color-success)]',
  warning: 'bg-[var(--color-warning)]',
};

function useLocale() {
  const { i18n } = useTranslation();
  return i18n.resolvedLanguage ?? i18n.language ?? 'en';
}

function usePercent() {
  const locale = useLocale();
  return (percent: number) => new Intl.NumberFormat(locale, { maximumFractionDigits: 0, style: 'percent' }).format(percent / 100);
}

interface QuantityProps {
  value: string;
  className?: string;
  unitClassName?: string;
  numberClassName?: string;
}

/** A formatted amount with its unit set back: smaller and muted after the number. */
function Quantity({ className, numberClassName, unitClassName, value }: QuantityProps) {
  const space = value.lastIndexOf(' ');
  if (space < 0) return <span className={className}><span className={numberClassName}>{value}</span></span>;
  return (
    <span className={className}>
      <span className={numberClassName}>{value.slice(0, space)}</span>
      {/* The space sits in the unit's smaller size, so the unit stays close to its number. */}
      <span className={cn('text-muted-foreground', unitClassName)}>{` ${value.slice(space + 1)}`}</span>
    </span>
  );
}

/**
 * A reading whose details open on hover. Under touch the card's link keeps the
 * whole card, and the node's own view carries the same details.
 */
function Detail({ children, className, content }: { children: ReactNode; className?: string; content: string | null }) {
  if (content === null) return <div className={className}>{children}</div>;
  return (
    <Tooltip>
      <TooltipTrigger render={<div className={cn('pointer-fine:relative pointer-fine:z-10', className)} />}>{children}</TooltipTrigger>
      <TooltipContent>{content}</TooltipContent>
    </Tooltip>
  );
}

function Dot() {
  return <span aria-hidden='true' className='flex-none text-[var(--color-text-faint)]'>·</span>;
}

function versionLabel(item: NodeStatus): string | null {
  const runtime = item.node.runtime;
  const exact = runtime?.running?.exact_core_version ?? runtime?.enabled_core?.exact_core_version;
  return exact ? `v${exact.replace(/^v/i, '')}` : null;
}

/**
 * The node's worst condition as one dot before its name: the fleet's
 * attention rules, plus host load at the card's own meter thresholds. Its
 * reasons open on hover.
 */
function HealthDot({ item, now }: { item: NodeStatus; now: number }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const format = usePercent();
  const { node } = item;
  let tone: Health = 'success';
  let summary = t('fleet.card.healthy');
  if (node.state === 'pending') {
    tone = 'neutral';
    summary = t('node.awaitingEnrollment');
  } else {
    const found = attentionItems([item]);
    const reasons = found.map(({ kind, percent }) => t(`overview.attention.kind.${kind}`, {
      percent: percent === undefined ? '' : format(percent),
      time: formatRelativeTime(node.last_seen_at, locale, now),
    }));
    const tones: Tone[] = found.flatMap(({ tone: each }) => each === 'neutral' ? [] : [each]);
    const load = hostLoad(item);
    const kinds = new Set(found.map(({ kind }) => kind));
    for (const [label, share, kind] of [
      [t('fleet.card.cpu'), load.cpu, null],
      [t('fleet.card.memory'), load.memory, 'memoryNear'],
      [t('fleet.card.disk'), load.disk, 'diskNear'],
    ] as const) {
      const high = loadTone(share);
      if (high === undefined || share === null) continue;
      tones.push(high);
      if (kind === null || !kinds.has(kind)) reasons.push(`${label} ${format(share)}`);
    }
    if (tones.includes('danger')) tone = 'danger';
    else if (tones.includes('warning')) tone = 'warning';
    if (reasons.length > 0) summary = reasons.join(' · ');
  }
  return (
    <Detail className='-mx-1 flex h-7 flex-none items-center self-center px-1' content={summary}>
      <span aria-hidden='true' className={cn('size-2 rounded-full', healthDot[tone])} />
      <span className='sr-only'>{summary}</span>
    </Detail>
  );
}

/** An online node's core: its state and version, with its uptime and platform on hover. */
function CoreState({ item, now }: { item: NodeStatus; now: number }) {
  const { t } = useTranslation();
  const { node } = item;
  const runtime = node.runtime;
  const started = runtime?.observation_state === 'running' ? runtime.running?.started_at : undefined;
  let state = 'fleet.card.runtimeUnknown';
  if (started !== undefined) state = 'telemetry.runtime.running';
  else if (runtime?.observation_state === 'stopped') state = 'telemetry.runtime.stopped';
  const version = versionLabel(item);
  const uptime = started === undefined
    ? null
    : t('fleet.card.uptime', {
        value: formatUptime(started, now, {
          day: t('telemetry.unit.day'),
          hour: t('telemetry.unit.hour'),
          minute: t('telemetry.unit.minute'),
          second: t('telemetry.unit.second'),
        }),
      });
  const platform = node.os && node.arch ? `${node.os}/${node.arch}` : null;
  const detail = [uptime, platform].filter(Boolean).join(' · ') || null;
  return (
    <Detail className='flex items-center gap-2 text-xs text-[var(--color-text-secondary)]' content={detail}>
      <span>{t(state)}</span>
      {version
        ? (
            <>
              <Dot />
              <span className='font-[family-name:var(--font-data)] text-muted-foreground'>{version}</span>
            </>
          )
        : null}
      {detail ? <span className='sr-only'>{`, ${detail}`}</span> : null}
    </Detail>
  );
}

interface RateProps {
  label: string;
  value: string;
  /** Whether the number keeps a fixed slot before its unit. */
  slot?: boolean;
  icon: LucideIcon;
  /** The series colour of the icon. */
  className?: string;
}

/**
 * A live reading: its label over its icon, in its series colour, and number.
 * A rate's number sits right-aligned in a slot three characters wide, so its
 * unit stays put as the value changes; a count has no unit and needs no slot.
 */
function Rate({ className, icon: Icon, label, slot = true, value }: RateProps) {
  const known = value !== EM_DASH;
  return (
    <div className='flex max-w-full min-w-0 flex-col items-center gap-1'>
      <dt className='text-xs text-muted-foreground'>{label}</dt>
      <dd className='flex max-w-full min-w-0 items-center gap-1.5'>
        <Icon aria-hidden='true' className={cn('size-4 flex-none', known && className ? className : 'text-muted-foreground')} />
        <Quantity
          className={cn('truncate font-[family-name:var(--font-data)] text-xl font-semibold tabular-nums @max-[21rem]/card:text-base', !known && 'text-muted-foreground')}
          numberClassName={slot ? 'inline-block min-w-[3ch] text-right' : undefined}
          unitClassName='text-xs font-normal'
          value={value}
        />
      </dd>
    </div>
  );
}

interface CapacityProps {
  label: string;
  value: ReactNode;
  /** The meter's share, or null while it cannot be known. */
  share: number | null;
  /** The figures behind the reading, on hover and for assistive technology. */
  detail: string | null;
  valueClassName?: string;
  meterTone?: 'success' | Tone;
}

/** One capacity reading: its label and value over ten cells, its figures on hover. */
function Capacity({ detail, label, meterTone, share, value, valueClassName }: CapacityProps) {
  return (
    <Detail className='flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1.5' content={detail}>
      <dt className='flex-none text-xs text-muted-foreground'>{label}</dt>
      <dd className={cn('min-w-0 flex-1 truncate font-[family-name:var(--font-data)] text-xs font-medium tabular-nums', valueClassName)}>
        {value}
        {detail ? <span className='sr-only'>{`, ${detail}`}</span> : null}
      </dd>
      <dd className='flex basis-full font-[family-name:var(--font-data)] text-xs'>
        {/* Load at rest stays muted, so only a share near its limit or the remaining quota takes colour. */}
        <CellMeter cells={METER_CELLS} className={cn(meterTone === undefined && 'text-muted-foreground', share === null && 'opacity-40')} percent={share ?? 0} tone={meterTone} />
      </dd>
    </Detail>
  );
}

function Load({ detail, label, percent }: { detail: string | null; label: string; percent: number | null }) {
  const format = usePercent();
  const tone = loadTone(percent);
  return (
    <Capacity
      detail={detail}
      label={label}
      meterTone={tone}
      share={percent}
      value={percent === null ? EM_DASH : format(percent)}
      valueClassName={tone ? toneText[tone] : percent === null ? 'text-muted-foreground' : undefined}
    />
  );
}

/**
 * What is left of this period's quota, as the capacity row's last reading:
 * the meter drains as the node uses traffic, stays success while more than
 * 10% is left, turns warning from there and empties once the quota is
 * reached; an unlimited node keeps a full meter. What was used, each
 * direction and the reset date open on hover.
 */
function Remaining({ item }: { item: NodeStatus }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const { current_traffic_period: period, quota_bytes: quota, quota_exceeded: reached } = item.metrics;
  const used = period.inbound_bytes + period.outbound_bytes;
  const remaining = quota ? Math.max(0, quota - used) : null;
  let left = 100;
  if (reached) left = 0;
  else if (quota) left = percentOf(remaining, quota) ?? 0;
  let tone: 'success' | Tone = 'success';
  if (reached) tone = 'danger';
  else if (quota && left <= 10) tone = 'warning';
  const resets = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' }).format(new Date(period.period_end));
  const detail = [
    t('fleet.card.used', { value: quota ? `${formatBytes(used, locale)} / ${formatBytes(quota, locale)}` : formatBytes(used, locale) }),
    `↓ ${formatBytes(period.inbound_bytes, locale)}`,
    `↑ ${formatBytes(period.outbound_bytes, locale)}`,
    t('fleet.card.resets', { date: resets }),
  ].join(' · ');
  // A reached quota says so in words, so the state never rests on colour alone.
  let value: ReactNode = t('fleet.card.unlimited');
  if (reached) value = t('fleet.card.quotaReached');
  else if (remaining !== null) value = <Quantity value={formatBytes(remaining, locale)} />;
  return (
    <Capacity
      detail={detail}
      label={t('fleet.card.remaining')}
      meterTone={tone}
      share={left}
      value={value}
      valueClassName={reached ? cn(toneText.danger, 'font-[family-name:var(--font-body)]') : tone === 'warning' ? toneText.warning : undefined}
    />
  );
}

/** CPU, memory, disk and what is left of the quota, two by two; host load reads "—" while the node is away. */
function CapacityGrid({ item }: { item: NodeStatus }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const load = hostLoad(item);
  const host = item.node.state === 'online' ? item.metrics.host : undefined;
  const pair = (used: number | undefined, total: number | undefined) => host ? `${formatBytes(used, locale)} / ${formatBytes(total, locale)}` : null;
  return (
    <dl className='mt-auto grid grid-cols-2 gap-3'>
      <Load
        detail={host ? `${t('fleet.card.cores', { count: host.cpu_count })} · ${t('fleet.card.load', { value: host.load_one.toFixed(2) })}` : null}
        label={t('fleet.card.cpu')}
        percent={load.cpu}
      />
      <Load detail={pair(host?.memory_used, host?.memory_total)} label={t('fleet.card.memory')} percent={load.memory} />
      <Load detail={pair(host?.disk_used, host?.disk_total)} label={t('fleet.card.disk')} percent={load.disk} />
      <Remaining item={item} />
    </dl>
  );
}

/** Download, upload and connections, each a label over its number and centred in its third of the row. */
function LiveRow({ item }: { item: NodeStatus }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const { metrics, node } = item;
  const rates = nodeRates(item);
  const live = metrics.live_sample;
  const idle = node.runtime?.observation_state === 'stopped';
  const connections = live ? live.active_connections : idle ? 0 : null;
  return (
    <dl className='grid h-12 grid-cols-3 items-center justify-items-center gap-3 *:max-w-full'>
      <Rate className='text-[var(--color-highlight-cyan)]' icon={ArrowDown} label={t('fleet.card.download')} value={formatRate(rates.download, locale)} />
      <Rate className='text-[var(--color-highlight-magenta)]' icon={ArrowUp} label={t('fleet.card.upload')} value={formatRate(rates.upload, locale)} />
      <Rate icon={Network} label={t('fleet.card.connections')} slot={false} value={connections === null ? EM_DASH : new Intl.NumberFormat(locale).format(connections)} />
    </dl>
  );
}

/**
 * An offline node in the live row's place and height: when it was last seen
 * over the core it last ran, centred.
 */
function Away({ item, now }: { item: NodeStatus; now: number }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const { node } = item;
  const version = versionLabel(item);
  return (
    <Detail className='flex h-12 max-w-full min-w-0 flex-col items-center justify-center gap-1 self-center' content={formatDateTime(node.last_seen_at, locale)}>
      <span className='text-xs text-muted-foreground'>{t('fleet.card.lastSeen')}</span>
      <span className='flex max-w-full min-w-0 items-center gap-2 text-sm'>
        <span className='truncate font-medium text-[var(--color-danger-text)]'>{formatRelativeTime(node.last_seen_at, locale, now)}</span>
        {version
          ? (
              <>
                <Dot />
                <span className='flex-none font-[family-name:var(--font-data)] text-xs text-muted-foreground'>{version}</span>
              </>
            )
          : null}
      </span>
    </Detail>
  );
}

/** A node that has not enrolled yet: in place of readings, a way to show its join code. */
function Enrollment({ item }: { item: NodeStatus }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [enrollment, setEnrollment] = useState<NodeEnrollment | null>(null);
  const issue = useMutation({
    mutationFn: () => client.createNodeEnrollment(item.node.id),
    onSuccess: setEnrollment,
  });
  return (
    <div className='flex flex-1 items-center justify-center gap-3'>
      <span aria-hidden='true' className='flex size-10 flex-none items-center justify-center rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground'>
        <KeyRound className='size-5' strokeWidth={1.5} />
      </span>
      <Button
        className='relative z-10'
        onClick={() => {
          if (!issue.isPending) issue.mutate();
        }}
        variant='outline'
      >
        {t(issue.isPending ? 'node.unavailable.issuing' : 'node.unavailable.issue')}
      </Button>
      {issue.error ? <ErrorNotice error={issue.error} title={t('node.unavailable.issueFailed')} /> : null}
      <NodeEnrollmentDialog enrollment={enrollment} onClose={() => setEnrollment(null)} title={t('enrollment.title')} />
    </div>
  );
}

/**
 * One node in the fleet, in two groups: its health, identity and core, with
 * its live rates; then its capacity, CPU, memory, disk and what is left of its
 * quota. An offline node keeps when it was last seen and its quota; one not
 * enrolled offers its join code.
 */
export function NodeCard({ item, now }: { item: NodeStatus; now: number }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const { metrics, node } = item;
  const pending = node.state === 'pending';
  const danger = node.state === 'offline' || metrics.quota_exceeded;
  const host = [node.hostname, node.public_ip].filter(Boolean).join(' · ');
  let body = <Enrollment item={item} />;
  if (!pending) {
    body = (
      <>
        {node.state === 'offline' ? <Away item={item} now={now} /> : <LiveRow item={item} />}
        <CapacityGrid item={item} />
      </>
    );
  }
  return (
    <article
      className={cn(
        '@container/card relative flex min-w-0 flex-col gap-4 rounded-[var(--radius-card)] border bg-[var(--color-surface-dense)] p-4 transition-colors duration-[var(--dur-short)] ease-out has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-ring has-[a:focus-visible]:ring-inset',
        danger ? 'border-[var(--color-danger-border)]' : 'hover:border-[var(--color-border-strong)]',
        pending && 'border-dashed',
      )}
    >
      <header className='flex items-start justify-between gap-3'>
        {/* The host follows the name on its line and only takes the room the name leaves, so it gives way first. */}
        <div className='flex min-w-0 items-baseline gap-2'>
          <HealthDot item={item} now={now} />
          <h2 className='min-w-0 truncate text-sm leading-7 font-semibold'>
            <Link className='outline-none after:absolute after:inset-0 after:rounded-[inherit]' to={nodePath(node.id)}>{node.name}</Link>
          </h2>
          <p className='min-w-0 flex-1 basis-0 truncate font-[family-name:var(--font-data)] text-xs text-muted-foreground'>
            {pending ? <span className='font-[family-name:var(--font-body)]'>{t('fleet.card.added', { time: formatRelativeTime(node.created_at, locale, now) })}</span> : host || t('fleet.card.noHost')}
          </p>
        </div>
        {/* A state badge marks only what needs attention; an online node shows its core instead. */}
        <div className='flex h-7 flex-none items-center'>
          {node.state === 'online' ? <CoreState item={item} now={now} /> : <NodeStateBadge state={node.state} />}
        </div>
      </header>
      <div className='flex flex-1 flex-col gap-6'>{body}</div>
    </article>
  );
}
