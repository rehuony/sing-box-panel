import type { KeyboardEvent, Ref, RefObject } from 'react';

import { cn } from '@shared/lib/utils';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useId, useLayoutEffect, useRef, useState } from 'react';

import type { Prompt } from './terminal-entries';
import type { CommandWord } from './command-words';

import { PromptLabel } from './prompt-label';
import { quoteWord, splitWords } from './command-words';

/** A value Tab can complete, with what it means and, for a command, the arguments it takes. */
export interface Completion {
  name: string;
  /** A command's arguments; a list where any value has them shows them in a column of their own. */
  usage?: string;
  description?: string;
}

/**
 * The candidates for one word of the line: the line's word values and the
 * position of the word being completed among them.
 */
export type CompletionSource = (words: readonly string[], index: number) => readonly Completion[];

/**
 * The completion menu. Nothing is chosen until Tab moves into it; the line
 * then shows the chosen value in place of the word, and leaving the menu
 * without accepting puts back what was typed.
 */
interface Menu {
  end: number;
  /** Where the completed word stands in the typed line. */
  start: number;
  /** The line as typed when the menu opened. */
  typed: string;
  chosen: number | null;
  candidates: readonly Completion[];
}

interface CommandLineProps {
  /** The input's name for assistive technology, which follows the prompt. */
  label: string;
  prompt: Prompt;
  className?: string;
  /** Escape at the shell prompt while no menu is open; without it Escape clears the line. */
  onExit?: () => void;
  /** A hint shown while the line is empty. */
  placeholder?: string;
  /** Escape or Ctrl+C at a code or confirmation prompt. */
  onCancel?: () => void;
  /** Whether the prompt shows its scope; false where the scope shows elsewhere, leaving the symbol. */
  promptScope?: boolean;
  /** Earlier commands, kept by the caller when the line outlives one mount. */
  history?: RefObject<string[]>;
  /** What Tab completes at the shell prompt, in the order the menu lists it. */
  completions?: CompletionSource;
  inputRef?: Ref<HTMLInputElement>;
  onSubmit: (input: string) => void;
  /** Where the menu renders, such as a box below the line's own; absent puts it under the line. */
  menuContainer?: HTMLElement | null;
}

/** The prefix every name shares. */
function sharedPrefix(names: readonly string[]): string {
  return names.reduce((prefix, name) => {
    let length = 0;
    while (length < prefix.length && prefix[length] === name[length]) length += 1;
    return prefix.slice(0, length);
  });
}

/** The word under the caret, or an empty word where the caret stands between words. */
function wordAt(line: string, caret: number): { index: number; values: string[]; word: CommandWord } {
  const words = splitWords(line);
  const index = words.findIndex(word => word.start <= caret && caret <= word.end);
  if (index >= 0) return { index, values: words.map(word => word.value), word: words[index]! };
  const before = words.filter(word => word.end < caret).length;
  const values = words.map(word => word.value);
  values.splice(before, 0, '');
  return { index: before, values, word: { end: caret, start: caret, value: '' } };
}

/**
 * The line a terminal reads input on. At the shell prompt it behaves as a
 * shell's: Up and Down recall earlier commands, and Tab completes the word
 * under the caret, opening a menu when several values fit.
 */
