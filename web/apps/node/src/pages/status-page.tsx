import type { ReactNode } from 'react';

import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { Check, Copy, Ellipsis, Link2, Server, Unlink } from 'lucide-react';
import { formatDateTime, formatRelativeTime } from '@shared/lib/format-time';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@shared/components/ui/dropdown-menu';
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

import type { FieldControl } from '@/components/console-field';
import type { Connection, ConsoleStatus, Enrollment, Session } from '@/api/generated';

import { ConsoleCard } from '@/components/console-card';
import { ConsoleFrame } from '@/components/console-frame';
import { EnrollmentForm } from '@/components/enrollment-form';
import { ConsoleRequestError, getStatus, logout, unenroll } from '@/api/console-client';

/** How often the page refreshes the node's link with its panel. */
const REFRESH_INTERVAL_MS = 5_000;
/** How long a copy action shows that it copied. */
const COPIED_MS = 1_500;

const stateTone: Record<Connection['state'], { dot: string; text: string }> = {
  connected: { dot: 'bg-[var(--color-success)]', text: 'text-[var(--color-success-text)]' },
  connecting: { dot: 'bg-[var(--color-info)]', text: 'text-[var(--color-info-text)]' },
  disconnected: { dot: 'bg-[var(--color-warning)]', text: 'text-[var(--color-warning-text)]' },
  unenrolled: { dot: 'bg-[var(--color-text-faint)]', text: 'text-muted-foreground' },
};

function panelAddress(enrollment: Enrollment): string {
  return `${enrollment.panel_endpoint}${enrollment.panel_path}`;
}

/** Rows that belong together, on a raised inset surface. */
function Group({ children, title }: { children: ReactNode; title: string }) {
  return (
    <section aria-label={title} className='flex flex-col gap-2'>
      <h3 className='px-1 text-xs font-medium text-muted-foreground'>{title}</h3>
      <dl className='divide-y overflow-hidden rounded-lg bg-[var(--color-surface-raised)]'>{children}</dl>
    </section>
  );
}

function Row({ children, copy, label }: { children: ReactNode; copy?: string; label: string }) {
  return (
    <div className={`flex min-h-11 items-center justify-between gap-6 py-2.5 pl-4 ${copy === undefined ? 'pr-4' : 'pr-2'}`}>
      <dt className='shrink-0 text-sm text-[var(--color-text-secondary)]'>{label}</dt>
      <dd className='flex min-w-0 items-center justify-end gap-1 text-right text-sm'>
        {children}
        {copy === undefined ? null : <CopyButton label={label} value={copy} />}
      </dd>
    </div>
  );
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
      aria-label={copied ? t('details.copied') : t('details.copy', { label })}
      className='shrink-0 text-muted-foreground'
      onClick={() => {
        void navigator.clipboard?.writeText(value).then(() => setCopied(true), () => undefined);
      }}
      size='icon-xs'
      variant='ghost'
    >
      {copied ? <Check aria-hidden='true' className='text-[var(--color-success)]' /> : <Copy aria-hidden='true' />}
    </Button>
  );
}

/** The node's own values, which the field behind the card mixes into its text. */
function nodeWords(status: ConsoleStatus): string[] {
  const { enrollment, node } = status;
  const words = [node.hostname, node.version, `${node.os}/${node.arch}`, node.os, node.arch, `protocol ${node.protocol}`];
  if (enrollment) {
    const port = /:(\d+)$/.exec(enrollment.panel_endpoint)?.[1];
    const { node_id: nodeId, panel_id: panelId } = enrollment;
    words.push(panelAddress(enrollment), nodeId, nodeId.slice(0, 9), nodeId.slice(-8), panelId);
    if (port) words.push(`:${port}`);
    words.push(enrollment.trust === 'pin' ? 'pinned' : 'system ca');
  }
  return words;
}

// An identifier stays on one line; hovering shows all of it.
function Code({ value }: { value: string }) {
  return <span className='block truncate font-mono text-xs leading-5' title={value}>{value}</span>;
}

