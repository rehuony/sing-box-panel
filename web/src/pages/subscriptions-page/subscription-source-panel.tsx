import { useCallback, useEffect, useState } from 'react';

import type { JsonObject, SubscriptionCursor, SubscriptionSource, SubscriptionSourceKind, SubscriptionSourceSummary } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

interface SourceDraft {
  name: string;
  config: string;
  enabled: boolean;
  snapshot: string;
  editing?: SubscriptionSource;
  kind: SubscriptionSourceKind;
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

function parseSnapshot(value: string): unknown {
  try {
    const parsed: unknown = JSON.parse(value);
    if (parsed === null || typeof parsed !== 'object') throw new Error('Snapshot is not an object.');
    return parsed;
  } catch {
    throw new Error('The source snapshot must be a JSON object or array.');
  }
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
    setDraft({ enabled: true, kind: 'local', name: '', config: '{}', snapshot: '[]' });
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
        snapshot: JSON.stringify(source.latest_snapshot ?? [], null, 2),
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
        setMessage(`Updated ${draft.name.trim()}. Its frozen snapshot did not change.`);
      } else {
        await client.createSubscriptionSource({
          name: draft.name.trim(), source_kind: draft.kind, config,
          latest_snapshot: parseSnapshot(draft.snapshot), enabled: draft.enabled,
        });
        setMessage(`Created ${draft.name.trim()}. Its snapshot will publish after apply.`);
      }
      setDraft(null);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  async function saveSnapshot() {
    if (!draft?.editing) return;
    try {
      const snapshot = parseSnapshot(draft.snapshot);
      setBusy(true);
      setActionError('');
      const updated = await client.updateSubscriptionSourceSnapshot(
        draft.editing.id,
        snapshot,
        draft.editing.updated_at,
      );
      setDraft({ ...draft, editing: updated });
      setMessage(`Stored a new candidate snapshot for ${updated.name}. Apply is still required.`);
      await load();
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
          <p>Candidate snapshots remain private until an activation freezes them.</p>
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
                    <th>Snapshot</th>
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
                      <td>{source.has_snapshot ? 'Candidate stored' : 'None'}</td>
                      <td>
                        <div className='table-actions'>
                          <button className='text-button' disabled={busy} onClick={() => void startEdit(source)} type='button'>Edit</button>
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
                <label htmlFor='source-snapshot'>Candidate snapshot JSON</label>
                <textarea className='data-editor' id='source-snapshot' onChange={(event) => setDraft({ ...draft, snapshot: event.target.value })} rows={7} spellCheck={false} value={draft.snapshot} />
                <span>{draft.editing ? 'Save snapshot separately. Metadata updates deliberately retain the existing snapshot.' : 'The initial snapshot is stored with this source.'}</span>
              </div>
              <label className='check-field'>
                <input checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} type='checkbox' />
                <span>
                  <strong>Enable this source</strong>
                  <small>Eligible for freezing during the next apply.</small>
                </span>
              </label>
              <div className='inline-actions'>
                <button className='button button--primary' disabled={busy} type='submit'>{busy ? 'Saving…' : draft.editing ? 'Save metadata' : 'Attach source'}</button>
                {draft.editing ? <button className='button button--secondary' disabled={busy} onClick={() => void saveSnapshot()} type='button'>Save snapshot candidate</button> : null}
                <button className='button button--secondary' disabled={busy} onClick={() => setDraft(null)} type='button'>Cancel</button>
              </div>
            </form>
          )}
    </section>
  );
}
