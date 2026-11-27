import type { KeyboardEvent } from 'react';

import { cn } from '@shared/lib/utils';
import { useRef, useState } from 'react';
import { ChevronDown } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Popover, PopoverContent, PopoverTrigger } from '@shared/components/ui/popover';

import type { RuleSetIconKey } from './rule-set-icons';

import { RULE_SET_ICONS, ruleSetIcon } from './rule-set-icons';

const COLUMNS = 6;

const moves: Record<string, (index: number, last: number) => number> = {
  ArrowDown: (index, last) => Math.min(last, index + COLUMNS),
  ArrowLeft: index => Math.max(0, index - 1),
  ArrowRight: (index, last) => Math.min(last, index + 1),
  ArrowUp: index => Math.max(0, index - COLUMNS),
  End: (_index, last) => last,
  Home: () => 0,
};

/**
 * A rule set's icon and the menu that changes it: six icons a row, one Tab
 * stop that the arrow, Home and End keys move through, and a line naming the
 * icon under the pointer or focus. In a field it sits at the start with a
 * chevron; in the editor's title bar it is the icon alone.
 */
export function RuleSetIconMenu({ onChange, value, variant }: {
  value: string;
  variant: 'field' | 'toolbar';
  onChange: (icon: RuleSetIconKey) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const current = ruleSetIcon(value);
  const [shown, setShown] = useState<RuleSetIconKey>(current.key);
  const checkedRef = useRef<HTMLButtonElement>(null);
  const name = (key: RuleSetIconKey) => t(`subscriptions.ruleSets.icons.${key}`);
  const index = RULE_SET_ICONS.findIndex(([key]) => key === shown);

  function move(event: KeyboardEvent<HTMLDivElement>) {
    const next = moves[event.key];
    const cells = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="radio"]')];
    const from = cells.indexOf(document.activeElement as HTMLButtonElement);
    if (!next || from < 0) return;
    event.preventDefault();
    const target = cells[next(from, cells.length - 1)];
    for (const cell of cells) cell.tabIndex = cell === target ? 0 : -1;
    target?.focus();
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) setShown(current.key);
      }}
    >
      <PopoverTrigger
        render={(
          <button
            aria-label={variant === 'field'
              ? t('subscriptions.ruleSets.iconOf', { name: name(current.key) })
              : t('subscriptions.ruleSets.changeIcon')}
            className={cn(
              'inline-flex flex-none cursor-pointer items-center justify-center rounded-[var(--radius-control)] border border-transparent text-[var(--color-text-secondary)] outline-none transition-colors duration-[var(--dur-micro)] ease-out',
              'hover:bg-[var(--color-hover)] hover:text-foreground focus-visible:border-primary aria-expanded:bg-[var(--color-hover)] aria-expanded:text-foreground motion-reduce:transition-none',
              variant === 'field' ? 'h-8 gap-0.5 px-1.5 [&>svg:first-child]:size-4.5' : 'size-7 [&>svg]:size-4',
            )}
            type='button'
          />
        )}
      >
        <current.Icon aria-hidden='true' />
        {variant === 'field' ? <ChevronDown aria-hidden='true' className='size-3 text-[var(--color-text-faint)]' /> : null}
      </PopoverTrigger>
      <PopoverContent align='start' className='w-[15.5rem] gap-0 p-1.5' initialFocus={checkedRef} sideOffset={6}>
        <div
          aria-label={t('subscriptions.ruleSets.icon')}
          className='grid grid-cols-6 gap-0.5'
          onKeyDown={move}
          role='radiogroup'
        >
          {RULE_SET_ICONS.map(([key, Icon]) => {
            const checked = key === current.key;
            return (
              <button
                aria-checked={checked}
                aria-label={name(key)}
                className={cn(
                  'flex h-9 cursor-pointer items-center justify-center rounded-[var(--radius-control)] text-[var(--color-text-secondary)] outline-none transition-colors duration-[var(--dur-micro)] ease-out [&_svg]:size-4.5',
                  'hover:bg-[var(--color-hover)] hover:text-foreground focus-visible:bg-[var(--color-hover)] focus-visible:text-foreground aria-checked:bg-accent aria-checked:text-foreground motion-reduce:transition-none',
                )}
                key={key}
                onClick={() => {
                  onChange(key);
                  setOpen(false);
                }}
                onFocus={() => setShown(key)}
                onMouseEnter={() => setShown(key)}
                ref={checked ? checkedRef : undefined}
                role='radio'
                tabIndex={checked ? 0 : -1}
                type='button'
              >
                <Icon aria-hidden='true' />
              </button>
            );
          })}
        </div>
        {/* A fixed line names the icon under the pointer or focus, so the menu never changes size. */}
        <div aria-hidden='true' className='mt-1.5 flex h-7 items-center justify-between border-t px-1.5 pt-1.5 text-xs text-muted-foreground'>
          <span className='truncate'>{name(shown)}</span>
          <span className='font-[family-name:var(--font-data)] text-[var(--color-text-faint)] tabular-nums'>{`${index + 1}/${RULE_SET_ICONS.length}`}</span>
        </div>
      </PopoverContent>
    </Popover>
  );
}