/** The node as a device: its tile with a presence dot, its hostname and its link state. */
function Identity({ action, stale, status }: { action?: ReactNode; stale: boolean; status: ConsoleStatus }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? 'en';
  const { connection, node } = status;
  const tone = stateTone[connection.state];
  return (
    <header className='flex items-center gap-4 p-5 sm:p-6'>
      <span aria-hidden='true' className='relative grid size-12 shrink-0 place-items-center rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-[var(--color-text-secondary)]'>
        <Server className='size-5' strokeWidth={1.75} />
        <span className={`absolute -right-1 -bottom-1 size-3.5 rounded-full border-[3px] border-card ${tone.dot}`} />
      </span>
      <div className='min-w-0 flex-1'>
        <h2 className='truncate font-[family-name:var(--font-display)] text-xl font-semibold' title={node.hostname}>{node.hostname}</h2>
        <p className='flex min-w-0 items-center gap-1.5 text-sm'>
          <span className={`shrink-0 font-medium ${tone.text}`}>{t(`connection.state.${connection.state}`)}</span>
          <span aria-hidden='true' className='text-[var(--color-text-faint)]'>·</span>
          {stale
            ? <span aria-hidden='true' className='truncate text-[var(--color-warning-text)]' title={t('console.stale')}>{t('console.staleShort')}</span>
            : <span className='truncate text-muted-foreground' title={formatDateTime(connection.since, locale)}>{formatRelativeTime(connection.since, locale)}</span>}
        </p>
        <span className='sr-only' role='status'>{stale ? t('console.stale') : ''}</span>
      </div>
      {action}
    </header>
  );
}

/** Why the node is not connected, while it keeps trying. */
function hasProblem(connection: Connection): boolean {
  return connection.last_error !== undefined || connection.retry_at !== undefined;
}

/**
 * The problem to show for next: its own, or while the node is connecting
 * again, the previous one, so the card keeps its height until it connects.
 */
function nextProblem(next: ConsoleStatus, previous: Connection | null): Connection | null {
  if (hasProblem(next.connection)) return next.connection;
  return next.connection.state === 'connecting' ? previous : null;
}

function ConnectionProblem({ connection, retrying }: { connection: Connection; retrying: boolean }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? 'en';
  return (
    <div className='flex flex-col gap-1 rounded-lg border border-[var(--color-warning-border)] bg-[var(--color-warning-soft)] px-4 py-3 text-sm'>
      {connection.last_error !== undefined
        ? <p className='font-mono text-xs leading-5 wrap-anywhere text-[var(--color-warning-text)]'>{connection.last_error}</p>
        : null}
      {connection.retry_at !== undefined
        ? (
            <p className='text-[var(--color-text-secondary)]'>
              {t('connection.retryAt')}
              {' · '}
              {retrying
                ? t('connection.retrying')
                : (
                    <span title={formatDateTime(connection.retry_at, locale)}>
                      {formatRelativeTime(connection.retry_at, locale)}
                    </span>
                  )}
            </p>
          )
        : null}
    </div>
  );
}

function NodeGroup({ status }: { status: ConsoleStatus }) {
  const { t } = useTranslation();
  return (
    <Group title={t('details.node')}>
      <Row label={t('details.version')}><Code value={status.node.version} /></Row>
      <Row label={t('details.platform')}><Code value={`${status.node.os}/${status.node.arch}`} /></Row>
      <Row label={t('details.protocol')}><Code value={status.node.protocol} /></Row>
    </Group>
  );
}

interface EnrollmentMenuProps {
  csrfToken: string;
  enrollment: Enrollment;
  onSignedOut: () => void;
  onStatus: (status: ConsoleStatus) => void;
}

/** Re-enrollment and unenrollment, kept in the card's menu, apart from the status they change. */
function EnrollmentMenu({ csrfToken, enrollment, onSignedOut, onStatus }: EnrollmentMenuProps) {
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
    <>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button aria-label={t('danger.more')} size='icon-sm' variant='ghost' />}>
          <Ellipsis aria-hidden='true' />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' sideOffset={8}>
          <DropdownMenuItem onClick={() => setReenrolling(true)}>
            <Link2 aria-hidden='true' />
            {t('danger.reenroll')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => setConfirmUnenroll(true)} variant='destructive'>
            <Unlink aria-hidden='true' />
            {t('danger.unenroll')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
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
    </>
  );
}

