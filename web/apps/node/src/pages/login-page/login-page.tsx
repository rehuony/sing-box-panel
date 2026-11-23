import type { FormEvent } from 'react';
import type { FieldControl } from '@shared/components/glyph-field';

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from '@shared/components/ui/toast-manager';

import type { ConsoleStatus, Session } from '@/api/generated';

import { ConsoleFrame } from '@/components/console-frame';
import { ConsoleRequestError, getStatus, login } from '@/api/console-client';

import type { PromptPhase } from './password-prompt';

import { PasswordPrompt } from './password-prompt';
import './login-page.css';

/** How long a sign-in shows its success before the console opens. */
const SUCCESS_HOLD_MS = 520;

/** One toast reports every refused attempt, so retries replace it instead of stacking. */
const REFUSAL_TOAST = 'console-sign-in-refused';

interface LoginPageProps {
  field: FieldControl;
  /** Receives the session and, when it could be read during the success, the node's status. */
  onSignedIn: (session: Session, status: ConsoleStatus | null) => void;
}

export function LoginPage({ field, onSignedIn }: LoginPageProps) {
  const { t } = useTranslation();
  const [password, setPassword] = useState('');
  const [refused, setRefused] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [phase, setPhase] = useState<PromptPhase>('idle');

  useEffect(() => {
    field.setBusy(phase === 'submitting');
    return () => field.setBusy(false);
  }, [field, phase]);

  useEffect(() => () => toast.close(REFUSAL_TOAST), []);

  function refuse(message: string) {
    setRefused(true);
    setAttempt(current => current + 1);
    field.pulse('error');
    toast.add({ id: REFUSAL_TOAST, priority: 'high', title: message, type: 'error' });
  }

  /** Typing again, or trying again, ends the last refusal. */
  function clear() {
    if (!refused) return;
    setRefused(false);
    toast.close(REFUSAL_TOAST);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (phase !== 'idle') return;
    if (!password) {
      refuse(t('login.error.empty'));
      return;
    }
    setPhase('submitting');
    clear();
    let session: Session;
    try {
      session = await login(password);
    } catch (reason) {
      setPhase('idle');
      refuse(reason instanceof ConsoleRequestError && reason.status === 401
        ? t('login.error.unauthorized')
        : reason instanceof ConsoleRequestError && reason.status === 429
          ? t('login.error.rateLimited')
          : t('login.error.unreachable'));
      return;
    }
    // The dots stay through the success; the page unmounts with them. The
    // status loads meanwhile, so the console opens on the node, not a spinner.
    setPhase('success');
    field.pulse('success');
    const hold = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : SUCCESS_HOLD_MS;
    const [status] = await Promise.all([
      getStatus().catch(() => null),
      new Promise(resolve => window.setTimeout(resolve, hold)),
    ]);
    onSignedIn(session, status);
  }

  return (
    <ConsoleFrame>
      <h1 className='sr-only' id='console-login-title'>{t('login.title')}</h1>
      <form aria-labelledby='console-login-title' className='w-full max-w-md' noValidate onSubmit={event => void handleSubmit(event)}>
        <PasswordPrompt
          attempt={attempt}
          onChange={(value) => {
            if (value.length > password.length) field.pulse('key');
            setPassword(value);
            clear();
          }}
          phase={phase}
          ref={field.anchor}
          refused={refused}
          value={password}
        />
      </form>
    </ConsoleFrame>
  );
}
