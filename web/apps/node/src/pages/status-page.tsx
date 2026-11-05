import type { ReactNode } from 'react';

import { useTranslation } from 'react-i18next';
import { Badge } from '@shared/components/ui/badge';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { formatDateTime, formatRelativeTime } from '@shared/lib/format-time';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';
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

import type { Connection, ConsoleStatus, Enrollment, Session } from '@/api/generated';

import { ConsoleFrame } from '@/components/console-frame';
import { EnrollmentForm } from '@/components/enrollment-form';
import { ConsoleRequestError, getStatus, logout, unenroll } from '@/api/console-client';

/** How often the page refreshes the node's link with its panel. */
const REFRESH_INTERVAL_MS = 5_000;

const stateTone: Record<Connection['state'], string> = {
  connected: 'bg-[var(--color-success)]',
  connecting: 'bg-[var(--color-info)]',
  disconnected: 'bg-[var(--color-warning)]',
  unenrolled: 'bg-[var(--color-text-faint)]',
};

const stateVariant: Record<Connection['state'], 'success' | 'warning' | 'info' | 'secondary'> = {
  connected: 'success',
  connecting: 'info',
  disconnected: 'warning',
  unenrolled: 'secondary',
};

function panelAddress(enrollment: Enrollment): string {
  return `${enrollment.panel_endpoint}${enrollment.panel_path}`;
}

function Section({ children, className = '', title }: { children: ReactNode; className?: string; title: string }) {
  return (
    <section aria-label={title} className={`flex flex-col gap-4 rounded-[var(--radius-card)] border bg-card p-4 sm:p-5 ${className}`}>
      <h2 className='text-sm font-semibold'>{title}</h2>
      {children}
    </section>
  );
}

function Rows({ children }: { children: ReactNode }) {
  return <dl className='grid grid-cols-[minmax(7rem,auto)_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm'>{children}</dl>;
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className='text-muted-foreground'>{label}</dt>
      <dd className='min-w-0'>{children}</dd>
    </>
  );
}

// An identifier stays on one line; hovering shows all of it.
function Code({ value }: { value: string }) {
  return <span className='block truncate font-mono text-xs leading-5' title={value}>{value}</span>;
}

function ConnectionSection({ status }: { status: ConsoleStatus }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? 'en';
  const { connection, enrollment } = status;
  return (
    <section aria-label={t('connection.title')} className='flex flex-col gap-3 rounded-[var(--radius-card)] border bg-card p-4 sm:p-5'>
      <div className='flex min-w-0 items-center gap-3'>
        <span aria-hidden='true' className={`size-2.5 shrink-0 rounded-full ${stateTone[connection.state]}`} />
        <p className='min-w-0 truncate text-xl font-semibold'>{t(`connection.state.${connection.state}`)}</p>
        <Badge className='ms-auto shrink-0' title={formatDateTime(connection.since, locale)} variant={stateVariant[connection.state]}>
          {t('connection.since', { time: formatRelativeTime(connection.since, locale) })}
        </Badge>
      </div>
      {enrollment
        ? (
            <p className='min-w-0 truncate text-sm text-muted-foreground' title={panelAddress(enrollment)}>
              {t('connection.panel', { address: panelAddress(enrollment) })}
            </p>
          )
        : null}
      {connection.retry_at !== undefined || connection.last_error !== undefined
        ? (
            <Rows>
              {connection.retry_at !== undefined
                ? (
                    <Row label={t('connection.retryAt')}>
                      <span title={formatDateTime(connection.retry_at, locale)}>
                        {formatRelativeTime(connection.retry_at, locale)}
                      </span>
                    </Row>
                  )
                : null}
              {connection.last_error !== undefined
                ? (
                    <Row label={t('connection.lastError')}>
                      <span className='font-mono text-xs leading-5 wrap-anywhere text-[var(--color-danger-text)]'>{connection.last_error}</span>
                    </Row>
                  )
                : null}
            </Rows>
          )
        : null}
    </section>
  );
}

function NodeRows({ status }: { status: ConsoleStatus }) {
  const { t } = useTranslation();
  return (
    <Rows>
      <Row label={t('details.hostname')}><Code value={status.node.hostname} /></Row>
      <Row label={t('details.version')}><Code value={status.node.version} /></Row>
      <Row label={t('details.platform')}><Code value={`${status.node.os}/${status.node.arch}`} /></Row>
      <Row label={t('details.protocol')}><Code value={status.node.protocol} /></Row>
    </Rows>
  );
}

interface EnrollmentActionsProps {
  csrfToken: string;
  enrollment: Enrollment;
  onSignedOut: () => void;
  onStatus: (status: ConsoleStatus) => void;
}

