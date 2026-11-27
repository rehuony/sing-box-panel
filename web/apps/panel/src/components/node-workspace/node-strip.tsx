import type { ReactNode } from 'react';
import type { TrafficDirection } from '@shared/lib/telemetry-format';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { useEffect, useId, useState } from 'react';
import { useShallow } from 'zustand/react/shallow';
import { Badge } from '@shared/components/ui/badge';
import { Spinner } from '@shared/components/ui/spinner';
import { ReadoutParts } from '@shared/components/terminal';
import { toast } from '@shared/components/ui/toast-manager';
import { formatDateTime, formatRelativeTime } from '@shared/lib/format-time';
import { CircleCheck, Play, RotateCw, Square, TriangleAlert } from 'lucide-react';
import { formatRate, formatUptime, trafficDirectionText, trafficValueText } from '@shared/lib/telemetry-format';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@shared/components/ui/alert-dialog';

import type { FleetNode, MetricsSnapshot } from '@/api/api-client';

import { useShellScope } from '@/components/status-line';
import { NodeStateBadge } from '@/components/node-state-badge';
import { StripAction, StripDivider, TitleStrip } from '@/components/title-strip';

import type { RuntimeAction } from './use-runtime-control';

import { useSharedTelemetry } from './telemetry-context';
import { useRuntimeControl } from './use-runtime-control';

function useNow(interval: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), interval);
    return () => window.clearInterval(timer);
  }, [interval]);
  return now;
}

function versionLabel(version: string): string {
  return `v${version.replace(/^v/i, '')}`;
}

/**
 * How long a sample's traffic stands for the present. A node samples every
 * second while it is watched and every ten seconds otherwise, and derives no
 * rate over a longer gap.
 */
const SAMPLE_LIFETIME_MS = 15_000;

interface Traffic {
  connections: number;
  upload: number | null;
  download: number | null;
}

const idleTraffic: Traffic = { connections: 0, download: 0, upload: 0 };

function rateOf(value: number | null): number | null {
  return value !== null && Number.isFinite(value) && value >= 0 ? value : null;
}

/**
 * The newest traffic a snapshot holds, or null when it holds none that still
 * stands. It lasts until the next sample replaces it, so the strip does not
 * blink between samples.
 */
function sampledTraffic(metrics: MetricsSnapshot | undefined): Traffic | null {
  const sample = metrics?.live_sample;
  if (metrics === undefined || !sample?.accepted) return null;
  const age = Date.parse(metrics.collected_at) - Date.parse(sample.sampled_at);
  if (!(age <= SAMPLE_LIFETIME_MS)) return null;
  return {
    connections: sample.active_connections,
    download: rateOf(sample.download_bytes_per_second),
    upload: rateOf(sample.upload_bytes_per_second),
  };
}

/**
 * One labelled reading, as a pane's row reads: its label, then its parts set
 * apart by dots; a part it lacks is left out. Stale values turn muted.
 */
function Reading({ children, label, stale = false }: { children: ReactNode[]; label: string; stale?: boolean }) {
  return (
    <dl className='flex shrink-0 items-center gap-2 px-1'>
      <dt className='text-muted-foreground'>{label}</dt>
      <dd className={cn('whitespace-nowrap tabular-nums', stale && 'text-muted-foreground')}><ReadoutParts>{children}</ReadoutParts></dd>
    </dl>
  );
}

