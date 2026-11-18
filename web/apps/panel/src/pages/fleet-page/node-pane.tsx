import type { MouseEvent, PointerEvent, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { memo, useEffect, useRef } from 'react';
import { CellMeter } from '@shared/components/cell-meter';
import { ReadoutParts, ReadoutRow } from '@shared/components/terminal';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import { formatDateTime, formatRelativeTime, formatShortDateTime } from '@shared/lib/format-time';
import { EM_DASH, formatBytes, formatDuration, formatRate, shareTone } from '@shared/lib/telemetry-format';

import type { NodeStatus } from '@/utils/fleet-summary';

import { useClock } from '@/hooks/use-clock';
import { nodePath } from '@/utils/node-path';
import { attentionItems, hostLoad, nodeRates, percentOf } from '@/utils/fleet-summary';

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

/** The element a pane's opening morphs from; one at a time carries the name. */
let opening: HTMLElement | null = null;

function useLocale() {
  const { i18n } = useTranslation();
  return i18n.resolvedLanguage ?? i18n.language ?? 'en';
}

function useUnits() {
  const { t } = useTranslation();
  return { day: t('telemetry.unit.day'), hour: t('telemetry.unit.hour'), minute: t('telemetry.unit.minute'), second: t('telemetry.unit.second') };
}

function percentText(locale: string, percent: number): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 0, style: 'percent' }).format(percent / 100);
}

function versionLabel(item: NodeStatus): string | null {
  const runtime = item.node.runtime;
  const exact = runtime?.running?.exact_core_version ?? runtime?.enabled_core?.exact_core_version;
  return exact ? `v${exact.replace(/^v/i, '')}` : null;
}

/** Details that open on hover; under touch the pane stays one link and the node's own view carries them. */
function Detail({ children, className, content }: { children: ReactNode; className?: string; content: string | null }) {
  if (content === null) return <span className={className}>{children}</span>;
  return (
    <Tooltip>
      <TooltipTrigger render={<span className={cn('pointer-fine:relative pointer-fine:z-10', className)} data-pane-detail='' />}>{children}</TooltipTrigger>
      <TooltipContent>{content}</TooltipContent>
    </Tooltip>
  );
}

/**
 * The node's worst condition: the fleet's attention rules and host load at
 * the meters' thresholds, its reasons on hover.
 */
function HealthDot({ item, now }: { item: NodeStatus; now: number }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const { node } = item;
  let tone: Health = 'success';
  let summary = t('fleet.pane.healthy');
  const found = attentionItems([item]);
  const reasons = found.map(({ kind, percent }) => t(`fleet.attention.${kind}`, {
    percent: percent === undefined ? '' : percentText(locale, percent),
    time: formatRelativeTime(node.last_seen_at, locale, now),
  }));
  const tones: Tone[] = found.map(({ tone: each }) => each);
  const load = hostLoad(item);
  const kinds = new Set(found.map(({ kind }) => kind));
  for (const [label, share, kind] of [
    [t('fleet.pane.cpu'), load.cpu, null],
    [t('fleet.pane.mem'), load.memory, 'memoryNear'],
    [t('fleet.pane.disk'), load.disk, 'diskNear'],
  ] as const) {
    const high = shareTone(share);
    if (high === undefined || share === null) continue;
    tones.push(high);
    if (kind === null || !kinds.has(kind)) reasons.push(`${label} ${percentText(locale, share)}`);
  }
  if (tones.includes('danger')) tone = 'danger';
  else if (tones.includes('warning')) tone = 'warning';
  if (reasons.length > 0) summary = reasons.join(' · ');
  return (
    <Detail className='-m-1 flex flex-none p-1' content={summary}>
      <span aria-hidden='true' className={cn('size-2 rounded-full', healthDot[tone])} />
      <span className='sr-only'>{summary}</span>
    </Detail>
  );
}

/** What the title bar's end says, worst first: away, quota reached, or an update to run. */
function TitleEnd({ item, now, release }: { item: NodeStatus; now: number; release: string | null }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const units = useUnits();
  const { metrics, node } = item;
  if (node.state === 'offline') {
    const since = node.last_seen_at ? formatDuration(now - Date.parse(node.last_seen_at), units).split(' ')[0] : EM_DASH;
    return (
      <span className={cn('flex-none', toneText.danger)} title={formatDateTime(node.last_seen_at, locale)}>
        {t('fleet.pane.offline', { time: since })}
      </span>
    );
  }
  if (metrics.quota_exceeded) return <span className={cn('flex-none', toneText.danger)}>{t('fleet.pane.quotaReached')}</span>;
  if (node.update_available && release) {
    const version = `v${release.replace(/^v/i, '')}`;
    return (
      <span className='flex-none text-[var(--color-info-text)]' title={t('fleet.pane.update', { version })}>
        <span aria-hidden='true'>{`↑ ${version}`}</span>
        <span className='sr-only'>{t('fleet.pane.update', { version })}</span>
      </span>
    );
  }
  return null;
}

