import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type { JsonObject, SubscriptionCursor, SubscriptionSource, SubscriptionSourceFormat, SubscriptionSourceKind, SubscriptionSourceSummary, SubscriptionSourceVersion } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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

interface SourceRestoreEvidence {
  sourceID: string;
  versionID: string;
  sourceName: string;
  versionSHA256: string;
  sourceUpdatedAt: string;
}

interface SourceDraft {
  name: string;
  config: string;
  enabled: boolean;
  sourceDocument: string;
  editing?: SubscriptionSource;
  kind: SubscriptionSourceKind;
  format: SubscriptionSourceFormat;
}

function parseObject(value: string, invalidJSONMessage: string, invalidObjectMessage: string): JsonObject {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    throw new Error(invalidJSONMessage);
  }
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(invalidObjectMessage);
  }
  return parsed as JsonObject;
}

export function SubscriptionSourcePanel() {
  const { i18n, t } = useTranslation();
  const client = useApiClient();
  const [sources, setSources] = useState<SubscriptionSourceSummary[] | null>(null);
  const [next, setNext] = useState<SubscriptionCursor>();
  const [loadError, setLoadError] = useState<unknown>(null);
  const [draft, setDraft] = useState<SourceDraft | null>(null);
  const [actionError, setActionError] = useState('');
  const [deleteCandidateID, setDeleteCandidateID] = useState<string | null>(null);
  const [message, setMessage] = useState('');
  const [acceptedTaskID, setAcceptedTaskID] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [versions, setVersions] = useState<SubscriptionSourceVersion[]>([]);
  const [versionSource, setVersionSource] = useState<SubscriptionSourceSummary | null>(null);
  const [versionNext, setVersionNext] = useState<SubscriptionCursor>();
  const [selectedVersion, setSelectedVersion] = useState<SubscriptionSourceVersion | null>(null);
  const [restoreEvidence, setRestoreEvidence] = useState<SourceRestoreEvidence | null>(null);
  const [loadingVersionID, setLoadingVersionID] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadingOlderVersions, setLoadingOlderVersions] = useState(false);
  const loadGenerationRef = useRef(0);
  const loadingMoreRef = useRef(false);
  const loadingOlderVersionsRef = useRef(false);
  const dateTimeFormatter = useMemo(
    () => new Intl.DateTimeFormat(i18n.resolvedLanguage ?? i18n.language ?? 'en', {
      dateStyle: 'medium',
      timeStyle: 'short',
    }),
    [i18n.language, i18n.resolvedLanguage],
  );
  const numberFormatter = useMemo(
    () => new Intl.NumberFormat(i18n.resolvedLanguage ?? i18n.language ?? 'en'),
    [i18n.language, i18n.resolvedLanguage],
  );

  const load = useCallback(async (signal?: AbortSignal, cursor?: SubscriptionCursor, append = false) => {
    const generation = ++loadGenerationRef.current;
    if (!append) {
      setSources(null);
      setNext(undefined);
    }
    try {
      setLoadError(null);
      const result = await client.listSubscriptionSources({
        limit: 50,
        beforeTime: cursor?.created_at,
        beforeID: cursor?.id,
      }, signal);
      if (!signal?.aborted && generation === loadGenerationRef.current) {
        setSources((current) => append ? [...(current ?? []), ...result.items] : result.items);
        setNext(result.next);
      }
    } catch (error) {
      if (!signal?.aborted && generation === loadGenerationRef.current) {
        if (!append) setSources(null);
        setLoadError(error);
      }
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  function startCreate() {
    setDraft({ enabled: true, format: 'auto', kind: 'local', name: '', config: '{}', sourceDocument: '' });
    setActionError('');
    setMessage('');
    setAcceptedTaskID(null);
  }

  async function startEdit(summary: SubscriptionSourceSummary) {
    setBusy(true);
    setActionError('');
    setMessage('');
    setAcceptedTaskID(null);
    try {
      const source = await client.getSubscriptionSource(summary.id);
      setDraft({
        editing: source,
        enabled: source.enabled,
        kind: source.source_kind,
        name: source.name,
        config: JSON.stringify(source.config, null, 2),
        format: 'auto',
        sourceDocument: '',
      });
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function saveMetadata() {
    if (draft === null) return;
    try {
      if (draft.name.trim() === '') throw new Error(t('subscriptions.source.validation.name'));
      const config = parseObject(
        draft.config,
        t('subscriptions.source.validation.configJSON'),
        t('subscriptions.source.validation.configObject'),
      );
      setBusy(true);
      setActionError('');
      setAcceptedTaskID(null);
      if (draft.editing) {
        await client.updateSubscriptionSource(draft.editing.id, {
          name: draft.name.trim(), source_kind: draft.kind, config, enabled: draft.enabled,
        }, draft.editing.updated_at);
        setMessage(t('subscriptions.source.message.updated', { name: draft.name.trim() }));
      } else {
        const created = await client.createSubscriptionSource({
          name: draft.name.trim(), source_kind: draft.kind, config,
          enabled: draft.enabled,
        });
        if (draft.sourceDocument.trim() !== '') {
          try {
            await client.createSubscriptionSourceVersion(
              created.id, draft.format, draft.sourceDocument, created.updated_at,
            );
          } catch (error) {
            setDraft({ ...draft, editing: created });
            setMessage(t('subscriptions.source.message.createdWithoutVersion', { name: created.name }));
            setActionError(describeRequestError(error));
            await load();
            return;
          }
        }
        setMessage(t('subscriptions.source.message.created', { name: draft.name.trim() }));
      }
      setDraft(null);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function saveSourceVersion() {
    if (!draft?.editing) return;
    try {
      if (draft.sourceDocument.trim() === '') throw new Error(t('subscriptions.source.validation.document'));
      setBusy(true);
      setActionError('');
      setAcceptedTaskID(null);
      const saved = await client.createSubscriptionSourceVersion(
        draft.editing.id,
        draft.format,
        draft.sourceDocument,
        draft.editing.updated_at,
      );
      setDraft({ ...draft, editing: saved.source, sourceDocument: '' });
      setMessage(t('subscriptions.source.message.activated', {
        format: saved.version.format,
        name: saved.source.name,
      }));
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function refresh(source: SubscriptionSourceSummary) {
    try {
      setBusy(true);
      setActionError('');
      setMessage('');
      setAcceptedTaskID(null);
      const task = await client.refreshSubscriptionSource(source.id);
      setAcceptedTaskID(task.id);
      setMessage(t('subscriptions.source.message.refreshAccepted', { id: task.id }));
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function showVersions(source: SubscriptionSourceSummary) {
    try {
      setBusy(true);
      setRestoreEvidence(null);
      const page = await client.listSubscriptionSourceVersions(source.id, { limit: 100 });
      setVersionSource(source);
      setVersions(page.items);
      setVersionNext(page.next);
      setSelectedVersion(null);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function inspectVersion(version: SubscriptionSourceVersion) {
    try {
      setLoadingVersionID(version.id);
      setActionError('');
      setSelectedVersion(await client.getSubscriptionSourceVersion(version.source_id, version.id));
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setLoadingVersionID(null);
    }
  }

  async function prepareRestore(version: SubscriptionSourceVersion) {
    if (versionSource === null) return;
    const requestedSourceID = versionSource.id;
    try {
      setBusy(true);
      setActionError('');
      setMessage('');
      setAcceptedTaskID(null);
      const [source, exactVersion] = await Promise.all([
        client.getSubscriptionSource(requestedSourceID),
        client.getSubscriptionSourceVersion(requestedSourceID, version.id),
      ]);
      if (
        source.id !== requestedSourceID
        || exactVersion.source_id !== requestedSourceID
        || exactVersion.id !== version.id
      ) {
        throw new Error(t('subscriptions.source.version.restoreIdentityMismatch'));
      }
      setRestoreEvidence({
        sourceID: source.id,
        sourceName: source.name,
        sourceUpdatedAt: source.updated_at,
        versionID: exactVersion.id,
        versionSHA256: exactVersion.sha256,
      });
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function restore(evidence: SourceRestoreEvidence) {
    try {
      setBusy(true);
      setActionError('');
      const source = await client.restoreSubscriptionSourceVersion(
        evidence.sourceID,
        evidence.versionID,
        evidence.sourceUpdatedAt,
      );
      setMessage(t('subscriptions.source.message.restored', {
        name: evidence.sourceName,
        version: evidence.versionID,
      }));
      await Promise.all([load(), showVersions({ ...source, has_version: true })]);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function remove(source: SubscriptionSourceSummary) {
    setBusy(true);
    setActionError('');
    setAcceptedTaskID(null);
    try {
      await client.deleteSubscriptionSource(source.id, source.updated_at);
      setMessage(t('subscriptions.source.message.deleted', { name: source.name }));
      setDeleteCandidateID(null);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function loadMore() {
    if (next === undefined || loadingMoreRef.current) return;
    loadingMoreRef.current = true;
    setLoadingMore(true);
    try {
      await load(undefined, next, true);
    } finally {
      loadingMoreRef.current = false;
      setLoadingMore(false);
    }
  }

  async function loadOlderVersions() {
    if (versionSource === null || versionNext === undefined || loadingOlderVersionsRef.current) return;
    loadingOlderVersionsRef.current = true;
    setLoadingOlderVersions(true);
    try {
      const page = await client.listSubscriptionSourceVersions(versionSource.id, {
        beforeID: versionNext.id,
        beforeTime: versionNext.created_at,
        limit: 100,
      });
      setVersions((current) => [...current, ...page.items]);
      setVersionNext(page.next);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      loadingOlderVersionsRef.current = false;
      setLoadingOlderVersions(false);
    }
  }

  return (
    <section className='subscription-panel' id='subscription-sources' aria-labelledby='subscription-sources-title'>
      <div className='subscription-panel__heading'>
        <div>
          <h2 id='subscription-sources-title'>{t('subscriptions.source.title')}</h2>
          <p>{t('subscriptions.source.description')}</p>
        </div>
        <span className='count-label'>
          {t('subscriptions.source.loadedCount', {
            count: numberFormatter.format(sources?.length ?? 0),
          })}
        </span>
        <button className='button button--primary' onClick={startCreate} type='button'>{t('subscriptions.source.attach')}</button>
      </div>
      {draft === null && versionSource === null
        ? <ActionError message={actionError} title={t('subscriptions.source.actionFailed')} />
        : null}
      {loadError === null ? null : <ErrorNotice error={loadError} title={t('subscriptions.source.loadFailed')} />}
      {message === '' || draft !== null
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>{t('subscriptions.source.updated')}</strong>
              <p>{message}</p>
              {acceptedTaskID === null
                ? null
                : <Link className='text-button' to='/tasks'>{t('subscriptions.source.message.viewTask')}</Link>}
            </div>
          )}
      {sources === null ? <div className='inline-loading' aria-busy='true'>{t('subscriptions.source.loading')}</div> : null}
      {sources?.length === 0
        ? (
            <div className='empty-state'>
              <strong>{t('subscriptions.source.empty.title')}</strong>
              <p>{t('subscriptions.source.empty.description')}</p>
            </div>
          )
        : null}
      {sources && sources.length > 0
        ? (
            <div className='entity-table-wrap'>
              <table className='data-table'>
                <thead>
                  <tr>
                    <th>{t('subscriptions.common.name')}</th>
                    <th>{t('subscriptions.source.field.kind')}</th>
                    <th>{t('subscriptions.source.column.currentVersion')}</th>
                    <th>{t('subscriptions.common.actions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {sources.map((source) => (
                    <tr key={source.id}>
                      <td data-label={t('subscriptions.common.name')}>
                        <strong>{source.name}</strong>
                        <small className='table-subline'>{source.enabled ? t('common.enabled') : t('common.disabled')}</small>
                      </td>
                      <td data-label={t('subscriptions.source.field.kind')}><code>{t(`subscriptions.source.kind.${source.source_kind}`)}</code></td>
                      <td data-label={t('subscriptions.source.column.currentVersion')}>
                        {source.current_version_id ? <code>{source.current_version_id}</code> : t('subscriptions.common.none')}
                      </td>
                      <td data-label={t('subscriptions.common.actions')}>
                        {deleteCandidateID === source.id
                          ? (
                              <div
                                aria-label={t('subscriptions.source.delete.confirmationAria', { name: source.name })}
                                className='inline-confirmation'
                                role='group'
                              >
                                <span className='inline-confirmation__prompt'>{t('subscriptions.source.delete.prompt')}</span>
                                <button
                                  aria-label={t('subscriptions.source.delete.confirmAria', { name: source.name })}
                                  className='button button--danger button--small'
                                  disabled={busy}
                                  onClick={() => void remove(source)}
                                  type='button'
                                >
                                  {busy ? t('subscriptions.common.deleting') : t('subscriptions.common.confirmDelete')}
                                </button>
                                <button
                                  aria-label={t('subscriptions.source.delete.keepAria', { name: source.name })}
                                  autoFocus
                                  className='text-button'
                                  disabled={busy}
                                  onClick={() => setDeleteCandidateID(null)}
                                  type='button'
                                >
                                  {t('subscriptions.source.delete.keep')}
                                </button>
                              </div>
                            )
                          : (
                              <div className='table-actions'>
                                <button className='text-button' disabled={busy} onClick={() => void startEdit(source)} type='button'>{t('common.edit')}</button>
                                <button className='text-button' disabled={busy || source.source_kind !== 'remote'} onClick={() => void refresh(source)} type='button'>{t('subscriptions.source.refresh')}</button>
                                <button className='text-button' disabled={busy} onClick={() => void showVersions(source)} type='button'>{t('subscriptions.source.versions')}</button>
                                <button
                                  aria-label={t('subscriptions.source.delete.actionAria', { name: source.name })}
                                  className='text-button text-button--danger'
                                  disabled={busy}
                                  onClick={() => setDeleteCandidateID(source.id)}
                                  type='button'
                                >
                                  {t('common.delete')}
                                </button>
                              </div>
                            )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        : null}
      {next
        ? (
            <button
              aria-busy={loadingMore}
              className='button button--secondary'
              disabled={busy || loadingMore}
              onClick={() => void loadMore()}
              type='button'
            >
              {loadingMore ? t('subscriptions.source.loadingMore') : t('subscriptions.source.loadMore')}
            </button>
          )
        : null}
      <Dialog open={draft !== null} onOpenChange={(open) => {
        if (!open && !busy) setDraft(null);
      }}>
        <DialogContent className='subscription-editor-dialog subscription-source-dialog'>
          <form className='subscription-dialog-form' onSubmit={(event) => {
            event.preventDefault();
            void saveMetadata();
          }}>
            <DialogHeader>
              <DialogTitle>
                {draft?.editing
                  ? t('subscriptions.source.edit', { name: draft.editing.name })
                  : t('subscriptions.source.attach')}
              </DialogTitle>
              <DialogDescription>
                {t('subscriptions.source.editorDescription')}
              </DialogDescription>
            </DialogHeader>
            <ActionError message={actionError} title={t('subscriptions.source.actionFailed')} />
            {message === '' ? null : <div className='notice notice--success' role='status'>{message}</div>}
            {draft === null
              ? null
              : (
                  <div className='subscription-dialog-form__body'>
                    <div className='form-grid'>
                      <div className='field-group'>
                        <label htmlFor='source-name'>{t('subscriptions.common.name')}</label>
                        <input id='source-name' maxLength={128} onChange={(event) => setDraft({ ...draft, name: event.target.value })} value={draft.name} />
                      </div>
                      <div className='field-group'>
                        <label htmlFor='source-kind'>{t('subscriptions.source.field.kind')}</label>
                        <select id='source-kind' onChange={(event) => setDraft({ ...draft, kind: event.target.value as SubscriptionSourceKind })} value={draft.kind}>
                          <option value='local'>{t('subscriptions.source.kind.local')}</option>
                          <option value='remote'>{t('subscriptions.source.kind.remote')}</option>
                        </select>
                      </div>
                    </div>
                    <div className='field-group'>
                      <label htmlFor='source-config'>{t('subscriptions.source.field.config')}</label>
                      <textarea className='data-editor' id='source-config' onChange={(event) => setDraft({ ...draft, config: event.target.value })} rows={5} spellCheck={false} value={draft.config} />
                      <span>
                        {t('subscriptions.source.field.configHelp')}
                      </span>
                    </div>
                    <div className='field-group'>
                      <label htmlFor='source-format'>{t('subscriptions.source.field.format')}</label>
                      <select id='source-format' onChange={(event) => setDraft({ ...draft, format: event.target.value as SubscriptionSourceFormat })} value={draft.format}>
                        <option value='auto'>{t('subscriptions.source.format.auto')}</option>
                        <option value='sing-box-json'>{t('subscriptions.source.format.singBoxJSON')}</option>
                        <option value='mihomo-yaml'>{t('subscriptions.source.format.mihomoYAML')}</option>
                        <option value='uri-list'>{t('subscriptions.source.format.links')}</option>
                      </select>
                    </div>
                    <div className='field-group'>
                      <label htmlFor='source-document'>{t('subscriptions.source.field.document')}</label>
                      <textarea className='data-editor' id='source-document' onChange={(event) => setDraft({ ...draft, sourceDocument: event.target.value })} rows={7} spellCheck={false} value={draft.sourceDocument} />
                      <span>
                        {draft.editing
                          ? t('subscriptions.source.field.documentEditHelp')
                          : t('subscriptions.source.field.documentCreateHelp')}
                      </span>
                    </div>
                    <label className='check-field'>
                      <input checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} type='checkbox' />
                      <span>
                        <strong>{t('subscriptions.source.field.enabled')}</strong>
                        <small>{t('subscriptions.source.field.enabledHelp')}</small>
                      </span>
                    </label>
                  </div>
                )}
            <DialogFooter className='subscription-dialog-actions'>
              <button className='button button--secondary' disabled={busy} onClick={() => setDraft(null)} type='button'>{t('common.cancel')}</button>
              {draft?.editing
                ? <button className='button button--secondary' disabled={busy} onClick={() => void saveSourceVersion()} type='button'>{t('subscriptions.source.validateVersion')}</button>
                : null}
              <button className='button button--primary' disabled={busy || draft === null} type='submit'>
                {busy
                  ? t('subscriptions.common.saving')
                  : draft?.editing ? t('subscriptions.source.saveMetadata') : t('subscriptions.source.attach')}
              </button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Sheet open={versionSource !== null} onOpenChange={(open) => {
        if (!open && !loadingOlderVersions) {
          setRestoreEvidence(null);
          setVersionSource(null);
          setVersionNext(undefined);
          setSelectedVersion(null);
        }
      }}>
        <SheetContent className='subscription-detail-sheet subscription-version-sheet' side='right'>
          <SheetHeader>
            <SheetTitle>{versionSource?.name ?? t('subscriptions.source.version.history')}</SheetTitle>
            <SheetDescription>
              {versionSource === null
                ? t('subscriptions.source.version.historyDescription')
                : t('subscriptions.source.version.historySummary', {
                    count: numberFormatter.format(versions.length),
                    id: versionSource.id,
                  })}
            </SheetDescription>
          </SheetHeader>
          <ActionError message={actionError} title={t('subscriptions.source.version.actionFailed')} />
          <div className='subscription-detail-sheet__body subscription-version-list'>
            {versions.length === 0
              ? <div className='empty-state'>{t('subscriptions.source.version.empty')}</div>
              : null}
            {versions.map((version) => (
              <article className='subscription-version-row' key={version.id}>
                <div>
                  <code>{version.id}</code>
                  <span>
                    {version.format}
                    {' · '}
                    {dateTimeFormatter.format(new Date(version.fetched_at))}
                  </span>
                </div>
                <div className='inline-actions'>
                  <button
                    className='text-button'
                    disabled={busy || loadingOlderVersions || loadingVersionID !== null}
                    onClick={() => void inspectVersion(version)}
                    type='button'
                  >
                    {loadingVersionID === version.id ? t('subscriptions.common.loading') : t('subscriptions.common.inspect')}
                  </button>
                  <button className='text-button' disabled={busy || loadingOlderVersions || version.id === versionSource?.current_version_id} onClick={() => void prepareRestore(version)} type='button'>
                    {version.id === versionSource?.current_version_id
                      ? t('subscriptions.source.version.current')
                      : t('subscriptions.source.version.restore')}
                  </button>
                </div>
              </article>
            ))}
          </div>
          <SheetFooter>
            {versionNext === undefined
              ? null
              : (
                  <button
                    aria-busy={loadingOlderVersions}
                    className='button button--secondary'
                    disabled={busy || loadingOlderVersions}
                    onClick={() => void loadOlderVersions()}
                    type='button'
                  >
                    {loadingOlderVersions
                      ? t('subscriptions.source.version.loadingOlder')
                      : t('subscriptions.source.version.loadOlder')}
                  </button>
                )}
            <button className='button button--secondary' disabled={loadingOlderVersions} onClick={() => {
              setVersionSource(null);
              setVersionNext(undefined);
              setSelectedVersion(null);
            }} type='button'>
              {t('subscriptions.detail.close')}
            </button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <AlertDialog
        onOpenChange={(open) => {
          if (!open) setRestoreEvidence(null);
        }}
        open={restoreEvidence !== null}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('subscriptions.source.version.confirmRestoreTitle')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('subscriptions.source.version.confirmRestoreDescription', {
                name: restoreEvidence?.sourceName ?? '',
                sha256: restoreEvidence?.versionSHA256 ?? '',
                source: restoreEvidence?.sourceID ?? '',
                updatedAt: restoreEvidence?.sourceUpdatedAt ?? '',
                version: restoreEvidence?.versionID ?? '',
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={busy || restoreEvidence === null
                || versionSource?.id !== restoreEvidence.sourceID}
              onClick={() => {
                if (restoreEvidence === null) return;
                const evidence = restoreEvidence;
                setRestoreEvidence(null);
                void restore(evidence);
              }}
            >
              {t('subscriptions.source.version.confirmRestoreAction')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <Dialog open={selectedVersion !== null} onOpenChange={(open) => {
        if (!open) setSelectedVersion(null);
      }}>
        <DialogContent className='subscription-version-dialog'>
          <DialogHeader>
            <DialogTitle>{selectedVersion?.id ?? t('subscriptions.source.version.detail')}</DialogTitle>
            <DialogDescription>{t('subscriptions.source.version.description')}</DialogDescription>
          </DialogHeader>
          {selectedVersion === null
            ? null
            : (
                <div className='subscription-version-dialog__body'>
                  <dl className='subscription-detail__grid'>
                    <div>
                      <dt>{t('subscriptions.source.version.format')}</dt>
                      <dd>{selectedVersion.format}</dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.source.version.sha256')}</dt>
                      <dd><code>{selectedVersion.sha256}</code></dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.source.version.nodes')}</dt>
                      <dd>{numberFormatter.format(selectedVersion.normalized_nodes.length)}</dd>
                    </div>
                    <div>
                      <dt>{t('subscriptions.source.version.diagnosticCount')}</dt>
                      <dd>{numberFormatter.format(selectedVersion.diagnostics.length)}</dd>
                    </div>
                  </dl>
                  <div className='subscription-version-detail__body'>
                    <section>
                      <h3>{t('subscriptions.source.version.sourceDocument')}</h3>
                      <pre>{selectedVersion.raw_body ?? t('subscriptions.source.version.missingBody')}</pre>
                    </section>
                    <section>
                      <h3>{t('subscriptions.source.version.diagnostics')}</h3>
                      <pre>
                        {JSON.stringify({
                          diagnostics: selectedVersion.diagnostics,
                          normalized_nodes: selectedVersion.normalized_nodes,
                        }, null, 2)}
                      </pre>
                    </section>
                  </div>
                </div>
              )}
        </DialogContent>
      </Dialog>
    </section>
  );
}
