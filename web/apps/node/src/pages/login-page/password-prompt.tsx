import type { Ref } from 'react';

import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { PromptInput } from '@shared/components/prompt-input';
import { Check, CornerDownLeft, Eye, EyeOff, X } from 'lucide-react';

export type PromptPhase = 'idle' | 'submitting' | 'success';

interface PasswordPromptProps {
  value: string;
  /** Changes on every refused attempt, which shakes the prompt once. */
  attempt: number;
  /** Why the last attempt was refused; empty while there is nothing to report. */
  message: string;
  messageId: string;
  phase: PromptPhase;
  ref?: Ref<HTMLDivElement>;
  onChange: (value: string) => void;
}

/**
 * The console password as a terminal prompt: `$`, the masked line, then the
 * visibility toggle and submit. A refusal prints below the prompt as its
 * output row, outside the flow so the prompt stays put.
 */
export function PasswordPrompt({ attempt, message, messageId, onChange, phase, ref, value }: PasswordPromptProps) {
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
  const invalid = message !== '';

  return (
    <div
      // Shares its view-transition name with the terminal window it becomes.
      className={cn('relative w-full font-[family-name:var(--font-data)] [view-transition-name:console-window]', shaking && 'prompt-shake')}
      onAnimationEnd={(event) => {
        if (event.animationName === 'prompt-shake') setShaking(false);
      }}
      ref={ref}
    >
      <div
        className={cn(
          'flex h-14 items-center gap-3 rounded-[var(--radius-card)] border bg-card pr-2 pl-4 shadow-[var(--shadow-card)] transition-[border-color,box-shadow] duration-[var(--dur-short)] ease-out',
          invalid
            ? 'rounded-b-none border-[var(--color-danger-border)] focus-within:border-[var(--color-danger)]'
            : success
              ? 'border-[var(--color-success-border)] focus-within:border-[var(--color-success)] focus-within:ring-3 focus-within:ring-[var(--color-success-soft)]'
              : 'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/25',
        )}
      >
        <span aria-hidden='true' className={cn('shrink-0 text-base select-none', success ? 'text-[var(--color-success)]' : 'text-primary')}>
          {success ? <Check className='size-4' /> : '$'}
        </span>
        <PromptInput
          aria-describedby={messageId}
          aria-invalid={invalid || undefined}
          aria-label={t('login.password')}
          autoComplete='current-password'
          // The page takes nothing else, so it opens on its password.
          autoFocus
          busy={busy}
          masked={!visible}
          onChange={onChange}
          placeholder={t('login.password')}
          value={value}
        />
        <Button
          aria-label={t(visible ? 'login.hide' : 'login.show')}
          aria-pressed={visible}
          className='shrink-0'
          onClick={() => setVisible(current => !current)}
          // Keep typing where it was: the toggle never takes focus from the password.
          onPointerDown={event => event.preventDefault()}
          size='icon-sm'
          type='button'
          variant='ghost'
        >
          {visible ? <EyeOff aria-hidden='true' /> : <Eye aria-hidden='true' />}
        </Button>
        <Button aria-label={t('login.submit')} className='shrink-0' size='icon-sm' type='submit'>
          {phase === 'submitting' ? <Spinner /> : success ? <Check aria-hidden='true' /> : <CornerDownLeft aria-hidden='true' />}
        </Button>
      </div>
      {/* The live region stays in place; the row inside it prints the refusal. */}
      <div className='absolute inset-x-0 top-full' id={messageId} role='alert'>
        {invalid
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
