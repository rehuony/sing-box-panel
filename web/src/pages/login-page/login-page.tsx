import type { FormEvent } from 'react';

import { Eye, EyeOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { ApiRequestError } from '@/api/api-client';
import { PanelLogo } from '@/components/panel-logo';
import { toast } from '@/components/ui/toast-manager';
import { LoadingState } from '@/components/loading-state';
import { useAuthSession } from '@/stores/auth-session.store';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';

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
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState('');
  const [invalidField, setInvalidField] = useState<'email' | 'password' | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const controllerRef = useRef<AbortController | null>(null);
  const toastIdRef = useRef<string | null>(null);
  const emailRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const returnTarget = safeReturnTarget((location.state as LoginLocationState | null)?.from);

  useEffect(() => () => {
    controllerRef.current?.abort();
    if (toastIdRef.current !== null) toast.close(toastIdRef.current);
  }, []);

  if (status === 'authenticated') return <Navigate replace to={returnTarget} />;
  if (status === 'checking') {
    return (
      <main className='login-page' aria-busy='true'>
        <section className='login-card login-card--status'>
          <div className='login-card__brand'><PanelLogo compact /></div>
          <LoadingState fullScreen={false} label={t('login.checking')} />
        </section>
      </main>
    );
  }
  if (status === 'unavailable') {
    return (
      <main className='login-page'>
        <section className='login-card login-card--unavailable'>
          <header className='login-card__brand'>
            <PanelLogo compact />
            <h1>{t('login.unavailable.title')}</h1>
            <p className='login-card__description' role='alert'>{t('login.unavailable.description')}</p>
          </header>
          <Button onClick={retrySession} type='button'>{t('login.unavailable.retry')}</Button>
        </section>
      </main>
    );
  }

  function clearError() {
    setError('');
    setInvalidField(null);
    if (toastIdRef.current !== null) {
      toast.close(toastIdRef.current);
      toastIdRef.current = null;
    }
  }

  function reportError(message: string, field: 'email' | 'password' | null = null) {
    clearError();
    setError(message);
    setInvalidField(field);
    toastIdRef.current = toast.add({ title: message, type: 'error' });
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (controllerRef.current !== null) return;
    const normalizedEmail = email.trim().toLowerCase();
    // The server validates addresses, including Unicode addresses that native
    // HTML email validation rejects. Only require a value before submitting.
    if (!normalizedEmail) {
      reportError(t('login.error.email'), 'email');
      emailRef.current?.focus();
      return;
    }
    if (!password) {
      reportError(t('login.error.empty'), 'password');
      passwordRef.current?.focus();
      return;
    }
    const controller = new AbortController();
    controllerRef.current = controller;
    clearError();
    setIsSubmitting(true);
    try {
      await login({ email: normalizedEmail, password }, controller.signal);
      setPassword('');
      navigate(returnTarget, { replace: true });
    } catch (reason) {
      if (controller.signal.aborted) return;
      reportError(reason instanceof ApiRequestError && reason.status === 401
        ? t('login.error.unauthorized')
        : reason instanceof ApiRequestError && reason.status === 429
          ? t('login.error.rateLimited')
          : t('login.error.unreachable'));
    } finally {
      if (!controller.signal.aborted) setIsSubmitting(false);
      controllerRef.current = null;
    }
  }

  return (
    <main className='login-page'>
      <section className='login-card' aria-labelledby='login-title'>
        <header className='login-card__brand'>
          <PanelLogo compact />
          <h1 id='login-title'>{t('login.title')}</h1>
        </header>
        <form noValidate onSubmit={handleSubmit} aria-busy={isSubmitting}>
          {error && <p className='sr-only' id='login-error'>{error}</p>}
          <FieldGroup>
            <Field className='login-card__input-field' data-invalid={invalidField === 'email'} data-disabled={isSubmitting}>
              <FieldLabel htmlFor='login-email'>{t('login.email.label')}</FieldLabel>
              <Input ref={emailRef} id='login-email' name='email' type='text' inputMode='email' autoComplete='username' autoCapitalize='none' autoCorrect='off' spellCheck={false} required maxLength={254} autoFocus disabled={isSubmitting} placeholder={t('login.email.placeholder')} value={email} onChange={(event) => {
                setEmail(event.target.value);
                clearError();
              }} aria-invalid={invalidField === 'email'} aria-describedby={error && invalidField !== 'password' ? 'login-error' : undefined} />
            </Field>
            <Field className='login-card__input-field' data-invalid={invalidField === 'password'} data-disabled={isSubmitting}>
              <FieldLabel htmlFor='login-password'>{t('login.password.label')}</FieldLabel>
              <InputGroup>
                <InputGroupInput ref={passwordRef} id='login-password' name='password' type={showPassword ? 'text' : 'password'} autoComplete='current-password' required disabled={isSubmitting} placeholder={t('login.password.placeholder')} value={password} onChange={(event) => {
                  setPassword(event.target.value);
                  clearError();
                }} aria-invalid={invalidField === 'password'} aria-describedby={error && invalidField !== 'email' ? 'login-error' : undefined} />
                <InputGroupAddon align='inline-end'>
                  <InputGroupButton size='icon-sm' aria-label={t(showPassword ? 'login.password.hide' : 'login.password.show')} aria-pressed={showPassword} disabled={isSubmitting} onClick={() => setShowPassword(current => !current)}>
                    {showPassword ? <EyeOff /> : <Eye />}
                  </InputGroupButton>
                </InputGroupAddon>
              </InputGroup>
            </Field>
          </FieldGroup>
          <Button className='login-card__submit' disabled={isSubmitting} size='lg' type='submit'>
            {isSubmitting && <Spinner data-icon='inline-start' />}
            {t(isSubmitting ? 'login.submit.pending' : 'login.submit.label')}
          </Button>
        </form>
      </section>
    </main>
  );
}
