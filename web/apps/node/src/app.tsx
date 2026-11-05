import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { useCallback, useEffect, useState } from 'react';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from '@shared/components/ui/empty';

import type { Session } from '@/api/generated';

import { LoginPage } from '@/pages/login-page';
import { StatusPage } from '@/pages/status-page';
import { ConsoleRequestError, getSession } from '@/api/console-client';

type ConsoleState
  = | { kind: 'checking' }
    | { kind: 'signed-out' }
    | { kind: 'signed-in'; session: Session }
    | { kind: 'disabled' }
    | { kind: 'unavailable' };

export function App() {
  const { t } = useTranslation();
  const [state, setState] = useState<ConsoleState>({ kind: 'checking' });

  const check = useCallback(async () => {
    setState({ kind: 'checking' });
    try {
      const session = await getSession();
      setState(session === null ? { kind: 'signed-out' } : { kind: 'signed-in', session });
    } catch (error) {
      setState(error instanceof ConsoleRequestError && error.code === 'console_disabled'
        ? { kind: 'disabled' }
        : { kind: 'unavailable' });
    }
  }, []);

  useEffect(() => {
    void check();
  }, [check]);

  switch (state.kind) {
    case 'checking':
      return (
        <main className='flex min-h-svh items-center justify-center' aria-busy='true'>
          <Spinner />
          <span className='sr-only'>{t('console.checking')}</span>
        </main>
      );
    case 'signed-out':
      return <LoginPage onSignedIn={session => setState({ kind: 'signed-in', session })} />;
    case 'signed-in':
      return <StatusPage session={state.session} onSignedOut={() => void check()} />;
    default:
      return (
        <main className='flex min-h-svh items-center justify-center p-4'>
          <Empty>
            <EmptyHeader>
              <EmptyTitle>
                <h1>{t(`console.${state.kind}.title`)}</h1>
              </EmptyTitle>
              <EmptyDescription>{t(`console.${state.kind}.description`)}</EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button variant='outline' onClick={() => void check()}>{t('common.retry')}</Button>
            </EmptyContent>
          </Empty>
        </main>
      );
  }
}
