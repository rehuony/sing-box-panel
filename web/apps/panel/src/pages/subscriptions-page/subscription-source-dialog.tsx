import type { TFunction } from 'i18next';
import type { FormEvent, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useEffect, useId, useMemo, useState } from 'react';
import { formatShortDateTime } from '@shared/lib/format-time';
import { useMutation, useQueryClient } from '@tanstack/react-query';
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
import { messageLine } from '@/utils/message-line';
import { useApiClient } from '@/api/api-client-context';
import { InfoTooltip } from '@/components/info-tooltip';
import { SelectField } from '@/components/select-field';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';

import type { SourceContent } from './source-content-field';

import { SourceRuleInput } from './source-rule-input';
import { useIntervalLabel } from './use-interval-label';
import { SourceNodeSummary } from './source-node-summary';
import { SourceContentField } from './source-content-field';
import { contentProtocols, formatChoiceLabel, SOURCE_FORMATS } from './source-format';

/** What the dialog does: add a source, or edit one's settings and a file source's content together. */
export type SourceDialogMode
  = | { kind: 'create' }
    | { kind: 'edit'; source: SubscriptionSource };

/** How long typing rests before the preview reads the content again. */
const PREVIEW_DELAY_MS = 500;
const INTERVALS = [0, 15, 60, 360, 720, 1440];
const fieldLabel = 'text-sm text-[var(--color-text-secondary)]';
/** A refused rule's path in a problem's fields, such as `filter.include[2]`; a whole list has no index. */
const RULE_PATH = /^filter\.(include|exclude)(?:\[(\d+)\])?$/;

interface Draft {
  url: string;
  name: string;
  refresh: number;
  exclude: string[];
  include: string[];
  kind: SubscriptionSourceKind;
  /** New content for a file source; null keeps what the source holds. */
  content: SourceContent | null;
  format: SubscriptionSourceFormatChoice;
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

function initialDraft(source: SubscriptionSource | undefined): Draft {
  return {
    name: source?.name ?? '',
    kind: source?.kind ?? 'url',
    url: source?.url ?? '',
    refresh: source?.refresh_minutes ?? 360,
    format: source?.format ?? 'auto',
    include: source?.filter.include ?? [],
    exclude: source?.filter.exclude ?? [],
    content: null,
  };
}

interface RuleProblem {
  code: string;
  /** Absent when the problem is the whole list, such as too many rules. */
  index?: number;
  list: 'exclude' | 'include';
}

/** The filter rules a refused request names in its problem's fields. */
function ruleProblems(error: unknown): RuleProblem[] {
  if (!(error instanceof ApiRequestError) || error.code !== 'subscription_source_invalid' || !error.fields) return [];
  return Object.entries(error.fields).flatMap(([path, code]) => {
    const match = RULE_PATH.exec(path);
    if (!match) return [];
    return [{ code, index: match[2] === undefined ? undefined : Number(match[2]), list: match[1] as RuleProblem['list'] }];
  });
}

/** Why a request was refused, naming the rule when a filter rule is the reason. */
function describeSourceError(error: unknown, filter: SubscriptionSourceFilter, t: TFunction): string {
  const [problem] = ruleProblems(error);
  if (!problem) return describeRequestError(error);
  return t(`subscriptions.sources.ruleErrors.${problem.code}`, {
    defaultValue: describeRequestError(error),
    list: t(`subscriptions.sources.field.${problem.list}`),
    rule: problem.index === undefined ? '' : filter[problem.list][problem.index] ?? '',
  });
}

/** An http or https URL, the only kind a source fetches. */
function webURL(value: string): boolean {
  try {
    return ['http:', 'https:'].includes(new URL(value).protocol);
  } catch {
    return false;
  }
}

/** One row of the form: the label column with its optional help, then what fills the rest; stacked below 520px. */
function FieldRow({ children, help, helpLabel, htmlFor, label, labelId }: {
  children: ReactNode;
  help?: string;
  /** Names the help when it covers more than this row. */
  helpLabel?: string;
  htmlFor?: string;
  label: string;
  labelId?: string;
}) {
  return (
    <div className='grid min-w-0 grid-cols-[8rem_minmax(0,1fr)] items-start gap-x-4 gap-y-1 py-2 max-[520px]:grid-cols-1'>
      <div className='flex min-h-(--control-height) items-center gap-1'>
        {htmlFor
          ? <label className={fieldLabel} htmlFor={htmlFor} id={labelId}>{label}</label>
          : <span className={fieldLabel} id={labelId}>{label}</span>}
        {help ? <InfoTooltip label={helpLabel ?? label}>{help}</InfoTooltip> : null}
      </div>
      <div className='flex min-w-0 flex-col gap-2 *:data-[slot=select-trigger]:w-full'>{children}</div>
    </div>
  );
}

/**
 * Adds a source, or edits one: its settings and, for a file source, new
 * content saved with them in one write. One line under the form says which
 * nodes the content holds and the filter keeps, and lists them on request.
 * The server refuses content that does not parse or holds a kept node that
 * does not convert, and names a refused filter rule, which the rule's pill
 * shows; its problem goes to the footer's message slot. An edited source's
 * footer also offers to delete it, which the panel confirms.
 */
export function SubscriptionSourceDialog({ mode, onClose, onDelete }: {
  mode: SourceDialogMode;
  onClose: () => void;
  onDelete?: (source: SubscriptionSource) => void;
}) {
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
    nodes: `${id}-nodes`,
    refresh: `${id}-refresh`,
    url: `${id}-url`,
  };
  const source = mode.kind === 'edit' ? mode.source : undefined;
  const [baseline] = useState(() => initialDraft(source));
  const [draft, setDraft] = useState(baseline);
  const [message, setMessage] = useState<Message | null>(null);
  // The version an edit was read at; a stale save reads the source again and keeps the draft.
  const [version, setVersion] = useState(source?.updated_at ?? '');
  const kind = source?.kind ?? draft.kind;
  const takesContent = kind === 'file';
  const dirty = draft.name !== baseline.name
    || draft.kind !== baseline.kind
    || draft.url !== baseline.url
    || draft.refresh !== baseline.refresh
    || draft.format !== baseline.format
    || draft.include.join('\n') !== baseline.include.join('\n')
    || draft.exclude.join('\n') !== baseline.exclude.join('\n')
    || draft.content !== null;
  useUnsavedChanges(dirty, onClose);

