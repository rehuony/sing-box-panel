import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Eye, GitCompareArrows, RotateCcw } from 'lucide-react';

import type {
  CanonicalDocument,
  CanonicalRevisionDiff,
  CanonicalRevisionPage,
  CanonicalSnapshot,
} from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';

import { encodeCanonicalValue } from './use-canonical-configuration';
import { parseCanonicalDocument, valueAtPointer } from './canonical-document';

export interface RevisionHistoryProps {
  revisionError: unknown;
  currentRevisionID: string;
  mutationsDisabled: boolean;
  loadingOlderRevisions: boolean;
  revisions: CanonicalRevisionPage | null;
  onLoadOlderRevisions: () => Promise<void>;
  onRestore: (reference: string, baseRevisionID: string) => Promise<void>;
}

interface DetailState {
  error: unknown;
  loading: boolean;
  revision: CanonicalSnapshot | null;
}

interface DiffState {
  error: unknown;
  loading: boolean;
  diff: CanonicalRevisionDiff | null;
}

interface RestoreEvidence {
  sequence: number;
  reference: string;
  baseRevisionID: string;
}

const initialDetailState: DetailState = { error: null, loading: false, revision: null };
const initialDiffState: DiffState = { diff: null, error: null, loading: false };

function formatTimestamp(timestamp: string, locale: string): string {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' })
    .format(new Date(timestamp));
}

function formatNumber(value: number, locale: string): string {
  return new Intl.NumberFormat(locale).format(value);
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === 'AbortError';
}

function encodedDiffValue(document: CanonicalDocument | null, path: string, fallback: unknown): string {
  const exactValue = document === null ? undefined : valueAtPointer(document, path);
  try {
    return encodeCanonicalValue(exactValue === undefined ? fallback : exactValue, 2);
  } catch {
    return '—';
  }
}

