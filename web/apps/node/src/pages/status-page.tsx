import type { ReactNode } from 'react';

import { useTranslation } from 'react-i18next';
import { Badge } from '@shared/components/ui/badge';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { Card, CardContent, CardHeader, CardTitle } from '@shared/components/ui/card';
import { Empty, EmptyDescription, EmptyHeader, EmptyTitle } from '@shared/components/ui/empty';

import type { Connection, ConsoleStatus, Session } from '@/api/generated';

import { ConsoleRequestError, getStatus, logout } from '@/api/console-client';

/** How often the page refreshes the node's link with its panel. */
const REFRESH_INTERVAL_MS = 5_000;

const stateVariant: Record<Connection['state'], 'success' | 'warning' | 'info' | 'secondary'> = {
  connected: 'success',
  connecting: 'info',
  disconnected: 'warning',
  unenrolled: 'secondary',
};

interface StatusPageProps {
  session: Session;
  onSignedOut: () => void;
}

export function StatusPage({ session, onSignedOut }: StatusPageProps) {
  const { i18n, t } = useTranslation();
  const [status, setStatus] = useState<ConsoleStatus | null>(null);
  const [stale, setStale] = useState(false);
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
    try {
      await logout(session.csrfToken);
    } finally {
      onSignedOut();
    }
  }

  const formatTime = (value: string | undefined) => value === undefined
    ? t('common.unavailable')
    : new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value));

  return (
    <main className='mx-auto flex w-full max-w-3xl flex-col gap-4 p-4 sm:p-6'>
      <header className='flex items-center justify-between gap-4'>
        <div className='min-w-0'>
          <h1 className='text-2xl font-semibold'>{t('console.title')}</h1>
          <p className='truncate text-sm text-muted-foreground' title={status?.node.hostname}>
            {status?.node.hostname ?? t('common.unavailable')}
          </p>
        </div>
        <Button variant='outline' onClick={() => void signOut()}>{t('status.signOut')}</Button>
      </header>
      <p className='min-h-5 text-sm text-[var(--color-warning-text)]' role='status'>{stale ? t('status.stale') : ''}</p>
      {status === null
        ? (
            <div className='flex min-h-48 items-center justify-center' aria-busy='true'>
              <Spinner />
            </div>
          )
        : (
            <>
              <Card>
                <CardHeader>
                  <CardTitle>{t('status.connection.title')}</CardTitle>
                </CardHeader>
                <CardContent>
                  <dl className='grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-4 gap-y-2 text-sm'>
                    <Row label={t('status.connection.stateLabel')}>
                      <Badge variant={stateVariant[status.connection.state]}>
                        {t(`status.connection.state.${status.connection.state}`)}
                      </Badge>
                    </Row>
                    <Row label={t('status.connection.since')}>{formatTime(status.connection.since)}</Row>
                    {status.connection.retry_at !== undefined && (
                      <Row label={t('status.connection.retryAt')}>{formatTime(status.connection.retry_at)}</Row>
                    )}
                    {status.connection.last_error !== undefined && (
                      <Row label={t('status.connection.lastError')}>
                        <span className='break-words font-mono text-xs'>{status.connection.last_error}</span>
                      </Row>
                    )}
                  </dl>
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>{t('status.panel.title')}</CardTitle>
                </CardHeader>
                <CardContent>
                  {status.enrollment === null
                    ? (
                        <Empty>
                          <EmptyHeader>
                            <EmptyTitle>{t('status.unenrolled.title')}</EmptyTitle>
                            <EmptyDescription>{t('status.unenrolled.description')}</EmptyDescription>
                          </EmptyHeader>
                        </Empty>
                      )
                    : (
                        <dl className='grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-4 gap-y-2 text-sm'>
                          <Row label={t('status.panel.endpoint')}>
                            <Code value={status.enrollment.panel_endpoint + status.enrollment.panel_path} />
                          </Row>
                          <Row label={t('status.panel.nodeId')}>
                            <Code value={status.enrollment.node_id} />
                          </Row>
                          <Row label={t('status.panel.trust.label')}>{t(`status.panel.trust.${status.enrollment.trust}`)}</Row>
                        </dl>
                      )}
                </CardContent>
              </Card>
              <Card>
                <CardHeader>
                  <CardTitle>{t('status.node.title')}</CardTitle>
                </CardHeader>
                <CardContent>
                  <dl className='grid grid-cols-[minmax(8rem,auto)_1fr] gap-x-4 gap-y-2 text-sm'>
                    <Row label={t('status.node.version')}><Code value={status.node.version} /></Row>
                    <Row label={t('status.node.platform')}>{`${status.node.os}/${status.node.arch}`}</Row>
                    <Row label={t('status.node.protocol')}>{status.node.protocol}</Row>
                  </dl>
                </CardContent>
              </Card>
            </>
          )}
    </main>
  );
}

// An identifier stays on one line; hovering shows all of it.
function Code({ value }: { value: string }) {
  return <span className='block truncate font-mono' title={value}>{value}</span>;
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className='text-muted-foreground'>{label}</dt>
      <dd className='min-w-0'>{children}</dd>
    </>
  );
}
