import type { CSSProperties, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@shared/components/ui/spinner';
import { useLayoutEffect, useMemo, useRef } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';

import type { BufferEntry, LineTone, LogLine, Readout } from './buffer-entries';

import { PromptLabel } from './prompt-label';
import { StatusReadout } from './status-readout';

/** A line types itself out at this pace, within the cap a live view allows. */
const TYPE_MS_PER_CHARACTER = 6;
const TYPE_MAX_MS = 360;
const TYPE_MAX_STEPS = 48;

const toneClass: Record<LineTone, string> = {
  danger: 'text-[var(--color-danger-text)]',
  muted: 'text-muted-foreground',
  plain: 'text-foreground',
  success: 'text-[var(--color-success-text)]',
  warning: 'text-[var(--color-warning-text)]',
};

function typing(line: LogLine): CSSProperties {
  const length = line.text.length + (line.term?.length ?? 0);
  return {
    '--type-duration': `${Math.min(TYPE_MAX_MS, length * TYPE_MS_PER_CHARACTER)}ms`,
    '--type-steps': Math.max(1, Math.min(TYPE_MAX_STEPS, length)),
  } as CSSProperties;
}

function Line({ clock, host, line }: { clock: Intl.DateTimeFormat; host: string; line: LogLine }) {
  // What was typed appears at once; what the node answers types itself out.
  const typed = line.prompt === undefined;
  return (
    <p
      className={cn('whitespace-pre-wrap wrap-anywhere', typed && 'status-line', toneClass[line.tone], line.indent && 'ps-[2ch]')}
      style={typed ? typing(line) : undefined}
    >
      {line.stamp
        ? <time className='me-[1ch] text-muted-foreground' dateTime={new Date(line.at).toISOString()}>{clock.format(line.at)}</time>
        : null}
      {line.prompt === undefined
        ? null
        : (
            <>
              <PromptLabel host={host} kind={line.prompt} />
              {' '}
            </>
          )}
      {line.term === undefined ? null : <span className='inline-block min-w-[11ch] pe-[1ch] text-foreground'>{line.term}</span>}
      {line.text}
      {line.pending ? <Spinner aria-hidden='true' className='ms-2 inline size-3.5 align-[-2px]' /> : null}
    </p>
  );
}

interface TerminalBufferProps {
  host: string;
  /** What the newest status output shows. */
  live: Readout;
  stale: boolean;
  /** The input line, after the last output; absent while a command runs. */
  children: ReactNode;
  entries: readonly BufferEntry[];
}

/**
 * Everything the terminal printed and its input line, in one region that
 * scrolls as a whole and follows new output unless the reader scrolled back.
 */
export function TerminalBuffer({ children, entries, host, live, stale }: TerminalBufferProps) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? 'en';
  const clock = useMemo(
    () => new Intl.DateTimeFormat(locale, { hour: '2-digit', hourCycle: 'h23', minute: '2-digit', second: '2-digit' }),
    [locale],
  );
  const viewportRef = useRef<HTMLDivElement>(null);
  const followRef = useRef(true);

  // Every change, a refresh included, keeps the input line in view while the reader follows it.
  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (viewport && followRef.current) viewport.scrollTop = viewport.scrollHeight;
  });

  return (
    <ScrollArea
      className='min-h-0 flex-1'
      viewportClassName='px-4 pt-1 pb-4'
      viewportProps={{
        onScroll: (event) => {
          const viewport = event.currentTarget;
          followRef.current = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < 8;
        },
      }}
      viewportRef={viewportRef}
    >
      <div aria-label={t('terminal.log.label')} aria-relevant='additions' role='log'>
        {entries.map(entry => entry.kind === 'status'
          ? <StatusReadout key={entry.id} {...(entry.frozen ?? live)} stale={!entry.frozen && stale} />
          : <Line clock={clock} host={host} key={entry.id} line={entry} />)}
      </div>
      {children}
    </ScrollArea>
  );
}
