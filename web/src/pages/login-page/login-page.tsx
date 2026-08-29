import type { FormEvent } from 'react';

import { useEffect, useRef, useState } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';

import { ApiRequestError } from '@/api/api-client';
import { PanelLogo } from '@/components/panel-logo';
import { useAuthSession } from '@/stores/auth-session.store';

import './login-page.css';

interface LoginLocationState {
  from?: string;
}

function describeLoginError(error: unknown): string {
  if (error instanceof ApiRequestError && error.status === 401) {
    return 'That management token was not accepted. Check the token and try again.';
  }

  if (error instanceof DOMException && error.name === 'AbortError') {
    return '';
  }

  return 'The panel could not be reached. Confirm the service is running, then try again.';
}

export function LoginPage() {
  const { login, retrySession, status } = useAuthSession();
  const location = useLocation();
  const navigate = useNavigate();
  const [token, setToken] = useState('');
  const [error, setError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const errorRef = useRef<HTMLDivElement>(null);
  const controllerRef = useRef<AbortController | null>(null);

  useEffect(
    () => () => {
      controllerRef.current?.abort();
    },
    [],
  );

  useEffect(() => {
    if (error.length > 0) {
      errorRef.current?.focus();
    }
  }, [error]);

  if (status === 'authenticated') {
    return <Navigate replace to='/' />;
  }

  if (status === 'checking') {
    return (
      <main className='loading-screen' aria-busy='true' aria-live='polite'>
        <span aria-hidden='true' className='loading-screen__mark' />
        <p>Checking panel session…</p>
      </main>
    );
  }

  if (status === 'unavailable') {
    return (
      <main className='loading-screen'>
        <div className='load-error' role='alert'>
          <p className='eyebrow'>Service unavailable</p>
          <h1>The panel service could not be reached.</h1>
          <p>Your session has not been changed. Check the server and try again.</p>
          <button className='button button--primary' onClick={retrySession} type='button'>Try again</button>
        </div>
      </main>
    );
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const normalizedToken = token.trim();
    if (normalizedToken.length === 0) {
      setError('Enter the management token to continue.');
      return;
    }

    const controller = new AbortController();
    controllerRef.current = controller;
    setError('');
    setIsSubmitting(true);

    try {
      await login(normalizedToken, controller.signal);
      const state = location.state as LoginLocationState | null;
      navigate(state?.from ?? '/', { replace: true });
    } catch (loginError) {
      const message = describeLoginError(loginError);
      setError(message);
    } finally {
      if (!controller.signal.aborted) {
        setIsSubmitting(false);
      }
    }
  }

  return (
    <main className='login-page'>
      <section className='login-page__identity' aria-labelledby='login-title'>
        <PanelLogo />
        <div className='login-page__statement'>
          <p className='eyebrow'>Local operations console</p>
          <h1 id='login-title'>Control the exact bytes that run.</h1>
          <p>
            One canonical configuration, projected deliberately to the selected
            sing-box core and pinned to the artifact you apply.
          </p>
        </div>

        <ol className='login-trace' aria-label='Panel configuration flow'>
          <li>
            <span>01</span>
            <strong>View</strong>
          </li>
          <li>
            <span>02</span>
            <strong>Revision</strong>
          </li>
          <li>
            <span>03</span>
            <strong>Artifact</strong>
          </li>
          <li>
            <span>04</span>
            <strong>Apply</strong>
          </li>
        </ol>
      </section>

      <section className='login-page__access' aria-labelledby='access-title'>
        <form className='login-card' noValidate onSubmit={handleSubmit}>
          <div>
            <p className='eyebrow'>Protected access</p>
            <h2 id='access-title'>Unlock this panel</h2>
            <p className='login-card__intro'>
              Use the management token generated during panel initialization.
            </p>
          </div>

          <div className='field-group'>
            <label htmlFor='management-token'>Management token</label>
            <input
              aria-describedby={
                error.length > 0
                  ? 'management-token-error management-token-hint'
                  : 'management-token-hint'
              }
              aria-invalid={error.length > 0}
              autoComplete='current-password'
              autoFocus
              disabled={isSubmitting}
              id='management-token'
              name='management-token'
              onChange={(event) => setToken(event.target.value)}
              spellCheck={false}
              type='password'
              value={token}
            />
            {error.length > 0
              ? (
                  <div
                    className='form-error'
                    id='management-token-error'
                    ref={errorRef}
                    role='alert'
                    tabIndex={-1}
                  >
                    <strong>Access was not opened</strong>
                    <span>{error}</span>
                  </div>
                )
              : null}
            <span id='management-token-hint'>
              The token stays on this device and is sent only to this panel.
            </span>
          </div>

          <button
            className='button button--primary button--wide'
            disabled={isSubmitting}
            type='submit'
          >
            {isSubmitting ? 'Opening console…' : 'Open console'}
          </button>

          <p className='login-card__footnote'>
            Password managers and pasted tokens are supported.
          </p>
        </form>
      </section>
    </main>
  );
}
