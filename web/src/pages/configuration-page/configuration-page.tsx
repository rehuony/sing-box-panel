import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  parse as parseLosslessJSON,
  stringify as stringifyLosslessJSON,
} from 'lossless-json';

import {
  ApiRequestError,
  type CanonicalRevisionPage,
  type Entity,
  type EntityCollection,
  type EntityList,
} from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice, describeRequestError } from '@/components/error-notice';
import { PageHeading } from '@/components/page-heading';
import { useControlPlane } from '@/stores/control-plane.store';

import { StartupWorkflow } from './startup-workflow';
import { VersionedCanonicalForm } from './versioned-canonical-form';
import './configuration-page.css';

type EntityLoadState =
  | { status: 'loading'; data: null; error: null }
  | { status: 'error'; data: null; error: unknown }
  | { status: 'ready'; data: EntityList; error: null };

interface EditorState {
  draft: string;
  existingID?: string;
  title: string;
}

const starterEntities: Record<EntityCollection, Entity> = {
  nodes: { id: 'new-node', kind: 'socks', enabled: true },
  rules: { id: 'new-rule', enabled: true },
};

function prettyJSON(value: unknown): string {
  const encoded = stringifyLosslessJSON(value, null, 2);
  if (encoded === undefined) {
    throw new Error('The entity cannot be encoded as JSON.');
  }
  return encoded;
}

function formatTimestamp(timestamp: string): string {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(timestamp));
}

function parseEntity(draft: string): Entity {
  let parsed: unknown;
  try {
    parsed = parseLosslessJSON(draft);
  } catch {
    throw new Error('The draft is not valid JSON. Check commas, quotes and braces.');
  }
  if (parsed === null || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('An entity must be one JSON object.');
  }
  const candidate = parsed as Record<string, unknown>;
  if (typeof candidate.id !== 'string' || candidate.id.trim() === '') {
    throw new Error('The entity needs a non-empty string “id”.');
  }
  if (typeof candidate.enabled !== 'boolean') {
    throw new Error('The entity needs an explicit boolean “enabled”.');
  }
  return candidate as Entity;
}

