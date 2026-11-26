import type { KeyboardEvent, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Check, CirclePlus, CornerDownLeft, Trash2, X, Zap } from 'lucide-react';
import { Fragment, memo, useEffect, useLayoutEffect, useRef, useState } from 'react';

import type { SubscriptionRule } from '@/api/api-client';

import { IconAction } from '@/components/icon-action';
import { SelectField } from '@/components/select-field';
import { StripDivider } from '@/components/title-strip';

import { ruleLine, ruleType } from './rule-text';
import { dataFace, gutterCell } from './rule-line-style';

export type MatchKind = Exclude<SubscriptionRule['kind'], 'remote'>;

/** A rule of the draft: `origin` is the index of the saved rule it started as, if any. */
export interface Row {
  key: number;
  origin: number | null;
  rule: SubscriptionRule;
}

/** A shown rule with its position in the whole list, counted from 1. */
export interface Entry {
  row: Row;
  position: number;
}

const KIND_ITEMS = (['domain', 'domain_suffix', 'domain_keyword', 'ip_cidr'] as const).map(value => ({ label: ruleType(value), value }));
/** Every line is this tall, its hairline included, so the list can draw only the lines in view. */
const LINE = 36;
/** Lines drawn beyond the view on each side, so scrolling never shows a gap. */
const OVERSCAN = 12;

// Every line keeps the card's 16px inset and one gutter as wide as the longest line number, which also holds the
// prompts: the number, the type, the value and an action that leaves on touch screens.
const lineGrid = 'grid grid-cols-[var(--rule-gutter)_8rem_minmax(0,1fr)_auto] items-center gap-x-3 px-4 @max-[30rem]/editor:grid-cols-[var(--rule-gutter)_6.75rem_minmax(0,1fr)_auto] @max-[30rem]/editor:gap-x-2';
// A line whose type is picked: in a narrow editor the picker takes the room it needs.
const pickedGrid = 'grid grid-cols-[var(--rule-gutter)_8rem_minmax(0,1fr)_auto] items-center gap-x-3 px-4 @max-[30rem]/editor:grid-cols-[var(--rule-gutter)_auto_minmax(0,1fr)_auto] @max-[30rem]/editor:gap-x-2';
// The picker's text lines up with the types below it, its chevron reaching into the gap.
const kindPicker = `-ms-2.5 w-fit max-w-[calc(100%+1.375rem)] ${dataFace} text-xs md:text-xs`;
// Its options are the configuration's keywords, in the face and size of the types they pick.
const kindOption = `${dataFace} text-xs`;
const lineInput = `terminal-input h-full min-w-0 flex-1 bg-transparent p-0 ${dataFace} text-base leading-5 text-foreground outline-none placeholder:text-[var(--color-text-faint)] md:text-compact`;
const prompt = 'text-[var(--color-highlight-yellow)]';
const refusedTone = 'text-[var(--color-danger-text)]';

/** A rule's type as its line writes it: IP-CIDR6 for an IPv6 range. */
function lineType(rule: SubscriptionRule): string {
  return ruleLine(rule).split(',')[0]!;
}

/**
 * A rule's value on one line: a match value, or a remote rule set's name,
 * then its link without the scheme and its format. Only the link's start
 * gives way, so the file and the format stay; the name takes at most 60%.
 */
