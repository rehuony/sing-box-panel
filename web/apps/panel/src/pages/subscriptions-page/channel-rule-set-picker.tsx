import type { KeyboardEvent } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { useId, useRef, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { Check, Layers3, Search } from 'lucide-react';
import { toast } from '@shared/components/ui/toast-manager';
import { useLocation, useNavigate } from 'react-router-dom';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Popover, PopoverContent, PopoverTrigger } from '@shared/components/ui/popover';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';

import type { SubscriptionFormat, SubscriptionRuleSet } from '@/api/api-client';

import { ruleSetIcon } from './rule-set-icons';

const count = 'flex-none font-[family-name:var(--font-data)] text-xs text-[var(--color-text-faint)] tabular-nums';
const line = 'flex h-9 w-full min-w-0 items-center gap-2.5 rounded-[var(--radius-control)] px-2 text-start text-sm';

function SetTile({ icon }: { icon: string }) {
  const drawn = ruleSetIcon(icon);
  return (
    <span aria-hidden='true' className='flex size-6 flex-none items-center justify-center rounded-[var(--radius-control)] bg-[var(--color-hover)] text-[var(--color-text-secondary)] [&_svg]:size-3.5'>
      <drawn.Icon />
    </span>
  );
}

/**
 * Adds the client's rule sets to a channel's exit rules, several at once: a
 * search over their names, then the sets the channel does not route yet to
 * check, in the order the rule sets tab keeps, and after them the ones it
 * routes, which cannot be checked again, all in a region of fixed height.
 * The footer names the exit new rules take, leads to the client's rule sets
 * and adds the checked sets. Down moves from the search into the list, Up
 * back from its first line; Space checks a line and Enter adds the checked
 * sets, or the focused one, or from the search its first match.
 */