export function ConfigurationPage() {
  const client = useApiClient();
  const controlPlane = useControlPlane();
  const [collection, setCollection] = useState<EntityCollection>('nodes');
  const [entities, setEntities] = useState<EntityLoadState>({
    status: 'loading',
    data: null,
    error: null,
  });
  const [revisions, setRevisions] = useState<CanonicalRevisionPage | null>(null);
  const [revisionError, setRevisionError] = useState<unknown>(null);
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [editorError, setEditorError] = useState('');
  const [saveMessage, setSaveMessage] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [hasConflict, setHasConflict] = useState(false);

  const loadEntities = useCallback(
    async (signal?: AbortSignal) => {
      setEntities((current) =>
        current.status === 'ready'
          ? current
          : { status: 'loading', data: null, error: null },
      );
      try {
        const data = await client.listEntities(collection, signal);
        if (!signal?.aborted) {
          setEntities({ status: 'ready', data, error: null });
        }
      } catch (error) {
        if (
          signal?.aborted ||
          (error instanceof DOMException && error.name === 'AbortError')
        ) {
          return;
        }
        setEntities({ status: 'error', data: null, error });
      }
    },
    [client, collection],
  );

  const loadRevisions = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setRevisionError(null);
        setRevisions(await client.listRevisions(signal));
      } catch (error) {
        if (
          signal?.aborted ||
          (error instanceof DOMException && error.name === 'AbortError')
        ) {
          return;
        }
        setRevisionError(error);
      }
    },
    [client],
  );

  useEffect(() => {
    const controller = new AbortController();
    void loadEntities(controller.signal);
    return () => controller.abort();
  }, [loadEntities]);

  useEffect(() => {
    const controller = new AbortController();
    void loadRevisions(controller.signal);
    return () => controller.abort();
  }, [loadRevisions]);

  const exactVersion = controlPlane.viewVersion || 'unresolved';
  const exactCapability = controlPlane.viewCapability?.resolution.exact_version === controlPlane.viewVersion
    ? controlPlane.viewCapability
    : null;
  const viewCapability = exactCapability?.support_level ?? 'unavailable';
  const manualMode = viewCapability === 'manual_json';
  const capabilityUnavailable = controlPlane.viewVersion === '';
  const structuredUnavailable = manualMode || viewCapability === 'unavailable' || capabilityUnavailable;
  const entityLabel = collection === 'nodes' ? 'node' : 'rule';

  const revisionID =
    entities.status === 'ready' ? entities.data.revision.id : null;
  const sortedEntities = useMemo(
    () => (entities.status === 'ready' ? entities.data.entities : []),
    [entities],
  );

  function startCreate() {
    setEditor({
      draft: prettyJSON(starterEntities[collection]),
      title: `Create ${entityLabel}`,
    });
    setEditorError('');
    setSaveMessage('');
    setHasConflict(false);
  }

  function startEdit(entity: Entity) {
    setEditor({
      draft: prettyJSON(entity),
      existingID: entity.id,
      title: `Edit ${entity.id}`,
    });
    setEditorError('');
    setSaveMessage('');
    setHasConflict(false);
  }

  async function saveDraft() {
    if (editor === null || revisionID === null) {
      return;
    }
    let entity: Entity;
    try {
      entity = parseEntity(editor.draft);
    } catch (error) {
      setEditorError(describeRequestError(error));
      return;
    }

    setIsSaving(true);
    setEditorError('');
    setSaveMessage('');
    setHasConflict(false);
    try {
      const result = await client.saveEntity(
        collection,
        entity,
        { baseRevision: revisionID, existingID: editor.existingID },
      );
      setSaveMessage(
        result.no_change
          ? `No change; revision #${result.revision.sequence} remains current.`
          : `Saved as revision #${result.revision.sequence}.`,
      );
      setEditor(null);
      await Promise.all([loadEntities(), loadRevisions(), controlPlane.refresh()]);
    } catch (error) {
      if (error instanceof ApiRequestError && error.status === 412) {
        setHasConflict(true);
        setEditorError(
          'The canonical revision changed on the server. Your draft is preserved; reload the current revision and review before saving again.',
        );
      } else {
        setEditorError(describeRequestError(error));
      }
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <div className="page-stack">
      <PageHeading
        eyebrow="Configuration / canonical"
        summary={`Structured edits target sing-box ${exactVersion}. Save advances canonical intent; render and apply remain explicit, separate operations.`}
        title="Save is not apply."
      />

      {manualMode ? (
        <section className="manual-mode-notice" aria-labelledby="manual-mode-title">
          <div>
            <p className="eyebrow">Manual JSON mode</p>
            <h2 id="manual-mode-title">Structured support is unavailable for {exactVersion}</h2>
            <p>
              Existing canonical intent remains intact. This version requires exact
              sing-box JSON bytes for startup and cannot use structured projection safely.
            </p>
          </div>
          <span className="support-pill support-pill--manual_json"><span className="support-pill__mark" aria-hidden="true" />Manual JSON</span>
        </section>
      ) : null}

      {exactCapability?.presentation === undefined || exactCapability.presentation.ui.length === 0 ? null : (
        <VersionedCanonicalForm
          capability={exactCapability}
          exactVersion={exactVersion}
          onSaved={async () => {
            await Promise.all([loadEntities(), loadRevisions(), controlPlane.refresh()]);
          }}
        />
      )}

      <StartupWorkflow
        baseRevision={revisionID}
        capability={
          viewCapability
        }
        exactVersion={exactVersion}
        onCanonicalChange={async () => {
          await Promise.all([loadEntities(), loadRevisions(), controlPlane.refresh()]);
        }}
      />

      {capabilityUnavailable ? (
        <section className="manual-mode-notice" aria-labelledby="capability-unavailable-title">
          <div>
            <p className="eyebrow">Exact version required</p>
            <h2 id="capability-unavailable-title">No core version is selected</h2>
            <p>
              Select an exact installed or catalog version before editing. The panel
              will not substitute the latest release when no running core exists.
            </p>
          </div>
        </section>
      ) : null}

      <div className="configuration-layout">
        <section className="entity-workspace" aria-labelledby="entities-title">
          <div className="workspace-toolbar">
            <div>
              <p className="eyebrow">Revision-scoped entities</p>
              <h2 id="entities-title">Nodes and rules</h2>
            </div>
            <button
              className="button button--primary"
              disabled={entities.status !== 'ready' || structuredUnavailable}
              onClick={startCreate}
              type="button"
            >
              Create {entityLabel}
            </button>
          </div>

          <div className="segmented-control" role="group" aria-label="Entity collection">
            {(['nodes', 'rules'] as const).map((candidate) => (
              <button
                aria-pressed={collection === candidate}
                className={collection === candidate ? 'is-selected' : ''}
                key={candidate}
                onClick={() => {
                  setCollection(candidate);
                  setEditor(null);
                  setEditorError('');
                  setSaveMessage('');
                }}
                type="button"
              >
                {candidate === 'nodes' ? 'Nodes' : 'Rules'}
              </button>
            ))}
          </div>

          {saveMessage === '' ? null : (
            <div className="notice notice--success" role="status">
              <strong>Canonical revision saved</strong>
              <p>{saveMessage}</p>
            </div>
          )}

          {entities.status === 'loading' ? (
            <div className="inline-loading" aria-busy="true">Loading {collection}…</div>
          ) : null}
          {entities.status === 'error' ? (
            <ErrorNotice error={entities.error} title={`Could not load ${collection}`} />
          ) : null}
          {entities.status === 'ready' && sortedEntities.length === 0 ? (
            <div className="empty-state">
              <strong>No {collection} in revision #{entities.data.revision.sequence}.</strong>
              <p>
                {manualMode
                  ? 'Save exact startup bytes in the startup pipeline above.'
                  : capabilityUnavailable
                    ? 'Select an exact core version before creating structured entities.'
                  : `Create the first ${entityLabel} as a JSON object.`}
              </p>
            </div>
          ) : null}
          {entities.status === 'ready' && sortedEntities.length > 0 ? (
            <div className="entity-table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>{collection === 'nodes' ? 'Kind' : 'Match shape'}</th>
                    <th>State</th>
                    <th><span className="visually-hidden">Actions</span></th>
                  </tr>
                </thead>
                <tbody>
                  {sortedEntities.map((entity) => (
                    <tr key={entity.id}>
                      <td><code>{entity.id}</code></td>
                      <td>
                        {collection === 'nodes'
                          ? entity.kind ?? 'unspecified'
                          : `${Math.max(Object.keys(entity).length - 2, 0)} fields`}
                      </td>
                      <td>
                        <span className={`state-label ${entity.enabled ? 'state-label--success' : 'state-label--neutral'}`}>
                          <span aria-hidden="true" /> {entity.enabled ? 'Enabled' : 'Disabled'}
                        </span>
                      </td>
                      <td>
                        <button
                          className="text-button"
                          disabled={structuredUnavailable}
                          onClick={() => startEdit(entity)}
                          type="button"
                        >
                          Edit JSON
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}

          {editor === null ? null : (
            <form
              className="json-editor"
              onSubmit={(event) => {
                event.preventDefault();
                void saveDraft();
              }}
            >
              <div className="section-heading">
                <div>
                  <p className="eyebrow">JSON entity editor</p>
                  <h3>{editor.title}</h3>
                </div>
                <span>Base {revisionID?.slice(0, 14)}…</span>
              </div>
              <label htmlFor="entity-json">Entity JSON</label>
              <textarea
                aria-describedby={editorError === '' ? 'entity-json-hint' : 'entity-json-error entity-json-hint'}
                aria-invalid={editorError !== ''}
                id="entity-json"
                onChange={(event) =>
                  setEditor((current) =>
                    current === null ? null : { ...current, draft: event.target.value },
                  )
                }
                rows={14}
                spellCheck={false}
                value={editor.draft}
              />
              {editorError === '' ? null : (
                <div className="form-error" id="entity-json-error" role="alert">
                  <strong>{hasConflict ? 'Revision conflict' : 'Draft was not saved'}</strong>
                  <span>{editorError}</span>
                </div>
              )}
              <small id="entity-json-hint">
                Unknown fields are retained. The server validates IDs, required fields and the current revision.
              </small>
              <div className="inline-actions">
                <button className="button button--primary" disabled={isSaving} type="submit">
                  {isSaving ? 'Saving…' : 'Save revision'}
                </button>
                {hasConflict ? (
                  <button
                    className="button button--warning"
                    onClick={() => void loadEntities()}
                    type="button"
                  >
                    Reload current revision
                  </button>
                ) : null}
                <button
                  className="button button--secondary"
                  disabled={isSaving}
                  onClick={() => setEditor(null)}
                  type="button"
                >
                  Discard draft
                </button>
              </div>
            </form>
          )}
        </section>

        <aside className="revision-panel" aria-labelledby="revision-history-title">
          <p className="eyebrow">Immutable history</p>
          <h2 id="revision-history-title">Recent revisions</h2>
          {revisionError === null ? null : <ErrorNotice error={revisionError} />}
          {revisions?.items.length === 0 ? (
            <div className="empty-state empty-state--compact">
              <strong>No canonical revision yet.</strong>
              <p>Create a node or rule after initialization.</p>
            </div>
          ) : null}
          {revisions === null ? (
            <div className="inline-loading" aria-busy="true">Loading history…</div>
          ) : (
            <ol className="revision-list">
              {revisions.items.map((revision, index) => (
                <li key={revision.id}>
                  <span className="revision-list__node" aria-hidden="true" />
                  <div>
                    <strong>Revision #{revision.sequence}</strong>
                    <span>{formatTimestamp(revision.created_at)}</span>
                    <code>{revision.sha256.slice(0, 12)}…</code>
                  </div>
                  {index === 0 ? <span className="rail-tag rail-tag--selected">head</span> : null}
                </li>
              ))}
            </ol>
          )}
        </aside>
      </div>

    </div>
  );
}