function RuleValue({ rule }: { rule: SubscriptionRule }) {
  const { t } = useTranslation();
  if (rule.kind !== 'remote' || !rule.remote) {
    return <span className={cn(dataFace, 'min-w-0 truncate text-compact')}>{rule.value}</span>;
  }
  const { accelerated, behavior, format, name, url } = rule.remote;
  const shown = url.replace(/^https?:\/\//, '');
  const file = shown.lastIndexOf('/') + 1;
  return (
    <span className={cn(dataFace, 'flex min-w-0 items-center gap-2 overflow-hidden whitespace-nowrap')}>
      <span className='max-w-[60%] flex-none truncate text-compact'>{name}</span>
      <span className='flex min-w-0 flex-1 items-center overflow-hidden text-xs text-muted-foreground'>
        {accelerated ? <Zap aria-label={t('subscriptions.ruleSets.accelerate')} className='me-1 size-3 flex-none' role='img' /> : null}
        <span className='min-w-0 shrink truncate'>{shown.slice(0, file)}</span>
        <span className='flex-none'>{shown.slice(file)}</span>
        <span aria-hidden='true' className='mx-1.5 flex-none text-[var(--color-text-faint)]'>·</span>
        <span className='flex-none'>{[format, behavior].filter(Boolean).join(' · ')}</span>
      </span>
    </span>
  );
}

/**
 * A rule's line at rest: its number, type and value as one action that
 * edits it, then remove, shown on hover and focus and left to the edit line
 * on touch screens. Only the line holding the list's focus is a tab stop.
 */
const RuleLine = memo(({ active, changed, flashing, onEdit, onFocusLine, onRemove, position, row }: {
  row: Row;
  position: number;
  /** Whether the line is the list's tab stop. */
  active: boolean;
  changed: boolean;
  flashing: boolean;
  onEdit: (key: number) => void;
  onRemove: (key: number) => void;
  onFocusLine: (key: number) => void;
}) => {
  const { t } = useTranslation();
  return (
    <li
      className={cn(
        lineGrid,
        'group/rule h-9 border-b transition-colors duration-[var(--dur-micro)] ease-out hover:bg-[var(--color-hover)]',
        flashing && 'bg-accent hover:bg-accent',
      )}
      data-rule-key={row.key}
    >
      <button
        aria-description={t('subscriptions.ruleSets.editRule')}
        className='col-span-3 grid h-full cursor-pointer grid-cols-subgrid items-center text-start outline-none focus-visible:ring-1 focus-visible:ring-primary focus-visible:ring-inset'
        data-rule-button=''
        onClick={() => onEdit(row.key)}
        onFocus={() => onFocusLine(row.key)}
        tabIndex={active ? 0 : -1}
        type='button'
      >
        <span className={cn(gutterCell, changed ? 'text-primary' : 'text-[var(--color-text-faint)] group-focus-within/rule:text-foreground')}>{position}</span>
        <span className={cn(dataFace, 'truncate text-xs text-[var(--color-text-secondary)]')}>{lineType(row.rule)}</span>
        <RuleValue rule={row.rule} />
      </button>
      <span className='invisible flex justify-end group-focus-within/rule:visible group-hover/rule:visible [@media(hover:none)]:hidden'>
        <IconAction label={t('subscriptions.ruleSets.removeRule')} onClick={() => onRemove(row.key)} size='icon-xs' tabIndex={-1}>
          <Trash2 aria-hidden='true' />
        </IconAction>
      </span>
    </li>
  );
});
RuleLine.displayName = 'RuleLine';

/**
 * A rule being edited in place: its type and value on the prompt's terms.
 * Enter or a press elsewhere applies it, Escape cancels; on touch screens,
 * which have no Escape, its end removes the rule instead.
 */
export function RuleEditLine({ onApply, onCancel, onInput, onRemove, position, refused, row }: {
  row: Row;
  position: number;
  /** Whether the last apply was refused. */
  refused: boolean;
  /** Applies the line; false keeps it open. */
  onApply: (kind: MatchKind, value: string) => boolean;
  onCancel: () => void;
  onRemove: () => void;
  /** Called on each change of the value. */
  onInput: () => void;
}) {
  const { t } = useTranslation();
  const lineRef = useRef<HTMLLIElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [kind, setKind] = useState<MatchKind>(row.rule.kind === 'remote' ? 'domain_suffix' : row.rule.kind);
  const [value, setValue] = useState(row.rule.value ?? '');

  // An unchanged line closes; a changed one applies, and stays open when refused.
  const finish = () => (kind === row.rule.kind && value.trim() === row.rule.value ? onCancel() : onApply(kind, value));
  const finishRef = useRef(finish);
  useLayoutEffect(() => {
    finishRef.current = finish;
  });
  useEffect(() => {
    const input = inputRef.current;
    input?.focus();
    input?.setSelectionRange(input.value.length, input.value.length);
    // A press anywhere else finishes the line, except in its type's list.
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target;
      if (!(target instanceof Element) || lineRef.current?.contains(target) || target.closest('[data-slot=select-content]')) return;
      finishRef.current();
    };
    document.addEventListener('pointerdown', onPointerDown, true);
    return () => document.removeEventListener('pointerdown', onPointerDown, true);
  }, []);

  return (
    <li className={cn(pickedGrid, 'h-9 border-b')} data-rule-key={row.key} ref={lineRef}>
      <span className={cn(gutterCell, refused ? refusedTone : 'text-foreground')}>{position}</span>
      <SelectField
        aria-label={t('subscriptions.ruleSets.field.kind')}
        className={kindPicker}
        itemClassName={kindOption}
        items={KIND_ITEMS}
        onValueChange={(next) => {
          setKind(next);
          inputRef.current?.focus();
        }}
        value={kind}
        variant='flat'
      />
      <span className='flex h-full min-w-0 items-center gap-1'>
        <input
          aria-invalid={refused || undefined}
          aria-label={t('subscriptions.ruleSets.field.value')}
          autoComplete='off'
          className={lineInput}
          data-slot='terminal-input'
          maxLength={128}
          onChange={(event) => {
            setValue(event.target.value);
            onInput();
          }}
          onKeyDown={(event) => {
            if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
              event.preventDefault();
              onApply(kind, value);
            } else if (event.key === 'Escape') {
              event.preventDefault();
              onCancel();
            }
          }}
          ref={inputRef}
          spellCheck={false}
          value={value}
        />
        <IconAction label={t('subscriptions.ruleSets.apply')} onClick={() => onApply(kind, value)} size='icon-xs'>
          <Check aria-hidden='true' />
        </IconAction>
      </span>
      <span className='flex'>
        <IconAction className='[@media(hover:none)]:hidden' label={t('common.cancel')} onClick={onCancel} size='icon-xs'>
          <X aria-hidden='true' />
        </IconAction>
        <IconAction className='hidden [@media(hover:none)]:inline-flex' label={t('subscriptions.ruleSets.removeRule')} onClick={onRemove} size='icon-xs'>
          <Trash2 aria-hidden='true' />
        </IconAction>
      </span>
    </li>
  );
}