function Unknown() {
  return <span className='text-muted-foreground'>{EM_DASH}</span>;
}

/** A share as ten cells with its number; load stays muted at rest and takes a tone only near its limit. */
function Share({ detail, percent }: { detail: string | null; percent: number | null }) {
  const locale = useLocale();
  if (percent === null) return <Unknown />;
  const tone = shareTone(percent);
  return (
    <Detail className='flex min-w-0 items-center gap-2' content={detail}>
      <CellMeter className={tone ? undefined : 'text-muted-foreground'} percent={percent} tone={tone} />
      <span className={cn('tabular-nums', tone && toneText[tone])}>{percentText(locale, percent)}</span>
      {detail ? <span className='sr-only'>{`, ${detail}`}</span> : null}
    </Detail>
  );
}

/**
 * The core's state, version and uptime: running in the text colour, stopped
 * in secondary text and unknown in warning. An offline node's row says when
 * it was last seen and the version it last ran; the title already says how
 * long ago.
 */
function CoreRow({ item, now }: { item: NodeStatus; now: number }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const units = useUnits();
  const { node } = item;
  const version = versionLabel(item);
  if (node.state === 'offline') {
    const seen = node.last_seen_at ? t('fleet.pane.lastSeen', { time: formatShortDateTime(node.last_seen_at, locale, now) }) : EM_DASH;
    return (
      <span className='flex min-w-0 text-[var(--color-text-secondary)]'>
        <span className='truncate' title={formatDateTime(node.last_seen_at, locale)}>{seen}</span>
        {version
          ? (
              <span className='flex-none'>
                <span aria-hidden='true' className='text-muted-foreground'>{' · '}</span>
                {version}
              </span>
            )
          : null}
      </span>
    );
  }
  const runtime = node.runtime;
  const started = runtime?.observation_state === 'running' ? runtime.running?.started_at : undefined;
  let state = <span className='text-[var(--color-warning-text)]' key='state'>{t('fleet.pane.unknown')}</span>;
  if (started !== undefined) state = <span key='state'>{t('fleet.pane.running')}</span>;
  else if (runtime?.observation_state === 'stopped') state = <span className='text-[var(--color-text-secondary)]' key='state'>{t('fleet.pane.stopped')}</span>;
  const platform = node.os && node.arch ? `${node.os}/${node.arch}` : null;
  return (
    <Detail className='min-w-0' content={platform}>
      <ReadoutParts>
        {[
          state,
          version,
          // The pane keeps one line a row, so uptime shows its largest unit; the node's window has the rest.
          started === undefined ? null : t('fleet.pane.uptime', { value: formatDuration(now - Date.parse(started), units).split(' ')[0] }),
        ]}
      </ReadoutParts>
    </Detail>
  );
}

/** Live download and upload rates after their arrows, then connections; a known zero reads 0. */
function NetRow({ item }: { item: NodeStatus }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const { metrics, node } = item;
  if (node.state !== 'online') return <Unknown />;
  const rates = nodeRates(item);
  const live = metrics.live_sample;
  const idle = node.runtime?.observation_state === 'stopped';
  const connections = live ? live.active_connections : idle ? 0 : null;
  return (
    <ReadoutParts>
      {[
        <span key='download'>
          <span aria-hidden='true' className='text-[var(--color-text-secondary)]'>↓ </span>
          <span className='sr-only'>{t('telemetry.metric.download')}</span>
          {formatRate(rates.download, locale)}
        </span>,
        <span key='upload'>
          <span aria-hidden='true' className='text-[var(--color-text-secondary)]'>↑ </span>
          <span className='sr-only'>{t('telemetry.metric.upload')}</span>
          {formatRate(rates.upload, locale)}
        </span>,
        connections === null ? null : t('fleet.pane.connections', { count: connections }),
      ]}
    </ReadoutParts>
  );
}

/**
 * What is left of this period's quota: a neutral meter that drains as the
 * node uses traffic, warning from 10% left and empty once reached, which says
 * so in danger text; an unlimited quota shows only its word.
 */
