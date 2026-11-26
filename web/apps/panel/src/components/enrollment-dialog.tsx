import type { FormEvent, Ref } from 'react';

import { Plus } from 'lucide-react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useEffect, useEffectEvent, useId, useRef, useState } from 'react';
import { Dialog, DialogContent, DialogFields, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { FleetNode } from '@/api/api-client';
import type { PendingEnrollment } from '@/stores/enrollment-dialog.store';

import { queries } from '@/api/queries';
import { nodePath } from '@/utils/node-path';
import { messageLine } from '@/utils/message-line';
import { useApiClient } from '@/api/api-client-context';
import { describeRequestError } from '@/components/error-notice';
import { useEnrollmentDialog } from '@/stores/enrollment-dialog.store';
import { useFleetStreamState } from '@/components/app-shell/use-fleet-stream';

/** How often the dialog reads the fleet while the stream that would bring the node is down. */
const FALLBACK_POLL_MS = 3_000;

function useSecondClock(active: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    // The clock idles while closed, so it catches up at once when it starts again.
    const first = window.setTimeout(() => setNow(Date.now()), 0);
    const timer = window.setInterval(() => setNow(Date.now()), 1_000);
    return () => {
      window.clearTimeout(first);
      window.clearInterval(timer);
    };
  }, [active]);
  return now;
}

