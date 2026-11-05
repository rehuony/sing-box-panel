import { Power, Unplug } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { useCallback, useEffect, useState } from 'react';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import type { Session } from '@/api/generated';

import { LoginPage } from '@/pages/login-page';
import { StatusPage } from '@/pages/status-page';
import { ConsoleFrame } from '@/components/console-frame';
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
        <ConsoleFrame>
          <div aria-busy='true' className='flex flex-1 items-center justify-center'>
            <Spinner />
            <span className='sr-only'>{t('console.checking')}</span>
          </div>
        </ConsoleFrame>
      );
    case 'signed-out':
      return <LoginPage onSignedIn={session => setState({ kind: 'signed-in', session })} />;
    case 'signed-in':
      return <StatusPage session={state.session} onSignedOut={() => void check()} />;
    default:
      return (
        <ConsoleFrame>
          <Empty className='mt-[min(12vh,6rem)] flex-none rounded-[var(--radius-card)] border border-solid bg-card'>
            <EmptyHeader>
              <EmptyMedia className='mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6'>
                {state.kind === 'disabled' ? <Power aria-hidden='true' strokeWidth={1.5} /> : <Unplug aria-hidden='true' strokeWidth={1.5} />}
              </EmptyMedia>
              <EmptyTitle>
                <h1>{t(`console.${state.kind}.title`)}</h1>
              </EmptyTitle>
              <EmptyDescription>{t(`console.${state.kind}.description`)}</EmptyDescription>
            </EmptyHeader>
            <EmptyContent>
              <Button variant='outline' onClick={() => void check()}>{t('common.retry')}</Button>
            </EmptyContent>
          </Empty>
        </ConsoleFrame>
      );
  }
}
