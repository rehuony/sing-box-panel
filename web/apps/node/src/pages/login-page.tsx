import type { FormEvent } from 'react';

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { Field, FieldLabel } from '@shared/components/ui/field';
import { Card, CardContent, CardHeader, CardTitle } from '@shared/components/ui/card';

import type { Session } from '@/api/generated';

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
    <main className='flex min-h-svh items-center justify-center p-4'>
      <Card className='w-full max-w-sm'>
        <CardHeader>
          <CardTitle>
            <h1 className='text-xl font-semibold'>{t('login.title')}</h1>
          </CardTitle>
        </CardHeader>
        <CardContent>
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
                onChange={event => setPassword(event.target.value)}
              />
            </Field>
            <p id='console-login-error' className='min-h-5 text-sm text-destructive' role='alert'>{error}</p>
            <Button type='submit' aria-busy={submitting || undefined}>
              {submitting && <Spinner />}
              {t('login.submit')}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