/** Re-enrollment and unenrollment, kept apart from the status they change. */
function EnrollmentActions({ csrfToken, enrollment, onSignedOut, onStatus }: EnrollmentActionsProps) {
  const { t } = useTranslation();
  const [reenrolling, setReenrolling] = useState(false);
  const [confirmUnenroll, setConfirmUnenroll] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [error, setError] = useState('');
  const address = panelAddress(enrollment);

  async function remove() {
    if (removing) return;
    setRemoving(true);
    setError('');
    try {
      onStatus(await unenroll(csrfToken));
      setConfirmUnenroll(false);
    } catch (reason) {
      if (reason instanceof ConsoleRequestError && (reason.status === 401 || reason.code === 'console_disabled')) {
        onSignedOut();
        return;
      }
      setError(t('danger.unenrollFailed'));
    } finally {
      setRemoving(false);
    }
  }

  return (
    <Section className='border-[var(--color-danger-border)]' title={t('danger.title')}>
      <div className='flex flex-col divide-y'>
        <div className='flex flex-wrap items-center gap-3 pb-4'>
          <div className='min-w-0 flex-1'>
            <p className='text-sm font-medium'>{t('danger.reenroll')}</p>
            <p className='text-sm text-muted-foreground'>{t('danger.reenrollDescription')}</p>
          </div>
          <Button onClick={() => setReenrolling(true)} variant='outline'>{t('danger.reenroll')}</Button>
        </div>
        <div className='flex flex-wrap items-center gap-3 pt-4'>
          <div className='min-w-0 flex-1'>
            <p className='text-sm font-medium'>{t('danger.unenroll')}</p>
            <p className='text-sm text-muted-foreground'>{t('danger.unenrollDescription')}</p>
          </div>
          <Button onClick={() => setConfirmUnenroll(true)} variant='destructive'>{t('danger.unenroll')}</Button>
        </div>
      </div>
      <Dialog onOpenChange={setReenrolling} open={reenrolling}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('danger.reenrollTitle')}</DialogTitle>
            <DialogDescription>{t('danger.reenrollConfirm', { address })}</DialogDescription>
          </DialogHeader>
          <EnrollmentForm
            actions={<DialogClose render={<Button type='button' variant='outline' />}>{t('common.cancel')}</DialogClose>}
            csrfToken={csrfToken}
            onEnrolled={(status) => {
              setReenrolling(false);
              onStatus(status);
            }}
            onSignedOut={onSignedOut}
            submitLabel={t('danger.reenroll')}
          />
        </DialogContent>
      </Dialog>
      <AlertDialog
        onOpenChange={(open) => {
          if (!open && !removing) {
            setConfirmUnenroll(false);
            setError('');
          }
        }}
        open={confirmUnenroll}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('danger.unenrollTitle', { address })}</AlertDialogTitle>
            <AlertDialogDescription>{t('danger.unenrollConfirm', { address })}</AlertDialogDescription>
          </AlertDialogHeader>
          {error ? <p className='text-sm text-[var(--color-danger-text)]' role='alert'>{error}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void remove()} variant='destructive'>
              {removing ? <Spinner data-icon='inline-start' /> : null}
              {t('danger.unenroll')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Section>
  );
}

interface StatusPageProps {
  session: Session;
  onSignedOut: () => void;
}

export function StatusPage({ session, onSignedOut }: StatusPageProps) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<ConsoleStatus | null>(null);
  const [stale, setStale] = useState(false);
  const [signingOut, setSigningOut] = useState(false);
  const signedOutRef = useRef(onSignedOut);
  signedOutRef.current = onSignedOut;

  useEffect(() => {
    let active = true;
    async function refresh() {
      try {
        const next = await getStatus();
        if (!active) return;
        setStatus(next);
        setStale(false);
      } catch (error) {
        if (!active) return;
        // A session that ended, or a console turned off, ends this page.
        if (error instanceof ConsoleRequestError && (error.status === 401 || error.code === 'console_disabled')) {
          signedOutRef.current();
          return;
        }
        setStale(true);
      }
    }
    void refresh();
    const timer = window.setInterval(() => void refresh(), REFRESH_INTERVAL_MS);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, []);

  async function signOut() {
    setSigningOut(true);
    try {
      await logout(session.csrfToken);
    } finally {
      onSignedOut();
    }
  }

  function accept(next: ConsoleStatus) {
    setStatus(next);
    setStale(false);
  }

  let content: ReactNode;
  if (status === null) {
    content = (
      <div aria-busy='true' className='flex min-h-48 items-center justify-center rounded-[var(--radius-card)] border bg-card'>
        <Spinner />
        <span className='sr-only'>{t('console.loading')}</span>
      </div>
    );
  } else if (status.enrollment === null) {
    content = (
      <>
        <Section title={t('enroll.title')}>
          <p className='-mt-2 text-sm text-muted-foreground'>{t('enroll.description')}</p>
          <EnrollmentForm csrfToken={session.csrfToken} onEnrolled={accept} onSignedOut={onSignedOut} submitLabel={t('enroll.submit')} />
        </Section>
        <Section title={t('details.node')}><NodeRows status={status} /></Section>
      </>
    );
  } else {
    content = (
      <>
        <ConnectionSection status={status} />
        <Section title={t('details.title')}>
          <Rows>
            <Row label={t('details.panelAddress')}><Code value={panelAddress(status.enrollment)} /></Row>
            <Row label={t('details.nodeId')}><Code value={status.enrollment.node_id} /></Row>
            <Row label={t('details.trust.label')}>{t(`details.trust.${status.enrollment.trust}`)}</Row>
          </Rows>
          <div className='border-t pt-4'><NodeRows status={status} /></div>
        </Section>
        <EnrollmentActions
          csrfToken={session.csrfToken}
          enrollment={status.enrollment}
          onSignedOut={onSignedOut}
          onStatus={accept}
        />
      </>
    );
  }

  return (
    <ConsoleFrame busy={signingOut} hostname={status?.node.hostname} onSignOut={() => void signOut()}>
      <h1 className='sr-only'>{t('console.title')}</h1>
      <p className='-mb-2 min-h-5 text-sm text-[var(--color-warning-text)]' role='status'>{stale ? t('console.stale') : ''}</p>
      {content}
    </ConsoleFrame>
  );
}
