import type { ClipboardEvent, DragEvent } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Fragment, useRef, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { formatBytes } from '@shared/lib/telemetry-format';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { ClipboardPaste, Eye, FileText, Upload, X } from 'lucide-react';
import { Popover, PopoverContent, PopoverTrigger } from '@shared/components/ui/popover';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

import type { SubscriptionSource } from '@/api/api-client';

import { IconAction } from '@/components/icon-action';

import type { SourceContent } from './source-content';

import { contentLabel } from './source-format';
import { pastedContent, readContentFile } from './source-content';

/** The view of new content shows at most this many lines, and of each at most this many characters. */
const VIEW_LINES = 2000;
const VIEW_LINE_CHARACTERS = 4000;

/**
 * New content as the server reads it, one numbered line a row, so a line a
 * problem names can be found. Long lines wrap; a very long content shows its
 * first lines and says so.
 */
function ContentView({ text }: { text: string }) {
  const { t } = useTranslation();
  const lines = text.split(/\r?\n/);
  const shown = lines.slice(0, VIEW_LINES);
  return (
    <>
      <ScrollArea viewportClassName='max-h-72'>
        <ol className='grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 px-3 py-2 font-[family-name:var(--font-data)] text-xs leading-5'>
          {shown.map((line, index) => (
            // Lines repeat, so their position is their identity.
            // eslint-disable-next-line react/no-array-index-key
            <li className='contents' key={index}>
              <span aria-hidden='true' className='text-end text-[var(--color-text-faint)] tabular-nums select-none'>{index + 1}</span>
              <span className='break-all whitespace-pre-wrap'>{line.length > VIEW_LINE_CHARACTERS ? `${line.slice(0, VIEW_LINE_CHARACTERS)}…` : line}</span>
            </li>
          ))}
        </ol>
      </ScrollArea>
      {lines.length > shown.length
        ? <p className='border-t px-3 py-1.5 text-xs text-muted-foreground'>{t('subscriptions.sources.content.firstLines', { count: shown.length, total: lines.length })}</p>
        : null}
    </>
  );
}

/**
 * A file source's content as one field the size of an input: what it holds,
 * then actions to paste or choose a file. The field takes a paste from the
 * keyboard while it has focus and a file dropped on it; new content shows
 * its size, an action that opens its text in a floating view, and the action
 * that puts back what the source holds.
 */
