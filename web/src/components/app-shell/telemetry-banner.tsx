import type { ComponentType } from 'react';

import { useTranslation } from 'react-i18next';
import { useEffect, useId, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  CircleCheck,
  Clock3,
  Ellipsis,
  PanelLeft,
  Play,
  RotateCw,
  Square,
  TriangleAlert,
} from 'lucide-react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { Separator } from '@/components/ui/separator';
import { useSidebar } from '@/components/ui/sidebar-context';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';

import type { RuntimeAction } from './use-runtime-control';

import { useSharedTelemetry } from './telemetry-context';
import { useRuntimeControl } from './use-runtime-control';
import { EM_DASH, formatRate, formatUptime } from './telemetry-format';

interface TelemetryMetricProps {
  label: string;
  value: string;
  title?: string;
  compactValue?: string;
  icon: ComponentType<{ 'aria-hidden'?: boolean }>;
  id: 'download' | 'total' | 'upload' | 'uptime' | 'version';
}

function TelemetryMetric({
  id,
  icon: Icon,
  label,
  value,
  compactValue = value,
  title = value,
}: TelemetryMetricProps) {
  const accessibleLabel = title === value
    ? `${label}: ${value}`
    : `${label}: ${value}. ${title}`;

  return (
    <Tooltip>
      <TooltipTrigger
        render={(
          <div
            aria-label={accessibleLabel}
            className='telemetry-metric'
            data-metric={id}
            role='group'
            tabIndex={0}
          />
        )}
      >
        <Icon aria-hidden={true} />
        <strong aria-hidden='true'>
          <span className='telemetry-metric__full'>{value}</span>
          <span className='telemetry-metric__compact'>{compactValue}</span>
        </strong>
      </TooltipTrigger>
      <TooltipContent>{accessibleLabel}</TooltipContent>
    </Tooltip>
  );
}

interface RuntimeConfirmationProps {
  disabled: boolean;
  onConfirm: () => void;
  action: Extract<RuntimeAction, 'restart' | 'stop'>;
}

