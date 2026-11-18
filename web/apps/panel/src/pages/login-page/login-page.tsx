import type { FormEvent, ReactNode } from 'react';

import { Unplug } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { toast } from '@shared/components/ui/toast-manager';
import { LanguageMenu } from '@shared/components/language-menu';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import { ThemeCycleButton } from '@shared/components/theme-cycle-button';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import { useTheme } from '@/theme';
import { ApiRequestError } from '@/api/api-client';
import { useBackdrop } from '@/components/backdrop';
import { PanelLogo } from '@/components/panel-logo';
import { useAuthSession } from '@/stores/auth-session.store';

import type { PromptLine, PromptPhase } from './login-prompt';

import { LoginPrompt } from './login-prompt';

/** How long a sign-in shows its success before the panel opens. */
const SUCCESS_HOLD_MS = 520;

/** One toast reports every refused attempt, so retries replace it instead of stacking. */
const REFUSAL_TOAST = 'sign-in-refused';

interface LoginLocationState {
  from?: string;
}

function safeReturnTarget(value: unknown): string {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) return '/';
  try {
    const target = new URL(value, window.location.origin);
    return target.origin === window.location.origin
      ? `${target.pathname}${target.search}${target.hash}`
      : '/';
  } catch {
    return '/';
  }
}

/** A first row across the window, then the view centred below it. */
function SignInFrame({ busy = false, children }: { busy?: boolean; children: ReactNode }) {
  const { cycleTheme, preference } = useTheme();
  return (
    <div className='flex min-h-svh flex-col'>
      <header className='flex h-16 shrink-0 items-center gap-3 px-4 sm:px-6'>
        <PanelLogo />
        <div className='ms-auto flex shrink-0 items-center gap-1'>
          <ThemeCycleButton onCycle={cycleTheme} preference={preference} />
          <LanguageMenu />
        </div>
      </header>
      {/* The bottom inset matches the first row, so the view sits at the window's centre. */}
      <main aria-busy={busy || undefined} className='flex min-w-0 flex-1 flex-col items-center justify-center px-4 pb-16 outline-none sm:px-6'>
        {children}
      </main>
    </div>
  );
}

export function LoginPage() {
  const { t } = useTranslation();
  const { login, retrySession, status } = useAuthSession();
  const field = useBackdrop();
  const location = useLocation();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [refused, setRefused] = useState(false);
  const [invalid, setInvalid] = useState<PromptLine | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [phase, setPhase] = useState<PromptPhase>('idle');
  const controllerRef = useRef<AbortController | null>(null);
  const emailRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const returnTarget = safeReturnTarget((location.state as LoginLocationState | null)?.from);

  useEffect(() => () => {
    controllerRef.current?.abort();
    toast.close(REFUSAL_TOAST);
  }, []);

  useEffect(() => {
    field.setBusy(phase === 'submitting');
    return () => field.setBusy(false);
  }, [field, phase]);

  // A sign-in in progress holds the prompt through its success before the panel opens.
  if (status === 'authenticated' && phase === 'idle') return <Navigate replace to={returnTarget} />;

  if (status === 'checking') {
    return (
      <SignInFrame busy>
        <Spinner />
        <span className='sr-only'>{t('login.checking')}</span>
      </SignInFrame>
    );
  }

  if (status === 'unavailable') {
    return (
      <SignInFrame>
        <Empty className='max-w-sm flex-none rounded-[var(--radius-card)] border border-solid bg-[var(--color-surface-translucent)] p-8 shadow-[var(--shadow-card)]' ref={field.anchor}>
          <EmptyHeader>
            <EmptyMedia className='mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6'>
              <Unplug aria-hidden='true' strokeWidth={1.5} />
            </EmptyMedia>
            <EmptyTitle><h1>{t('login.unavailable.title')}</h1></EmptyTitle>
            <EmptyDescription role='alert'>{t('login.unavailable.description')}</EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Button onClick={retrySession} variant='outline'>{t('login.unavailable.retry')}</Button>
          </EmptyContent>
        </Empty>
      </SignInFrame>
    );
  }

  function refuse(text: string, line: PromptLine | null) {
    setRefused(true);
    setInvalid(line);
    toast.add({ id: REFUSAL_TOAST, priority: 'high', title: text, type: 'error' });
    setAttempt(current => current + 1);
    field.pulse('error');
    if (line === 'email') emailRef.current?.focus();
    if (line === 'password') passwordRef.current?.focus();
  }

  /** Typing again, or trying again, ends the last refusal. */
  function clear() {
    if (!refused) return;
    setRefused(false);
    setInvalid(null);
    toast.close(REFUSAL_TOAST);
  }

  async function submit() {
    if (phase !== 'idle') return;
    const normalizedEmail = email.trim().toLowerCase();
    // The server validates addresses, including Unicode addresses that native
    // HTML email validation rejects. Only require a value before submitting.
    if (!normalizedEmail) {
      refuse(t('login.error.email'), 'email');
      return;
    }
    if (!password) {
      refuse(t('login.error.empty'), 'password');
      return;
    }
    const controller = new AbortController();
    controllerRef.current = controller;
    clear();
    setPhase('submitting');
    try {
      await login({ email: normalizedEmail, password }, controller.signal);
    } catch (reason) {
      if (controller.signal.aborted) return;
      setPhase('idle');
      refuse(reason instanceof ApiRequestError && reason.status === 401
        ? t('login.error.unauthorized')
        : reason instanceof ApiRequestError && reason.status === 429
          ? t('login.error.rateLimited')
          : t('login.error.unreachable'), null);
      return;
    } finally {
      controllerRef.current = null;
    }
    setPhase('success');
    field.pulse('success');
    const hold = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : SUCCESS_HOLD_MS;
    await new Promise(resolve => window.setTimeout(resolve, hold));
    if (controller.signal.aborted) return;
    // The browser cross-fades from the prompt to the panel where it can.
    void navigate(returnTarget, { replace: true, viewTransition: true });
  }

  return (
    <SignInFrame>
      <h1 className='sr-only' id='login-title'>{t('login.title')}</h1>
      <form
        aria-busy={phase === 'submitting' || undefined}
        aria-labelledby='login-title'
        className='w-full max-w-md'
        noValidate
        onSubmit={(event: FormEvent<HTMLFormElement>) => {
          event.preventDefault();
          void submit();
        }}
      >
        <LoginPrompt
          attempt={attempt}
          email={email}
          emailRef={emailRef}
          invalid={invalid}
          onEmailChange={(value) => {
            if (value.length > email.length) field.pulse('key');
            setEmail(value);
            clear();
          }}
          onEmailEnter={(event) => {
            event.preventDefault();
            if (email.trim() !== '' && password === '') passwordRef.current?.focus();
            else void submit();
          }}
          onPasswordChange={(value) => {
            if (value.length > password.length) field.pulse('key');
            setPassword(value);
            clear();
          }}
          password={password}
          passwordRef={passwordRef}
          phase={phase}
          ref={field.anchor}
          refused={refused}
        />
      </form>
    </SignInFrame>
  );
}