/**
 * The rules as numbered lines that scroll inside the list, drawing only the
 * lines in view. The list is one tab stop: the arrow keys, Home, End and the
 * page keys move between lines, Enter edits one and Delete removes it.
 */
export function RuleList({
  editLine, editingKey, entries, flashKey, followSignal, isChanged, label, onEdit, onRemove,
}: {
  entries: readonly Entry[];
  label: string;
  /** The line being edited, drawn in its row's place. */
  editingKey: number | null;
  editLine: ReactNode;
  /** A line to bring into view and highlight. */
  flashKey: number | null;
  /** Changes whenever lines were added, to follow them to the list's end. */
  followSignal: number;
  isChanged: (row: Row) => boolean;
  onEdit: (key: number) => void;
  onRemove: (key: number) => void;
}) {
  const listRef = useRef<HTMLOListElement>(null);
  const frameRef = useRef(0);
  const focusRef = useRef<number | null>(null);
  const editedRef = useRef<number | null>(editingKey);
  const [view, setView] = useState({ top: 0, height: 0 });
  const [activeKey, setActiveKey] = useState<number | null>(null);
  const active = entries.some(({ row }) => row.key === activeKey) ? activeKey : entries[0]?.row.key ?? null;

  useLayoutEffect(() => {
    const list = listRef.current;
    if (!list) return;
    // The observer reports the list's size as it starts and whenever it changes.
    const observer = new ResizeObserver(() => setView({ top: list.scrollTop, height: list.clientHeight }));
    observer.observe(list);
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frameRef.current);
    };
  }, []);

  /**
   * Scrolls the line at this index into view, centred when it was out of it;
   * the scroll then draws the lines around it.
   */
  function reveal(index: number) {
    const list = listRef.current;
    if (!list) return;
    const top = index * LINE;
    if (top < list.scrollTop || top + LINE > list.scrollTop + list.clientHeight) {
      list.scrollTop = Math.max(0, top - (list.clientHeight - LINE) / 2);
    }
  }

  useLayoutEffect(() => {
    if (followSignal !== 0 && listRef.current) listRef.current.scrollTop = listRef.current.scrollHeight;
  }, [followSignal]);

  // A refused duplicate is brought into view, without moving the focus to it.
  const flashIndex = flashKey === null ? -1 : entries.findIndex(({ row }) => row.key === flashKey);
  useLayoutEffect(() => {
    if (flashIndex >= 0) reveal(flashIndex);
  }, [flashIndex]);

  // A line that closes its edit takes the focus back, unless the press that closed it went elsewhere.
  useLayoutEffect(() => {
    const closed = editedRef.current;
    editedRef.current = editingKey;
    if (closed === null || editingKey !== null) return;
    const focused = document.activeElement;
    const outside = focused && focused !== document.body && !listRef.current?.contains(focused);
    if (!outside) focusRef.current = closed;
  }, [editingKey]);

  // A requested focus lands once its line is drawn.
  useLayoutEffect(() => {
    const key = focusRef.current;
    if (key === null) return;
    const button = listRef.current?.querySelector<HTMLElement>(`[data-rule-key="${key}"] [data-rule-button]`);
    if (!button) {
      // A line that is gone takes no focus later.
      if (!entries.some(({ row }) => row.key === key)) focusRef.current = null;
      return;
    }
    focusRef.current = null;
    button.focus({ preventScroll: true });
  });

  function moveTo(index: number) {
    const entry = entries[Math.max(0, Math.min(entries.length - 1, index))];
    if (!entry) return;
    reveal(entries.indexOf(entry));
    setActiveKey(entry.row.key);
    focusRef.current = entry.row.key;
    // A line already drawn takes the focus at once.
    listRef.current?.querySelector<HTMLElement>(`[data-rule-key="${entry.row.key}"] [data-rule-button]`)?.focus({ preventScroll: true });
  }

  function onKeyDown(event: KeyboardEvent<HTMLOListElement>) {
    if (!(event.target instanceof HTMLElement) || !event.target.hasAttribute('data-rule-button')) return;
    const index = entries.findIndex(({ row }) => row.key === active);
    const page = Math.max(1, Math.floor(view.height / LINE) - 1);
    const steps: Record<string, number> = { ArrowDown: 1, ArrowUp: -1, PageDown: page, PageUp: -page };
    if (event.key in steps) {
      event.preventDefault();
      moveTo(index + steps[event.key]!);
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault();
      moveTo(event.key === 'Home' ? 0 : entries.length - 1);
    } else if ((event.key === 'Delete' || event.key === 'Backspace') && active !== null) {
      event.preventDefault();
      // The next line takes the removed one's place, or the one before it at the end.
      const next = entries[index + 1] ?? entries[index - 1];
      onRemove(active);
      if (next) {
        setActiveKey(next.row.key);
        focusRef.current = next.row.key;
      }
    }
  }

  // A list that just got shorter, as a search narrows it, may not have reported its new scroll yet.
  const height = Math.max(view.height, LINE * 20);
  const top = Math.min(view.top, Math.max(0, entries.length * LINE - height));
  const first = Math.max(0, Math.floor(top / LINE) - OVERSCAN);
  const last = Math.min(entries.length, Math.ceil((top + height) / LINE) + OVERSCAN);
  const drawn = entries.slice(first, last);
  // The tab stop is the active line while it is drawn, otherwise the first line drawn, so Tab always reaches the list.
  const tabStop = drawn.some(({ row }) => row.key === active) ? active : drawn[0]?.row.key ?? null;
  return (
    <ol
      aria-label={label}
      // Drawn lines change as it scrolls, so the browser must not hold any of them in place.
      className='min-h-0 flex-1 overflow-y-auto overscroll-contain [overflow-anchor:none]'
      onKeyDown={onKeyDown}
      onScroll={(event) => {
        const list = event.currentTarget;
        cancelAnimationFrame(frameRef.current);
        frameRef.current = requestAnimationFrame(() => setView({ top: list.scrollTop, height: list.clientHeight }));
      }}
      ref={listRef}
    >
      {/* The lines out of view keep their room as spacers, so the scrollbar measures the whole list. */}
      {first > 0 ? <li aria-hidden='true' className='list-none' style={{ height: first * LINE }} /> : null}
      {drawn.map(({ position, row }) => (row.key === editingKey
        ? <Fragment key={row.key}>{editLine}</Fragment>
        : (
            <RuleLine
              active={row.key === tabStop}
              changed={isChanged(row)}
              flashing={row.key === flashKey}
              key={row.key}
              onEdit={onEdit}
              onFocusLine={setActiveKey}
              onRemove={onRemove}
              position={position}
              row={row}
            />
          )))}
      {last < entries.length ? <li aria-hidden='true' className='list-none' style={{ height: (entries.length - last) * LINE }} /> : null}
    </ol>
  );
}

