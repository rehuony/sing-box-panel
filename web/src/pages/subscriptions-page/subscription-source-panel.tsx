import { useCallback, useEffect, useState } from 'react';

import type { JsonObject, SubscriptionCursor, SubscriptionSource, SubscriptionSourceFormat, SubscriptionSourceKind, SubscriptionSourceSummary, SubscriptionSourceVersion } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

interface SourceDraft {
  name: string;
  config: string;
  enabled: boolean;
  sourceDocument: string;
  editing?: SubscriptionSource;
  kind: SubscriptionSourceKind;
  format: SubscriptionSourceFormat;
}

function parseObject(value: string, label: string): JsonObject {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    throw new Error(`${label} is not valid JSON.`);
  }
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error(`${label} must be one JSON object.`);
  }
  return parsed as JsonObject;
}

export function SubscriptionSourcePanel() {
  const client = useApiClient();
  const [sources, setSources] = useState<SubscriptionSourceSummary[] | null>(null);
  const [next, setNext] = useState<SubscriptionCursor>();
  const [loadError, setLoadError] = useState<unknown>(null);
  const [draft, setDraft] = useState<SourceDraft | null>(null);
  const [actionError, setActionError] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  const [versions, setVersions] = useState<SubscriptionSourceVersion[]>([]);
  const [versionSource, setVersionSource] = useState<SubscriptionSourceSummary | null>(null);

  const load = useCallback(async (signal?: AbortSignal, cursor?: SubscriptionCursor, append = false) => {
    try {
      setLoadError(null);
      const result = await client.listSubscriptionSources({
        limit: 50,
        beforeTime: cursor?.created_at,
        beforeID: cursor?.id,
      }, signal);
      if (!signal?.aborted) {
        setSources((current) => append ? [...(current ?? []), ...result.items] : result.items);
        setNext(result.next);
      }
    } catch (error) {
      if (!signal?.aborted) setLoadError(error);
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
  }

  async function startEdit(summary: SubscriptionSourceSummary) {
    setBusy(true);
    setActionError('');
    setMessage('');
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
      if (draft.name.trim() === '') throw new Error('Enter a source name.');
      const config = parseObject(draft.config, 'Source configuration');
      setBusy(true);
      setActionError('');
      if (draft.editing) {
        await client.updateSubscriptionSource(draft.editing.id, {
          name: draft.name.trim(), source_kind: draft.kind, config, enabled: draft.enabled,
        }, draft.editing.updated_at);
        setMessage(`Updated ${draft.name.trim()}. Its current source version did not change.`);
      } else {
        const created = await client.createSubscriptionSource({
          name: draft.name.trim(), source_kind: draft.kind, config,
          enabled: draft.enabled,
        });
        if (draft.sourceDocument.trim() !== '') {
          await client.createSubscriptionSourceVersion(
            created.id, draft.format, draft.sourceDocument, created.updated_at,
          );
        }
        setMessage(`Created ${draft.name.trim()}. Its current source version is live immediately.`);
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
      if (draft.sourceDocument.trim() === '') throw new Error('Paste a source document.');
      setBusy(true);
      setActionError('');
      const saved = await client.createSubscriptionSourceVersion(
        draft.editing.id,
        draft.format,
        draft.sourceDocument,
        draft.editing.updated_at,
      );
      setDraft({ ...draft, editing: saved.source, sourceDocument: '' });
      setMessage(`Activated a validated ${saved.version.format} version for ${saved.source.name}.`);
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
      const task = await client.refreshSubscriptionSource(source.id);
      setMessage(`Queued refresh task ${task.id}. A failure will preserve the current version.`);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function showVersions(source: SubscriptionSourceSummary) {
    try {
      setBusy(true);
      const page = await client.listSubscriptionSourceVersions(source.id, { limit: 100 });
      setVersionSource(source);
      setVersions(page.items);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function restore(version: SubscriptionSourceVersion) {
    if (!versionSource) return;
    try {
      setBusy(true);
      const source = await client.getSubscriptionSource(versionSource.id);
      await client.restoreSubscriptionSourceVersion(source.id, version.id, source.updated_at);
      await Promise.all([load(), showVersions({ ...versionSource, current_version_id: version.id })]);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function remove(source: SubscriptionSourceSummary) {
    setBusy(true);
    setActionError('');
    try {
      await client.deleteSubscriptionSource(source.id, source.updated_at);
      setMessage(`Deleted ${source.name}.`);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className='subscription-panel' aria-labelledby='subscription-sources-title'>
      <div className='subscription-panel__heading'>
        <div>
          <p className='eyebrow'>02 / Attached input</p>
          <h2 id='subscription-sources-title'>Sources</h2>
          <p>Validated versions are append-only; the current pointer is live and refresh failures preserve it.</p>
        </div>
        <span className='count-label'>
          {sources?.length ?? 0}
          {' '}
          loaded
        </span>
        <button className='button button--primary' onClick={startCreate} type='button'>Attach source</button>
      </div>
      <ActionError message={actionError} title='Source change failed' />
      {loadError === null ? null : <ErrorNotice error={loadError} title='Could not load sources' />}
      {message === ''
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>Source updated</strong>
              <p>{message}</p>
            </div>
          )}
      {sources === null ? <div className='inline-loading' aria-busy='true'>Loading sources…</div> : null}
      {sources?.length === 0
        ? (
            <div className='empty-state'>
              <strong>No attached sources.</strong>
              <p>Additions from the final startup JSON can still be published by channels.</p>
            </div>
          )
        : null}
      {sources && sources.length > 0
        ? (
            <div className='entity-table-wrap'>
              <table className='data-table'>
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Kind</th>
                    <th>Current version</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {sources.map((source) => (
                    <tr key={source.id}>
                      <td>
                        <strong>{source.name}</strong>
                        <small className='table-subline'>{source.enabled ? 'Enabled' : 'Disabled'}</small>
                      </td>
                      <td><code>{source.source_kind}</code></td>
                      <td>{source.current_version_id ? <code>{source.current_version_id}</code> : 'None'}</td>
                      <td>
                        <div className='table-actions'>
                          <button className='text-button' disabled={busy} onClick={() => void startEdit(source)} type='button'>Edit</button>
                          <button className='text-button' disabled={busy || source.source_kind !== 'remote'} onClick={() => void refresh(source)} type='button'>Refresh</button>
                          <button className='text-button' disabled={busy} onClick={() => void showVersions(source)} type='button'>Versions</button>
                          <button className='text-button text-button--danger' disabled={busy} onClick={() => void remove(source)} type='button'>Delete</button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        : null}
      {versionSource
        ? (
            <div className='control-form'>
              <div className='section-heading'>
                <div>
                  <p className='eyebrow'>Version history</p>
                  <h3>{versionSource.name}</h3>
                </div>
                <span>
                  {versions.length}
                  {' '}
                  loaded
                </span>
              </div>
              {versions.map((version) => (
                <div className='inline-actions' key={version.id}>
                  <code>{version.id}</code>
                  <span>
                    {version.format}
                    {' '}
                    ·
                    {' '}
                    {new Date(version.fetched_at).toLocaleString()}
                  </span>
                  <button className='text-button' disabled={busy || version.id === versionSource.current_version_id} onClick={() => void restore(version)} type='button'>{version.id === versionSource.current_version_id ? 'Current' : 'Restore'}</button>
                </div>
              ))}
              <button className='button button--secondary' onClick={() => setVersionSource(null)} type='button'>Close history</button>
            </div>
          )
        : null}
      {next
        ? <button className='button button--secondary' disabled={busy} onClick={() => void load(undefined, next, true)} type='button'>Load more sources</button>
        : null}
      {draft === null
        ? null
        : (
            <form className='control-form' onSubmit={(event) => {
              event.preventDefault();
              void saveMetadata();
            }}>
              <div className='section-heading'>
                <div>
                  <p className='eyebrow'>Source editor</p>
                  <h3>{draft.editing ? `Edit ${draft.editing.name}` : 'Attach source'}</h3>
                </div>
                <span>CAS protected</span>
              </div>
              <div className='form-grid'>
                <div className='field-group'>
                  <label htmlFor='source-name'>Name</label>
                  <input id='source-name' maxLength={128} onChange={(event) => setDraft({ ...draft, name: event.target.value })} value={draft.name} />
                </div>
                <div className='field-group'>
                  <label htmlFor='source-kind'>Kind</label>
                  <select id='source-kind' onChange={(event) => setDraft({ ...draft, kind: event.target.value as SubscriptionSourceKind })} value={draft.kind}>
                    <option value='local'>Local</option>
                    <option value='remote'>Remote</option>
                  </select>
                </div>
              </div>
              <div className='field-group'>
                <label htmlFor='source-config'>Source configuration JSON</label>
                <textarea className='data-editor' id='source-config' onChange={(event) => setDraft({ ...draft, config: event.target.value })} rows={5} spellCheck={false} value={draft.config} />
                <span>Remote credentials are accepted by the server contract but are never echoed into logs.</span>
              </div>
              <div className='field-group'>
                <label htmlFor='source-format'>Source document format</label>
                <select id='source-format' onChange={(event) => setDraft({ ...draft, format: event.target.value as SubscriptionSourceFormat })} value={draft.format}>
                  <option value='auto'>Auto detect</option>
                  <option value='sing-box-json'>sing-box JSON</option>
                  <option value='mihomo-yaml'>Mihomo YAML</option>
                  <option value='uri-list'>Share links</option>
                </select>
              </div>
              <div className='field-group'>
                <label htmlFor='source-document'>New source document</label>
                <textarea className='data-editor' id='source-document' onChange={(event) => setDraft({ ...draft, sourceDocument: event.target.value })} rows={7} spellCheck={false} value={draft.sourceDocument} />
                <span>{draft.editing ? 'Save as a new validated version. Existing versions are never overwritten.' : 'Optional for creation; remote sources can be refreshed later.'}</span>
              </div>
              <label className='check-field'>
                <input checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} type='checkbox' />
                <span>
                  <strong>Enable this source</strong>
                  <small>Its current successful version is eligible for live publication.</small>
                </span>
              </label>
              <div className='inline-actions'>
                <button className='button button--primary' disabled={busy} type='submit'>{busy ? 'Saving…' : draft.editing ? 'Save metadata' : 'Attach source'}</button>
                {draft.editing ? <button className='button button--secondary' disabled={busy} onClick={() => void saveSourceVersion()} type='button'>Validate and activate version</button> : null}
                <button className='button button--secondary' disabled={busy} onClick={() => setDraft(null)} type='button'>Cancel</button>
              </div>
            </form>
          )}
    </section>
  );
}
