import type { ReactNode } from 'react';

import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useId, useState } from 'react';
import { useShallow } from 'zustand/react/shallow';
import { Badge } from '@shared/components/ui/badge';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { ReadoutParts } from '@shared/components/terminal';
import { toast } from '@shared/components/ui/toast-manager';
import { buttonVariants } from '@shared/components/ui/button-variants';
import { formatDateTime, formatRelativeTime } from '@shared/lib/format-time';
import { EM_DASH, formatRate, formatUptime } from '@shared/lib/telemetry-format';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import { ChevronLeft, ChevronRight, CircleCheck, Play, RotateCw, Square, TriangleAlert } from 'lucide-react';
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

import type { FleetNode } from '@/api/api-client';

import { nodeWindowPath } from '@/utils/shell-windows';
import { useShellScope } from '@/components/status-line';
import { NodeStateBadge } from '@/components/node-state-badge';
import { StripDivider, TitleStrip } from '@/components/title-strip';

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

function versionLabel(version: string | undefined): string {
  return version ? `v${version.replace(/^v/i, '')}` : EM_DASH;
}

/** One labelled reading, as a pane's row reads: its label, then its parts set apart by dots. */
function Reading({ children, label }: { children: ReactNode[]; label: string }) {
  return (
    <dl className='flex shrink-0 items-center gap-2 px-1'>
      <dt className='text-muted-foreground'>{label}</dt>
      <dd className='whitespace-nowrap tabular-nums'><ReadoutParts>{children}</ReadoutParts></dd>
    </dl>
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
    <Button className='max-sm:w-(--control-height-sm) max-sm:px-0' disabled={disabled} onClick={onClick} size='sm' variant='flat'>
      <Icon aria-hidden='true' data-icon='inline-start' />
      <span className='max-sm:sr-only'>{t(`telemetry.control.${action}`)}</span>
    </Button>
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
  context: ReactNode;
  actions?: ReactNode;
}

/**
 * The node scope's title strip: the node's name and state, its labelled
 * readings in the data face, and actions on this node only at the end, with
 * dividers between the groups. Below 1024px the readings take a second line
 * that scrolls sideways.
 */
function StripLayout({ actions, context, node }: StripLayoutProps) {
  const { t } = useTranslation();
  return (
    <TitleStrip className='[view-transition-name:node-window]' label={t('node.objectRow', { name: node.name })}>
      <div className='flex min-w-0 flex-1 items-center gap-2 lg:flex-none'>
        <Neighbours nodeId={node.id} />
        <span className='truncate text-sm font-semibold' title={node.name}>{node.name}</span>
        <NodeStateBadge state={node.state} />
      </div>
      <StripDivider className='max-lg:hidden' />
      <div className='flex min-w-0 items-center gap-2 overflow-x-auto font-[family-name:var(--font-data)] text-compact whitespace-nowrap [scrollbar-width:none] max-lg:order-last max-lg:basis-full lg:flex-1'>
        {context}
      </div>
      <div className='ml-auto flex shrink-0 items-center gap-1' data-slot='runtime-controls'>
        {actions}
      </div>
    </TitleStrip>
  );
}

/**
 * The previous and next nodes in the fleet's name order, wrapping around and
 * keeping the current window. They lead the strip so they stay under the
 * pointer while names of other lengths come and go.
 */
function Neighbours({ nodeId }: { nodeId: string }) {
  const { t } = useTranslation();
  const { items, scope } = useShellScope();
  const index = items.findIndex(item => item.node.id === nodeId);
  if (index < 0 || items.length < 2) return null;
  const window = scope.kind === 'node' ? scope.window ?? 'overview' : 'overview';
  return (
    <div className='flex flex-none items-center gap-0.5 max-md:hidden'>
      {(['previous', 'next'] as const).map((direction) => {
        const target = items[(index + (direction === 'previous' ? -1 : 1) + items.length) % items.length]!;
        const label = t(`shell.bar.${direction}`, { name: target.node.name });
        const Icon = direction === 'previous' ? ChevronLeft : ChevronRight;
        return (
          <Tooltip key={direction}>
            <TooltipTrigger
              render={<Link aria-label={label} className={buttonVariants({ size: 'icon-sm', variant: 'flat' })} to={nodeWindowPath(target.node.id, window)} />}
            >
              <Icon aria-hidden='true' />
            </TooltipTrigger>
            <TooltipContent>{label}</TooltipContent>
          </Tooltip>
        );
      })}
    </div>
  );
}

/** The live strip of a connected node, with its runtime readings and controls. */
function LiveStrip({ node }: { node: FleetNode }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const telemetry = useSharedTelemetry(useShallow(s => ({
    runtimeStatus: s.runtimeStatus, snapshot: s.snapshot, rates: s.rates,
    liveStale: s.liveStale, trafficError: s.trafficError, acceptRuntimeStatus: s.acceptRuntimeStatus,
  })));
  const reconnecting = telemetry.trafficError !== null || (telemetry.liveStale && telemetry.snapshot !== null);
  const runtimeControl = useRuntimeControl({ onRuntimeStatus: telemetry.acceptRuntimeStatus });
  const now = useNow(30_000);

  const runtimeStatus = telemetry.runtimeStatus;
  const verifiedRunning = runtimeStatus?.observation_state === 'running'
    && runtimeStatus.running !== undefined;
  const verifiedStopped = runtimeStatus?.observation_state === 'stopped';
  const runtimeLabel = verifiedRunning
    ? t('telemetry.runtime.running')
    : verifiedStopped
      ? t('telemetry.runtime.stopped')
      : t('telemetry.runtime.unknown');
  const runtimeDetail = verifiedRunning
    ? t('telemetry.runtime.runningDetail')
    : verifiedStopped
      ? t('telemetry.runtime.stoppedDetail')
      : runtimeStatus?.observation_state === 'stale'
        ? t('telemetry.runtime.stale')
        : runtimeStatus?.observation_state === 'inspection_unavailable'
          ? t('telemetry.runtime.inspectionUnavailable')
          : t('telemetry.runtime.evidenceUnavailable');
  const runningIdentity = verifiedRunning ? runtimeStatus.running : undefined;
  const enabledVersion = runtimeStatus?.enabled_core?.exact_core_version;
  // The version that runs, as a pane reads it, or the one a start would run.
  const coreVersion = runningIdentity?.exact_core_version ?? enabledVersion;
  const trafficAvailable = telemetry.snapshot?.live_sample?.accepted === true && !telemetry.liveStale;
  const uploadRate = verifiedStopped ? 0 : trafficAvailable ? telemetry.rates.uploadBytesPerSecond : null;
  const downloadRate = verifiedStopped ? 0 : trafficAvailable ? telemetry.rates.downloadBytesPerSecond : null;
  const liveConnections = telemetry.snapshot?.live_sample?.active_connections ?? null;
  const connections = verifiedStopped ? 0 : trafficAvailable ? liveConnections : null;
  const startedAt = formatDateTime(runningIdentity?.started_at, locale);
  const uptimeTitle = verifiedStopped || startedAt === ''
    ? runtimeDetail
    : t('telemetry.startedAt', { value: startedAt });
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

  const context = (
    <>
      <Reading label={t('fleet.pane.core')}>
        {[
          <span className={runtimeTone} key='state' title={runtimeDetail}>{runtimeLabel}</span>,
          <span key='version' title={`${t('telemetry.metric.version')}: ${coreVersion ?? EM_DASH}`}>{versionLabel(coreVersion)}</span>,
          verifiedRunning ? <span key='uptime' title={uptimeTitle}>{t('fleet.pane.uptime', { value: formatUptime(runningIdentity?.started_at, now, units) })}</span> : null,
        ]}
      </Reading>
      <StripDivider />
      <Reading label={t('fleet.pane.net')}>
        {[
          <span key='download'>
            <span aria-hidden='true' className='text-[var(--color-text-secondary)]'>↓ </span>
            <span className='sr-only'>{t('telemetry.metric.download')}</span>
            <span>{formatRate(downloadRate, locale)}</span>
          </span>,
          <span key='upload'>
            <span aria-hidden='true' className='text-[var(--color-text-secondary)]'>↑ </span>
            <span className='sr-only'>{t('telemetry.metric.upload')}</span>
            <span>{formatRate(uploadRate, locale)}</span>
          </span>,
          connections === null
            ? <span key='connections' title={t('telemetry.metric.connections')}>{EM_DASH}</span>
            : <span key='connections'>{t('fleet.pane.connections', { count: connections })}</span>,
        ]}
      </Reading>
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

/** The strip of a node that is not connected: what it last reported, marked stale. */
function ReportedStrip({ node }: { node: FleetNode }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const now = useNow(60_000);
  const version = node.runtime?.enabled_core?.exact_core_version;
  const context = (
    <Reading label={t('fleet.pane.core')}>
      {[
        <span key='version' title={`${t('telemetry.metric.version')}: ${version ?? EM_DASH}`}>{versionLabel(version)}</span>,
        <span className='text-[var(--color-warning-text)]' key='seen' title={formatDateTime(node.last_seen_at, locale)}>
          {t('node.lastSeen', { time: formatRelativeTime(node.last_seen_at, locale, now) })}
        </span>,
      ]}
    </Reading>
  );
  return <StripLayout context={context} node={node} />;
}

/** The node's title strip, live while the node is connected and as last reported otherwise. */
export function NodeStrip({ node }: { node: FleetNode }) {
  return node.state === 'online' ? <LiveStrip node={node} /> : <ReportedStrip node={node} />;
}
