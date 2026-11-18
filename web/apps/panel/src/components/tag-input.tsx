import type { KeyboardEvent } from 'react';

import { X } from 'lucide-react';
import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';

/** A node carries at most this many tags. */
export const MAX_TAGS = 16;
const TAG_PATTERN = /^[a-z0-9_-]{1,32}$/;

type TagRejection = 'full' | 'invalid';

interface TagInputProps {
  id: string;
  invalid?: boolean;
  value: readonly string[];
  onChange: (tags: string[]) => void;
  /** Why a typed tag was not added; the form says so in its message slot. */
  onReject: (reason: TagRejection) => void;
}

/**
 * Tags as chips with an input after them: Enter, a comma or a space adds what
 * was typed, in lower case; Backspace on an empty input removes the last
 * chip, and each chip has its own remove action. Tags stay sorted and
 * distinct.
 */
export function TagInput({ id, invalid = false, onChange, onReject, value }: TagInputProps) {
  const { t } = useTranslation();
  const [text, setText] = useState('');

  function add(raw: string): boolean {
    const tag = raw.trim().toLowerCase();
    if (tag === '') return true;
    if (!TAG_PATTERN.test(tag)) {
      onReject('invalid');
      return false;
    }
    if (value.includes(tag)) return true;
    if (value.length >= MAX_TAGS) {
      onReject('full');
      return false;
    }
    onChange([...value, tag].sort());
    return true;
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'Enter' || event.key === ',' || event.key === ' ') {
      event.preventDefault();
      if (add(text)) setText('');
    } else if (event.key === 'Backspace' && text === '' && value.length > 0) {
      event.preventDefault();
      onChange(value.slice(0, -1));
    }
  }

  return (
    <div
      className={cn(
        'flex min-h-(--control-height) w-full min-w-0 flex-wrap items-center gap-1 rounded-lg border border-input bg-[var(--color-field)] px-1.5 py-1',
        'focus-within:outline-1 focus-within:outline-input-focus focus-within:-outline-offset-1 focus-within:outline-solid',
        invalid && 'border-destructive',
      )}
    >
      {value.map(tag => (
        <span
          className='inline-flex h-6 max-w-full min-w-0 items-center gap-0.5 rounded-[var(--radius-pill)] border bg-[var(--color-surface-raised)] ps-2 pe-0.5 font-[family-name:var(--font-data)] text-xs'
          key={tag}
        >
          <span className='truncate'>{tag}</span>
          <button
            aria-label={t('tags.remove', { tag })}
            className='inline-flex size-5 flex-none cursor-pointer items-center justify-center rounded-full text-muted-foreground outline-none hover:bg-[var(--color-hover)] hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring'
            onClick={() => onChange(value.filter(each => each !== tag))}
            type='button'
          >
            <X aria-hidden='true' className='size-3' />
          </button>
        </span>
      ))}
      <input
        aria-invalid={invalid || undefined}
        autoCapitalize='off'
        autoComplete='off'
        className='h-7 min-w-[8ch] flex-1 bg-transparent px-1 font-[family-name:var(--font-data)] text-base outline-none placeholder:text-muted-foreground md:text-sm'
        id={id}
        maxLength={32}
        onBlur={() => {
          if (add(text)) setText('');
        }}
        onChange={event => setText(event.target.value)}
        onKeyDown={onKeyDown}
        placeholder={value.length === 0 ? t('tags.placeholder') : undefined}
        spellCheck={false}
        value={text}
      />
    </div>
  );
}