function countdown(milliseconds: number): string {
  const seconds = Math.max(0, Math.ceil(milliseconds / 1_000));
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`;
}

/** The node the code enrolled, once the fleet shows it enrolled after the code was issued. */
function useEnrolledNode(pending: PendingEnrollment | null): FleetNode | undefined {
  const client = useApiClient();
  const down = useFleetStreamState(state => state.down);
  const status = useQuery({
    ...queries.fleetStatus(client),
    enabled: pending !== null,
    refetchInterval: pending !== null && down ? FALLBACK_POLL_MS : false,
  });
  if (pending === null) return undefined;
  const found = status.data?.items.find(item => item.node.id === pending.enrollment.node_id)?.node;
  return found && found.enrolled_at !== pending.enrolledBefore ? found : undefined;
}

interface CopyActionProps {
  label: string;
  value: string;
  /** The toast that confirms the copy. */
  copied: string;
  ref?: Ref<HTMLButtonElement>;
}

function CopyAction({ copied, label, ref, value }: CopyActionProps) {
  const { t } = useTranslation();
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      toast.add({ title: copied, type: 'success' });
    } catch {
      toast.add({ title: t('enrollment.copyFailed'), type: 'error' });
    }
  }
  return <Button onClick={() => void copy()} ref={ref} size='xs' variant='flat'>{label}</Button>;
}

/** One reading of the enrolled node: its label, then its value in the data face. */
function Reading({ label, value }: { label: string; value: string }) {
  return (
    <div className='contents'>
      <dt className='text-muted-foreground'>{label}</dt>
      <dd className='min-w-0 truncate font-[family-name:var(--font-data)]' title={value}>{value}</dd>
    </div>
  );
}

/**
 * Adds a node, or enrolls one again, in one dialog of three states: naming
 * the node, waiting for the host to run the command, and the node enrolled.
 * Leaving while waiting discards the unused code; the node is never touched.
 * The shell holds the one dialog, which the enrollment dialog store opens.
 */
export function EnrollmentDialog() {
  const { t } = useTranslation();
  const { close: hide, open, reenrollment } = useEnrollmentDialog();
  const client = useApiClient();
  const navigate = useNavigate();
  const nameId = useId();
  const messageId = useId();
  const [name, setName] = useState('');
  const [missing, setMissing] = useState(false);
  const [pending, setPending] = useState<PendingEnrollment | null>(null);
  // The code stays shown while the dialog closes; only an open dialog counts down and leaves it unused.
  const current = pending ?? reenrollment;
  const enrolled = useEnrolledNode(current);
  const waiting = current !== null && enrolled === undefined;
  const now = useSecondClock(open && waiting);
  const remaining = current ? Date.parse(current.enrollment.expires_at) - now : 0;
  const expired = waiting && remaining <= 0;
  // The code the dialog would leave unused if it went away now.
  const unusedRef = useRef<string | null>(null);
  unusedRef.current = open && waiting ? current.enrollment.node_id : null;
  const copyRef = useRef<HTMLButtonElement>(null);
  const openRef = useRef<HTMLButtonElement>(null);
  // Focus follows the dialog's state, since the control that had it goes away.
  const code = current?.enrollment.code;
  useEffect(() => {
    if (open && code) copyRef.current?.focus();
  }, [code, open]);
  const enrolledId = enrolled?.id;
  useEffect(() => {
    if (enrolledId) openRef.current?.focus();
  }, [enrolledId]);

  const issue = useMutation({
    mutationFn: async () => {
      if (reenrollment) {
        const enrollment = await client.createNodeEnrollment(reenrollment.enrollment.node_id);
        return { enrollment, enrolledBefore: reenrollment.enrolledBefore };
      }
      return { enrollment: await client.createNode(name.trim()), enrolledBefore: null };
    },
    onSuccess: setPending,
  });

  function cancelUnused(keepalive = false) {
    const id = unusedRef.current;
    unusedRef.current = null;
    if (id !== null) void client.cancelNodeEnrollment(id, keepalive).catch(() => undefined);
  }

  // A page that goes away mid-wait leaves no usable code behind.
  const onPageHide = useEffectEvent(() => cancelUnused(true));
  useEffect(() => {
    window.addEventListener('pagehide', onPageHide);
    return () => {
      window.removeEventListener('pagehide', onPageHide);
      cancelUnused();
    };
  // eslint-disable-next-line react/exhaustive-deps -- only on leaving, with the code unused at that moment
  }, []);

  function close() {
    cancelUnused();
    hide();
  }

  function reset() {
    setName('');
    setMissing(false);
    setPending(null);
    issue.reset();
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (current !== null || issue.isPending) return;
    if (name.trim() === '') setMissing(true);
    else issue.mutate();
  }

  let title = t('fleet.add.title');
  if (reenrollment) title = t('enrollment.reenrollTitle', { name: reenrollment.enrollment.name });
  const naming = current === null;
  const shownName = current?.enrollment.name ?? '';
  const host = enrolled ? [enrolled.hostname, enrolled.public_ip].filter(Boolean).join(' · ') : '';
  const system = enrolled ? [enrolled.os && enrolled.arch ? `${enrolled.os}/${enrolled.arch}` : '', enrolled.build_version].filter(Boolean).join(' · ') : '';

  let message = '';
  if (issue.error) message = describeRequestError(issue.error);
  else if (naming && missing) message = t('fleet.add.nameRequired');
  else if (expired) message = t('enrollment.expired');

  return (
    <Dialog
      disablePointerDismissal={waiting}
      onOpenChange={(next) => {
        if (next) return;
        close();
      }}
      onOpenChangeComplete={(next) => {
        if (!next) reset();
      }}
      open={open}
    >
      <DialogContent>
        <form className='flex min-w-0 flex-col gap-5' noValidate onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
          </DialogHeader>
          {naming
            ? (
                <DialogFields>
                  <label htmlFor={nameId}>{t('fleet.add.name')}</label>
                  <Input
                    aria-describedby={messageId}
                    aria-invalid={missing || issue.error ? true : undefined}
                    autoComplete='off'
                    id={nameId}
                    maxLength={64}
                    onChange={(event) => {
                      setName(event.target.value);
                      setMissing(false);
                      if (issue.error) issue.reset();
                    }}
                    placeholder='edge-tokyo'
                    spellCheck={false}
                    value={name}
                  />
                </DialogFields>
              )
            : (
                // Both later states share one cell, so the dialog keeps its height when the node arrives.
                <div className='grid rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] *:col-start-1 *:row-start-1'>
                  <div aria-hidden={enrolled ? true : undefined} className={cn('flex min-w-0 flex-col gap-2 p-3', enrolled && 'invisible')} inert={enrolled ? true : undefined}>
                    <div className='flex min-w-0 items-center justify-between gap-2'>
                      <span className='text-xs text-muted-foreground'>{t('enrollment.runOnHost')}</span>
                      <span className='-my-1 -me-1 flex flex-none items-center gap-0.5'>
                        <CopyAction copied={t('enrollment.copiedCommand')} label={t('enrollment.copyCommand')} ref={copyRef} value={current.enrollment.command} />
                        <CopyAction copied={t('enrollment.copiedCode')} label={t('enrollment.copyCode')} value={current.enrollment.code} />
                      </span>
                    </div>
                    <p className='font-[family-name:var(--font-data)] text-compact break-all select-text'>
                      <span aria-hidden='true' className='text-primary select-none'>$ </span>
                      {current.enrollment.command}
                    </p>
                  </div>
                  {enrolled
                    ? (
                        <div className='flex min-w-0 flex-col gap-2 p-3'>
                          <div className='flex min-w-0 items-center gap-2'>
                            <span aria-hidden='true' className={cn('size-2 flex-none rounded-full', enrolled.state === 'online' ? 'bg-[var(--color-success)]' : 'bg-[var(--color-danger)]')} />
                            <span className='truncate text-sm font-medium'>{enrolled.name}</span>
                          </div>
                          <dl className='grid grid-cols-[max-content_minmax(0,1fr)] gap-x-3 gap-y-1 text-compact'>
                            {host ? <Reading label={t('enrollment.host')} value={host} /> : null}
                            {system ? <Reading label={t('enrollment.system')} value={system} /> : null}
                          </dl>
                        </div>
                      )
                    : null}
                </div>
              )}
          <DialogFooter className='flex-nowrap justify-between'>
            <p className='flex min-h-5 min-w-0 flex-1 items-center gap-2 text-compact' id={messageId} role='status'>
              {message
                ? <span className={cn('truncate', expired && !issue.error ? 'text-[var(--color-warning-text)]' : 'text-[var(--color-danger-text)]')}>{messageLine(message)}</span>
                : null}
              {!message && waiting
                ? (
                    <>
                      <Spinner className='size-4 flex-none' />
                      <span className='truncate' title={t('enrollment.waiting', { name: shownName })}>{t('enrollment.waiting', { name: shownName })}</span>
                      <span
                        className='flex-none font-[family-name:var(--font-data)] text-[var(--color-text-secondary)] tabular-nums'
                        title={t('enrollment.remaining')}
                      >
                        {countdown(remaining)}
                      </span>
                    </>
                  )
                : null}
              {enrolled ? <span className='truncate text-[var(--color-success-text)]'>{t('enrollment.enrolled')}</span> : null}
            </p>
            {naming
              ? (
                  <>
                    <Button onClick={close} type='button' variant='outline'>{t('common.cancel')}</Button>
                    <Button aria-busy={issue.isPending || undefined} type='submit'>{t('enrollment.generate')}</Button>
                  </>
                )
              : null}
            {waiting
              ? (
                  <>
                    <Button onClick={close} type='button' variant='outline'>{t('common.cancel')}</Button>
                    {expired
                      ? (
                          <Button
                            aria-busy={issue.isPending || undefined}
                            onClick={() => {
                              // A new code replaces the unused one: by node when enrolling again, by name when adding.
                              if (!issue.isPending) issue.mutate();
                            }}
                            type='button'
                          >
                            {t('enrollment.regenerate')}
                          </Button>
                        )
                      : null}
                  </>
                )
              : null}
            {enrolled
              ? (
                  <>
                    <Button onClick={hide} type='button' variant='outline'>{t('enrollment.done')}</Button>
                    <Button
                      onClick={() => {
                        hide();
                        void navigate(nodePath(enrolled.id));
                      }}
                      ref={openRef}
                      type='button'
                    >
                      {t('enrollment.open')}
                    </Button>
                  </>
                )
              : null}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * The action that opens the dialog to add a node. In a title strip it is flat
 * and keeps only its icon on a phone; elsewhere it is the area's primary
 * action.
 */
export function AddNodeAction({ strip = false }: { strip?: boolean }) {
  const { t } = useTranslation();
  const add = useEnrollmentDialog(state => state.add);
  return (
    <Button
      className={strip ? 'max-sm:w-(--control-height-sm) max-sm:px-0' : undefined}
      onClick={add}
      size={strip ? 'sm' : 'default'}
      variant={strip ? 'flat' : 'default'}
    >
      <Plus aria-hidden='true' data-icon='inline-start' />
      <span className={strip ? 'max-sm:sr-only' : undefined}>{t('fleet.add.action')}</span>
    </Button>
  );
}
