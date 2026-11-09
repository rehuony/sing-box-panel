import type { KeyboardEvent, Ref } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { useId, useLayoutEffect, useRef, useState } from 'react';

import type { PromptKind } from './buffer-entries';

import { PromptLabel } from './prompt-label';

/** A command Tab can complete, with what it does. */
export interface Completion {
  name: string;
  description: string;
}

/**
 * The completion menu under the line. Nothing is chosen until Tab moves into
 * it; the line then shows the chosen command, and leaving the menu without
 * accepting puts back what was typed.
 */
interface Menu {
  typed: string;
  chosen: number | null;
  candidates: readonly Completion[];
}

interface CommandLineProps {
  host: string;
  /** The input's name for assistive technology, which follows the prompt. */
  label: string;
  prompt: PromptKind;
  onCancel: () => void;
  /** A hint shown while the line is empty. */
  placeholder?: string;
  /** The commands Tab completes at the shell prompt, in the order the menu lists them. */
  commands: readonly Completion[];
  inputRef: Ref<HTMLInputElement>;
  onSubmit: (input: string) => void;
}

/** The commands a typed word could begin, and the prefix they all share. */
function candidatesFor(commands: readonly Completion[], typed: string): { candidates: Completion[]; shared: string } {
  const word = typed.toLowerCase();
  const candidates = commands.filter(command => command.name.startsWith(word));
  const shared = candidates.length === 0
    ? typed
    : candidates.map(command => command.name).reduce((prefix, name) => {
        let length = 0;
        while (length < prefix.length && prefix[length] === name[length]) length += 1;
        return prefix.slice(0, length);
      });
  return { candidates, shared };
}

/**
 * The line the terminal reads input on, after its last output. At the shell
 * prompt it behaves as a shell's: Up and Down recall earlier commands, and Tab
 * completes a command name, opening a menu when several fit.
 */
export function CommandLine({
  commands,
  host,
  inputRef,
  label,
  placeholder,
  prompt,
  onCancel,
  onSubmit,
}: CommandLineProps) {
  const { t } = useTranslation();
  const shell = prompt === 'shell';
  const [value, setValue] = useState('');
  const [menu, setMenu] = useState<Menu | null>(null);
  const entriesRef = useRef<string[]>([]);
  // Where Up and Down stand in the history; its length is the line being typed.
  const [cursor, setCursor] = useState(0);
  const draftRef = useRef('');
  const menuRef = useRef<HTMLUListElement>(null);
  const menuId = useId();

  // The menu opens under the line, which may sit at the bottom of the view.
  useLayoutEffect(() => {
    menuRef.current?.scrollIntoView({ block: 'nearest' });
  }, [menu]);

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
    setValue(next === entries.length ? draftRef.current : entries[next]);
  }

  /** A command fills the line with a space after it, ready to run; nothing runs until Enter. */
  function accept(name: string) {
    setValue(`${name} `);
    setMenu(null);
  }

  // A prefix of one command completes it; a prefix of several, or an empty line, opens the menu.
  function complete() {
    const typed = value.trimStart();
    // Commands take no arguments, so nothing completes after the first word.
    if (/\s/.test(typed)) return;
    const { candidates, shared } = candidatesFor(commands, typed);
    if (candidates.length === 0) return;
    if (candidates.length === 1) {
      accept(candidates[0].name);
      return;
    }
    const prefix = shared.length > typed.length ? shared : typed;
    setValue(prefix);
    setMenu({ candidates, chosen: null, typed: prefix });
  }

  function choose(current: Menu, step: -1 | 1) {
    const count = current.candidates.length;
    const chosen = current.chosen === null
      ? (step === 1 ? 0 : count - 1)
      : (current.chosen + step + count) % count;
    setMenu({ ...current, chosen });
    setValue(current.candidates[chosen].name);
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
        accept(chosen.name);
        return true;
      case 'Escape':
        setValue(current.typed);
        setMenu(null);
        return true;
      default:
        return false;
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (menu && onMenuKey(event, menu)) {
      event.preventDefault();
      return;
    }
    const input = event.currentTarget;
    const selected = input.selectionStart !== input.selectionEnd;
    if (event.key === 'Enter') {
      event.preventDefault();
      submit();
    } else if (event.key === 'Escape' || (event.key === 'c' && event.ctrlKey && !selected)) {
      // Ctrl+C copies a selection as usual; without one it interrupts, as in a shell.
      event.preventDefault();
      take();
      if (!shell) onCancel();
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
  return (
    <>
      <div className='flex flex-wrap items-baseline gap-x-[1ch]'>
        <PromptLabel host={host} kind={prompt} />
        <input
          aria-activedescendant={chosenId}
          aria-autocomplete={shell ? 'list' : undefined}
          aria-controls={menu ? menuId : undefined}
          aria-expanded={shell ? menu !== null : undefined}
          aria-label={label}
          autoCapitalize='off'
          autoComplete='off'
          autoCorrect='off'
          className='status-prompt min-w-[12ch] flex-1 bg-transparent p-0 text-base leading-5 text-[var(--color-terminal-text)] outline-none placeholder:text-[var(--color-terminal-muted)] sm:text-compact'
          data-slot='status-prompt-input'
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
          ref={inputRef}
          role={shell ? 'combobox' : undefined}
          spellCheck={false}
          value={value}
        />
      </div>
      {menu
        ? (
            <ul aria-label={t('terminal.completions')} id={menuId} ref={menuRef} role='listbox'>
              {menu.candidates.map((command, index) => {
                const chosen = index === menu.chosen;
                return (
                  <li aria-selected={chosen} className='flex gap-[1ch]' id={`${menuId}-${index}`} key={command.name} role='option'>
                    {/* The chosen command reads in reverse, its block reaching a little past the text. */}
                    <span
                      className={cn(
                        'inline-block min-w-[11ch] rounded-[2px]',
                        chosen
                          ? 'bg-[var(--color-terminal-text)] text-[var(--color-terminal)] shadow-[0_0_0_2px_var(--color-terminal-text)]'
                          : 'text-[var(--color-terminal-text)]',
                      )}
                    >
                      {command.name}
                    </span>
                    <span className={chosen ? 'text-[var(--color-terminal-text)]' : 'text-[var(--color-terminal-muted)]'}>
                      {command.description}
                    </span>
                  </li>
                );
              })}
            </ul>
          )
        : null}
    </>
  );
}