export function RevisionHistory({
  currentRevisionID,
  loadingOlderRevisions,
  mutationsDisabled,
  onLoadOlderRevisions,
  onRestore,
  revisionError,
  revisions,
}: RevisionHistoryProps) {
  const client = useApiClient();
  const { i18n, t } = useTranslation();
  const [detailState, setDetailState] = useState<DetailState>(initialDetailState);
  const [diffState, setDiffState] = useState<DiffState>(initialDiffState);
  const [detailOpen, setDetailOpen] = useState(false);
  const [diffOpen, setDiffOpen] = useState(false);
  const [restoreEvidence, setRestoreEvidence] = useState<RestoreEvidence | null>(null);
  const [selectedFrom, setSelectedFrom] = useState('');
  const [selectedTo, setSelectedTo] = useState('');
  const detailAbortRef = useRef<AbortController | null>(null);
  const diffAbortRef = useRef<AbortController | null>(null);
  const items = revisions?.items ?? [];
  const defaultTo = items.find((revision) => revision.id === currentRevisionID)?.id ?? items[0]?.id ?? '';
  const defaultFrom = items.find((revision) => revision.id !== defaultTo)?.id ?? defaultTo;
  const fromReference = selectedFrom || defaultFrom;
  const toReference = selectedTo || defaultTo;

  useEffect(() => () => {
    detailAbortRef.current?.abort();
    diffAbortRef.current?.abort();
  }, []);

  const diffDocuments = useMemo(() => {
    if (diffState.diff === null) return { from: null, to: null };
    try {
      return {
        from: parseCanonicalDocument(diffState.diff.from),
        to: parseCanonicalDocument(diffState.diff.to),
      };
    } catch {
      return { from: null, to: null };
    }
  }, [diffState.diff]);

  async function inspectRevision(reference: string) {
    detailAbortRef.current?.abort();
    const controller = new AbortController();
    detailAbortRef.current = controller;
    setDetailOpen(true);
    setDetailState({ error: null, loading: true, revision: null });
    try {
      const revision = await client.getRevision(reference, controller.signal);
      if (detailAbortRef.current !== controller) return;
      setDetailState({ error: null, loading: false, revision });
    } catch (error) {
      if (isAbortError(error) || detailAbortRef.current !== controller) return;
      setDetailState({ error, loading: false, revision: null });
    }
  }

  async function compareRevisions() {
    if (fromReference === '' || toReference === '' || fromReference === toReference) return;
    diffAbortRef.current?.abort();
    const controller = new AbortController();
    diffAbortRef.current = controller;
    setDiffOpen(true);
    setDiffState({ diff: null, error: null, loading: true });
    try {
      const diff = await client.diffRevisions(fromReference, toReference, controller.signal);
      if (diffAbortRef.current !== controller) return;
      setDiffState({ diff, error: null, loading: false });
    } catch (error) {
      if (isAbortError(error) || diffAbortRef.current !== controller) return;
      setDiffState({ diff: null, error, loading: false });
    }
  }

  function changeDiffReference(side: 'from' | 'to', reference: string) {
    diffAbortRef.current?.abort();
    setDiffState(initialDiffState);
    if (side === 'from') setSelectedFrom(reference);
    else setSelectedTo(reference);
  }

  return (
    <section className='revision-section' aria-labelledby='revision-history-title'>
      <div className='section-heading'>
        <h2 id='revision-history-title'>{t('configuration.history.title')}</h2>
        <span>{t('configuration.history.loaded', { count: formatNumber(items.length, i18n.language) })}</span>
      </div>

      {revisionError === null
        ? null
        : (
            <ErrorNotice error={revisionError} title={t('configuration.history.error')} />
          )}

      <div className='revision-history__list' aria-label={t('configuration.history.list')} role='region' tabIndex={0}>
        {items.map((revision) => (
          <article className='revision-card' key={revision.id}>
            <div>
              <strong>
                {t('configuration.revision', {
                  sequence: formatNumber(revision.sequence, i18n.language),
                })}
              </strong>
              <span>{formatTimestamp(revision.created_at, i18n.language)}</span>
              <code>{revision.id}</code>
            </div>
            <div className='revision-card__actions'>
              <Button
                aria-label={t('configuration.history.viewLabel', {
                  sequence: formatNumber(revision.sequence, i18n.language),
                })}
                onClick={() => void inspectRevision(revision.id)}
                size='sm'
                type='button'
                variant='ghost'
              >
                <Eye aria-hidden data-icon='inline-start' />
                {t('configuration.history.view')}
              </Button>
              {revision.id === currentRevisionID
                ? (
                    <span className='state-label state-label--success'>{t('configuration.history.current')}</span>
                  )
                : (
                    <Button
                      disabled={mutationsDisabled}
                      onClick={() => setRestoreEvidence({
                        baseRevisionID: currentRevisionID,
                        reference: revision.id,
                        sequence: revision.sequence,
                      })}
                      size='sm'
                      type='button'
                      variant='ghost'
                    >
                      <RotateCcw aria-hidden data-icon='inline-start' />
                      {t('configuration.history.restore')}
                    </Button>
                  )}
            </div>
          </article>
        ))}
      </div>

      <div className='revision-history__footer'>
        {revisions?.next_before_sequence === undefined
          ? <span />
          : (
              <Button
                aria-busy={loadingOlderRevisions}
                disabled={loadingOlderRevisions}
                onClick={() => void onLoadOlderRevisions()}
                type='button'
                variant='outline'
              >
                {loadingOlderRevisions ? t('configuration.history.loadingOlder') : t('configuration.history.loadOlder')}
              </Button>
            )}
        <div className='revision-compare-controls'>
          <label>
            <span>{t('configuration.history.from')}</span>
            <SelectField
              aria-label={t('configuration.history.from')}
              onValueChange={(value) => changeDiffReference('from', value)}
              value={fromReference}
              items={items.map((revision) => ({ value: revision.id, label: `#${formatNumber(revision.sequence, i18n.language)}` }))}
            />
          </label>
          <label>
            <span>{t('configuration.history.to')}</span>
            <SelectField
              aria-label={t('configuration.history.to')}
              onValueChange={(value) => changeDiffReference('to', value)}
              value={toReference}
              items={items.map((revision) => ({ value: revision.id, label: `#${formatNumber(revision.sequence, i18n.language)}` }))}
            />
          </label>
          <Button
            disabled={diffState.loading || fromReference === '' || fromReference === toReference}
            onClick={() => void compareRevisions()}
            type='button'
            variant='outline'
          >
            <GitCompareArrows aria-hidden data-icon='inline-start' />
            {diffState.loading ? t('configuration.history.comparing') : t('configuration.history.compare')}
          </Button>
        </div>
      </div>

      <AlertDialog
        onOpenChange={(open) => {
          if (!open) setRestoreEvidence(null);
        }}
        open={restoreEvidence !== null}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('configuration.history.confirmRestoreTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('configuration.history.confirmRestoreDescription', {
                base: restoreEvidence?.baseRevisionID ?? '',
                id: restoreEvidence?.reference ?? '',
                sequence: restoreEvidence === null
                  ? ''
                  : formatNumber(restoreEvidence.sequence, i18n.language),
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={mutationsDisabled || restoreEvidence === null
                || currentRevisionID !== restoreEvidence.baseRevisionID}
              onClick={() => {
                if (restoreEvidence === null) return;
                const evidence = restoreEvidence;
                setRestoreEvidence(null);
                void onRestore(evidence.reference, evidence.baseRevisionID);
              }}
            >
              {t('configuration.history.confirmRestoreAction')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog onOpenChange={setDetailOpen} open={detailOpen}>
        <DialogContent className='configuration-history-dialog'>
          <DialogHeader>
            <DialogTitle>{t('configuration.history.detailTitle')}</DialogTitle>
            <DialogDescription>{t('configuration.history.detailDescription')}</DialogDescription>
          </DialogHeader>
          {detailState.loading ? <div className='inline-loading' aria-busy='true'>{t('configuration.history.loading')}</div> : null}
          {detailState.error === null ? null : <ErrorNotice error={detailState.error} title={t('configuration.history.detailError')} />}
          {detailState.revision === null
            ? null
            : (
                <div className='revision-detail'>
                  <dl>
                    <div>
                      <dt>{t('configuration.history.created')}</dt>
                      <dd>{formatTimestamp(detailState.revision.created_at, i18n.language)}</dd>
                    </div>
                    <div>
                      <dt>SHA-256</dt>
                      <dd><code>{detailState.revision.sha256}</code></dd>
                    </div>
                  </dl>
                  <pre aria-label={t('configuration.history.jsonLabel', {
                    sequence: formatNumber(detailState.revision.sequence, i18n.language),
                  })} tabIndex={0}>
                    {detailState.revision.document_json}
                  </pre>
                </div>
              )}
        </DialogContent>
      </Dialog>

      <Dialog onOpenChange={setDiffOpen} open={diffOpen}>
        <DialogContent className='configuration-history-dialog'>
          <DialogHeader>
            <DialogTitle>{t('configuration.history.diffTitle')}</DialogTitle>
            <DialogDescription>{t('configuration.history.diffDescription')}</DialogDescription>
          </DialogHeader>
          {diffState.loading ? <div className='inline-loading' aria-busy='true'>{t('configuration.history.comparing')}</div> : null}
          {diffState.error === null ? null : <ErrorNotice error={diffState.error} title={t('configuration.history.diffError')} />}
          {diffState.diff !== null && diffState.diff.changes.length === 0
            ? (
                <p className='configuration-empty-copy'>{t('configuration.history.noChanges')}</p>
              )
            : null}
          {diffState.diff === null || diffState.diff.changes.length === 0
            ? null
            : (
                <ul className='revision-diff-list' aria-label={t('configuration.history.changes')} tabIndex={0}>
                  {diffState.diff.changes.map((change) => (
                    <li key={change.path}>
                      <code className='revision-diff-list__path'>{change.path || '/'}</code>
                      <div className='revision-diff-list__values'>
                        <div>
                          <span>{t('configuration.history.before')}</span>
                          <pre>{change.from.present ? encodedDiffValue(diffDocuments.from, change.path, change.from.value) : t('configuration.history.notPresent')}</pre>
                        </div>
                        <div>
                          <span>{t('configuration.history.after')}</span>
                          <pre>{change.to.present ? encodedDiffValue(diffDocuments.to, change.path, change.to.value) : t('configuration.history.notPresent')}</pre>
                        </div>
                      </div>
                    </li>
                  ))}
                </ul>
              )}
        </DialogContent>
      </Dialog>
    </section>
  );
}