/** A rate after its arrow, in a slot as wide as the longest rate, so the line stays still as it changes. */
/** One rate after its arrow in its direction's colour, which it takes too unless it is zero; not known, it reads 0. */
function Rate({ direction, label, value }: { direction: TrafficDirection; label: string; value: number | null }) {
  const { i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  return (
    <span>
      <span aria-hidden='true' className={trafficDirectionText[direction]}>{direction === 'download' ? '↓ ' : '↑ '}</span>
      <span className='sr-only'>{label}</span>
      <span className={cn('inline-block min-w-[9ch] text-end', trafficValueText(direction, value))}>{formatRate(value, locale)}</span>
    </span>
  );
}

/** A runtime action in the strip's flat form; on a phone it keeps only its icon. */
function RuntimeButton({ action, disabled, onClick }: {
  action: RuntimeAction;
  disabled?: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const Icon = action === 'start' ? Play : action === 'stop' ? Square : RotateCw;
  return (
    <StripAction disabled={disabled} icon={<Icon aria-hidden='true' data-icon='inline-start' />} label={t(`telemetry.control.${action}`)} onClick={onClick} />
  );
}

interface RuntimeConfirmationProps {
  onConfirm: () => void;
  action: Extract<RuntimeAction, 'restart' | 'stop'>;
}

function RuntimeConfirmation({ action, onConfirm }: RuntimeConfirmationProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const restarting = action === 'restart';

  return (
    <AlertDialog onOpenChange={setOpen} open={open}>
      <RuntimeButton action={action} onClick={() => setOpen(true)} />
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t(`telemetry.confirm.${action}.title`)}</AlertDialogTitle>
          <AlertDialogDescription>
            {t(`telemetry.confirm.${action}.description`)}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('telemetry.confirm.cancel')}</AlertDialogCancel>
          <AlertDialogAction
            onClick={() => {
              setOpen(false);
              onConfirm();
            }}
            variant={restarting ? 'default' : 'destructive'}
          >
            {t(`telemetry.confirm.${action}.action`)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function StreamStatus({ reconnecting }: { reconnecting: boolean }) {
  const { t } = useTranslation();
  const id = useId();
  useEffect(() => {
    if (!reconnecting) return;
    let cancelled = false;
    queueMicrotask(() => {
      if (!cancelled) toast.add({ id, title: t('telemetry.stream.reconnecting'), description: t('telemetry.stream.detail'), type: 'warning', timeout: 0 });
    });
    return () => {
      cancelled = true;
      toast.close(id);
    };
  }, [reconnecting, id, t]);
  return null;
}

interface StripLayoutProps {
  node: FleetNode;
  /** The readings; none leaves the strip without their group. */
  context: ReactNode;
  actions?: ReactNode;
}

/**
 * The node scope's title strip: the node's name and state, its labelled
 * readings in the data face, and actions on this node only at the end, with
 * dividers between the groups. The status line leads back to the fleet and to
 * the neighbouring nodes. Below 1024px the readings take a second line that
 * scrolls sideways.
 */
function StripLayout({ actions, context, node }: StripLayoutProps) {
  const { t } = useTranslation();
  return (
    <TitleStrip className='[view-transition-name:node-window]' label={t('node.objectRow', { name: node.name })}>
      <div className='flex min-w-0 flex-1 items-center gap-2 lg:flex-none'>
        <span className='truncate font-[family-name:var(--font-display)] text-xl font-semibold' title={node.name}>{node.name}</span>
        <NodeStateBadge state={node.state} />
      </div>
      {context === null
        ? null
        : (
            <>
              <StripDivider className='max-lg:hidden' />
              <div className='flex min-w-0 items-center gap-2 overflow-x-auto font-[family-name:var(--font-data)] text-compact whitespace-nowrap [scrollbar-width:none] max-lg:order-last max-lg:basis-full lg:flex-1'>
                {context}
              </div>
            </>
          )}
      <div className='ml-auto flex shrink-0 items-center gap-1' data-slot='runtime-controls'>
        {actions}
      </div>
    </TitleStrip>
  );
}

/**
 * The live strip of a connected node, with its runtime readings and controls.
 * Until the node's own stream answers it reads what the fleet last heard from
 * the node, so a window opens, and a switch to the next node lands, with its
 * readings and controls already in place.
 */
function LiveStrip({ node }: { node: FleetNode }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const telemetry = useSharedTelemetry(useShallow(s => ({
    runtimeStatus: s.runtimeStatus, snapshot: s.snapshot,
    liveStale: s.liveStale, trafficError: s.trafficError, acceptRuntimeStatus: s.acceptRuntimeStatus,
  })));
  const reported = useShellScope().items.find(item => item.node.id === node.id);
  const reconnecting = telemetry.trafficError !== null || (telemetry.liveStale && telemetry.snapshot !== null);
  const runtimeControl = useRuntimeControl({ onRuntimeStatus: telemetry.acceptRuntimeStatus });
  const now = useNow(30_000);

  const runtimeStatus = telemetry.runtimeStatus ?? node.runtime ?? null;
  const verifiedRunning = runtimeStatus?.observation_state === 'running'
    && runtimeStatus.running !== undefined;
  const verifiedStopped = runtimeStatus?.observation_state === 'stopped';
  // A node with no core set has nothing to stop; it says so rather than stopped.
  const notEnabled = verifiedStopped && !runtimeStatus.enabled_core;
  let runtimeLabel = t('fleet.pane.unknown');
  if (verifiedRunning) runtimeLabel = t('telemetry.runtime.running');
  else if (notEnabled) runtimeLabel = t('fleet.pane.noCore');
  else if (verifiedStopped) runtimeLabel = t('telemetry.runtime.stopped');
  const runtimeDetail = verifiedRunning
    ? t('telemetry.runtime.runningDetail')
    : verifiedStopped
      ? t(notEnabled ? 'telemetry.runtime.notEnabledDetail' : 'telemetry.runtime.stoppedDetail')
      : runtimeStatus?.observation_state === 'stale'
        ? t('telemetry.runtime.stale')
        : runtimeStatus?.observation_state === 'inspection_unavailable'
          ? t('telemetry.runtime.inspectionUnavailable')
          : t('telemetry.runtime.evidenceUnavailable');
  const runningIdentity = verifiedRunning ? runtimeStatus.running : undefined;
  const enabledVersion = runtimeStatus?.enabled_core?.exact_core_version;
  // The version that runs, as a pane reads it, or the one a start would run.
  const coreVersion = runningIdentity?.exact_core_version ?? enabledVersion;
  // A stopped core moves nothing; otherwise the newest sample, until the next one replaces it.
  const traffic = verifiedStopped ? idleTraffic : sampledTraffic(telemetry.snapshot ?? reported?.metrics);
  const startedAt = runningIdentity?.started_at;
  const action = runtimeControl.state.action;
  const actionLabel = action === null ? '' : t(`telemetry.control.${action}`);
  const actionMessage = action === null || runtimeControl.state.phase === 'idle'
    ? ''
    : t(`telemetry.action.${runtimeControl.state.phase === 'verification_timeout' ? 'timedOut' : runtimeControl.state.phase}`, { action: actionLabel });
  const actionVariant = runtimeControl.state.phase === 'verified'
    ? 'success'
    : runtimeControl.state.phase === 'failed'
      ? 'destructive'
      : runtimeControl.state.phase === 'verification_timeout' ? 'warning' : 'info';
  const units = { day: t('telemetry.unit.day'), hour: t('telemetry.unit.hour'), minute: t('telemetry.unit.minute'), second: t('telemetry.unit.second') };
  let runtimeTone = 'text-[var(--color-warning-text)]';
  if (verifiedRunning) runtimeTone = '';
  else if (verifiedStopped) runtimeTone = 'text-[var(--color-text-secondary)]';

  function runRuntimeAction(nextAction: RuntimeAction) {
    void runtimeControl.run(nextAction, runtimeStatus);
  }

  // Readings the node has not given are left out rather than marked; traffic leaves with its group.
  const context = (
    <>
      <Reading label={t('fleet.pane.core')}>
        {[
          <span className={runtimeTone} key='state' title={runtimeDetail}>{runtimeLabel}</span>,
          coreVersion ? <span key='version' title={t('telemetry.metric.version')}>{versionLabel(coreVersion)}</span> : null,
          startedAt
            ? (
                <span key='uptime' title={t('telemetry.startedAt', { value: formatDateTime(startedAt, locale) })}>
                  {t('fleet.pane.uptime', { value: formatUptime(startedAt, now, units) })}
                </span>
              )
            : null,
        ]}
      </Reading>
      {traffic === null
        ? null
        : (
            <>
              <StripDivider />
              <Reading label={t('fleet.pane.net')} stale={reconnecting}>
                {[
                  <Rate direction='download' key='download' label={t('telemetry.metric.download')} value={traffic.download} />,
                  <Rate direction='upload' key='upload' label={t('telemetry.metric.upload')} value={traffic.upload} />,
                  <span key='connections'>{t('fleet.pane.connections', { count: traffic.connections })}</span>,
                ]}
              </Reading>
            </>
          )}
      <StreamStatus reconnecting={reconnecting} />
    </>
  );

  const actions = actionMessage !== ''
    ? (
        <Badge className='max-w-72 min-w-0' role='status' title={actionMessage} variant={actionVariant}>
          {runtimeControl.busy
            ? <Spinner data-icon='inline-start' />
            : runtimeControl.state.phase === 'verified'
              ? <CircleCheck aria-hidden='true' data-icon='inline-start' />
              : runtimeControl.state.phase === 'failed'
                || runtimeControl.state.phase === 'verification_timeout'
                ? <TriangleAlert aria-hidden='true' data-icon='inline-start' />
                : null}
          <span className='truncate'>{actionMessage}</span>
        </Badge>
      )
    : (
        <div aria-label={t('telemetry.control.label')} className='flex items-center gap-1' role='group'>
          {verifiedStopped ? <RuntimeButton action='start' disabled={!enabledVersion} onClick={() => runRuntimeAction('start')} /> : null}
          {verifiedRunning
            ? (
                <>
                  <RuntimeConfirmation action='stop' onConfirm={() => runRuntimeAction('stop')} />
                  <RuntimeConfirmation action='restart' onConfirm={() => runRuntimeAction('restart')} />
                </>
              )
            : null}
        </div>
      );

  return <StripLayout actions={actions} context={context} node={node} />;
}

/**
 * The strip of a node that is not connected: when it was last seen, in
 * warning, then the core it last reported; what it never reported is left out.
 */
function ReportedStrip({ node }: { node: FleetNode }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const now = useNow(60_000);
  const seen = node.last_seen_at;
  const version = node.runtime?.enabled_core?.exact_core_version;
  const context = seen || version
    ? (
        <>
          {seen
            ? (
                <span className='px-1 text-[var(--color-warning-text)]' title={formatDateTime(seen, locale)}>
                  {t('node.lastSeen', { time: formatRelativeTime(seen, locale, now) })}
                </span>
              )
            : null}
          {seen && version ? <StripDivider /> : null}
          {version
            ? (
                <Reading label={t('fleet.pane.core')}>
                  {[<span key='version' title={t('telemetry.metric.version')}>{versionLabel(version)}</span>]}
                </Reading>
              )
            : null}
        </>
      )
    : null;
  return <StripLayout context={context} node={node} />;
}

/** The node's title strip, live while the node is connected and as last reported otherwise. */
export function NodeStrip({ node }: { node: FleetNode }) {
  return node.state === 'online' ? <LiveStrip node={node} /> : <ReportedStrip node={node} />;
}
