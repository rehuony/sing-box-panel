import type { FormEvent, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { FileText, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { Textarea } from '@shared/components/ui/textarea';
import { formatBytes } from '@shared/lib/telemetry-format';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useId, useMemo, useRef, useState } from 'react';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type {
  SubscriptionSource,
  SubscriptionSourceFilter,
  SubscriptionSourceFormatChoice,
  SubscriptionSourceKind,
  SubscriptionSourcePreview,
  SubscriptionSourcePreviewRequest,
} from '@/api/api-client';

import { ApiRequestError } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { InfoTooltip } from '@/components/info-tooltip';
import { SelectField } from '@/components/select-field';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';

import { useIntervalLabel } from './use-interval-label';

/** What the dialog does: add a source, change one's settings, or give a file source new content. */
export type SourceDialogMode
  = | { kind: 'create' }
    | { kind: 'edit'; source: SubscriptionSource }
    | { kind: 'content'; source: SubscriptionSource };

/** How long typing rests before the preview reads the content again. */
const PREVIEW_DELAY_MS = 500;
/** The largest content a source holds. */
const MAX_CONTENT_BYTES = 4 * 1024 * 1024;
const FORMATS = ['auto', 'sing-box', 'mihomo', 'share-links'] as const satisfies readonly SubscriptionSourceFormatChoice[];
const INTERVALS = [0, 15, 60, 360, 720, 1440];

interface Draft {
  url: string;
  name: string;
  content: string;
  exclude: string;
  include: string;
  refresh: number;
  kind: SubscriptionSourceKind;
  format: SubscriptionSourceFormatChoice;
  /** The file the content was read from, while it came from one. */
  file: { name: string; size: number } | null;
}

type Field = 'content' | 'name' | 'url';

/** A preview's answer and the request it answers, so an answer to an older request never shows. */
interface PreviewResult {
  error?: unknown;
  preview?: SubscriptionSourcePreview;
  request: SubscriptionSourcePreviewRequest;
}

interface Message {
  text: string;
  field?: Field;
  tone: 'danger' | 'warning';
}

function initialDraft(mode: SourceDialogMode): Draft {
  const source = mode.kind === 'create' ? undefined : mode.source;
  return {
    name: source?.name ?? '',
    kind: source?.kind ?? 'url',
    url: source?.url ?? '',
    refresh: source?.refresh_minutes ?? 360,
    format: source?.format ?? 'auto',
    include: source?.filter.include.join('\n') ?? '',
    exclude: source?.filter.exclude.join('\n') ?? '',
    content: '',
    file: null,
  };
}

function rules(text: string): string[] {
  return text.split('\n').map(line => line.trim()).filter(Boolean);
}

/** An http or https URL, the only kind a source fetches. */
function webURL(value: string): boolean {
  try {
    return ['http:', 'https:'].includes(new URL(value).protocol);
  } catch {
    return false;
  }
}

/** One field row: the label column with its optional help, then the control filling the rest. */
function FieldRow({ children, help, htmlFor, label }: {
  children: ReactNode;
  help?: string;
  htmlFor?: string;
  label: string;
}) {
  return (
    <div className='grid min-w-0 grid-cols-[8rem_minmax(0,1fr)] items-start gap-x-4 gap-y-1 max-[520px]:grid-cols-1'>
      <div className='flex min-h-(--control-height) items-center gap-1'>
        {htmlFor
          ? <label className='text-sm text-[var(--color-text-secondary)]' htmlFor={htmlFor}>{label}</label>
          : <span className='text-sm text-[var(--color-text-secondary)]'>{label}</span>}
        {help ? <InfoTooltip label={label}>{help}</InfoTooltip> : null}
      </div>
      <div className='flex min-w-0 flex-col gap-2 *:data-[slot=select-trigger]:w-full'>{children}</div>
    </div>
  );
}

/** What the content holds and which nodes the filter keeps, or why it cannot be saved. */
function SourcePreview({ error, loading, preview, waiting }: {
  error: unknown;
  loading: boolean;
  preview: SubscriptionSourcePreview | undefined;
  waiting: boolean;
}) {
  const { t } = useTranslation();
  let status: ReactNode = null;
  let body: ReactNode = null;
  if (waiting) {
    status = <span className='text-muted-foreground'>{t('subscriptions.sources.preview.waiting')}</span>;
  } else if (loading) {
    status = (
      <span className='flex items-center gap-2 text-muted-foreground'>
        <Spinner className='size-3.5' />
        {t('subscriptions.sources.preview.loading')}
      </span>
    );
  } else if (error !== undefined) {
    status = <span className='text-[var(--color-danger-text)]'>{t('subscriptions.sources.preview.failed', { reason: describeRequestError(error) })}</span>;
  } else if (preview) {
    const kept = preview.items.filter(item => item.kept).length;
    const failure = preview.error;
    status = failure
      ? (
          <span className='text-[var(--color-danger-text)]'>
            {t(failure.location ? 'subscriptions.sources.preview.nodeError' : 'subscriptions.sources.preview.contentError', {
              code: failure.reason ? `${failure.code} (${failure.reason})` : failure.code,
              location: failure.location ?? '',
              name: failure.name ?? '',
            })}
          </span>
        )
      : (
          <span>
            {t('subscriptions.sources.preview.summary', { kept, total: preview.items.length })}
            {preview.format ? <span className='text-muted-foreground'>{` · ${t(`subscriptions.sources.formats.${preview.format}`)}`}</span> : null}
          </span>
        );
    body = preview.items.length === 0
      ? null
      : (
          <ul className='max-h-56 overflow-y-auto border-t font-[family-name:var(--font-data)] text-xs'>
            {preview.items.map(item => (
              <li
                className={cn('grid grid-cols-[minmax(0,6rem)_minmax(0,1fr)_auto_auto] items-center gap-3 px-3 py-1', !item.kept && 'text-muted-foreground')}
                data-kept={item.kept}
                key={item.location}
              >
                <span className='truncate text-muted-foreground' title={item.location}>{item.location}</span>
                <span className={cn('truncate', item.location === failure?.location && 'text-[var(--color-danger-text)]')} title={item.name}>{item.name}</span>
                <span className='text-[var(--color-text-secondary)]'>{item.protocol}</span>
                <span className={item.kept ? '' : 'text-muted-foreground'}>{t(item.kept ? 'subscriptions.sources.preview.kept' : 'subscriptions.sources.preview.excluded')}</span>
              </li>
            ))}
          </ul>
        );
  }
  return (
    <section aria-label={t('subscriptions.sources.preview.title')} aria-busy={loading || undefined} className='flex min-w-0 flex-col rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)]'>
      <p className='flex min-h-9 min-w-0 items-center gap-2 px-3 py-2 text-sm' role='status'>
        <span className='flex-none text-xs text-muted-foreground'>{t('subscriptions.sources.preview.title')}</span>
        <span className='min-w-0 flex-1 wrap-anywhere'>{status}</span>
      </p>
      {body}
    </section>
  );
}

/**
 * Adds a source, changes one's settings, or gives a file source new content,
 * with a live preview of the nodes the content holds and which the filter
 * keeps. The server refuses content that does not parse or holds a kept node
 * that does not convert; its problem goes to the footer's message slot.
 */
export function SubscriptionSourceDialog({ mode, onClose }: { mode: SourceDialogMode; onClose: () => void }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const cache = useQueryClient();
  const intervalLabel = useIntervalLabel();
  const id = useId();
  const ids = {
    content: `${id}-content`,
    exclude: `${id}-exclude`,
    format: `${id}-format`,
    include: `${id}-include`,
    kind: `${id}-kind`,
    message: `${id}-message`,
    name: `${id}-name`,
    refresh: `${id}-refresh`,
    url: `${id}-url`,
  };
  const fileRef = useRef<HTMLInputElement>(null);
  const [baseline] = useState(() => initialDraft(mode));
  const [draft, setDraft] = useState(baseline);
  const [message, setMessage] = useState<Message | null>(null);
  const source = mode.kind === 'create' ? undefined : mode.source;
  // The version an edit was read at; a stale save reads the source again and keeps the draft.
  const [version, setVersion] = useState(source?.updated_at ?? '');
  const kind = source?.kind ?? draft.kind;
  const settings = mode.kind !== 'content';
  const takesContent = mode.kind === 'content' || (mode.kind === 'create' && kind === 'file');
  const takesURL = settings && kind === 'url';
  const dirty = (Object.keys(baseline) as (keyof Draft)[]).some(key => draft[key] !== baseline[key]);
  useUnsavedChanges(dirty, onClose);

  function update(patch: Partial<Draft>) {
    setDraft(current => ({ ...current, ...patch }));
    if (message) setMessage(null);
  }

  const filter = useMemo<SubscriptionSourceFilter>(
    () => mode.kind === 'content' ? mode.source.filter : { include: rules(draft.include), exclude: rules(draft.exclude) },
    [draft.exclude, draft.include, mode],
  );
  const url = draft.url.trim();
  const content = draft.content;
  const format = draft.format;
  // The preview reads what saving would read: the URL or content typed here, or the stored content.
  const request = useMemo<SubscriptionSourcePreviewRequest | null>(() => {
    if (takesContent) return content.trim() === '' ? null : { content, filter, format };
    if (takesURL && (mode.kind === 'create' || url !== source?.url)) return webURL(url) ? { filter, format, url } : null;
    return source ? { filter, format, source_id: source.id } : null;
  }, [content, filter, format, mode.kind, source, takesContent, takesURL, url]);
  const [result, setResult] = useState<PreviewResult | null>(null);
  useEffect(() => {
    if (request === null) return;
    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      client.previewSubscriptionSource(request, controller.signal)
        .then(preview => setResult({ request, preview }))
        .catch((error: unknown) => {
          if (!controller.signal.aborted) setResult({ error, request });
        });
    }, PREVIEW_DELAY_MS);
    return () => {
      window.clearTimeout(timer);
      controller.abort();
    };
  }, [client, request]);
  const current = result?.request === request ? result : null;

  const save = useMutation({
    mutationFn: async () => {
      if (mode.kind === 'content') return client.replaceSubscriptionSourceContent(mode.source.id, { content, format });
      const name = draft.name.trim();
      if (mode.kind === 'edit') {
        const location = kind === 'url' ? { refresh_minutes: draft.refresh, url } : {};
        return client.updateSubscriptionSource(mode.source.id, { filter, format, name, ...location }, version);
      }
      const origin = kind === 'url' ? { refresh_minutes: draft.refresh, url } : { content };
      return client.createSubscriptionSource({ filter, format, kind, name, ...origin });
    },
    onSuccess: () => {
      void cache.invalidateQueries({ queryKey: ['sources'] });
      toast.add({ title: t('subscriptions.sources.saved'), type: 'success' });
      onClose();
    },
    onError: (error) => {
      if (error instanceof ApiRequestError && error.status === 412 && source) {
        setMessage({ text: t('subscriptions.sources.stale'), tone: 'warning' });
        void client.getSubscriptionSource(source.id).then(fresh => setVersion(fresh.updated_at)).catch(() => undefined);
        return;
      }
      setMessage({ text: describeRequestError(error), tone: 'danger' });
    },
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    if (save.isPending) return;
    if (settings && draft.name.trim() === '') setMessage({ field: 'name', text: t('subscriptions.sources.validation.name'), tone: 'danger' });
    else if (takesURL && !webURL(url)) setMessage({ field: 'url', text: t('subscriptions.sources.validation.url'), tone: 'danger' });
    else if (takesContent && content.trim() === '') setMessage({ field: 'content', text: t('subscriptions.sources.validation.content'), tone: 'danger' });
    else save.mutate();
  }

  async function readFile(file: File | undefined) {
    if (!file) return;
    if (file.size > MAX_CONTENT_BYTES) {
      setMessage({ field: 'content', text: t('subscriptions.sources.fileTooLarge'), tone: 'danger' });
      return;
    }
    try {
      update({ content: await file.text(), file: { name: file.name, size: file.size } });
    } catch (error) {
      setMessage({ field: 'content', text: describeRequestError(error), tone: 'danger' });
    }
  }

  const invalid = (field: Field) => message?.field === field || undefined;
  let title = t('subscriptions.sources.add');
  if (mode.kind === 'edit') title = t('subscriptions.sources.editTitle');
  if (mode.kind === 'content') title = t('subscriptions.sources.replaceTitle', { name: mode.source.name });
  const intervals = [...new Set([...INTERVALS, draft.refresh])].sort((left, right) => left - right);

  return (
    <Dialog open onOpenChange={(open) => {
      if (!open && !save.isPending) onClose();
    }}>
      <DialogContent className='flex max-h-[min(48rem,calc(100dvh-2rem))] flex-col overflow-hidden sm:max-w-[40rem]'>
        <form className='flex min-h-0 min-w-0 flex-1 flex-col gap-6' noValidate onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className='truncate'>{title}</DialogTitle>
            <DialogDescription className='sr-only'>{t('subscriptions.sources.description')}</DialogDescription>
          </DialogHeader>
          <div className='-mx-6 flex min-h-0 flex-1 flex-col gap-(--form-field-gap) overflow-y-auto overscroll-contain px-6 py-0.5'>
            {settings
              ? (
                  <FieldRow htmlFor={ids.name} label={t('subscriptions.sources.field.name')}>
                    <Input aria-describedby={ids.message} aria-invalid={invalid('name')} autoComplete='off' id={ids.name} maxLength={128} onChange={event => update({ name: event.target.value })} value={draft.name} />
                  </FieldRow>
                )
              : null}
            {mode.kind === 'create'
              ? (
                  <FieldRow htmlFor={ids.kind} label={t('subscriptions.sources.field.kind')}>
                    <SelectField<SubscriptionSourceKind>
                      id={ids.kind}
                      items={[{ label: t('subscriptions.sources.kind.url'), value: 'url' }, { label: t('subscriptions.sources.kind.file'), value: 'file' }]}
                      onValueChange={value => update({ kind: value })}
                      value={draft.kind}
                    />
                  </FieldRow>
                )
              : null}
            {takesURL
              ? (
                  <>
                    <FieldRow htmlFor={ids.url} label={t('subscriptions.sources.field.url')}>
                      <Input
                        aria-describedby={ids.message}
                        aria-invalid={invalid('url')}
                        autoComplete='off'
                        className='font-[family-name:var(--font-data)]'
                        id={ids.url}
                        maxLength={4096}
                        onChange={event => update({ url: event.target.value })}
                        placeholder='https://'
                        spellCheck={false}
                        type='url'
                        value={draft.url}
                      />
                    </FieldRow>
                    <FieldRow htmlFor={ids.refresh} label={t('subscriptions.sources.field.refresh')}>
                      <SelectField<number>
                        id={ids.refresh}
                        items={intervals.map(value => ({ label: intervalLabel(value), value }))}
                        onValueChange={value => update({ refresh: value })}
                        value={draft.refresh}
                      />
                    </FieldRow>
                  </>
                )
              : null}
            {takesContent
              ? (
                  <FieldRow htmlFor={draft.file ? undefined : ids.content} label={t('subscriptions.sources.field.content')}>
                    {draft.file
                      ? (
                          <div className='flex min-h-(--control-height) min-w-0 items-center gap-2 rounded-[var(--radius-control)] border bg-[var(--color-field)] ps-3 pe-1'>
                            <FileText aria-hidden='true' className='size-4 flex-none text-muted-foreground' />
                            <span className='min-w-0 flex-1 truncate font-[family-name:var(--font-data)] text-compact' title={draft.file.name}>{draft.file.name}</span>
                            <span className='flex-none text-xs text-muted-foreground tabular-nums'>{formatBytes(draft.file.size, locale)}</span>
                            <Button aria-label={t('subscriptions.sources.removeFile')} onClick={() => update({ content: '', file: null })} size='icon-xs' type='button' variant='ghost'>
                              <X aria-hidden='true' />
                            </Button>
                          </div>
                        )
                      : (
                          <Textarea
                            aria-describedby={ids.message}
                            aria-invalid={invalid('content')}
                            className='max-h-48 min-h-24 font-[family-name:var(--font-data)] text-xs field-sizing-fixed md:text-xs'
                            id={ids.content}
                            onChange={event => update({ content: event.target.value })}
                            placeholder={t('subscriptions.sources.contentPlaceholder')}
                            spellCheck={false}
                            value={draft.content}
                            wrap='off'
                          />
                        )}
                    <div>
                      <input
                        accept='.json,.yaml,.yml,.txt,.conf,text/plain,application/json,application/yaml'
                        className='hidden'
                        onChange={(event) => {
                          void readFile(event.target.files?.[0]);
                          event.target.value = '';
                        }}
                        ref={fileRef}
                        tabIndex={-1}
                        type='file'
                      />
                      <Button onClick={() => fileRef.current?.click()} size='sm' type='button' variant='outline'>
                        {t('subscriptions.sources.chooseFile')}
                      </Button>
                    </div>
                  </FieldRow>
                )
              : null}
            <FieldRow htmlFor={ids.format} label={t('subscriptions.sources.field.format')}>
              <SelectField<SubscriptionSourceFormatChoice>
                id={ids.format}
                items={FORMATS.map(value => ({ label: t(`subscriptions.sources.formats.${value}`), value }))}
                onValueChange={value => update({ format: value })}
                value={draft.format}
              />
            </FieldRow>
            {settings
              ? (
                  <>
                    <FieldRow help={t('subscriptions.sources.filterHelp')} htmlFor={ids.include} label={t('subscriptions.sources.field.include')}>
                      <Textarea className='max-h-32 min-h-16 font-[family-name:var(--font-data)] text-xs field-sizing-content md:text-xs' id={ids.include} onChange={event => update({ include: event.target.value })} spellCheck={false} value={draft.include} wrap='off' />
                    </FieldRow>
                    <FieldRow htmlFor={ids.exclude} label={t('subscriptions.sources.field.exclude')}>
                      <Textarea className='max-h-32 min-h-16 font-[family-name:var(--font-data)] text-xs field-sizing-content md:text-xs' id={ids.exclude} onChange={event => update({ exclude: event.target.value })} spellCheck={false} value={draft.exclude} wrap='off' />
                    </FieldRow>
                  </>
                )
              : null}
            <SourcePreview
              error={current?.error}
              loading={request !== null && current === null}
              preview={current?.preview}
              waiting={request === null}
            />
          </div>
          <DialogFooter className='flex-nowrap'>
            <p
              className={cn('min-h-5 min-w-0 flex-1 truncate text-sm', message?.tone === 'warning' ? 'text-[var(--color-warning-text)]' : 'text-[var(--color-danger-text)]')}
              id={ids.message}
              role='status'
              title={message?.text}
            >
              {message?.text}
            </p>
            <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
            <Button aria-busy={save.isPending || undefined} type='submit'>{t(mode.kind === 'content' ? 'subscriptions.sources.replace' : 'subscriptions.sources.save')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
