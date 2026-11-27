import type { ClipboardEvent, KeyboardEvent, ReactNode } from 'react';

import { X } from 'lucide-react';
import { cn } from '@shared/lib/utils';
import { useEffect, useRef, useState } from 'react';

/** Text split into finished tokens and what is still being typed. */
export interface TokenSplit {
  rest: string;
  tokens: string[];
}

interface TokenInputProps {
  id: string;
  invalid?: boolean;
  maxLength?: number;
  describedBy?: string;
  placeholder?: string;
  value: readonly string[];
  onRemove: (index: number) => void;
  /** Takes finished tokens and returns those it refused, which stay in the input. */
  onAdd: (tokens: string[]) => string[];
  removeLabel: (token: string) => string;
  /** A token the server refused; its text reads in danger. */
  tokenInvalid?: (index: number) => boolean;
  renderToken?: (token: string) => ReactNode;
  /** Splits text into tokens; `final` when Enter, a paste or leaving the field ends what was typed. */
  split: (text: string, final: boolean) => TokenSplit;
}

/**
 * Tokens as pills inside one field, with the input after them. What `split`
 * finishes while typing becomes a pill at once; Enter, a paste and leaving the
 * field finish the rest. Backspace in an empty input removes the last pill,
 * and each pill has its own remove action. The field grows to four lines and
 * then scrolls.
 */
export function TokenInput({
  id,
  invalid = false,
  maxLength,
  onAdd,
  onRemove,
  placeholder,
  removeLabel,
  renderToken,
  split,
  tokenInvalid,
  value,
  describedBy,
}: TokenInputProps) {
  const [text, setText] = useState('');
  const fieldRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const count = value.length;

  // A new pill keeps the input in view.
  useEffect(() => {
    const field = fieldRef.current;
    if (field) field.scrollTop = field.scrollHeight;
  }, [count]);

  function take(next: string, final: boolean) {
    const { rest, tokens } = split(next, final);
    if (tokens.length === 0) {
      setText(rest);
      return;
    }
    const refused = onAdd(tokens);
    setText([...refused, rest].filter(Boolean).join(' '));
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.nativeEvent.isComposing) return;
    if (event.key === 'Enter') {
      event.preventDefault();
      take(text, true);
    } else if (event.key === 'Backspace' && text === '' && count > 0) {
      event.preventDefault();
      onRemove(count - 1);
    }
  }

  function onPaste(event: ClipboardEvent<HTMLInputElement>) {
    const pasted = event.clipboardData.getData('text');
    if (pasted === '') return;
    event.preventDefault();
    const input = event.currentTarget;
    const start = input.selectionStart ?? text.length;
    const end = input.selectionEnd ?? text.length;
    take(text.slice(0, start) + pasted + text.slice(end), true);
  }

  return (
    <div
      className={cn(
        'flex max-h-28 min-h-(--control-height) w-full min-w-0 cursor-text flex-wrap items-center gap-1 overflow-y-auto rounded-lg border border-input bg-[var(--color-field)] px-1.5 py-1',
        'focus-within:border-[var(--input-focus)]',
        invalid && 'border-destructive focus-within:border-destructive',
      )}
      onMouseDown={(event) => {
        // A press on the field's free space puts the caret in its input.
        if (event.target === event.currentTarget) {
          event.preventDefault();
          inputRef.current?.focus();
        }
      }}
      ref={fieldRef}
    >
      {value.map((token, index) => (
        <span
          className='inline-flex h-6 max-w-full min-w-0 cursor-default items-center gap-1 rounded-[var(--radius-pill)] border bg-[var(--color-surface-raised)] ps-2.5 pe-1 font-[family-name:var(--font-data)] text-xs'
          key={token}
          title={token}
        >
          <span className={cn('truncate', tokenInvalid?.(index) && 'text-[var(--color-danger-text)]')}>
            {renderToken ? renderToken(token) : token}
          </span>
          <button
            aria-label={removeLabel(token)}
            className='inline-flex size-4 flex-none cursor-pointer items-center justify-center rounded-full text-muted-foreground outline-none hover:bg-[var(--color-hover)] hover:text-foreground focus-visible:shadow-[inset_0_0_0_1px_var(--ring)]'
            onClick={() => {
              onRemove(index);
              inputRef.current?.focus();
            }}
            // Pressing the action keeps focus in the input, so leaving it does not finish what was typed first.
            onMouseDown={event => event.preventDefault()}
            type='button'
          >
            <X aria-hidden='true' className='size-3' strokeWidth={2.25} />
          </button>
        </span>
      ))}
      <input
        aria-describedby={describedBy}
        aria-invalid={invalid || undefined}
        autoCapitalize='off'
        autoComplete='off'
        className='h-7 min-w-[8ch] flex-1 bg-transparent px-1 font-[family-name:var(--font-data)] text-base outline-none placeholder:font-[family-name:var(--font-body)] placeholder:text-muted-foreground md:text-sm'
        data-slot='token-input'
        id={id}
        maxLength={maxLength}
        onBlur={() => take(text, true)}
        onChange={event => take(event.target.value, false)}
        onKeyDown={onKeyDown}
        onPaste={onPaste}
        placeholder={count === 0 ? placeholder : undefined}
        ref={inputRef}
        spellCheck={false}
        value={text}
      />
    </div>
  );
}