interface StatusPageProps {
  session: Session;
  field: FieldControl;
  onSignedOut: () => void;
  /** The status read while signing in, if any; the page opens on it without loading. */
  initialStatus: ConsoleStatus | null;
}

export function StatusPage({ field, initialStatus, session, onSignedOut }: StatusPageProps) {
  const { t } = useTranslation();
  const [status, setStatus] = useState<ConsoleStatus | null>(initialStatus);
  const [problem, setProblem] = useState<Connection | null>(
    () => initialStatus ? nextProblem(initialStatus, null) : null,
  );
  const [stale, setStale] = useState(false);
  const [signingOut, setSigningOut] = useState(false);
  const signedOutRef = useRef(onSignedOut);
  signedOutRef.current = onSignedOut;

  const fieldRef = useRef(field);
  fieldRef.current = field;
  const hadStatusRef = useRef(initialStatus !== null);

  useEffect(() => {
    if (status) field.setWords(nodeWords(status));
  }, [field, status]);

  useEffect(() => {
    let active = true;
    async function refresh(pulse: boolean) {
      try {
        const next = await getStatus();
        if (!active) return;
        setStatus(next);
        setProblem(previous => nextProblem(next, previous));
        setStale(false);
        // Each refresh ripples once through the field behind the card.
        if (pulse) fieldRef.current.pulse('refresh');
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
    if (!hadStatusRef.current) void refresh(false);
    const timer = window.setInterval(() => void refresh(true), REFRESH_INTERVAL_MS);
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
    setProblem(previous => nextProblem(next, previous));
    setStale(false);
  }

  let card: ReactNode;
  if (status === null) {
    card = (
      <ConsoleCard aria-busy='true' className='grid min-h-48 place-items-center' ref={field.anchor}>
        <Spinner />
        <span className='sr-only'>{t('console.loading')}</span>
      </ConsoleCard>
    );
  } else if (status.enrollment === null) {
    card = (
      <ConsoleCard aria-label={status.node.hostname} ref={field.anchor}>
        <Identity stale={stale} status={status} />
        <div className='flex flex-col gap-6 border-t px-5 pt-5 pb-6 sm:px-6'>
          <section aria-labelledby='console-enroll-title' className='flex flex-col gap-4'>
            <div className='flex flex-col gap-1'>
              <h3 className='text-sm font-semibold' id='console-enroll-title'>{t('enroll.title')}</h3>
              <p className='text-sm text-muted-foreground'>{t('enroll.description')}</p>
            </div>
            <EnrollmentForm csrfToken={session.csrfToken} onEnrolled={accept} onSignedOut={onSignedOut} submitLabel={t('enroll.submit')} />
          </section>
          <NodeGroup status={status} />
        </div>
      </ConsoleCard>
    );
  } else {
    card = (
      <ConsoleCard aria-label={status.node.hostname} ref={field.anchor}>
        <Identity
          action={(
            <EnrollmentMenu
              csrfToken={session.csrfToken}
              enrollment={status.enrollment}
              onSignedOut={onSignedOut}
              onStatus={accept}
            />
          )}
          stale={stale}
          status={status}
        />
        <div className='flex flex-col gap-5 border-t px-5 pt-5 pb-6 sm:px-6'>
          {problem ? <ConnectionProblem connection={problem} retrying={!hasProblem(status.connection)} /> : null}
          <Group title={t('details.panel')}>
            <Row copy={panelAddress(status.enrollment)} label={t('details.address')}><Code value={panelAddress(status.enrollment)} /></Row>
            <Row copy={status.enrollment.node_id} label={t('details.nodeId')}><Code value={status.enrollment.node_id} /></Row>
            <Row label={t('details.trust.label')}>{t(`details.trust.${status.enrollment.trust}`)}</Row>
          </Group>
          <NodeGroup status={status} />
        </div>
      </ConsoleCard>
    );
  }

  return (
    <ConsoleFrame busy={signingOut} onSignOut={() => void signOut()}>
      <h1 className='sr-only'>{t('console.title')}</h1>
      {card}
    </ConsoleFrame>
  );
}
