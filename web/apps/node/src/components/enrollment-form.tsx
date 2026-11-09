import type { FormEvent, ReactNode } from 'react';

import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Label } from '@shared/components/ui/label';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { Textarea } from '@shared/components/ui/textarea';

import type { ConsoleStatus } from '@/api/generated';

import { ConsoleRequestError, enroll } from '@/api/console-client';

interface EnrollmentFormProps {
  csrfToken: string;
  /** Actions before the submit action, such as cancel in a dialog. */
  actions?: ReactNode;
  submitLabel: string;
  /** Called when the console session ended while enrolling. */
  onSignedOut: () => void;
  /** Receives the node's status once the panel accepted the code. */
  onEnrolled: (status: ConsoleStatus) => void;
}

/** Takes the one-time enrollment code the panel shows and redeems it with that panel. */
export function EnrollmentForm({ actions, csrfToken, onEnrolled, onSignedOut, submitLabel }: EnrollmentFormProps) {
  const { t } = useTranslation();
  const id = useId();
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (submitting) return;
    const value = code.trim();
    if (value === '') {
      setError(t('enroll.error.empty'));
      return;
    }
    setSubmitting(true);
    setError('');
    try {
      const status = await enroll(value, csrfToken);
      setCode('');
      onEnrolled(status);
    } catch (reason) {
      if (reason instanceof ConsoleRequestError && (reason.status === 401 || reason.code === 'console_disabled')) {
        onSignedOut();
        return;
      }
      setError(reason instanceof ConsoleRequestError && reason.code === 'enrollment_rejected'
        ? t('enroll.error.rejected')
        : reason instanceof ConsoleRequestError && reason.code === 'enrollment_failed'
          ? t('enroll.error.unreachable')
          : reason instanceof ConsoleRequestError && (reason.code === 'enrollment_code_invalid' || reason.status === 400 || reason.status === 413)
            ? t('enroll.error.invalid')
            : t('enroll.error.failed'));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form className='flex flex-col gap-4' noValidate onSubmit={event => void submit(event)}>
      <div className='flex flex-col gap-2'>
        <Label htmlFor={`${id}-code`}>{t('enroll.code')}</Label>
        <Textarea
          aria-describedby={`${id}-error`}
          aria-invalid={error ? true : undefined}
          autoComplete='off'
          className='min-h-24 resize-none font-mono text-base break-all md:text-xs'
          id={`${id}-code`}
          maxLength={4200}
          onChange={(event) => {
            setCode(event.target.value);
            if (error) setError('');
          }}
          placeholder={t('enroll.placeholder')}
          rows={4}
          spellCheck={false}
          value={code}
        />
      </div>
      <div className='flex flex-wrap items-center justify-end gap-3'>
        <p className='min-h-5 min-w-0 flex-1 text-sm text-[var(--color-danger-text)]' id={`${id}-error`} role='alert'>{error}</p>
        {actions}
        <Button type='submit'>
          {submitting ? <Spinner data-icon='inline-start' /> : null}
          {submitLabel}
        </Button>
      </div>
    </form>
  );
}