function RuntimeConfirmation({ action, disabled, onConfirm }: RuntimeConfirmationProps) {
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
              disabled={disabled}
              onClick={() => setOpen(true)}
              size='icon-sm'
              title={label}
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

interface MobileTelemetryMenuProps {
  canStop: boolean;
  canStart: boolean;
  canRestart: boolean;
  onAction: (action: RuntimeAction) => void;
}

function MobileTelemetryMenu({
  canRestart,
  canStart,
  canStop,
  onAction,
}: MobileTelemetryMenuProps) {
  const { t } = useTranslation();
  const [confirmation, setConfirmation] = useState<Extract<RuntimeAction, 'restart' | 'stop'> | null>(null);
  const hasControls = canStart || canStop || canRestart;

  if (!hasControls) return null;

  return (
    <div className='telemetry-mobile-actions'>
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
                  <DropdownMenuItem
                    onClick={() => setConfirmation('stop')}
                    variant='destructive'
                  >
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

function DashboardStreamStatus() {
  const { t } = useTranslation();
  const triggerId = useId();
  const contentId = useId();
  const [open, setOpen] = useState(false);

  return (
    <Tooltip open={open} onOpenChange={setOpen} triggerId={triggerId}>
      <TooltipTrigger
        id={triggerId}
        aria-label={t('telemetry.stream.detail')}
        aria-describedby={open ? contentId : undefined}
        closeOnClick={false}
        onClick={() => setOpen(true)}
        render={<Badge className='telemetry-stream-status' variant='warning' render={<button type='button' />} />}
      >
        <Spinner aria-hidden='true' data-icon='inline-start' />
        <span className='telemetry-stream-status__full' aria-hidden='true'>{t('telemetry.stream.reconnecting')}</span>
        <span className='telemetry-stream-status__compact' aria-hidden='true'>{t('telemetry.stream.compact')}</span>
      </TooltipTrigger>
      <TooltipContent id={contentId} role='tooltip' side='bottom'>{t('telemetry.stream.detail')}</TooltipContent>
    </Tooltip>
  );
}

export function TelemetryBanner() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const { setOpenMobile } = useSidebar();
  const telemetry = useSharedTelemetry();
  const reconnecting = telemetry.dashboardStale && telemetry.dashboardSnapshot !== null;
  const runtimeControl = useRuntimeControl({ onRuntimeStatus: telemetry.acceptRuntimeStatus });
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(timer);
  }, []);

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
  const trafficAvailable = telemetry.snapshot?.available === true;
  const uploadRate = verifiedStopped ? 0 : trafficAvailable ? telemetry.rates.uploadBytesPerSecond : null;
  const downloadRate = verifiedStopped ? 0 : trafficAvailable ? telemetry.rates.downloadBytesPerSecond : null;
  const durationLabels = {
    day: t('telemetry.unit.day'),
    hour: t('telemetry.unit.hour'),
    minute: t('telemetry.unit.minute'),
    second: t('telemetry.unit.second'),
  };
  const uptime = verifiedStopped ? `0${durationLabels.second}` : formatUptime(runningIdentity?.started_at, now, durationLabels);
  const compactUptime = verifiedStopped
    ? '0s'
    : formatUptime(runningIdentity?.started_at, now, {
      day: 'd', hour: 'h', minute: 'm', second: 's',
    }).split(' ')[0];
  const parsedStartedAt = runningIdentity?.started_at === undefined
    ? Number.NaN
    : new Date(runningIdentity.started_at).getTime();
  const startedAtTitle = verifiedStopped
    ? runtimeDetail
    : Number.isFinite(parsedStartedAt)
      ? t('telemetry.startedAt', {
          value: new Intl.DateTimeFormat(locale, {
            dateStyle: 'medium',
            timeStyle: 'medium',
          }).format(new Date(parsedStartedAt)),
        })
      : EM_DASH;
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

  function runRuntimeAction(nextAction: RuntimeAction) {
    void runtimeControl.run(nextAction, runtimeStatus);
  }

  return (
    <header className='telemetry-banner' aria-label={t('telemetry.ariaLabel')}>
      <div className='telemetry-banner__identity'>
        <Button
          aria-label={t('telemetry.toggleNavigation')}
          className='telemetry-banner__navigation-trigger'
          onClick={() => setOpenMobile(true)}
          size='icon-sm'
          variant='ghost'
        >
          <PanelLeft aria-hidden='true' />
        </Button>
        <div className='telemetry-runtime'>
          <Badge className={import.meta.env.MODE === 'demo' ? 'telemetry-demo-badge' : undefined} title={runtimeDetail} variant={import.meta.env.MODE === 'demo' ? 'secondary' : verifiedRunning ? 'success' : runtimeControl.state.phase === 'failed' ? 'destructive' : 'secondary'}>
            <span aria-hidden='true' className='telemetry-runtime__dot' />
            {import.meta.env.MODE === 'demo' ? t('telemetry.demo') : runtimeLabel}
          </Badge>
          <Badge className='telemetry-version' title={`${t('telemetry.metric.version')}: ${enabledVersion ?? EM_DASH}`} variant='secondary'>
            <span className='telemetry-version__label'>
              {enabledVersion ? `v${enabledVersion.replace(/^v/i, '')}` : EM_DASH}
            </span>
          </Badge>
        </div>
      </div>

      <div className='telemetry-banner__center' data-reconnecting={reconnecting || undefined}>
        <div className='telemetry-banner__metrics'>
          <TelemetryMetric
            icon={Clock3}
            id='uptime'
            label={t('telemetry.metric.uptime')}
            title={startedAtTitle}
            value={uptime}
            compactValue={compactUptime}
          />
          <Separator orientation='vertical' />
          <TelemetryMetric
            icon={ArrowUp}
            id='upload'
            label={t('telemetry.metric.upload')}
            value={formatRate(uploadRate, locale)}
            compactValue={formatRate(uploadRate, locale).replace(/\s/g, '')}
          />
          <Separator orientation='vertical' />
          <TelemetryMetric
            icon={ArrowDown}
            id='download'
            label={t('telemetry.metric.download')}
            value={formatRate(downloadRate, locale)}
            compactValue={formatRate(downloadRate, locale).replace(/\s/g, '')}
          />
        </div>
        <div className='telemetry-banner__notice' role='status' aria-atomic='true'>
          {reconnecting ? <DashboardStreamStatus /> : null}
        </div>
      </div>

      <div className='telemetry-banner__actions'>
        {actionMessage !== ''
          ? (
              <Badge
                className='runtime-action-progress'
                role='status'
                title={actionMessage}
                variant={actionVariant}
              >
                {runtimeControl.busy
                  ? <Spinner data-icon='inline-start' />
                  : runtimeControl.state.phase === 'verified'
                    ? <CircleCheck aria-hidden='true' data-icon='inline-start' />
                    : runtimeControl.state.phase === 'failed'
                      || runtimeControl.state.phase === 'verification_timeout'
                      ? <TriangleAlert aria-hidden='true' data-icon='inline-start' />
                      : null}
                <span>{actionMessage}</span>
              </Badge>
            )
          : (
              <>
                <div className='telemetry-runtime-controls telemetry-runtime-controls--desktop' aria-label={t('telemetry.control.label')}>
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
                                title={t('telemetry.control.start')}
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
                            <RuntimeConfirmation
                              action='stop'
                              disabled={false}
                              onConfirm={() => runRuntimeAction('stop')}
                            />
                            <RuntimeConfirmation
                              action='restart'
                              disabled={false}
                              onConfirm={() => runRuntimeAction('restart')}
                            />
                          </>
                        )
                      : null}
                </div>
                <MobileTelemetryMenu
                  canRestart={verifiedRunning}
                  canStart={verifiedStopped && Boolean(enabledVersion)}
                  canStop={verifiedRunning}
                  onAction={runRuntimeAction}
                />
              </>
            )}
      </div>
    </header>
  );
}
