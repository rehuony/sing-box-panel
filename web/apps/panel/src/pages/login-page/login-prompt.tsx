import type { KeyboardEvent, ReactNode, Ref } from 'react';

import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { PromptInput } from '@shared/components/prompt-input';
import { Check, CornerDownLeft, Eye, EyeOff, X } from 'lucide-react';

export type PromptPhase = 'idle' | 'submitting' | 'success';
export type PromptLine = 'email' | 'password';

interface LoginPromptProps {
  email: string;
  /** Changes on every refused attempt, which shakes the prompt once. */
  attempt: number;
  /** Why the last attempt was refused; empty while there is nothing to report. */
  message: string;
  password: string;
  phase: PromptPhase;
  ref?: Ref<HTMLDivElement>;
  /** The line the refusal is about, or null when it is about both. */
  invalid: PromptLine | null;
  emailRef?: Ref<HTMLInputElement>;
  passwordRef?: Ref<HTMLInputElement>;
  onEmailChange: (value: string) => void;
  onPasswordChange: (value: string) => void;
  /** Enter on the email line moves on to the password unless both are filled. */
  onEmailEnter: (event: KeyboardEvent<HTMLInputElement>) => void;
}

const MESSAGE_ID = 'login-error';

/** One line of the prompt: its label in the data face, then the input. */
function Line({ children, label, success }: { children: ReactNode; label: string; success: boolean }) {
  return (
    <div className='flex h-12 items-center gap-3 pr-2 pl-4'>
      <span aria-hidden='true' className={cn('w-[9ch] shrink-0 text-base select-none', success ? 'text-[var(--color-success)]' : 'text-primary')}>
        {label}
      </span>
      {children}
    </div>
  );
}

/**
 * The panel sign-in as a terminal login: a `login:` line for the email and a
 * `password:` line with its visibility toggle and submit. A refusal prints
 * below as the prompt's output row, outside the flow so the prompt stays put.
 */
export function LoginPrompt({
  attempt,
  email,
  emailRef,
  invalid,
  message,
  onEmailChange,
  onEmailEnter,
  onPasswordChange,
  password,
  passwordRef,
  phase,
  ref,
}: LoginPromptProps) {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);
  const [shaking, setShaking] = useState(false);
  const [shookFor, setShookFor] = useState(attempt);
  if (attempt !== shookFor) {
    setShookFor(attempt);
    setShaking(true);
  }
  const busy = phase !== 'idle';
  const success = phase === 'success';
  const refused = message !== '';
  const describes = (line: PromptLine) => refused && (invalid === null || invalid === line) ? MESSAGE_ID : undefined;

  return (
    <div
      className={cn('relative w-full font-[family-name:var(--font-data)]', shaking && 'prompt-shake')}
      onAnimationEnd={(event) => {
        if (event.animationName === 'prompt-shake') setShaking(false);
      }}
      ref={ref}
    >
      <div
        className={cn(
          'divide-y rounded-[var(--radius-card)] border bg-card shadow-[var(--shadow-card)] transition-[border-color,box-shadow] duration-[var(--dur-short)] ease-out',
          refused
            ? 'rounded-b-none border-[var(--color-danger-border)] focus-within:border-[var(--color-danger)]'
            : success
              ? 'border-[var(--color-success-border)] focus-within:border-[var(--color-success)] focus-within:ring-3 focus-within:ring-[var(--color-success-soft)]'
              : 'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/25',
        )}
      >
        <Line label='login:' success={success}>
          <PromptInput
            aria-describedby={describes('email')}
            aria-invalid={invalid === 'email' || undefined}
            aria-label={t('login.email.label')}
            autoCapitalize='none'
            autoComplete='username'
            autoCorrect='off'
            // The page takes nothing else, so it opens on its first line.
            autoFocus
            busy={busy}
            enterKeyHint='next'
            inputMode='email'
            masked={false}
            maxLength={254}
            name='email'
            onChange={onEmailChange}
            onKeyDown={(event) => {
              if (event.key === 'Enter') onEmailEnter(event);
            }}
            placeholder={t('login.email.placeholder')}
            ref={emailRef}
            value={email}
          />
        </Line>
        <Line label='password:' success={success}>
          <PromptInput
            aria-describedby={describes('password')}
            aria-invalid={invalid === 'password' || undefined}
            aria-label={t('login.password.label')}
            autoComplete='current-password'
            busy={busy}
            enterKeyHint='go'
            masked={!visible}
            name='password'
            onChange={onPasswordChange}
            placeholder={t('login.password.placeholder')}
            ref={passwordRef}
            value={password}
          />
          <Button
            aria-label={t(visible ? 'login.password.hide' : 'login.password.show')}
            aria-pressed={visible}
            className='shrink-0'
            onClick={() => setVisible(current => !current)}
            // Keep typing where it was: the toggle never takes focus from the line.
            onPointerDown={event => event.preventDefault()}
            size='icon-sm'
            type='button'
            variant='ghost'
          >
            {visible ? <EyeOff aria-hidden='true' /> : <Eye aria-hidden='true' />}
          </Button>
          <Button aria-label={t(phase === 'submitting' ? 'login.submit.pending' : 'login.submit.label')} className='shrink-0' size='icon-sm' type='submit'>
            {phase === 'submitting' ? <Spinner /> : success ? <Check aria-hidden='true' /> : <CornerDownLeft aria-hidden='true' />}
          </Button>
        </Line>
      </div>
      {/* The live region stays in place; the row inside it prints the refusal. */}
      <div className='absolute inset-x-0 top-full' id={MESSAGE_ID} role='alert'>
        {refused
          ? (
              <p className='prompt-output flex items-start gap-2 rounded-b-[var(--radius-card)] border border-t-0 border-[var(--color-danger-border)] bg-card px-4 py-2.5 text-sm text-[var(--color-danger-text)] shadow-[var(--shadow-card)]'>
                <X aria-hidden='true' className='mt-0.5 size-4 shrink-0' strokeWidth={2.5} />
                <span className='min-w-0'>{message}</span>
              </p>
            )
          : null}
      </div>
    </div>
  );
}
