import type { ReactNode } from 'react';

import { flushSync } from 'react-dom';
import { Power, Unplug } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import type { ConsoleStatus, Session } from '@/api/generated';
import type { ConsoleFieldHandle, FieldControl } from '@/components/console-field';

import { LoginPage } from '@/pages/login-page';
import { StatusPage } from '@/pages/status-page';
import { ConsoleField } from '@/components/console-field';
import { ConsoleFrame } from '@/components/console-frame';
import { ConsoleRequestError, getSession } from '@/api/console-client';

type ConsoleState
  = | { kind: 'checking' }
    | { kind: 'signed-out' }
    | { kind: 'signed-in'; session: Session; status: ConsoleStatus | null }
    | { kind: 'disabled' }
    | { kind: 'unavailable' };

function prefersReducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

export function App() {
  const { t } = useTranslation();
  const [state, setState] = useState<ConsoleState>({ kind: 'checking' });
  const fieldRef = useRef<ConsoleFieldHandle>(null);
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [fieldBusy, setFieldBusy] = useState(false);
  const [fieldWords, setFieldWords] = useState<readonly string[]>([]);
  const field = useMemo<FieldControl>(() => ({
    anchor: setAnchor,
    pulse: kind => fieldRef.current?.pulse(kind),
    setBusy: setFieldBusy,
    setWords: words => setFieldWords(current => current.join('\n') === words.join('\n') ? current : words),
  }), []);

  const check = useCallback(async () => {
    setState({ kind: 'checking' });
    try {
      const session = await getSession();
      setState(session === null ? { kind: 'signed-out' } : { kind: 'signed-in', session, status: null });
    } catch (error) {
      setState(error instanceof ConsoleRequestError && error.code === 'console_disabled'
        ? { kind: 'disabled' }
        : { kind: 'unavailable' });
    }
  }, []);

  useEffect(() => {
    void check();
  }, [check]);

  // The prompt becomes the terminal window: the browser morphs one into the other where it can.
  function enter(session: Session, status: ConsoleStatus | null) {
    // A view transition captures the page as soon as its callback returns, so the update must be synchronous.
    // eslint-disable-next-line react/dom-no-flush-sync
    const show = () => flushSync(() => setState({ kind: 'signed-in', session, status }));
    if (typeof document.startViewTransition === 'function' && !prefersReducedMotion()) {
      document.startViewTransition(show);
    } else {
      show();
    }
  }

  let view: ReactNode;
  switch (state.kind) {
    case 'checking':
      view = (
        <ConsoleFrame>
          <div aria-busy='true'>
            <Spinner />
            <span className='sr-only'>{t('console.checking')}</span>
          </div>
        </ConsoleFrame>
      );
      break;
    case 'signed-out':
      view = <LoginPage field={field} onSignedIn={enter} />;
      break;
    case 'signed-in':
      view = (
        <StatusPage
          field={field}
          initialStatus={state.status}
          session={state.session}
          onSignedOut={() => void check()}
        />
      );
      break;
    default:
      view = (
        <ConsoleFrame>
          <Empty className='max-w-sm flex-none rounded-[var(--radius-card)] border border-solid bg-card p-8 shadow-[var(--shadow-card)]' ref={field.anchor}>
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

  // One field behind every view, so sign-in and the signed-in view share it.
  // The wrapper isolates it above the page background.
  return (
    <div className='relative isolate'>
      <ConsoleField
        anchor={anchor}
        busy={fieldBusy}
        mode={state.kind === 'signed-in' ? 'calm' : 'interactive'}
        ref={fieldRef}
        words={fieldWords}
      />
      {view}
    </div>
  );
}