function QuotaRow({ item }: { item: NodeStatus }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const { current_traffic_period: period, quota_bytes: quota, quota_exceeded: reached } = item.metrics;
  const used = period.inbound_bytes + period.outbound_bytes;
  const remaining = quota ? Math.max(0, quota - used) : null;
  let left = 100;
  if (reached) left = 0;
  else if (quota) left = percentOf(remaining, quota) ?? 0;
  let tone: Tone | undefined;
  if (reached) tone = 'danger';
  else if (quota && left <= 10) tone = 'warning';
  const resets = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' }).format(new Date(period.period_end));
  const detail = [
    t('fleet.pane.used', { value: quota ? `${formatBytes(used, locale)} / ${formatBytes(quota, locale)}` : formatBytes(used, locale) }),
    `↓ ${formatBytes(period.inbound_bytes, locale)}`,
    `↑ ${formatBytes(period.outbound_bytes, locale)}`,
    t('fleet.pane.resets', { date: resets }),
  ].join(' · ');
  let value = t('fleet.pane.unlimited');
  if (reached) value = t('fleet.pane.reached');
  else if (remaining !== null) value = t('fleet.pane.left', { value: formatBytes(remaining, locale) });
  return (
    <Detail className='flex min-w-0 items-center gap-2' content={detail}>
      {/* A reached quota keeps its empty cells; the danger is in its word. */}
      {quota ? <CellMeter className={tone === 'warning' ? undefined : 'text-muted-foreground'} percent={left} tone={tone === 'warning' ? tone : undefined} /> : null}
      <span className={cn('truncate tabular-nums', tone ? toneText[tone] : !quota && 'text-[var(--color-text-secondary)]')}>{value}</span>
      <span className='sr-only'>{`, ${detail}`}</span>
    </Detail>
  );
}

function Readings({ item, now }: { item: NodeStatus; now: number }) {
  const { t } = useTranslation();
  const locale = useLocale();
  const load = hostLoad(item);
  const host = item.node.state === 'online' ? item.metrics.host : undefined;
  const pair = (used: number | undefined, total: number | undefined) => host ? `${formatBytes(used, locale)} / ${formatBytes(total, locale)}` : null;
  return (
    // Each reading keeps one line, so the pane's six rows hold its fixed height.
    <dl className='grid grid-cols-[max-content_minmax(0,1fr)] content-center gap-x-3 px-3 py-2.5 [&_dd]:overflow-hidden [&_dd]:whitespace-nowrap'>
      <ReadoutRow label={t('fleet.pane.core')}><CoreRow item={item} now={now} /></ReadoutRow>
      <ReadoutRow label={t('fleet.pane.net')}><NetRow item={item} /></ReadoutRow>
      <ReadoutRow label={t('fleet.pane.cpu')}>
        <Share
          detail={host ? `${t('fleet.pane.cores', { count: host.cpu_count })} · ${t('fleet.pane.load', { value: host.load_one.toFixed(2) })}` : null}
          percent={load.cpu}
        />
      </ReadoutRow>
      <ReadoutRow label={t('fleet.pane.mem')}><Share detail={pair(host?.memory_used, host?.memory_total)} percent={load.memory} /></ReadoutRow>
      <ReadoutRow label={t('fleet.pane.disk')}><Share detail={pair(host?.disk_used, host?.disk_total)} percent={load.disk} /></ReadoutRow>
      <ReadoutRow label={t('fleet.pane.quota')}><QuotaRow item={item} /></ReadoutRow>
    </dl>
  );
}

/** How long a touch holds a pane before it selects it. */
const LONG_PRESS_MS = 500;
/** How far, in pixels, a holding touch may move before it counts as a scroll. */
const PRESS_SLOP = 10;

interface NodePaneProps {
  /** Whether opening the node morphs the pane into its window. */
  morph: boolean;
  item: NodeStatus;
  selected: boolean;
  /** While on, a click toggles the pane instead of opening the node. */
  selecting: boolean;
  /** The panel's release, which a node with an update available can run. */
  release: string | null;
  /** Toggles the pane and enters selection mode; with range, selects from the last pane toggled to this one. */
  onToggle: (id: string, range: boolean) => void;
}

/**
 * One node as a terminal pane: a 32px title bar with its health, name, host
 * and what needs saying, then six rows of readings. A click opens the node;
 * in selection mode, with Ctrl, Cmd or Shift, or after a touch held on it, it
 * toggles the pane instead, and Space toggles the focused pane. The pane
 * stays a link, so a middle click or its context menu opens a new tab; in
 * selection mode it reads as a checkbox, checked while the pane is selected.
 */
