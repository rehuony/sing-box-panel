import type { Ref } from 'react';

import { cn } from '@shared/lib/utils';
import { Search, X } from 'lucide-react';
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

/**
 * A bar's search: an icon and a flat field that takes the room between its
 * dividers. While focused it fills with the hover wash over the field
 * surface, opaque so no glyph sits behind typed text, without a hairline,
 * and its icon takes the text colour; a clear action shows while it holds
 * text. Forms keep the bordered field.
 */
export function StripSearch({ className, label, onValueChange, ref, value }: StripSearchProps) {
  const { t } = useTranslation();
  return (
    <div
      className={cn(
        'flex h-(--control-height-sm) min-w-0 flex-1 cursor-text items-center gap-2 rounded-[var(--radius-control)] ps-2.5 pe-0.5 text-muted-foreground transition-colors',
        'focus-within:bg-[color-mix(in_oklab,var(--foreground)_6%,var(--color-field))] focus-within:text-foreground',
        className,
      )}
      onPointerDown={(event) => {
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
        onChange={event => onValueChange(event.target.value)}
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