export function CommandLine({
  className,
  completions,
  history,
  inputRef,
  label,
  menuContainer,
  placeholder,
  prompt,
  promptScope = true,
  onCancel,
  onExit,
  onSubmit,
}: CommandLineProps) {
  const { t } = useTranslation();
  const shell = prompt.kind === 'shell';
  const [value, setValue] = useState('');
  const [menu, setMenu] = useState<Menu | null>(null);
  // The value under the pointer, which takes the highlight from the chosen one until the keyboard moves again.
  const [pointed, setPointed] = useState<number | null>(null);
  const ownHistoryRef = useRef<string[]>([]);
  const entriesRef = history ?? ownHistoryRef;
  // Where Up and Down stand in the history; its length is the line being typed.
  const [cursor, setCursor] = useState(() => entriesRef.current.length);
  const draftRef = useRef('');
  const elementRef = useRef<HTMLInputElement | null>(null);
  const caretRef = useRef<number | null>(null);
  const menuRef = useRef<HTMLUListElement>(null);
  const menuId = useId();

  // A completion puts the caret after the value it inserted.
  useLayoutEffect(() => {
    const caret = caretRef.current;
    if (caret === null) return;
    caretRef.current = null;
    elementRef.current?.setSelectionRange(caret, caret);
  }, [value]);

  // The chosen value, or the menu itself, stays in view in whatever scrolls around it.
  useLayoutEffect(() => {
    if (menu === null) return;
    const chosen = menu.chosen === null ? null : document.getElementById(`${menuId}-${menu.chosen}`);
    (chosen ?? menuRef.current)?.scrollIntoView({ block: 'nearest' });
  }, [menu, menuId]);

  function setInput(element: HTMLInputElement | null) {
    elementRef.current = element;
    if (typeof inputRef === 'function') inputRef(element);
    else if (inputRef) inputRef.current = element;
  }

  function show(line: string, caret = line.length) {
    // An unchanged line renders nothing, so the caret moves now rather than after a render that never comes.
    if (line === value) {
      elementRef.current?.setSelectionRange(caret, caret);
      return;
    }
    caretRef.current = caret;
    setValue(line);
  }

  function take(): string {
    const input = value;
    setValue('');
    setMenu(null);
    setCursor(entriesRef.current.length);
    return input;
  }

  function submit() {
    const input = take();
    const entry = input.trim();
    if (shell && entry !== '' && entriesRef.current.at(-1) !== entry) {
      entriesRef.current = [...entriesRef.current, entry];
      setCursor(entriesRef.current.length);
    }
    onSubmit(input);
  }

  function browse(step: -1 | 1) {
    const entries = entriesRef.current;
    const next = Math.min(entries.length, Math.max(0, cursor + step));
    if (next === cursor) return;
    if (cursor === entries.length) draftRef.current = value;
    setCursor(next);
    show(next === entries.length ? draftRef.current : entries[next]!);
  }

  /** The typed line with one value in place of the completed word; a final value at the line's end gains a space. */
  function filled(current: Pick<Menu, 'end' | 'start' | 'typed'>, name: string, final: boolean): { caret: number; line: string } {
    const word = quoteWord(name);
    const rest = current.typed.slice(current.end);
    const spaced = final && !/^\s/.test(rest) ? ' ' : '';
    const head = `${current.typed.slice(0, current.start)}${word}${spaced}`;
    return { caret: head.length, line: `${head}${rest}` };
  }

  function accept(current: Pick<Menu, 'end' | 'start' | 'typed'>, name: string) {
    const { caret, line } = filled(current, name, true);
    show(line, caret);
    setMenu(null);
  }

  // A prefix of one value completes it; a prefix of several, or an empty word, opens the menu.
  function complete() {
    if (!completions) return;
    const caret = elementRef.current?.selectionStart ?? value.length;
    const { index, values, word } = wordAt(value, caret);
    const typed = word.value.toLowerCase();
    const candidates = completions(values, index).filter(candidate => candidate.name.toLowerCase().startsWith(typed));
    const range = { end: word.end, start: word.start, typed: value };
    if (candidates.length === 0) return;
    if (candidates.length === 1) {
      accept(range, candidates[0]!.name);
      return;
    }
    const prefix = sharedPrefix(candidates.map(candidate => candidate.name));
    if (prefix.length > word.value.length) {
      // The shared prefix fills the word, and the menu opens on the longer line.
      const { caret: end, line } = filled(range, prefix, false);
      show(line, end);
      setMenu({ candidates, chosen: null, end, start: word.start, typed: line });
      setPointed(null);
      return;
    }
    setMenu({ candidates, chosen: null, ...range });
    setPointed(null);
  }

  function choose(current: Menu, step: -1 | 1) {
    const count = current.candidates.length;
    const chosen = current.chosen === null
      ? (step === 1 ? 0 : count - 1)
      : (current.chosen + step + count) % count;
    setMenu({ ...current, chosen });
    setPointed(null);
    const { caret, line } = filled(current, current.candidates[chosen]!.name, false);
    show(line, caret);
  }

  /** Handles a key while the menu is open; returns whether the menu took it. */
  function onMenuKey(event: KeyboardEvent<HTMLInputElement>, current: Menu): boolean {
    const chosen = current.chosen === null ? null : current.candidates[current.chosen];
    switch (event.key) {
      case 'Tab':
      case 'ArrowDown':
        choose(current, event.key === 'Tab' && event.shiftKey ? -1 : 1);
        return true;
      case 'ArrowUp':
        choose(current, -1);
        return true;
      case 'Enter':
      case ' ':
        // With nothing chosen, Enter runs the line and a space is typed, as without the menu.
        if (!chosen) return false;
        accept(current, chosen.name);
        return true;
      case 'Escape':
        show(current.typed, current.end);
        setMenu(null);
        return true;
      default:
        return false;
    }
  }

  /**
   * A shell's line-editing keys: Ctrl+A and Ctrl+E move to the ends of the line, Ctrl+U deletes back to its
   * start and Ctrl+W the word before the caret. They close the menu, as typing does. Returns whether the key
   * was one of them.
   */
  function editLine(key: string, input: HTMLInputElement): boolean {
    const caret = input.selectionStart ?? value.length;
    switch (key) {
      case 'a':
        show(value, 0);
        break;
      case 'e':
        show(value, value.length);
        break;
      case 'u':
      case 'w': {
        const from = key === 'u' ? 0 : value.slice(0, caret).replace(/\S*\s*$/, '').length;
        show(`${value.slice(0, from)}${value.slice(input.selectionEnd ?? caret)}`, from);
        setCursor(entriesRef.current.length);
        break;
      }
      default:
        return false;
    }
    setMenu(null);
    return true;
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (menu && onMenuKey(event, menu)) {
      event.preventDefault();
      event.stopPropagation();
      return;
    }
    const input = event.currentTarget;
    const control = event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey;
    if (control && editLine(event.key.toLowerCase(), input)) {
      event.preventDefault();
      return;
    }
    const selected = input.selectionStart !== input.selectionEnd;
    if (event.key === 'Enter') {
      event.preventDefault();
      submit();
    } else if (event.key === 'Escape' && shell && onExit) {
      event.preventDefault();
      event.stopPropagation();
      onExit();
    } else if (event.key === 'Escape' || (event.key === 'c' && event.ctrlKey && !selected)) {
      // Ctrl+C copies a selection as usual; without one it interrupts, as in a shell.
      event.preventDefault();
      take();
      if (!shell) onCancel?.();
    } else if (shell && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
      event.preventDefault();
      browse(event.key === 'ArrowUp' ? -1 : 1);
    } else if (event.key === 'Tab' && !event.shiftKey) {
      // Tab stays on the line, as in a shell, at every prompt; Shift+Tab still leaves it while no menu is open.
      event.preventDefault();
      if (shell) complete();
    }
  }

  const chosenId = menu && menu.chosen !== null ? `${menuId}-${menu.chosen}` : undefined;
  // A list of commands with arguments sets them in a column between name and description, except on a phone,
  // whose rows are touch height and keep only name and description.
  const usage = menu?.candidates.some(candidate => candidate.usage !== undefined) ?? false;
  const list = menu
    ? (
        <ul aria-label={t('terminal.completions')} className='p-1' id={menuId} onPointerLeave={() => setPointed(null)} ref={menuRef} role='listbox'>
          {menu.candidates.map((candidate, index) => (
            <li
              aria-selected={index === menu.chosen}
              className={cn(
                'flex h-7 cursor-pointer items-center max-sm:h-10 gap-[1ch] rounded-[var(--radius-control)] px-2 whitespace-nowrap',
                index === (pointed ?? menu.chosen) && 'bg-[var(--color-hover)]',
              )}
              id={`${menuId}-${index}`}
              key={candidate.name}
              // A press takes the value as Enter does, and the line keeps focus.
              onMouseDown={(event) => {
                event.preventDefault();
                accept(menu, candidate.name);
              }}
              onPointerMove={() => setPointed(index)}
              role='option'
            >
              <span className={cn('flex-none truncate text-foreground', usage ? 'w-[10ch]' : 'w-[16ch]')} title={candidate.name}>
                {candidate.name}
              </span>
              {usage ? <span className='w-[15ch] flex-none truncate text-[var(--color-text-faint)] max-sm:hidden'>{candidate.usage}</span> : null}
              {candidate.description ? <span className='min-w-0 truncate text-muted-foreground'>{candidate.description}</span> : null}
            </li>
          ))}
        </ul>
      )
    : null;

  return (
    <>
      <div className={cn('flex min-w-0 flex-wrap items-baseline gap-x-[1ch]', className)}>
        {/* Where the line cannot wrap, the prompt gives way before the input. */}
        <span className='min-w-0 truncate'><PromptLabel prompt={prompt} scope={promptScope} /></span>
        <input
          aria-activedescendant={chosenId}
          aria-autocomplete={shell ? 'list' : undefined}
          aria-controls={menu ? menuId : undefined}
          aria-expanded={shell ? menu !== null : undefined}
          aria-label={label}
          autoCapitalize='off'
          autoComplete='off'
          autoCorrect='off'
          className='terminal-input min-w-[12ch] flex-1 bg-transparent p-0 text-base leading-5 text-foreground outline-none placeholder:text-muted-foreground sm:text-compact'
          data-slot='terminal-input'
          enterKeyHint='send'
          maxLength={4200}
          // Leaving the line keeps what it shows and closes the menu.
          onBlur={() => setMenu(null)}
          onChange={(event) => {
            setValue(event.target.value);
            setMenu(null);
            setCursor(entriesRef.current.length);
          }}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          ref={setInput}
          role={shell ? 'combobox' : undefined}
          spellCheck={false}
          value={value}
        />
      </div>
      {list && menuContainer ? createPortal(list, menuContainer) : list}
    </>
  );
}
