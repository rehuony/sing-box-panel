import type { Ref } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { Check, CornerDownLeft, Eye, EyeOff, X } from 'lucide-react';
import { useEffect, useLayoutEffect, useRef, useState } from 'react';

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

/** One mirror cell, in em; the input's letter spacing widens its characters to match. */
const CELL_EM = 0.75;
const PROBE_LENGTH = 40;

interface Spacing {
  text: number;
  password: number;
}

/** The width the browser gives one character of an input of type, in the font of reference. */
function nativeAdvance(reference: HTMLInputElement, type: 'password' | 'text'): number {
  const style = getComputedStyle(reference);
  const probe = document.createElement('input');
  probe.type = type;
  probe.tabIndex = -1;
  probe.value = '0'.repeat(PROBE_LENGTH);
  Object.assign(probe.style, {
    border: '0', fontFamily: style.fontFamily, fontSize: style.fontSize, fontWeight: style.fontWeight,
    left: '-10000px', letterSpacing: '0', padding: '0', position: 'fixed', top: '0', width: '1px',
  });
  document.body.append(probe);
  const advance = probe.scrollWidth / PROBE_LENGTH;
  probe.remove();
  return advance;
}

/** A UTF-16 offset of the input as an index into its code points, which the mirror draws. */
function codePointIndex(value: string, offset: number): number {
  return [...value.slice(0, offset)].length;
}

/**
 * The console password as a terminal prompt. The real input stays in place
 * and keeps the value, focus, selection and autofill, but draws nothing; a
 * mirror draws each character as a dot in a cell as wide as the input's
 * character, so pointer selection lands on the dots it covers. The mirror
 * draws the selection and the cursor too. A refusal prints below the prompt
 * as its output row, outside the flow so the prompt stays put.
 */
export function PasswordPrompt({ attempt, message, messageId, onChange, phase, ref, value }: PasswordPromptProps) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const mirrorRef = useRef<HTMLSpanElement>(null);
  const [visible, setVisible] = useState(false);
  const [focused, setFocused] = useState(false);
  const [selection, setSelection] = useState({ end: value.length, start: value.length });
  const [spacing, setSpacing] = useState<Spacing | null>(null);
  const [shaking, setShaking] = useState(false);
  const [shookFor, setShookFor] = useState(attempt);
  if (attempt !== shookFor) {
    setShookFor(attempt);
    setShaking(true);
  }

  // Autofocus in a window without focus fires no focus event; read the state once.
  useLayoutEffect(() => {
    if (document.activeElement === inputRef.current) setFocused(true);
  }, []);

  // Widen the input's characters to the mirror's cells, again once the data face loads.
  useLayoutEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    let active = true;
    function measure() {
      if (!active || !input) return;
      const cell = Number.parseFloat(getComputedStyle(input).fontSize) * CELL_EM;
      const password = nativeAdvance(input, 'password');
      const text = nativeAdvance(input, 'text');
      if (password > 0 && text > 0) setSpacing({ password: cell - password, text: cell - text });
    }
    measure();
    void document.fonts?.ready.then(measure);
    return () => {
      active = false;
    };
  }, []);

  // Dragging a selection changes it before any key or pointer release.
  useEffect(() => {
    function follow() {
      if (document.activeElement === inputRef.current) syncSelection();
    }
    document.addEventListener('selectionchange', follow);
    return () => document.removeEventListener('selectionchange', follow);
  }, []);

  // The mirror scrolls with the input, so overflowing dots stay under the caret.
  useLayoutEffect(() => {
    const input = inputRef.current;
    const mirror = mirrorRef.current;
    if (input && mirror) mirror.scrollLeft = input.scrollLeft;
  }, [value, selection, visible, spacing]);

  const busy = phase !== 'idle';
  const success = phase === 'success';
  const invalid = message !== '';
  const characters = [...value];
  const start = Math.min(codePointIndex(value, selection.start), characters.length);
  const end = Math.min(codePointIndex(value, selection.end), characters.length);
  const showCursor = focused && !busy && start === end;
  const letterSpacing = spacing ? `${visible ? spacing.text : spacing.password}px` : undefined;

  function syncSelection() {
    const input = inputRef.current;
    if (!input) return;
    const from = input.selectionStart ?? input.value.length;
    const to = input.selectionEnd ?? from;
    setSelection(current => current.start === from && current.end === to ? current : { end: to, start: from });
    if (mirrorRef.current) mirrorRef.current.scrollLeft = input.scrollLeft;
  }

  return (
    <div
      // Shares its view-transition name with the terminal window it becomes.
      className={cn('relative w-full font-[family-name:var(--font-data)] [view-transition-name:console-window]', shaking && 'login-prompt--shake')}
      onAnimationEnd={(event) => {
        if (event.animationName === 'login-prompt-shake') setShaking(false);
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
        <label className='relative flex h-full min-w-0 flex-1 cursor-text items-center'>
          <input
            aria-describedby={messageId}
            aria-invalid={invalid || undefined}
            aria-label={t('login.password')}
            autoComplete='current-password'
            // The page takes nothing else, so it opens on its password.
            autoFocus
            className='login-prompt__input absolute inset-0 size-full bg-transparent text-base text-transparent caret-transparent outline-none'
            // The prompt draws focus on itself, so the global field outline stays off.
            data-slot='login-prompt-input'
            onBlur={() => setFocused(false)}
            onChange={(event) => {
              onChange(event.target.value);
              syncSelection();
            }}
            onFocus={() => {
              setFocused(true);
              syncSelection();
            }}
            onKeyUp={syncSelection}
            onScroll={syncSelection}
            onSelect={syncSelection}
            readOnly={busy}
            ref={inputRef}
            spellCheck={false}
            style={{ letterSpacing }}
            type={visible ? 'text' : 'password'}
            value={value}
          />
          <span aria-hidden='true' className='login-prompt__mirror pointer-events-none flex h-full min-w-0 items-center overflow-hidden text-base whitespace-pre' ref={mirrorRef}>
            {characters.map((character, index) => (
              // Each character keeps its index, so only a new one pops in.
              // eslint-disable-next-line react/no-array-index-key
              <span className='login-prompt__glyph' data-selected={(focused && index >= start && index < end) || undefined} key={index}>
                {showCursor && index === start ? <span className='login-prompt__caret login-prompt__caret--bar' /> : null}
                {visible ? character : <span className='login-prompt__dot' />}
              </span>
            ))}
            {value !== '' || focused
              ? (
                  // A last, empty cell holds the cursor after the last character.
                  <span className='login-prompt__glyph login-prompt__end'>
                    {showCursor && start === characters.length ? <span className='login-prompt__caret' /> : null}
                  </span>
                )
              : null}
            {value === '' && !focused
              ? <span className='truncate font-light text-[color-mix(in_oklab,var(--color-text-faint)_70%,transparent)]'>{t('login.password')}</span>
              : null}
          </span>
        </label>
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
              <p className='login-prompt__output flex items-start gap-2 rounded-b-[var(--radius-card)] border border-t-0 border-[var(--color-danger-border)] bg-card px-4 py-2.5 text-sm text-[var(--color-danger-text)] shadow-[var(--shadow-card)]'>
                <X aria-hidden='true' className='mt-0.5 size-4 shrink-0' strokeWidth={2.5} />
                <span className='min-w-0'>{message}</span>
              </p>
            )
          : null}
      </div>
    </div>
  );
}