export function ChannelRuleSetPicker({ busy, exitName, format, onAdd, ruleSets, used }: {
  busy: boolean;
  /** The name of the exit new rules take. */
  exitName: string;
  format: SubscriptionFormat;
  /** Every client's sets, undefined while they load. */
  ruleSets: readonly SubscriptionRuleSet[] | undefined;
  /** The sets the channel routes already. */
  used: ReadonlySet<string>;
  /** Adds rules for the sets, in the order the rule sets tab keeps. */
  onAdd: (ids: string[]) => void;
}) {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const location = useLocation();
  const navigate = useNavigate();
  const addedId = useId();
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [checked, setChecked] = useState<ReadonlySet<string>>(() => new Set());
  const searchRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const sets = (ruleSets ?? []).filter(set => set.client === format);
  const query = search.trim().toLowerCase();
  const shown = query === '' ? sets : sets.filter(set => set.name.toLowerCase().includes(query));
  const free = shown.filter(set => !used.has(set.id));
  const taken = shown.filter(set => used.has(set.id));
  const rules = (set: SubscriptionRuleSet) => set.rules.length.toLocaleString(locale);

  function add(ids: readonly string[]) {
    const chosen = sets.filter(set => ids.includes(set.id) && !used.has(set.id)).map(set => set.id);
    if (!chosen.length) {
      toast.add({ title: t('channels.chooseRuleSets'), type: 'info' });
      return;
    }
    onAdd(chosen);
    setOpen(false);
  }

  function toggle(id: string) {
    setChecked((current) => {
      const next = new Set(current);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  }

  function manage() {
    setOpen(false);
    void navigate(
      { pathname: location.pathname, search: location.search, hash: `#subscription-rules/${format}` },
      { state: location.state },
    );
  }

  function move(event: KeyboardEvent<HTMLDivElement>) {
    const lines = [...event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="checkbox"]')];
    const from = lines.indexOf(document.activeElement as HTMLButtonElement);
    if (from < 0) return;
    if (event.key === 'Enter') {
      // Enter adds rather than checks, which Space does.
      event.preventDefault();
      add(checked.size ? [...checked] : [lines[from].dataset.id ?? '']);
      return;
    }
    if (event.key === 'ArrowUp' && from === 0) {
      event.preventDefault();
      searchRef.current?.focus();
      return;
    }
    const to = {
      ArrowDown: Math.min(lines.length - 1, from + 1),
      ArrowUp: from - 1,
      End: lines.length - 1,
      Home: 0,
    }[event.key];
    if (to === undefined) return;
    event.preventDefault();
    for (const each of lines) each.tabIndex = each === lines[to] ? 0 : -1;
    lines[to].focus();
  }

  let region;
  const state = 'flex h-72 flex-none items-center justify-center px-4 text-center text-sm text-muted-foreground';
  if (ruleSets === undefined) {
    region = <p className={state} role='status'>{t('common.loading')}</p>;
  } else if (!sets.length) {
    region = <p className={state}>{t('channels.noClientRuleSets', { client: format })}</p>;
  } else if (!shown.length) {
    region = <p className={state}>{t('channels.noRuleSetMatches')}</p>;
  } else {
    region = (
      <ScrollArea className='h-72 flex-none' viewportClassName='px-1 pb-1'>
        {free.length > 0 && (
          <div aria-label={t('channels.addRuleSetRoute')} onKeyDown={move} ref={listRef} role='group'>
            {free.map((set, index) => {
              const on = checked.has(set.id);
              return (
                <button
                  aria-checked={on}
                  className={cn(line, 'cursor-pointer outline-none transition-colors duration-[var(--dur-micro)] ease-out hover:bg-[var(--color-hover)] focus-visible:bg-[var(--color-hover)] motion-reduce:transition-none')}
                  data-id={set.id}
                  key={set.id}
                  onClick={() => toggle(set.id)}
                  role='checkbox'
                  tabIndex={index === 0 ? 0 : -1}
                  type='button'
                >
                  <span
                    aria-hidden='true'
                    className={cn(
                      'flex size-3.5 flex-none items-center justify-center rounded-[4px] border',
                      on ? 'border-primary bg-primary text-primary-foreground' : 'border-[var(--color-border-strong)]',
                    )}
                  >
                    {on && <Check className='size-3' strokeWidth={3} />}
                  </span>
                  <SetTile icon={set.icon} />
                  <span className='min-w-0 flex-1 truncate' title={set.name}>{set.name}</span>
                  <span className={count}>{rules(set)}</span>
                </button>
              );
            })}
          </div>
        )}
        {taken.length > 0 && (
          <>
            <p className='flex h-7 items-end px-2 pb-1 text-xs text-[var(--color-text-faint)]' id={addedId}>{t('channels.addedCount', { n: taken.length })}</p>
            <ul aria-labelledby={addedId}>
              {taken.map(set => (
                <li className={cn(line, 'text-[var(--color-text-faint)]')} key={set.id}>
                  <Check aria-label={t('channels.alreadyAdded')} className='size-3.5 flex-none text-[var(--color-success-text)]' role='img' />
                  <span className='opacity-60'><SetTile icon={set.icon} /></span>
                  <span className='min-w-0 flex-1 truncate' title={set.name}>{set.name}</span>
                  <span className={count}>{rules(set)}</span>
                </li>
              ))}
            </ul>
          </>
        )}
      </ScrollArea>
    );
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (next) {
          setSearch('');
          setChecked(new Set());
        }
      }}
    >
      <PopoverTrigger render={<Button aria-label={t('channels.addRuleSetRoute')} className='max-sm:w-(--control-height-sm) max-sm:px-0' disabled={busy} size='sm' variant='flat' />}>
        <Layers3 aria-hidden='true' data-icon='inline-start' />
        <span className='max-sm:sr-only'>{t('channels.addRuleSetRoute')}</span>
      </PopoverTrigger>
      <PopoverContent
        align='end'
        className='w-90 max-w-[calc(100vw-2rem)] gap-0 p-0'
        // A touch opens no keyboard; it lands on the first set instead.
        initialFocus={type => (type === 'touch' ? listRef.current?.querySelector<HTMLElement>('[tabindex="0"]') : searchRef.current) ?? false}
        sideOffset={6}
      >
        <div className='flex-none p-2'>
          <InputGroup>
            <InputGroupAddon><Search aria-hidden='true' /></InputGroupAddon>
            <InputGroupInput
              aria-label={t('channels.searchRuleSets')}
              autoComplete='off'
              onChange={event => setSearch(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'ArrowDown') {
                  const target = listRef.current?.querySelector<HTMLElement>('[tabindex="0"]');
                  if (!target) return;
                  event.preventDefault();
                  target.focus();
                } else if (event.key === 'Enter') {
                  event.preventDefault();
                  add(checked.size ? [...checked] : query && free[0] ? [free[0].id] : []);
                }
              }}
              placeholder={t('channels.searchRuleSets')}
              ref={searchRef}
              value={search}
            />
          </InputGroup>
        </div>
        {region}
        <div className='flex flex-none items-center gap-2 border-t p-2 ps-3'>
          <p className='min-w-0 flex-1 truncate text-xs text-muted-foreground' title={exitName}>
            {t('channels.newRulesExit')}
            {' '}
            <span className='text-[var(--color-text-secondary)]'>{exitName}</span>
          </p>
          <Button onClick={manage} size='sm' type='button' variant='flat'>{t('channels.manageRuleSets')}</Button>
          <Button onClick={() => add([...checked])} size='sm' type='button'>
            {checked.size ? t('channels.addRuleSetsCount', { count: checked.size }) : t('channels.addRuleSets')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