export const NodePane = memo(({ item, morph, onToggle, release, selected, selecting }: NodePaneProps) => {
  const { t } = useTranslation();
  const now = useClock();
  const { node } = item;
  const host = [node.hostname, node.public_ip].filter(Boolean).join(' · ');
  const pressRef = useRef<{ timer: number; x: number; y: number } | null>(null);
  // The click and context menu that end a long press are not the user's next action.
  const swallowRef = useRef(false);

  function cancelPress() {
    if (pressRef.current) window.clearTimeout(pressRef.current.timer);
    pressRef.current = null;
  }
  useEffect(() => cancelPress, []);

  function longPress() {
    cancelPress();
    swallowRef.current = true;
    if (!selected) onToggle(node.id, false);
  }

  function toggles(event: MouseEvent): boolean {
    return selecting || event.metaKey || event.ctrlKey || event.shiftKey;
  }

  function onLinkClick(event: MouseEvent<HTMLAnchorElement>) {
    if (swallowRef.current) {
      swallowRef.current = false;
      event.preventDefault();
      return;
    }
    if (toggles(event)) {
      event.preventDefault();
      onToggle(node.id, event.shiftKey);
      return;
    }
    if (!morph) return;
    if (opening) opening.style.viewTransitionName = '';
    opening = event.currentTarget.closest('article');
    if (opening) opening.style.viewTransitionName = 'node-window';
  }

  // Readings with details sit above the pane's link so they can show them; a click on one acts on the pane.
  function onPaneClick(event: MouseEvent<HTMLElement>) {
    if (event.defaultPrevented || !(event.target instanceof Element) || !event.target.closest('[data-pane-detail]')) return;
    if (swallowRef.current) {
      swallowRef.current = false;
      return;
    }
    if (toggles(event)) onToggle(node.id, event.shiftKey);
    else event.currentTarget.querySelector<HTMLAnchorElement>('h2 a')?.click();
  }

  function onPointerDown(event: PointerEvent<HTMLElement>) {
    swallowRef.current = false;
    if (event.pointerType !== 'touch') return;
    cancelPress();
    pressRef.current = { timer: window.setTimeout(longPress, LONG_PRESS_MS), x: event.clientX, y: event.clientY };
  }

  function onPointerMove(event: PointerEvent<HTMLElement>) {
    const start = pressRef.current;
    if (start && Math.hypot(event.clientX - start.x, event.clientY - start.y) > PRESS_SLOP) cancelPress();
  }

  function onContextMenu(event: MouseEvent<HTMLElement>) {
    // A touch held this long is a long press, wherever the browser opens its menu first.
    if (pressRef.current) longPress();
    if (swallowRef.current) event.preventDefault();
  }

  return (
    // The handlers only route pointer input on readings or from touch; keyboards reach the pane's link directly.
    <article
      aria-label={node.name}
      className={cn(
        'relative flex h-[var(--pane-height)] min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border font-[family-name:var(--font-data)] text-compact transition-colors duration-[var(--dur-short)] ease-out pointer-coarse:select-none has-[a:focus-visible]:ring-2 has-[a:focus-visible]:ring-ring has-[a:focus-visible]:ring-inset',
        selected ? 'border-primary/50 bg-accent' : 'bg-[var(--color-surface-translucent)] hover:border-[var(--color-border-strong)]',
      )}
      onClick={onPaneClick}
      onContextMenu={onContextMenu}
      onPointerCancel={cancelPress}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={cancelPress}
    >
      <header className={cn('flex h-8 flex-none items-center gap-2 border-b px-3', selected && 'border-primary/25')}>
        <HealthDot item={item} now={now} />
        <h2 className='min-w-0 truncate font-[family-name:var(--font-body)] text-sm font-medium' title={node.name}>
          <Link
            aria-checked={selecting ? selected : undefined}
            className='outline-none [-webkit-touch-callout:none] after:absolute after:inset-0 after:rounded-[inherit]'
            onClick={onLinkClick}
            onKeyDown={(event) => {
              if (event.key !== ' ') return;
              event.preventDefault();
              onToggle(node.id, event.shiftKey);
            }}
            role={selecting ? 'checkbox' : undefined}
            to={nodePath(node.id)}
            viewTransition={morph && !selecting}
          >
            {node.name}
          </Link>
        </h2>
        {/* The host takes the room the name and the end leave, so it gives way first; its details add the tags. */}
        <Detail
          className='min-w-0 flex-1 basis-0 truncate text-xs text-muted-foreground'
          content={[host, node.tags.length > 0 ? t('fleet.pane.tags', { tags: node.tags.join(', ') }) : ''].filter(Boolean).join(' · ') || null}
        >
          {host || t('fleet.pane.noHost')}
        </Detail>
        <TitleEnd item={item} now={now} release={release} />
      </header>
      <Readings item={item} now={now} />
    </article>
  );
});
NodePane.displayName = 'NodePane';