  function update(patch: Partial<Draft>) {
    setDraft(current => ({ ...current, ...patch }));
    if (message) setMessage(null);
  }

  const filter = useMemo<SubscriptionSourceFilter>(
    () => ({ include: draft.include, exclude: draft.exclude }),
    [draft.exclude, draft.include],
  );
  const url = draft.url.trim();
  const format = draft.format;
  const content = takesContent ? draft.content : null;
  // The preview reads what saving would read: the URL or content given here, or the stored content.
  const request = useMemo<SubscriptionSourcePreviewRequest | null>(() => {
    if (content) return { content: content.text, filter, format };
    if (!source) return kind === 'url' && webURL(url) ? { filter, format, url } : null;
    if (kind === 'url' && url !== source.url) return webURL(url) ? { filter, format, url } : null;
    return { filter, format, source_id: source.id };
  }, [content, filter, format, kind, source, url]);
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
  // While the preview reads the stored content, its nodes say which protocols that content holds.
  const storedProtocols = request?.source_id && current?.preview ? contentProtocols(current.preview.items) : undefined;
  const refused = ruleProblems(current?.error);
  const refusedRules = (list: RuleProblem['list']) => refused.flatMap(problem => problem.list === list && problem.index !== undefined ? [problem.index] : []);
  let failure: string | null = null;
  if (current?.error !== undefined) {
    failure = refused.length > 0
      ? describeSourceError(current.error, filter, t)
      : t('subscriptions.sources.preview.failed', { reason: describeRequestError(current.error) });
  }

