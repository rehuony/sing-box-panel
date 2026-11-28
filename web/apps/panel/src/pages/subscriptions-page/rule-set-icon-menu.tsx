import type { KeyboardEvent } from 'react';
import type { TablerIcon } from '@tabler/icons-react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { useId, useRef, useState } from 'react';
import { ChevronDown, Search } from 'lucide-react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Popover, PopoverContent, PopoverTrigger } from '@shared/components/ui/popover';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';

import type { RuleSetIconKey } from './rule-set-icons';

import { RULE_SET_ICON_SECTIONS, RULE_SET_ICONS, ruleSetIcon } from './rule-set-icons';

const COLUMNS = 8;

const moves: Record<string, (index: number, last: number) => number> = {
  ArrowDown: (index, last) => Math.min(last, index + COLUMNS),
  ArrowLeft: index => Math.max(0, index - 1),
  ArrowRight: (index, last) => Math.min(last, index + 1),
  ArrowUp: index => Math.max(0, index - COLUMNS),
  End: (_index, last) => last,
  Home: () => 0,
};

function IconCell({ checked, Icon, label, onPick, onShow, tabStop }: {
  Icon: TablerIcon;
  label: string;
  checked: boolean;
  /** Whether the cell is the icons' one Tab stop. */
  tabStop: boolean;
  onPick: () => void;
  onShow: () => void;
}) {
  return (
    <button
      aria-checked={checked}
      aria-label={label}
      className={cn(
        'flex h-9 cursor-pointer items-center justify-center rounded-[var(--radius-control)] text-[var(--color-text-secondary)] outline-none transition-colors duration-[var(--dur-micro)] ease-out [&_svg]:size-4.5',
        'hover:bg-[var(--color-hover)] hover:text-foreground focus-visible:bg-[var(--color-hover)] focus-visible:text-foreground aria-checked:bg-accent aria-checked:text-foreground motion-reduce:transition-none',
      )}
      onClick={onPick}
      onFocus={onShow}
      onMouseEnter={onShow}
      role='radio'
      tabIndex={tabStop ? 0 : -1}
      type='button'
    >
      <Icon aria-hidden='true' />
    </button>
  );
}

/**
 * A rule set's icon and the menu that changes it: a search over the icons'
 * names, then the general and service icons eight a row in a region of fixed
 * height, one Tab stop that the arrow, Home and End keys move through, and a
 * line naming the icon under the pointer or focus. The search takes focus,
 * except from a touch, and Down moves from it into the icons, Up back from
 * their first row; Enter picks the first match. In a field the icon sits at
 * the start with a chevron; in the editor's title bar it is the icon alone.
 */
