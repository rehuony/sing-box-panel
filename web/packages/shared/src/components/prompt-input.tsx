import type { CSSProperties, InputHTMLAttributes, Ref } from 'react';

import { useEffect, useImperativeHandle, useLayoutEffect, useRef, useState } from 'react';

type NativeProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'className' | 'onChange' | 'placeholder' | 'readOnly' | 'style' | 'type' | 'value'>;

interface PromptInputProps extends NativeProps {
  value: string;
  /** While true the line keeps its value and shows no cursor. */
  busy?: boolean;
  /** Draws each character as a dot rather than itself. */
  masked: boolean;
  /** Shown while the line is empty and unfocused. */
  placeholder: string;
  ref?: Ref<HTMLInputElement>;
  onChange: (value: string) => void;
}

/** One masked cell, in em; the input's letter spacing widens its dots to match. */
const DOT_CELL_EM = 0.75;
const PROBE_LENGTH = 40;

/** The input's widths of one character, in pixels. */
interface Advance {
  /** A masked cell. */
  dot: number;
  text: number;
  password: number;
}

/** The width the browser gives one character of an input of type, in the font of reference. */
function nativeAdvance(reference: HTMLInputElement, type: 'password' | 'text'): number {
  const style = getComputedStyle(reference);
  const probe = document.createElement('input');
  probe.type = type;
  probe.tabIndex = -1;
  probe.value = '0'.repeat(PROBE_LENGTH);
  Object.assign(probe.style, {
    border: '0', fontFamily: style.fontFamily, fontSize: style.fontSize, fontWeight: style.fontWeight,
    left: '-10000px', letterSpacing: '0', padding: '0', position: 'fixed', top: '0', width: '1px',
  });
  document.body.append(probe);
  const advance = probe.scrollWidth / PROBE_LENGTH;
  probe.remove();
  return advance;
}

/** A UTF-16 offset of the input as an index into its code points, which the mirror draws. */
function codePointIndex(value: string, offset: number): number {
  return [...value.slice(0, offset)].length;
}

/**
 * A terminal prompt's input line. The real input stays in place and keeps the
 * value, focus, selection and autofill, but draws nothing; a mirror draws each
 * character, or a dot for it, in a cell as wide as the input's character, so
 * pointer selection lands on the cells it covers. The mirror draws the
 * selection and the cursor too: a block after the last character, a bar
 * between characters.
 */
export function PromptInput({
  busy = false,
  masked,
  onBlur,
  onChange,
  onFocus,
  onKeyUp,
  placeholder,
  ref,
  value,
  ...props
}: PromptInputProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const mirrorRef = useRef<HTMLSpanElement>(null);
  const [focused, setFocused] = useState(false);
  const [selection, setSelection] = useState({ end: value.length, start: value.length });
  const [advance, setAdvance] = useState<Advance | null>(null);
  useImperativeHandle(ref, () => inputRef.current!, []);

  // Autofocus in a window without focus fires no focus event; read the state once.
  useLayoutEffect(() => {
    if (document.activeElement === inputRef.current) setFocused(true);
  }, []);

  // Measure the input's characters for the mirror's cells, again once the data face loads.
  useLayoutEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    let active = true;
    function measure() {
      if (!active || !input) return;
      const dot = Number.parseFloat(getComputedStyle(input).fontSize) * DOT_CELL_EM;
      const password = nativeAdvance(input, 'password');
      const text = nativeAdvance(input, 'text');
      if (password > 0 && text > 0) setAdvance({ dot, password, text });
    }
    measure();
    void document.fonts?.ready.then(measure);
    return () => {
      active = false;
    };
  }, []);

  // Dragging a selection changes it before any key or pointer release.
  useEffect(() => {
    function follow() {
      if (document.activeElement === inputRef.current) syncSelection();
    }
    document.addEventListener('selectionchange', follow);
    return () => document.removeEventListener('selectionchange', follow);
  }, []);

  // The mirror scrolls with the input, so overflowing cells stay under the caret.
  useLayoutEffect(() => {
    const input = inputRef.current;
    const mirror = mirrorRef.current;
    if (input && mirror) mirror.scrollLeft = input.scrollLeft;
  }, [value, selection, masked, advance]);

  const characters = [...value];
  const start = Math.min(codePointIndex(value, selection.start), characters.length);
  const end = Math.min(codePointIndex(value, selection.end), characters.length);
  const showCursor = focused && !busy && start === end;
  // Text keeps the face's own advance; dots sit in wider cells the input's spacing matches.
  const letterSpacing = advance && masked ? `${advance.dot - advance.password}px` : undefined;
  const cell = advance ? `${masked ? advance.dot : advance.text}px` : `${DOT_CELL_EM}em`;

  function syncSelection() {
    const input = inputRef.current;
    if (!input) return;
    const from = input.selectionStart ?? input.value.length;
    const to = input.selectionEnd ?? from;
    setSelection(current => current.start === from && current.end === to ? current : { end: to, start: from });
    if (mirrorRef.current) mirrorRef.current.scrollLeft = input.scrollLeft;
  }

  return (
    <label className='relative flex h-full min-w-0 flex-1 cursor-text items-center'>
      <input
        {...props}
        className='prompt-input absolute inset-0 size-full bg-transparent text-base text-transparent caret-transparent outline-none'
        // The prompt draws focus on its own box, so the global field outline stays off.
        data-slot='prompt-input'
        onBlur={(event) => {
          setFocused(false);
          onBlur?.(event);
        }}
        onChange={(event) => {
          onChange(event.target.value);
          syncSelection();
        }}
        onFocus={(event) => {
          setFocused(true);
          syncSelection();
          onFocus?.(event);
        }}
        onKeyUp={(event) => {
          syncSelection();
          onKeyUp?.(event);
        }}
        onScroll={syncSelection}
        onSelect={syncSelection}
        readOnly={busy}
        ref={inputRef}
        spellCheck={false}
        style={{ letterSpacing }}
        type={masked ? 'password' : 'text'}
        value={value}
      />
      <span
        aria-hidden='true'
        className='prompt-input__mirror pointer-events-none flex h-full min-w-0 items-center overflow-hidden text-base whitespace-pre'
        ref={mirrorRef}
        style={{ '--prompt-cell': cell } as CSSProperties}
      >
        {characters.map((character, index) => (
          // Each character keeps its index, so only a new one pops in.
          // eslint-disable-next-line react/no-array-index-key
          <span className='prompt-input__cell' data-selected={(focused && index >= start && index < end) || undefined} key={index}>
            {showCursor && index === start ? <span className='prompt-input__caret prompt-input__caret--bar' /> : null}
            {masked ? <span className='prompt-input__dot' /> : character}
          </span>
        ))}
        {value !== '' || focused
          ? (
              // A last, empty cell holds the cursor after the last character.
              <span className='prompt-input__cell prompt-input__end'>
                {showCursor && start === characters.length ? <span className='prompt-input__caret' /> : null}
              </span>
            )
          : null}
        {value === '' && !focused
          ? <span className='truncate font-light text-[color-mix(in_oklab,var(--color-text-faint)_70%,transparent)]'>{placeholder}</span>
          : null}
      </span>
    </label>
  );
}
