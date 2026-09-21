import type { FormEvent } from 'react';

import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { ApiRequestError } from '@/api/api-client';
import { ErrorNotice } from '@/components/error-notice';
import { useAuthSession } from '@/stores/auth-session.store';

import './login-page.css';

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

export function LoginPage() {
  const { t } = useTranslation();
  const { login, retrySession, status } = useAuthSession();
  const location = useLocation();
  const navigate = useNavigate();
  const [token, setToken] = useState('');
  const [error, setError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const controllerRef = useRef<AbortController | null>(null);
  const locationState = location.state as LoginLocationState | null;
  const returnTarget = safeReturnTarget(locationState?.from);

  useEffect(() => () => controllerRef.current?.abort(), []);

  if (status === 'authenticated') return <Navigate replace to={returnTarget} />;

  if (status === 'checking') {
    return (
      <main className='loading-screen' aria-busy='true' aria-live='polite'>
        <span aria-hidden='true' className='loading-screen__mark' />
        <p>{t('login.checking', { defaultValue: 'Checking panel session…' })}</p>
      </main>
    );
  }

  if (status === 'unavailable') {
    return (
      <main className='loading-screen'>
        <div className='load-error'>
          <ErrorNotice title={t('login.unavailable.title', { defaultValue: 'The panel service could not be reached.' })} error={t('login.unavailable.description', { defaultValue: 'Your session has not changed. Check the server and try again.' })} />
          <Button onClick={retrySession} type='button'>
            {t('login.unavailable.retry', { defaultValue: 'Try again' })}
          </Button>
        </div>
      </main>
    );
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalizedToken = token.trim();
    if (normalizedToken === '') {
      setError(t('login.error.empty', { defaultValue: 'Enter the management token to continue.' }));
      return;
    }

    const controller = new AbortController();
    controllerRef.current = controller;
    setError('');
    setIsSubmitting(true);
    try {
      await login(normalizedToken, controller.signal);
      navigate(returnTarget, { replace: true });
    } catch (loginError) {
      if (loginError instanceof DOMException && loginError.name === 'AbortError') return;
      setError(loginError instanceof ApiRequestError && loginError.status === 401
        ? t('login.error.unauthorized', { defaultValue: 'That management token was not accepted.' })
        : t('login.error.unreachable', { defaultValue: 'The panel could not be reached. Try again.' }));
    } finally {
      if (!controller.signal.aborted) setIsSubmitting(false);
    }
  }

  return (
    <main className='login-page'>
      <section className='login-card' aria-labelledby='login-title'>
        <h1 id='login-title'>{t('login.title')}</h1>

        <form noValidate onSubmit={handleSubmit}>
          <div className='field-group'>
            <label className='sr-only' htmlFor='management-token'>
              {t('login.token.label', { defaultValue: 'Management token' })}
            </label>
            <Input
              aria-describedby={error === '' ? undefined : 'management-token-error'}
              aria-invalid={error !== ''}
              autoComplete='current-password'
              autoFocus
              disabled={isSubmitting}
              id='management-token'
              name='management-token'
              onChange={(event) => setToken(event.target.value)}
              placeholder={t('login.token.placeholder', { defaultValue: 'Enter token' })}
              spellCheck={false}
              type='password'
              value={token}
            />
            {error === ''
              ? null
              : (
                  <ErrorNotice id='management-token-error' error={error} />
                )}
          </div>
          <Button className='login-card__submit' disabled={isSubmitting} size='lg' type='submit'>
            {isSubmitting
              ? t('login.submit.pending', { defaultValue: 'Opening…' })
              : t('login.submit.label', { defaultValue: 'Open panel' })}
          </Button>
        </form>
      </section>
    </main>
  );
}