/**
 * The prompt line closing the card: `λ` in the gutter, the type a bare value
 * takes, the value, add, which a narrow editor leaves to the return key, and
 * add remote rule set. A pasted block of lines adds every line it can read.
 */
export function RuleEntry({ autoFocus, onAddRemote, onFocused, onInput, onSubmit, refused }: {
  /** Takes the focus as it appears, as for a set just created. */
  autoFocus: boolean;
  /** Whether the last submission was refused. */
  refused: boolean;
  /** Adds the lines with the type a bare value takes; false keeps the input. */
  onSubmit: (input: string, kind: MatchKind) => boolean;
  /** Called on each change of the value. */
  onInput: () => void;
  onAddRemote: () => void;
  onFocused: () => void;
}) {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const [kind, setKind] = useState<MatchKind>('domain_suffix');
  const [entry, setEntry] = useState('');

  useEffect(() => {
    if (!autoFocus) return;
    inputRef.current?.focus();
    onFocused();
  }, [autoFocus, onFocused]);

  function submit(input: string) {
    if (onSubmit(input, kind)) setEntry('');
  }

  return (
    <div className={cn(pickedGrid, 'h-10 flex-none border-t')}>
      <span aria-hidden='true' className={cn(gutterCell, 'text-compact', refused ? refusedTone : prompt)}>λ</span>
      <SelectField
        aria-label={t('subscriptions.ruleSets.field.kind')}
        className={kindPicker}
        itemClassName={kindOption}
        items={KIND_ITEMS}
        onValueChange={(next) => {
          setKind(next);
          inputRef.current?.focus();
        }}
        value={kind}
        variant='flat'
      />
      <input
        aria-invalid={refused || undefined}
        aria-label={t('subscriptions.ruleSets.addRule')}
        autoComplete='off'
        className={lineInput}
        data-slot='terminal-input'
        onChange={(event) => {
          setEntry(event.target.value);
          onInput();
        }}
        onKeyDown={(event) => {
          if (event.key === 'Enter' && !event.nativeEvent.isComposing) {
            event.preventDefault();
            submit(entry);
          } else if (event.key === 'Escape' && (entry !== '' || refused)) {
            event.preventDefault();
            setEntry('');
            onInput();
          }
        }}
        onPaste={(event) => {
          const pasted = event.clipboardData.getData('text');
          if (!/\n/.test(pasted.trim())) return;
          event.preventDefault();
          submit(pasted);
        }}
        placeholder={kind === 'ip_cidr' ? '192.168.0.0/16' : 'example.com'}
        ref={inputRef}
        spellCheck={false}
        value={entry}
      />
      <span className='flex items-center gap-1'>
        <IconAction
          className='@max-[30rem]/editor:hidden'
          label={t('subscriptions.ruleSets.addRule')}
          onClick={() => {
            submit(entry);
            inputRef.current?.focus();
          }}
          size='icon-xs'
        >
          <CornerDownLeft aria-hidden='true' />
        </IconAction>
        <StripDivider className='@max-md/editor:hidden' />
        <Button className='@max-2xl/editor:w-(--control-height-sm) @max-2xl/editor:px-0' onClick={onAddRemote} size='sm' type='button' variant='flat'>
          <CirclePlus aria-hidden='true' data-icon='inline-start' />
          <span className='@max-2xl/editor:sr-only'>{t('subscriptions.ruleSets.addRemote')}</span>
        </Button>
      </span>
    </div>
  );
}