export function RuleSetIconMenu({ onChange, value, variant }: {
  value: string;
  variant: 'field' | 'toolbar';
  onChange: (icon: RuleSetIconKey) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const current = ruleSetIcon(value);
  const [shown, setShown] = useState<RuleSetIconKey>(current.key);
  const searchRef = useRef<HTMLInputElement>(null);
  const gridRef = useRef<HTMLDivElement>(null);
  const id = useId();
  const name = (key: RuleSetIconKey) => t(`subscriptions.ruleSets.icons.${key}`);
  const query = search.trim().toLowerCase();
  // A search matches a key or a name, so a service is found by either spelling.
  const matches = query === ''
    ? null
    : RULE_SET_ICONS.filter(([key]) => key.includes(query) || name(key).toLowerCase().includes(query));
  const visible = matches ?? RULE_SET_ICONS;
  // The icons' one Tab stop: the current icon when it shows, else the first.
  const stop = visible.some(([key]) => key === current.key) ? current.key : visible[0]?.[0];
  const named = visible.some(([key]) => key === shown) ? shown : stop;
  const position = visible.findIndex(([key]) => key === named);

  function pick(key: RuleSetIconKey) {
    onChange(key);
    setOpen(false);
  }

  function move(event: KeyboardEvent<HTMLDivElement>) {
    const cells = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="radio"]')];
    const from = cells.indexOf(document.activeElement as HTMLButtonElement);
    if (from < 0) return;
    if (event.key === 'ArrowUp' && from < COLUMNS) {
      event.preventDefault();
      searchRef.current?.focus();
      return;
    }
    const next = moves[event.key];
    if (!next) return;
    event.preventDefault();
    const target = cells[next(from, cells.length - 1)];
    for (const cell of cells) cell.tabIndex = cell === target ? 0 : -1;
    target?.focus();
  }

  function cell([key, Icon]: readonly [RuleSetIconKey, TablerIcon]) {
    return (
      <IconCell
        checked={key === current.key}
        Icon={Icon}
        key={key}
        label={name(key)}
        onPick={() => pick(key)}
        onShow={() => setShown(key)}
        tabStop={key === stop}
      />
    );
  }

  const grid = 'grid grid-cols-8 gap-0.5';
  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) {
          setSearch('');
          setShown(current.key);
        }
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
      <PopoverContent
        align='start'
        className='w-[calc(8*2.25rem+7*0.125rem+0.75rem)] max-w-[calc(100vw-2rem)] gap-0 p-1.5'
        initialFocus={(type) => {
          const checked = gridRef.current?.querySelector<HTMLElement>('[aria-checked="true"]');
          checked?.scrollIntoView({ block: 'nearest' });
          // A touch opens no keyboard; it lands on the current icon instead.
          return (type === 'touch' ? checked : searchRef.current) ?? true;
        }}
        sideOffset={6}
      >
        <InputGroup className='h-(--control-height-sm) flex-none'>
          <InputGroupAddon><Search aria-hidden='true' /></InputGroupAddon>
          <InputGroupInput
            aria-label={t('subscriptions.ruleSets.searchIcons')}
            autoComplete='off'
            className='h-full min-h-0'
            onChange={event => setSearch(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'ArrowDown') {
                const target = gridRef.current?.querySelector<HTMLElement>('[tabindex="0"]');
                if (!target) return;
                event.preventDefault();
                target.focus();
              } else if (event.key === 'Enter' && matches?.[0]) {
                event.preventDefault();
                pick(matches[0][0]);
              }
            }}
            placeholder={t('subscriptions.ruleSets.searchIcons')}
            ref={searchRef}
            value={search}
          />
        </InputGroup>
        {/* The icons keep one height whatever the search leaves, so the menu never changes size. */}
        {visible.length
          ? (
              <ScrollArea className='mt-1.5 h-64 flex-none'>
                <div aria-label={t('subscriptions.ruleSets.icon')} onKeyDown={move} ref={gridRef} role='radiogroup'>
                  {matches
                    ? <div className={grid}>{matches.map(cell)}</div>
                    : RULE_SET_ICON_SECTIONS.map(([section, icons]) => (
                        <div aria-labelledby={`${id}-${section}`} key={section} role='group'>
                          <p className='flex h-6 items-center px-1.5 text-xs text-[var(--color-text-faint)]' id={`${id}-${section}`}>
                            {t(`subscriptions.ruleSets.iconSections.${section}`)}
                          </p>
                          <div className={grid}>{icons.map(cell)}</div>
                        </div>
                      ))}
                </div>
              </ScrollArea>
            )
          : <p className='mt-1.5 flex h-64 flex-none items-center justify-center text-sm text-muted-foreground'>{t('subscriptions.ruleSets.noIcons')}</p>}
        {/* A fixed line names the icon under the pointer or focus. */}
        <div aria-hidden='true' className='mt-1.5 flex h-7 items-center justify-between border-t px-1.5 pt-1.5 text-xs text-muted-foreground'>
          <span className='truncate'>{named ? name(named) : ''}</span>
          <span className='font-[family-name:var(--font-data)] text-[var(--color-text-faint)] tabular-nums'>{visible.length ? `${position + 1}/${visible.length}` : ''}</span>
        </div>
      </PopoverContent>
    </Popover>
  );
}
