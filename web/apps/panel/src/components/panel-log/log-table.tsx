import type { MouseEvent, ReactNode } from 'react';

import { TableRow } from '@shared/components/ui/table';

import { formatLogTimeCompact } from '@/utils/panel-log-presentation';

/** When a log line happened in the data face, without the seconds on a phone; the details hold the full time. */
export function LogTime({ language, value }: { language?: string; value: string }) {
  return (
    <time className='font-[family-name:var(--font-data)] text-xs text-muted-foreground tabular-nums' dateTime={value}>
      <span className='max-sm:hidden'>{formatLogTimeCompact(value, language)}</span>
      <span className='sm:hidden'>{formatLogTimeCompact(value, language, { seconds: false })}</span>
    </time>
  );
}

/**
 * Opens a log line's details. The trigger is the line's title when the
 * keyboard opened them, where focus returns afterwards; a press passes none,
 * so the list never holds focus or shows it after the pointer was used.
 */
type OpenLog = (trigger: HTMLButtonElement | null) => void;

/**
 * A log line that opens its details from anywhere on it; its title button is
 * its keyboard target, and keyboard focus there shows only the hover wash.
 */
export function LogRow({ children, onOpen }: { children: ReactNode; onOpen: OpenLog }) {
  function openFromRow(event: MouseEvent<HTMLTableRowElement>) {
    if ((event.target as Element).closest('button')) return;
    onOpen(null);
  }
  return <TableRow interactive onClick={openFromRow}>{children}</TableRow>;
}

interface LogRowTitleProps {
  title: string;
  detail?: string;
  onOpen: OpenLog;
  /** 0 for the one line of the table that the Tab key reaches, else -1. */
  tabIndex: number;
  onFocus: () => void;
}

/** A log line's title, the button that opens its details, with what failed after it in muted data text. */
export function LogRowTitle({ detail, onFocus, onOpen, tabIndex, title }: LogRowTitleProps) {
  return (
    <button
      aria-haspopup='dialog'
      className='block min-w-0 cursor-pointer truncate text-start outline-none'
      data-row-title=''
      tabIndex={tabIndex}
      type='button'
      onFocus={onFocus}
      // A click the keyboard made has no press count; only that one returns focus here.
      // The line holds what fits; the hover shows the event with all of its context.
      title={detail ? `${title} · ${detail}` : title}
      onClick={event => onOpen(event.detail === 0 ? event.currentTarget : null)}
    >
      {title}
      {/* The space sits outside the muted part, so the accessible name keeps it too. */}
      {detail && ' '}
      {detail && <span className='font-[family-name:var(--font-data)] text-xs text-muted-foreground'>{`· ${detail}`}</span>}
    </button>
  );
}
