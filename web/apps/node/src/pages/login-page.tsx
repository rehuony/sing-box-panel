import type { FormEvent } from 'react';

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { Field, FieldLabel } from '@shared/components/ui/field';

import type { Session } from '@/api/generated';

import { ConsoleFrame } from '@/components/console-frame';
import { ConsoleRequestError, login } from '@/api/console-client';

interface LoginPageProps {
  onSignedIn: (session: Session) => void;
}

export function LoginPage({ onSignedIn }: LoginPageProps) {
  const { t } = useTranslation();
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting) return;
    if (!password) {
      setError(t('login.error.empty'));
      return;
    }
    setSubmitting(true);
    setError('');
    try {
      const session = await login(password);
      setPassword('');
      onSignedIn(session);
    } catch (reason) {
      setError(reason instanceof ConsoleRequestError && reason.status === 401
        ? t('login.error.unauthorized')
        : reason instanceof ConsoleRequestError && reason.status === 429
          ? t('login.error.rateLimited')
          : t('login.error.unreachable'));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <ConsoleFrame>
      <section aria-labelledby='console-login-title' className='mx-auto mt-[min(12vh,6rem)] flex w-full max-w-sm flex-col gap-4 rounded-[var(--radius-card)] border bg-card p-5'>
        <h1 className='text-xl font-semibold' id='console-login-title'>{t('login.title')}</h1>
        <form className='flex flex-col gap-4' noValidate onSubmit={event => void handleSubmit(event)}>
          <Field>
            <FieldLabel htmlFor='console-password'>{t('login.password')}</FieldLabel>
            <Input
              id='console-password'
              type='password'
              autoComplete='current-password'
              value={password}
              aria-invalid={error ? true : undefined}
              aria-describedby='console-login-error'
              onChange={(event) => {
                setPassword(event.target.value);
                if (error) setError('');
              }}
            />
          </Field>
          <p id='console-login-error' className='min-h-5 text-sm text-[var(--color-danger-text)]' role='alert'>{error}</p>
          <Button type='submit'>
            {submitting && <Spinner data-icon='inline-start' />}
            {t('login.submit')}
          </Button>
        </form>
      </section>
    </ConsoleFrame>
  );
}
