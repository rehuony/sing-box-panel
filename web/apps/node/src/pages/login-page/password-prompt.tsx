import type { Ref } from 'react';

import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { PromptInput } from '@shared/components/prompt-input';
import { Check, CornerDownLeft, Eye, EyeOff } from 'lucide-react';

export type PromptPhase = 'idle' | 'submitting' | 'success';

interface PasswordPromptProps {
  value: string;
  /** Changes on every refused attempt, which shakes the prompt once. */
  attempt: number;
  /** The last attempt was refused; the prompt stays marked until typing resumes. */
  refused: boolean;
  phase: PromptPhase;
  ref?: Ref<HTMLDivElement>;
  onChange: (value: string) => void;
}

/**
 * The console password as a terminal prompt: `$`, the masked line, then the
 * visibility toggle and submit. A refusal shakes the prompt and marks it in
 * danger; the page reports why in a toast.
 */
export function PasswordPrompt({ attempt, onChange, phase, ref, refused, value }: PasswordPromptProps) {
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
          refused
            ? 'border-[var(--color-danger-border)] focus-within:border-[var(--color-danger)]'
            : success
              ? 'border-[var(--color-success-border)] focus-within:border-[var(--color-success)] focus-within:ring-3 focus-within:ring-[var(--color-success-soft)]'
              : 'focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/25',
        )}
      >
        <span aria-hidden='true' className={cn('shrink-0 text-base select-none', success ? 'text-[var(--color-success)]' : 'text-primary')}>
          {success ? <Check className='size-4' /> : '$'}
        </span>
        <PromptInput
          aria-invalid={refused || undefined}
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
    </div>
  );
}
