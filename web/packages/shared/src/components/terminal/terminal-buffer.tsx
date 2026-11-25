import type { CSSProperties, ReactNode, Ref } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@shared/components/ui/spinner';
import { useLayoutEffect, useMemo, useRef } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';

import type { LineTone, LogLine, TerminalBlock } from './terminal-entries';

import { PromptLabel } from './prompt-label';
import { isLogLine } from './terminal-entries';

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

/** A line's columns: a term beside its text, or a command and its usage in fixed columns, without usage on a phone. */
function Columns({ term, text, usage }: { term: string; text: string; usage?: string }) {
  if (usage === undefined) {
    return (
      <>
        {/* A long term, such as a node's name, ends in an ellipsis at half the line. */}
        <span className='max-w-1/2 min-w-[11ch] flex-none truncate pe-[1ch] text-foreground' title={term}>
          {term}
        </span>
        <span className='min-w-0'>{text}</span>
      </>
    );
  }
  return (
    <>
      <span className='w-[10ch] flex-none truncate text-foreground'>{term}</span>
      <span className='w-[15ch] flex-none truncate text-[var(--color-text-faint)] max-sm:hidden'>{usage}</span>
      <span className='min-w-0'>{text}</span>
    </>
  );
}

function Line({ clock, line, promptScope }: { clock: Intl.DateTimeFormat; line: LogLine; promptScope: boolean }) {
  // What was typed appears at once; what the terminal answers types itself out.
  const typed = line.prompt === undefined;
  return (
    <p
      className={cn(
        'whitespace-pre-wrap wrap-anywhere',
        typed && 'terminal-line',
        toneClass[line.tone],
        line.indent && 'ps-[2ch]',
        // A term leads its own column, so a description that wraps stays beside it.
        line.term !== undefined && 'flex',
        line.usage !== undefined && 'gap-[1ch]',
      )}
      data-prompt={line.prompt?.kind}
      style={typed ? typing(line) : undefined}
    >
      {line.stamp
        ? <time className='me-[1ch] text-muted-foreground' dateTime={new Date(line.at).toISOString()}>{clock.format(line.at)}</time>
        : null}
      {line.prompt === undefined
        ? null
        : (
            <>
              <PromptLabel prompt={line.prompt} scope={promptScope} />
              {' '}
            </>
          )}
      {line.term === undefined ? line.text : <Columns term={line.term} text={line.text} usage={line.usage} />}
      {line.pending ? <Spinner aria-hidden='true' className='ms-2 inline size-3.5 align-[-2px]' /> : null}
    </p>
  );
}

interface TerminalBufferProps<Block extends TerminalBlock> {
  className?: string;
  /** What follows the last output, such as the input line; absent while a command runs. */
  children?: ReactNode;
  /** Whether echoes repeat their prompt's scope; false where the scope shows elsewhere, leaving the symbol. */
  promptScope?: boolean;
  viewportClassName?: string;
  /** The log's own element, such as a place a completion menu renders into. */
  logRef?: Ref<HTMLDivElement>;
  entries: readonly (Block | LogLine)[];
  /** Renders an entry that is not a line, such as a readout. */
  renderBlock?: (block: Block) => ReactNode;
}

/**
 * Everything a terminal printed and what follows it, in one region that
 * scrolls as a whole and follows new output unless the reader scrolled back.
 */
export function TerminalBuffer<Block extends TerminalBlock>({
  children,
  className,
  entries,
  logRef,
  promptScope = true,
  renderBlock,
  viewportClassName = 'px-4 pt-1 pb-4',
}: TerminalBufferProps<Block>) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? 'en';
  const clock = useMemo(
    () => new Intl.DateTimeFormat(locale, { hour: '2-digit', hourCycle: 'h23', minute: '2-digit', second: '2-digit' }),
    [locale],
  );
  const viewportRef = useRef<HTMLDivElement>(null);
  const followRef = useRef(true);

  // Every change, a refresh included, keeps the newest line in view while the reader follows it.
  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (viewport && followRef.current) viewport.scrollTop = viewport.scrollHeight;
  });

  return (
    <ScrollArea
      // A column, so the viewport shrinks to the buffer's share even where the
      // container caps its height without setting one, as the command output does.
      className={cn('flex min-h-0 flex-1 flex-col', className)}
      viewportClassName={cn('min-h-0 flex-1', viewportClassName)}
      viewportProps={{
        onScroll: (event) => {
          const viewport = event.currentTarget;
          followRef.current = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < 8;
        },
      }}
      viewportRef={viewportRef}
    >
      <div aria-label={t('terminal.log.label')} aria-relevant='additions' ref={logRef} role='log'>
        {entries.map(entry => isLogLine(entry)
          ? <Line clock={clock} key={entry.id} line={entry} promptScope={promptScope} />
          : <div className='contents' key={entry.id}>{renderBlock?.(entry as Block)}</div>)}
      </div>
      {children}
    </ScrollArea>
  );
}
