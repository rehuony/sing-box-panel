import type { ComponentType, ReactNode } from 'react';

import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useId, useState } from 'react';
import { useShallow } from 'zustand/react/shallow';
import { Badge } from '@shared/components/ui/badge';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { toast } from '@shared/components/ui/toast-manager';
import { buttonVariants } from '@shared/components/ui/button-variants';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@shared/components/ui/tooltip';
import {
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  CircleCheck,
  Clock3,
  Ellipsis,
  Play,
  RotateCw,
  Square,
  TriangleAlert,
} from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@shared/components/ui/dropdown-menu';
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

import { NodeStateBadge } from '@/components/node-state-badge';
import { ShellNavigationTrigger } from '@/components/app-shell';
import { EM_DASH, formatRate, formatUptime } from '@/utils/telemetry-format';
import { formatDateTime, formatRelativeTime } from '@/utils/format-relative-time';

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

interface ContextValueProps {
  label: string;
  tone?: string;
  value: string;
  title?: string;
  icon: ComponentType<{ 'aria-hidden'?: boolean; 'className'?: string }>;
}

/** One labelled reading in the object row; its label lives in the tooltip and accessible name. */
function ContextValue({ icon: Icon, label, title = '', tone = 'text-muted-foreground', value }: ContextValueProps) {
  const accessibleLabel = title === '' || title === value ? `${label}: ${value}` : `${label}: ${value}. ${title}`;
  return (
    <Tooltip>
      <TooltipTrigger
        render={(
          <div
            aria-label={accessibleLabel}
            className='flex h-[var(--control-height-xs)] shrink-0 items-center gap-1.5 rounded-[var(--radius-control)] px-1 outline-none'
            role='group'
            tabIndex={0}
          />
        )}
      >
        <Icon aria-hidden={true} className={`size-3.5 ${tone}`} />
        <span aria-hidden='true' className='font-mono text-xs whitespace-nowrap text-foreground tabular-nums'>{value}</span>
      </TooltipTrigger>
      <TooltipContent>{accessibleLabel}</TooltipContent>
    </Tooltip>
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
  const Icon = restarting ? RotateCw : Square;
  const label = t(`telemetry.control.${action}`);

  return (
    <AlertDialog onOpenChange={setOpen} open={open}>
      <Tooltip>
        <TooltipTrigger
          render={(
            <Button
              aria-label={label}
              onClick={() => setOpen(true)}
              size='icon-sm'
              variant='ghost'
            />
          )}
        >
          <Icon aria-hidden='true' />
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
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

interface RuntimeMenuProps {
  canStop: boolean;
  canStart: boolean;
  canRestart: boolean;
  onAction: (action: RuntimeAction) => void;
}

/** The runtime actions in one menu where the row is too narrow for separate buttons. */
function RuntimeMenu({ canRestart, canStart, canStop, onAction }: RuntimeMenuProps) {
  const { t } = useTranslation();
  const [confirmation, setConfirmation] = useState<Extract<RuntimeAction, 'restart' | 'stop'> | null>(null);

  if (!canStart && !canStop && !canRestart) return null;

  return (
    <div className='md:hidden'>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={(
            <Button aria-label={t('telemetry.moreActions')} size='icon-sm' variant='ghost' />
          )}
        >
          <Ellipsis aria-hidden='true' />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' sideOffset={8}>
          <DropdownMenuGroup aria-label={t('telemetry.control.label')}>
            {canStart
              ? (
                  <DropdownMenuItem onClick={() => onAction('start')}>
                    <Play aria-hidden='true' />
                    {t('telemetry.control.start')}
                  </DropdownMenuItem>
                )
              : null}
            {canStop
              ? (
                  <DropdownMenuItem onClick={() => setConfirmation('stop')} variant='destructive'>
                    <Square aria-hidden='true' />
                    {t('telemetry.control.stop')}
                  </DropdownMenuItem>
                )
              : null}
            {canRestart
              ? (
                  <DropdownMenuItem onClick={() => setConfirmation('restart')}>
                    <RotateCw aria-hidden='true' />
                    {t('telemetry.control.restart')}
                  </DropdownMenuItem>
                )
              : null}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>

      <AlertDialog
        onOpenChange={(open) => {
          if (!open) setConfirmation(null);
        }}
        open={confirmation !== null}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmation === null ? '' : t(`telemetry.confirm.${confirmation}.title`)}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmation === null ? '' : t(`telemetry.confirm.${confirmation}.description`)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('telemetry.confirm.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (confirmation !== null) onAction(confirmation);
                setConfirmation(null);
              }}
              variant={confirmation === 'stop' ? 'destructive' : 'default'}
            >
              {confirmation === null ? '' : t(`telemetry.confirm.${confirmation}.action`)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
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

interface ObjectRowLayoutProps {
  node: FleetNode;
  context: ReactNode;
  actions?: ReactNode;
}

/** Back, the node's name and state, its context values, and actions on this node only. */
function ObjectRowLayout({ actions, context, node }: ObjectRowLayoutProps) {
  const { t } = useTranslation();
  return (
    <header
      aria-label={t('node.objectRow', { name: node.name })}
      className='flex min-h-[var(--shell-header-height)] flex-col justify-center gap-1 rounded-[var(--radius-card)] border bg-card px-2 py-2 sm:px-3 lg:flex-row lg:items-center lg:gap-3'
    >
      <div className='flex min-w-0 items-center gap-2 lg:contents'>
        <ShellNavigationTrigger />
        <Tooltip>
          <TooltipTrigger
            render={<Link aria-label={t('node.back')} className={buttonVariants({ size: 'icon-sm', variant: 'ghost' })} to='/nodes' />}
          >
            <ArrowLeft aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('node.back')}</TooltipContent>
        </Tooltip>
        <div className='flex min-w-0 flex-1 items-center gap-2 lg:flex-none'>
          <span className='truncate text-xl font-semibold' title={node.name}>{node.name}</span>
          <NodeStateBadge state={node.state} />
        </div>
        <div className='ml-auto flex shrink-0 items-center gap-1 lg:order-last' data-slot='runtime-controls'>
          {actions}
        </div>
      </div>
      <div className='flex min-w-0 items-center gap-2 overflow-x-auto [scrollbar-width:none] lg:flex-1'>
        {context}
      </div>
    </header>
  );
}

/** The live object row of a connected node, with its runtime readings and controls. */
function LiveObjectRow({ node }: { node: FleetNode }) {
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
  const trafficAvailable = telemetry.snapshot?.live_sample?.accepted === true && !telemetry.liveStale;
  const uploadRate = verifiedStopped ? 0 : trafficAvailable ? telemetry.rates.uploadBytesPerSecond : null;
  const downloadRate = verifiedStopped ? 0 : trafficAvailable ? telemetry.rates.downloadBytesPerSecond : null;
  const uptime = formatUptime(runningIdentity?.started_at, now, {
    day: t('telemetry.unit.day'),
    hour: t('telemetry.unit.hour'),
    minute: t('telemetry.unit.minute'),
    second: t('telemetry.unit.second'),
  });
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
  const runtimeTone = verifiedRunning ? 'text-[var(--color-success)]' : 'text-[var(--color-text-faint)]';

  function runRuntimeAction(nextAction: RuntimeAction) {
    void runtimeControl.run(nextAction, runtimeStatus);
  }

  const context = (
    <>
      <span
        className='flex h-[var(--control-height-xs)] shrink-0 items-center gap-1.5 px-1 text-xs font-medium whitespace-nowrap'
        title={runtimeDetail}
      >
        <span aria-hidden='true' className={`size-2 rounded-full bg-current ${runtimeTone}`} />
        {runtimeLabel}
      </span>
      <span className='shrink-0 truncate px-1 font-mono text-xs text-muted-foreground' title={`${t('telemetry.metric.version')}: ${enabledVersion ?? EM_DASH}`}>
        {versionLabel(enabledVersion)}
      </span>
      <ContextValue icon={Clock3} label={t('telemetry.metric.uptime')} title={uptimeTitle} value={uptime} />
      <ContextValue icon={ArrowUp} label={t('telemetry.metric.upload')} tone='text-chart-2' value={formatRate(uploadRate, locale)} />
      <ContextValue icon={ArrowDown} label={t('telemetry.metric.download')} tone='text-primary' value={formatRate(downloadRate, locale)} />
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
        <>
          <div aria-label={t('telemetry.control.label')} className='hidden items-center gap-1 md:flex' role='group'>
            {verifiedStopped
              ? (
                  <Tooltip>
                    <TooltipTrigger
                      render={(
                        <Button
                          aria-label={t('telemetry.control.start')}
                          disabled={!enabledVersion}
                          onClick={() => runRuntimeAction('start')}
                          size='icon-sm'
                          variant='ghost'
                        />
                      )}
                    >
                      <Play aria-hidden='true' />
                    </TooltipTrigger>
                    <TooltipContent>{t('telemetry.control.start')}</TooltipContent>
                  </Tooltip>
                )
              : verifiedRunning
                ? (
                    <>
                      <RuntimeConfirmation action='stop' onConfirm={() => runRuntimeAction('stop')} />
                      <RuntimeConfirmation action='restart' onConfirm={() => runRuntimeAction('restart')} />
                    </>
                  )
                : null}
          </div>
          <RuntimeMenu
            canRestart={verifiedRunning}
            canStart={verifiedStopped && Boolean(enabledVersion)}
            canStop={verifiedRunning}
            onAction={runRuntimeAction}
          />
        </>
      );

  return <ObjectRowLayout actions={actions} context={context} node={node} />;
}

/** The object row of a node that is not connected: what it last reported, marked stale. */
function ReportedObjectRow({ node }: { node: FleetNode }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const now = useNow(60_000);
  const version = node.runtime?.enabled_core?.exact_core_version;
  const context = node.state === 'pending'
    ? <span className='px-1 text-xs text-muted-foreground'>{t('node.awaitingEnrollment')}</span>
    : (
        <>
          <span className='shrink-0 px-1 font-mono text-xs text-muted-foreground' title={`${t('telemetry.metric.version')}: ${version ?? EM_DASH}`}>
            {versionLabel(version)}
          </span>
          <span className='shrink-0 px-1 text-xs whitespace-nowrap text-[var(--color-warning-text)]' title={formatDateTime(node.last_seen_at, locale)}>
            {t('node.lastSeen', { time: formatRelativeTime(node.last_seen_at, locale, now) })}
          </span>
        </>
      );
  return <ObjectRowLayout context={context} node={node} />;
}

export function NodeObjectRow({ node }: { node: FleetNode }) {
  return node.state === 'online' ? <LiveObjectRow node={node} /> : <ReportedObjectRow node={node} />;
}
