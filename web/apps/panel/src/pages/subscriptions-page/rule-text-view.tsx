import type { Ref } from 'react';

import { cn } from '@shared/lib/utils';
import { Textarea } from '@shared/components/ui/textarea';
import { useImperativeHandle, useLayoutEffect, useRef, useState } from 'react';

import type { RuleTextErrorCode, RuleTextIssue, RuleTextNoteCode } from './rule-text';

import { dataFace, gutterCell } from './rule-line-style';

type Issue = RuleTextIssue<RuleTextErrorCode | RuleTextNoteCode>;

/** What the gutter marks on the text's lines, numbered from 1. */
export interface RuleTextMarks {
  /** Lines that lost a policy or `no-resolve`. */
  notes: ReadonlySet<number>;
  /** Lines that cannot be read. */
  errors: ReadonlySet<number>;
  /** Lines whose rule is new or changed. */
  changed: ReadonlySet<number>;
  /** Each marked line's first problem, or what was left out of it. */
  issues: ReadonlyMap<number, Issue>;
}

export interface RuleTextHandle {
  /** Focuses the text with the caret at the start of a line and scrolls the line into view. */
  revealLine: (line: number) => void;
}

/** Every line of the text is this tall, so the gutter draws only the numbers in view. */
const LINE = 20;
/** The text's inset above its first line. */
const INSET = 12;
/** Numbers drawn beyond the view on each side, so scrolling never shows a gap. */
const OVERSCAN = 20;

/**
 * Rule set rules as classic rule lines over the gutter of their numbers. A
 * line that cannot be read takes danger on its number over a danger band,
 * one that lost a policy takes warning, a changed one the accent, each
 * number saying why on hover; the number of the caret's line reads in the
 * text colour. The gutter draws only the numbers in view, so the caret and
 * each keystroke redraw a screenful of them, however long the text.
 */
export function RuleTextView({ describe, invalid, label, lineCount, marks, onChange, ref, text }: {
  text: string;
  /** The text's lines, which the parent counts for the gutter's width. */
  lineCount: number;
  marks: RuleTextMarks;
  invalid: boolean;
  /** The text's accessible name. */
  label: string;
  /** What a marked number says on hover. */
  describe: (issue: Issue) => string;
  onChange: (text: string) => void;
  ref: Ref<RuleTextHandle>;
}) {
  const areaRef = useRef<HTMLTextAreaElement>(null);
  const gutterRef = useRef<HTMLDivElement>(null);
  const bandsRef = useRef<HTMLDivElement>(null);
  const frameRef = useRef(0);
  const [caretLine, setCaretLine] = useState(0);
  const [view, setView] = useState({ top: 0, height: 0 });

  // The gutter and the bands follow the text's scroll at once; the numbers drawn follow on the next frame.
  function follow(area: HTMLTextAreaElement) {
    if (gutterRef.current) gutterRef.current.scrollTop = area.scrollTop;
    if (bandsRef.current) bandsRef.current.style.transform = `translateY(${-area.scrollTop}px)`;
    cancelAnimationFrame(frameRef.current);
    frameRef.current = requestAnimationFrame(() => setView({ top: area.scrollTop, height: area.clientHeight }));
  }

  useLayoutEffect(() => {
    const area = areaRef.current;
    if (!area) return;
    // The observer reports the text's size as it starts and whenever it changes.
    const observer = new ResizeObserver(() => setView({ top: area.scrollTop, height: area.clientHeight }));
    observer.observe(area);
    return () => {
      observer.disconnect();
      cancelAnimationFrame(frameRef.current);
    };
  }, []);

  // Drawing other numbers keeps the gutter where the text is.
  useLayoutEffect(() => {
    if (areaRef.current && gutterRef.current) gutterRef.current.scrollTop = areaRef.current.scrollTop;
  });

  useImperativeHandle(ref, () => ({
    revealLine: (line) => {
      const area = areaRef.current;
      if (!area) return;
      const start = area.value.split('\n').slice(0, line - 1).reduce((offset, each) => offset + each.length + 1, 0);
      area.focus();
      area.setSelectionRange(start, start);
      setCaretLine(line);
      area.scrollTop = Math.max(0, (line - 1) * LINE - area.clientHeight / 2);
      if (gutterRef.current) gutterRef.current.scrollTop = area.scrollTop;
      if (bandsRef.current) bandsRef.current.style.transform = `translateY(${-area.scrollTop}px)`;
      setView({ top: area.scrollTop, height: area.clientHeight });
    },
  }), []);

  // A text that just got shorter may not have reported its new scroll yet.
  const height = Math.max(view.height, LINE * 40);
  const top = Math.min(view.top, Math.max(0, lineCount * LINE + 2 * INSET - height));
  const first = Math.max(0, Math.floor((top - INSET) / LINE) - OVERSCAN);
  const last = Math.min(lineCount, Math.ceil((top + height) / LINE) + OVERSCAN);
  const drawn = Array.from({ length: Math.max(0, last - first) }, (_, index) => first + index + 1);
  const lineTone = (line: number) => {
    if (marks.errors.has(line)) return 'text-[var(--color-danger-text)]';
    if (marks.notes.has(line)) return 'text-[var(--color-warning-text)]';
    if (marks.changed.has(line)) return 'text-primary';
    return line === caretLine ? 'text-foreground' : 'text-[var(--color-text-faint)]';
  };

  return (
    <div className='relative flex min-h-0 flex-1 overflow-hidden'>
      {/* A line that cannot be read sits on a danger band, which scrolls with the text. */}
      <div aria-hidden='true' className='pointer-events-none absolute inset-0 overflow-hidden'>
        <div className='absolute inset-x-0 top-0' ref={bandsRef}>
          {drawn.filter(line => marks.errors.has(line)).map(line => (
            <span className='absolute inset-x-0 h-5 bg-[var(--color-danger-soft)]' key={line} style={{ top: `calc(0.75rem + ${(line - 1) * 1.25}rem)` }} />
          ))}
        </div>
      </div>
      {/* The numbers out of view keep their room as spacers, so the gutter scrolls as far as the text. */}
      <div aria-hidden='true' className={cn(gutterCell, 'relative w-[calc(1rem+var(--rule-gutter))] flex-none overflow-hidden py-3 ps-4 select-none [overflow-anchor:none]')} ref={gutterRef}>
        {first > 0 ? <div style={{ height: first * LINE }} /> : null}
        {drawn.map((line) => {
          const issue = marks.issues.get(line);
          return <div className={lineTone(line)} key={line} title={issue ? describe(issue) : undefined}>{line}</div>;
        })}
        {last < lineCount ? <div style={{ height: (lineCount - last) * LINE }} /> : null}
      </div>
      <Textarea
        aria-invalid={invalid || undefined}
        aria-label={label}
        className={cn(dataFace, 'relative min-h-0 flex-1 resize-none rounded-none border-0 bg-transparent py-3 ps-3 pe-4 text-compact leading-5 focus-visible:outline-none aria-invalid:ring-0 md:text-compact dark:bg-transparent @max-[30rem]/editor:ps-2')}
        onChange={event => onChange(event.target.value)}
        onScroll={event => follow(event.currentTarget)}
        onSelect={(event) => {
          const area = event.currentTarget;
          setCaretLine(area.value.slice(0, area.selectionStart).split('\n').length);
        }}
        ref={areaRef}
        spellCheck={false}
        value={text}
        wrap='off'
      />
    </div>
  );
}