  const save = useMutation({
    mutationFn: async () => {
      const name = draft.name.trim();
      const location = kind === 'url' ? { refresh_minutes: draft.refresh, url } : {};
      if (source) {
        const changes = { filter, format, name, ...location, ...(content ? { content: content.text } : {}) };
        return client.updateSubscriptionSource(source.id, changes, version);
      }
      return client.createSubscriptionSource({ filter, format, kind, name, ...(kind === 'url' ? location : { content: content?.text ?? '' }) });
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
      setMessage({ text: describeSourceError(error, filter, t), tone: 'danger' });
    },
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    if (save.isPending) return;
    if (draft.name.trim() === '') setMessage({ field: 'name', text: t('subscriptions.sources.validation.name'), tone: 'danger' });
    else if (kind === 'url' && !webURL(url)) setMessage({ field: 'url', text: t('subscriptions.sources.validation.url'), tone: 'danger' });
    else if (!source && takesContent && !content) setMessage({ field: 'content', text: t('subscriptions.sources.validation.content'), tone: 'danger' });
    else save.mutate();
  }

  const invalid = (field: Field) => message?.field === field || undefined;
  const intervals = [...new Set([...INTERVALS, draft.refresh])].sort((left, right) => left - right);
  const summary = source
    ? [
        t(`subscriptions.sources.kind.${source.kind}`),
        t('subscriptions.sources.nodeCount', { count: source.node_count }),
        formatShortDateTime(source.content_at, locale),
      ].filter(Boolean).join(' · ')
    : '';

  return (
    <Dialog open onOpenChange={(open) => {
      if (!open && !save.isPending) onClose();
    }}>
      <DialogContent className='flex max-h-[min(48rem,calc(100dvh-2rem))] flex-col overflow-hidden sm:max-w-[40rem]'>
        <form className='flex min-h-0 min-w-0 flex-1 flex-col gap-4' noValidate onSubmit={submit}>
          <DialogHeader className='gap-1'>
            <DialogTitle className='truncate'>{source ? source.name : t('subscriptions.sources.add')}</DialogTitle>
            {source
              ? (
                  <DialogDescription className='truncate font-[family-name:var(--font-data)] text-xs text-[var(--color-text-secondary)]' title={summary}>
                    {summary}
                  </DialogDescription>
                )
              : <DialogDescription className='sr-only'>{t('subscriptions.sources.description')}</DialogDescription>}
          </DialogHeader>
          <div className='-mx-6 flex min-h-0 flex-1 flex-col overflow-y-auto overscroll-contain px-6 py-0.5'>
            <div className='flex min-w-0 flex-col divide-y'>
              <FieldRow htmlFor={ids.name} label={t('subscriptions.sources.field.name')}>
                <Input aria-describedby={ids.message} aria-invalid={invalid('name')} autoComplete='off' id={ids.name} maxLength={128} onChange={event => update({ name: event.target.value })} value={draft.name} />
              </FieldRow>
              {source
                ? null
                : (
                    <FieldRow htmlFor={ids.kind} label={t('subscriptions.sources.field.kind')}>
                      <SelectField<SubscriptionSourceKind>
                        id={ids.kind}
                        items={[{ label: t('subscriptions.sources.kind.url'), value: 'url' }, { label: t('subscriptions.sources.kind.file'), value: 'file' }]}
                        onValueChange={value => update({ kind: value })}
                        value={draft.kind}
                      />
                    </FieldRow>
                  )}
              {kind === 'url'
                ? (
                    // The address and how often it is fetched share one row.
                    <FieldRow htmlFor={ids.url} label={t('subscriptions.sources.field.url')}>
                      <div className='flex min-w-0 flex-wrap gap-2'>
                        <Input
                          aria-describedby={ids.message}
                          aria-invalid={invalid('url')}
                          autoComplete='off'
                          className='min-w-48 flex-1 font-[family-name:var(--font-data)]'
                          id={ids.url}
                          maxLength={4096}
                          onChange={event => update({ url: event.target.value })}
                          placeholder='https://'
                          spellCheck={false}
                          type='url'
                          value={draft.url}
                        />
                        <SelectField<number>
                          aria-label={t('subscriptions.sources.field.refresh')}
                          className='w-36 flex-none max-[520px]:w-full'
                          id={ids.refresh}
                          items={intervals.map(value => ({ label: intervalLabel(value), value }))}
                          onValueChange={value => update({ refresh: value })}
                          value={draft.refresh}
                        />
                      </div>
                    </FieldRow>
                  )
                : (
                    <FieldRow label={t('subscriptions.sources.field.content')} labelId={ids.content}>
                      <SourceContentField
                        current={source}
                        describedBy={ids.message}
                        invalid={invalid('content')}
                        labelId={ids.content}
                        onChange={next => update({ content: next })}
                        onError={text => setMessage({ field: 'content', text, tone: 'danger' })}
                        protocols={storedProtocols}
                        value={draft.content}
                      />
                    </FieldRow>
                  )}
              <FieldRow htmlFor={ids.format} label={t('subscriptions.sources.field.format')}>
                <SelectField<SubscriptionSourceFormatChoice>
                  id={ids.format}
                  items={SOURCE_FORMATS.map(value => ({ label: formatChoiceLabel(value, t), value }))}
                  onValueChange={value => update({ format: value })}
                  value={draft.format}
                />
              </FieldRow>
              <FieldRow
                help={t('subscriptions.sources.filterHelp')}
                helpLabel={t('subscriptions.sources.field.filter')}
                htmlFor={ids.include}
                label={t('subscriptions.sources.field.include')}
              >
                <SourceRuleInput
                  describedBy={ids.message}
                  id={ids.include}
                  invalidRules={refusedRules('include')}
                  onChange={rules => update({ include: rules })}
                  value={draft.include}
                />
              </FieldRow>
              <FieldRow htmlFor={ids.exclude} label={t('subscriptions.sources.field.exclude')}>
                <SourceRuleInput
                  describedBy={ids.message}
                  id={ids.exclude}
                  invalidRules={refusedRules('exclude')}
                  onChange={rules => update({ exclude: rules })}
                  value={draft.exclude}
                />
              </FieldRow>
              <FieldRow label={t('subscriptions.sources.field.nodes')} labelId={ids.nodes}>
                <SourceNodeSummary
                  failure={failure}
                  labelId={ids.nodes}
                  loading={request !== null && current === null}
                  preview={current?.preview}
                  waiting={request === null}
                />
              </FieldRow>
            </div>
          </div>
          <DialogFooter className='flex-nowrap'>
            <p
              className={cn('min-h-5 min-w-0 flex-1 truncate text-sm', message?.tone === 'warning' ? 'text-[var(--color-warning-text)]' : 'text-[var(--color-danger-text)]')}
              id={ids.message}
              role='status'
              title={message ? messageLine(message.text) : undefined}
            >
              {message ? messageLine(message.text) : null}
            </p>
            <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
            {source && onDelete
              ? <Button onClick={() => onDelete(source)} type='button' variant='destructive'>{t('subscriptions.sources.deleteTitle')}</Button>
              : null}
            <Button aria-busy={save.isPending || undefined} type='submit'>{t('subscriptions.sources.save')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