export function SourceContentField({ current, describedBy, invalid, labelId, onChange, onError, protocols, value }: {
  /** The edited source, whose stored content the field stands for until new content is given. */
  current: Pick<SubscriptionSource, 'content_format'> | undefined;
  /** The protocols of the stored content's nodes, once read. */
  protocols?: readonly string[];
  value: SourceContent | null;
  onChange: (content: SourceContent | null) => void;
  /** Says why content could not be taken, in the form's message slot. */
  onError: (text: string) => void;
  labelId: string;
  invalid?: boolean;
  describedBy?: string;
}) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const fileRef = useRef<HTMLInputElement>(null);
  const fieldRef = useRef<HTMLDivElement>(null);
  const [dropping, setDropping] = useState(false);

  function take(text: string) {
    if (text.trim() === '') return;
    const content = pastedContent(text, t);
    if (typeof content === 'string') onError(content);
    else onChange(content);
  }

  async function read(file: File | undefined) {
    if (!file) return;
    const content = await readContentFile(file, t);
    if (typeof content === 'string') onError(content);
    else onChange(content);
  }

  function onPaste(event: ClipboardEvent<HTMLDivElement>) {
    const text = event.clipboardData.getData('text');
    if (text.trim() === '') return;
    event.preventDefault();
    take(text);
  }

  function onDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDropping(false);
    void read(event.dataTransfer.files[0]);
  }

  // What the field stands for: its name first, then its details in the data face.
  let parts: string[] = [];
  if (value?.origin.kind === 'file') {
    parts = [value.origin.name, formatBytes(value.size, locale)];
  } else if (value) {
    parts = [t('subscriptions.sources.content.pasted'), t('subscriptions.sources.content.lines', { count: value.origin.lines }), formatBytes(value.size, locale)];
  } else if (current) {
    const label = contentLabel(current.content_format, protocols, t);
    parts = [t('subscriptions.sources.content.current'), ...(label ? [label] : [])];
  }
  const summary = parts.length === 0
    ? <span className='text-muted-foreground'>{t('subscriptions.sources.content.placeholder')}</span>
    : parts.map((part, index) => index === 0
        ? <span className={cn(value?.origin.kind === 'file' && 'font-[family-name:var(--font-data)] text-compact')} key={part}>{part}</span>
        : (
            <Fragment key={part}>
              <span aria-hidden='true' className='mx-1.5 text-[var(--color-text-faint)]'>·</span>
              <span className='font-[family-name:var(--font-data)] text-xs text-[var(--color-text-secondary)] tabular-nums'>{part}</span>
            </Fragment>
          ));
  const reset = current ? t('subscriptions.sources.content.restore') : t('subscriptions.sources.content.clear');

  return (
    <div
      aria-describedby={describedBy}
      aria-labelledby={labelId}
      className={cn(
        'flex h-(--control-height) min-w-0 items-center gap-2 rounded-lg border border-input bg-[var(--color-field)] ps-3 pe-1 text-sm outline-none',
        'focus-visible:border-[var(--input-focus)]',
        dropping && 'outline-1 -outline-offset-1 outline-input-focus outline-dashed',
        invalid && 'border-destructive focus-visible:border-destructive',
      )}
      data-slot='source-content'
      onDragLeave={() => setDropping(false)}
      onDragOver={(event) => {
        event.preventDefault();
        setDropping(true);
      }}
      onDrop={onDrop}
      onPaste={onPaste}
      ref={fieldRef}
      role='group'
      tabIndex={0}
    >
      <FileText aria-hidden='true' className='size-4 flex-none text-muted-foreground' />
      <span className='min-w-0 flex-1 truncate' role='status' title={parts.join(' · ') || undefined}>{summary}</span>
      <span className='flex flex-none items-center gap-0.5'>
        {value
          ? (
              <IconAction label={reset} onClick={() => onChange(null)} size='icon-xs'>
                <X aria-hidden='true' />
              </IconAction>
            )
          : null}
        {value
          ? (
              <Popover>
                <Tooltip>
                  <TooltipTrigger
                    render={<PopoverTrigger render={<Button aria-label={t('subscriptions.sources.content.view')} size='icon-xs' type='button' variant='ghost' />} />}
                  >
                    <Eye aria-hidden='true' />
                  </TooltipTrigger>
                  <TooltipContent>{t('subscriptions.sources.content.view')}</TooltipContent>
                </Tooltip>
                <PopoverContent
                  align='start'
                  anchor={fieldRef}
                  aria-label={t('subscriptions.sources.content.view')}
                  className='w-(--anchor-width) gap-0 overflow-hidden p-0'
                  sideOffset={6}
                >
                  <ContentView text={value.text} />
                </PopoverContent>
              </Popover>
            )
          : null}
        <IconAction
          label={t('subscriptions.sources.content.paste')}
          onClick={() => {
            // The clipboard is absent outside a secure context, and reading it may be refused.
            const reading = navigator.clipboard?.readText() ?? Promise.reject(new Error('clipboard unavailable'));
            reading.then(take, () => onError(t('subscriptions.sources.content.clipboardUnavailable')));
          }}
          size='icon-xs'
        >
          <ClipboardPaste aria-hidden='true' />
        </IconAction>
        <IconAction label={t('subscriptions.sources.chooseFile')} onClick={() => fileRef.current?.click()} size='icon-xs'>
          <Upload aria-hidden='true' />
        </IconAction>
      </span>
      <input
        accept='.json,.yaml,.yml,.txt,.conf,text/plain,application/json,application/yaml'
        className='hidden'
        onChange={(event) => {
          void read(event.target.files?.[0]);
          event.target.value = '';
        }}
        ref={fileRef}
        tabIndex={-1}
        type='file'
      />
    </div>
  );
}
