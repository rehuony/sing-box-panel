import type { KeyboardEvent, ReactNode, Ref } from 'react';

import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { PromptInput } from '@shared/components/prompt-input';
import { Check, CornerDownLeft, Eye, EyeOff } from 'lucide-react';

export type PromptPhase = 'idle' | 'submitting' | 'success';
export type PromptLine = 'email' | 'password';

interface LoginPromptProps {
  email: string;
  /** Changes on every refused attempt, which shakes the prompt once. */
  attempt: number;
  password: string;
  /** The last attempt was refused; the prompt stays marked until typing resumes. */
  refused: boolean;
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
 * `password:` line with its visibility toggle and submit. A refusal shakes the
 * prompt and marks it in danger; the page reports why in a toast.
 */
export function LoginPrompt({
  attempt,
  email,
  emailRef,
  invalid,
  onEmailChange,
  onEmailEnter,
  onPasswordChange,
  password,
  passwordRef,
  phase,
  ref,
  refused,
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
            ? 'border-[var(--color-danger-border)] focus-within:border-[var(--color-danger)]'
            : success
              ? 'border-[var(--color-success-border)] focus-within:border-[var(--color-success)] focus-within:ring-3 focus-within:ring-[var(--color-success-soft)]'
              : 'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/25',
        )}
      >
        <Line label='login:' success={success}>
          <PromptInput
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
    </div>
  );
}
