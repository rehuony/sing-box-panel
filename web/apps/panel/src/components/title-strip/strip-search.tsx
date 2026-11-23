import type { Ref } from 'react';

import { cn } from '@shared/lib/utils';
import { Search, X } from 'lucide-react';
import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';

interface StripSearchProps {
  /** Names the field and shows while it is empty. */
  label: string;
  value: string;
  className?: string;
  ref?: Ref<HTMLInputElement>;
  onValueChange: (value: string) => void;
}

/** Focus that follows a press this soon came from the pointer; a touch focuses only after it lifts. */
const POINTER_FOCUS_MS = 1_000;

/**
 * A bar's search: an icon and a flat field that takes the room between its
 * dividers, on the raised surface while focused and ringed when the keyboard
 * brought focus there, with a clear action while it holds text. Forms keep
 * the bordered field.
 */
export function StripSearch({ className, label, onValueChange, ref, value }: StripSearchProps) {
  const { t } = useTranslation();
  const pressedAtRef = useRef(Number.NEGATIVE_INFINITY);
  const [keyboardFocus, setKeyboardFocus] = useState(false);
  return (
    <div
      className={cn(
        'flex h-(--control-height-sm) min-w-0 flex-1 cursor-text items-center gap-2 rounded-[var(--radius-control)] ps-2.5 pe-0.5 text-muted-foreground transition-colors hover:bg-[var(--color-hover)] has-[input:focus]:bg-[var(--color-hover)] data-keyboard-focus:ring-2 data-keyboard-focus:ring-ring',
        className,
      )}
      data-keyboard-focus={keyboardFocus || undefined}
      onPointerDown={(event) => {
        pressedAtRef.current = event.timeStamp;
        // A press on the icon or the free space puts the caret in the field.
        if (event.target === event.currentTarget || !(event.target instanceof Element) || event.target.closest('svg')) {
          event.preventDefault();
          event.currentTarget.querySelector('input')?.focus();
        }
      }}
    >
      <Search aria-hidden='true' className='size-4 flex-none' />
      <Input
        aria-label={label}
        className='h-full min-h-0 border-0 bg-transparent px-0 py-0 text-ellipsis text-foreground placeholder:text-[var(--color-text-faint)] focus-visible:outline-none md:text-compact dark:bg-transparent [&::-webkit-search-cancel-button]:appearance-none'
        onBlur={() => {
          // A press counts only toward the focus it brought.
          pressedAtRef.current = Number.NEGATIVE_INFINITY;
          setKeyboardFocus(false);
        }}
        onChange={event => onValueChange(event.target.value)}
        onFocus={event => setKeyboardFocus(event.timeStamp - pressedAtRef.current > POINTER_FOCUS_MS)}
        onKeyDown={(event) => {
          if (event.key === 'Escape' && value !== '') {
            event.preventDefault();
            onValueChange('');
          }
        }}
        placeholder={label}
        ref={ref}
        type='search'
        value={value}
      />
      {value === ''
        ? null
        : (
            <Button aria-label={t('common.clearSearch')} className='flex-none' onClick={() => onValueChange('')} size='icon-xs' variant='ghost'>
              <X aria-hidden='true' />
            </Button>
          )}
    </div>
  );
}
