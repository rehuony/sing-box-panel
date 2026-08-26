import { useCallback, useEffect, useMemo, useState } from 'react';

import type {
  ApiClient,
  CapabilityLevel,
  CoreArtifact,
  ManualArtifact,
  ManualReattachPreview,
  ManualReplacePreview,
  MonitoringTier,
  StartupArtifactState,
  StartupArtifactSummary,
} from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

export interface StartupWorkflowProps {
  exactVersion: string;
  baseRevision: string | null;
  capability: CapabilityLevel;
  onCanonicalChange: () => Promise<void>;
}

interface Candidate {
  id: string;
  sha256: string;
  createdAt?: string;
  coreArtifactID: string;
  state: StartupArtifactState;
  kind: 'structured' | 'manual';
}

interface ManualPreviewState {
  baseRevision: string;
  selectedCore: string;
  allowCompatible: boolean;
  preview: ManualReplacePreview;
}

const manualStarter = '{\n  "log": {\n    "level": "info"\n  },\n  "outbounds": []\n}\n';

function formatTimestamp(value?: string): string {
  if (!value) return 'Created in this browser session';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}

function stateClass(state: StartupArtifactState): string {
  if (state === 'ready') return 'state-label--success';
  if (state === 'failed' || state === 'stale') return 'state-label--warning';
  return 'state-label--neutral';
}

function displayValue(value: { present: boolean; value?: unknown }): string {
  return value.present ? JSON.stringify(value.value) : 'Absent';
}

async function listVerifiedCoreArtifacts(
  client: ApiClient,
  exactVersion: string,
  signal?: AbortSignal,
): Promise<CoreArtifact[]> {
  const artifacts: CoreArtifact[] = [];
  let beforeTime: string | undefined;
  let beforeID: string | undefined;
  let previousCursor = '';

  for (;;) {
    const page = await client.listCoreArtifacts(
      {
        beforeID,
        beforeTime,
        exactVersion,
        limit: 200,
        verificationState: 'verified',
      },
      signal,
    );
    artifacts.push(...page.items);
    if (page.next === undefined) return artifacts;

    const cursor = `${page.next.created_at}\n${page.next.id}`;
    if (cursor === previousCursor) {
      throw new Error('Core artifact pagination returned a repeated cursor.');
    }
    previousCursor = cursor;
    beforeTime = page.next.created_at;
    beforeID = page.next.id;
  }
}

export function StartupWorkflow({
  baseRevision,
  capability,
  exactVersion,
  onCanonicalChange,
}: StartupWorkflowProps) {
  const client = useApiClient();
  const [artifacts, setArtifacts] = useState<CoreArtifact[] | null>(null);
  const [manualArtifacts, setManualArtifacts] = useState<ManualArtifact[] | null>(null);
  const [startupArtifacts, setStartupArtifacts] = useState<StartupArtifactSummary[] | null>(null);
  const [selectedCore, setSelectedCore] = useState('');
  const [selectedCandidate, setSelectedCandidate] = useState('');
  const [monitoringTier, setMonitoringTier] = useState<MonitoringTier>('process_only');
  const [allowCompatible, setAllowCompatible] = useState(false);
  const [manualOpenOverride, setManualOpenOverride] = useState<boolean | null>(null);
  const [manualRaw, setManualRaw] = useState(manualStarter);
  const [manualPreviewState, setManualPreviewState] = useState<ManualPreviewState | null>(null);
  const [reattach, setReattach] = useState<ManualReattachPreview | null>(null);
  const [decisions, setDecisions] = useState<Record<string, 'current' | 'manual'>>({});
  const [loadError, setLoadError] = useState<unknown>(null);
  const [actionError, setActionError] = useState('');
  const [message, setMessage] = useState('');
  const [busyAction, setBusyAction] = useState('');

  const load = useCallback(async (signal?: AbortSignal) => {
    if (exactVersion === '' || exactVersion === 'unresolved' || exactVersion === 'Not selected') {
      setArtifacts([]);
      setManualArtifacts([]);
      setStartupArtifacts([]);
      return;
    }
    setLoadError(null);
    try {
      const [cores, manuals, startups] = await Promise.all([
        listVerifiedCoreArtifacts(client, exactVersion, signal),
        client.listManualArtifacts({ coreVersion: exactVersion, limit: 50 }, signal),
        client.listStartupArtifacts({ coreVersion: exactVersion, limit: 100 }, signal),
      ]);
      if (!signal?.aborted) {
        const matching = cores;
        setArtifacts(matching);
        setManualArtifacts(manuals.items);
        setStartupArtifacts(startups.items);
        setSelectedCore((current) => {
          if (matching.some((item) => item.id === current)) return current;
          return matching.length === 1 ? matching[0].id : '';
        });
      }
    } catch (error) {
      if (!signal?.aborted) setLoadError(error);
    }
  }, [client, exactVersion]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load]);

  const manualPreview = manualPreviewState !== null
    && manualPreviewState.allowCompatible === allowCompatible
    && manualPreviewState.baseRevision === baseRevision
    && manualPreviewState.selectedCore === selectedCore
    ? manualPreviewState.preview
    : null;

  const candidates = useMemo<Candidate[]>(() => {
    return (startupArtifacts ?? []).map((artifact) => ({
      coreArtifactID: artifact.core_artifact_id,
      createdAt: artifact.created_at,
      id: artifact.id,
      kind: artifact.kind,
      sha256: artifact.config_sha256,
      state: artifact.state,
    }));
  }, [startupArtifacts]);

  const structuredSupported = capability === 'native_structured' || capability === 'compatible_structured';
  const manualOpen = manualOpenOverride ?? capability === 'manual_json';

  async function renderStructured() {
    if (selectedCore === '') {
      setActionError('Select one exact verified core artifact before rendering.');
      return;
    }
    setBusyAction('render');
    setActionError('');
    setMessage('');
    try {
      const rendered = await client.renderStructured({
        coreVersion: exactVersion,
        coreArtifactID: selectedCore,
        allowCompatible,
      });
      setStartupArtifacts((current) => [{
        id: rendered.artifact.id,
        kind: 'structured',
        canonical_revision_id: rendered.artifact.canonical_revision_id,
        exact_core_version: rendered.artifact.exact_core_version,
        capability_commit: rendered.artifact.capability_commit,
        capability_digest: rendered.artifact.capability_digest,
        renderer_version: rendered.artifact.renderer_version,
        core_artifact_id: rendered.artifact.core_artifact_id,
        config_sha256: rendered.artifact.config_sha256,
        diagnostics: rendered.artifact.diagnostics,
        state: rendered.artifact.state,
        created_at: new Date().toISOString(),
      }, ...(current ?? []).filter((item) => item.id !== rendered.artifact.id)]);
      setSelectedCandidate(rendered.artifact.id);
      setMessage(`Rendered ${rendered.artifact.id}; check task ${rendered.task.id} is ${rendered.task.status}. Apply remains separate.`);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusyAction('');
    }
  }

  async function previewManual() {
    if (baseRevision === null || selectedCore === '') {
      setActionError('A canonical base revision and one exact verified core artifact are required.');
      return;
    }
    setBusyAction('manual-preview');
    setActionError('');
    setMessage('');
    try {
      const preview = await client.previewManualReplacement({
        baseRevision,
        coreVersion: exactVersion,
        coreArtifactID: selectedCore,
        raw: manualRaw,
        allowCompatible,
      });
      setManualPreviewState({
        allowCompatible,
        baseRevision,
        preview,
        selectedCore,
      });
      setMessage(preview.reverse.available
        ? `Reverse preview proved ${preview.reverse.residual_paths.length} residual path(s). Review the proposed canonical document before saving.`
        : `No proven reverse mapping (${preview.reverse.reason_code ?? 'unavailable'}). All listed paths remain manual-owned.`);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusyAction('');
    }
  }

  async function replaceManual() {
    if (baseRevision === null || selectedCore === '' || manualPreview === null) {
      setActionError('Generate and review a reverse-mapping preview before saving.');
      return;
    }
    setBusyAction('manual');
    setActionError('');
    setMessage('');
    try {
      const saved = await client.replaceManualArtifact({
        baseRevision,
        coreVersion: exactVersion,
        coreArtifactID: selectedCore,
        raw: manualRaw,
        allowCompatible,
      });
      setSelectedCandidate(saved.artifact.id);
      setManualPreviewState(null);
      setMessage(`Saved exact bytes as ${saved.artifact.id}; reverse evidence was recomputed and check task ${saved.task.id} is ${saved.task.status}.`);
      await Promise.all([load(), onCanonicalChange()]);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusyAction('');
    }
  }

  async function apply() {
    const candidate = candidates.find((item) => item.id === selectedCandidate);
    if (!candidate || candidate.state !== 'ready') {
      setActionError('Select a ready startup candidate. Pending, failed and stale candidates cannot be applied.');
      return;
    }
    setBusyAction('apply');
    setActionError('');
    setMessage('');
    try {
      const queued = await client.activateStartupArtifact(candidate.id, monitoringTier);
      setMessage(`Apply queued as ${queued.task.id} for immutable bundle ${queued.activation.activation_bundle_id}.`);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusyAction('');
    }
  }

  async function discard(artifact: ManualArtifact) {
    setBusyAction(`discard:${artifact.id}`);
    setActionError('');
    try {
      await client.discardManualArtifact(artifact.id);
      if (selectedCandidate === artifact.id) setSelectedCandidate('');
      setMessage(`Discarded manual candidate ${artifact.id}.`);
      await load();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusyAction('');
    }
  }

  async function previewReattach(artifact: ManualArtifact) {
    setBusyAction(`reattach:${artifact.id}`);
    setActionError('');
    try {
      const preview = await client.previewManualReattach(artifact.id);
      setReattach(preview);
      setDecisions({});
      setMessage(preview.conflicts.length === 0
        ? 'Reattach preview has no conflicts. Review residual paths before applying.'
        : `Reattach preview requires ${preview.conflicts.length} explicit decision(s).`);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusyAction('');
    }
  }

  async function applyReattach() {
    if (reattach === null) return;
    if (reattach.conflicts.some((conflict) => decisions[conflict.path] === undefined)) {
      setActionError('Choose current or manual for every conflict before reattaching.');
      return;
    }
    setBusyAction('reattach-apply');
    setActionError('');
    try {
      const result = await client.applyManualReattach(reattach.evidence.startup_artifact_id, {
        evidence: reattach.evidence,
        decisions,
      });
      setReattach(null);
      setSelectedCandidate(result.artifact.id);
      setMessage(`Reattached into revision #${result.revision.sequence}; startup check ${result.task.id} is ${result.task.status}.`);
      await Promise.all([load(), onCanonicalChange()]);
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setBusyAction('');
    }
  }

  return (
    <section className='startup-workflow' aria-labelledby='startup-workflow-title'>
      <div className='workflow-heading'>
        <div>
          <p className='eyebrow'>Save → Render → Apply</p>
          <h2 id='startup-workflow-title'>Startup pipeline</h2>
          <p>
            A canonical save never restarts the core. Rendering creates immutable candidate bytes;
            apply accepts only a checked ready candidate.
          </p>
        </div>
        <button className='button button--secondary button--small' onClick={() => void load()} type='button'>Refresh candidates</button>
      </div>
      <div className='workflow-steps' aria-label='Configuration lifecycle'>
        <div>
          <span>1</span>
          <strong>Save intent</strong>
          <small>{baseRevision ? `Head ${baseRevision.slice(0, 12)}…` : 'No canonical head'}</small>
        </div>
        <div>
          <span>2</span>
          <strong>Render bytes</strong>
          <small>
            Exact
            {' '}
            {exactVersion}
          </small>
        </div>
        <div>
          <span>3</span>
          <strong>Apply bundle</strong>
          <small>Runtime task after check</small>
        </div>
      </div>

      <ActionError message={actionError} title='Startup workflow stopped' />
      {loadError === null ? null : <ErrorNotice error={loadError} title='Could not load startup candidates' />}
      {message === ''
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>Workflow updated</strong>
              <p>{message}</p>
            </div>
          )}

      <div className='startup-controls'>
        <div className='startup-control-card'>
          <p className='eyebrow'>Render target</p>
          <h3>Exact core artifact</h3>
          <div className='field-group'>
            <label htmlFor='render-core-artifact'>Verified artifact</label>
            <select id='render-core-artifact' onChange={(event) => setSelectedCore(event.target.value)} value={selectedCore}>
              <option value=''>Select an exact artifact</option>
              {artifacts?.map((artifact) => (
                <option key={artifact.id} value={artifact.id}>
                  {artifact.exact_version}
                  {' '}
                  ·
                  {' '}
                  {artifact.variant}
                  {' '}
                  ·
                  {' '}
                  {artifact.arch}
                  {' '}
                  ·
                  {' '}
                  {artifact.id}
                </option>
              ))}
            </select>
            <span>{artifacts?.length === 0 ? `No verified artifact is installed for ${exactVersion}.` : 'Multiple artifacts are never guessed.'}</span>
          </div>
          {capability === 'compatible_structured'
            ? (
                <label className='check-field'>
                  <input checked={allowCompatible} onChange={(event) => setAllowCompatible(event.target.checked)} type='checkbox' />
                  <span>
                    <strong>Accept compatible projection and reverse mapping</strong>
                    <small>This manifest was not authored as native for the exact version.</small>
                  </span>
                </label>
              )
            : null}
          <div className='inline-actions'>
            <button className='button button--primary' disabled={!structuredSupported || selectedCore === '' || busyAction !== '' || (capability === 'compatible_structured' && !allowCompatible)} onClick={() => void renderStructured()} type='button'>{busyAction === 'render' ? 'Rendering…' : 'Render structured candidate'}</button>
            <button className='button button--secondary' disabled={selectedCore === '' || busyAction !== ''} onClick={() => setManualOpenOverride(!manualOpen)} type='button'>{manualOpen ? 'Close manual editor' : 'Use manual JSON'}</button>
          </div>
          {!structuredSupported ? <small className='unsupported-copy'>Structured render is unavailable for this exact version. Manual JSON remains available and does not replace canonical intent.</small> : null}
        </div>

        <div className='startup-control-card'>
          <p className='eyebrow'>Activation target</p>
          <h3>Checked startup candidate</h3>
          <div className='field-group'>
            <label htmlFor='startup-candidate'>Candidate</label>
            <select id='startup-candidate' onChange={(event) => setSelectedCandidate(event.target.value)} value={selectedCandidate}>
              <option value=''>Select a ready candidate</option>
              {candidates.map((candidate) => (
                <option key={candidate.id} value={candidate.id}>
                  {candidate.kind}
                  {' '}
                  ·
                  {' '}
                  {candidate.state}
                  {' '}
                  ·
                  {' '}
                  {candidate.id}
                </option>
              ))}
            </select>
            <span>Only “ready” can apply. A stale candidate remains reviewable for reattach.</span>
          </div>
          <div className='field-group'>
            <label htmlFor='monitoring-tier'>Monitoring tier</label>
            <select id='monitoring-tier' onChange={(event) => setMonitoringTier(event.target.value as MonitoringTier)} value={monitoringTier}><option value='process_only'>Process only</option></select>
            <span>
              A live metrics collector is not configured, so stronger tiers are unavailable rather than simulated.
            </span>
          </div>
          <button className='button button--warning button--wide' disabled={busyAction !== '' || candidates.find((item) => item.id === selectedCandidate)?.state !== 'ready'} onClick={() => void apply()} type='button'>{busyAction === 'apply' ? 'Queuing apply…' : 'Apply ready candidate'}</button>
        </div>
      </div>

      {manualOpen
        ? (
            <div className='manual-byte-editor'>
              <div className='section-heading'>
                <div>
                  <p className='eyebrow'>Exact byte ownership</p>
                  <h3>Manual JSON / JSONC</h3>
                </div>
                <span>UTF-8 · comments preserved</span>
              </div>
              <label htmlFor='manual-json'>Startup bytes</label>
              <textarea id='manual-json' onChange={(event) => {
                setManualRaw(event.target.value);
                setManualPreviewState(null);
              }} rows={16} spellCheck={false} value={manualRaw} />
              <small>
                Configuration can contain credentials. It is sent in the request body, never argv,
                and this editor does not persist a browser draft.
              </small>
              <div className='inline-actions'>
                <button className='button button--secondary' disabled={busyAction !== '' || baseRevision === null || selectedCore === ''} onClick={() => void previewManual()} type='button'>{busyAction === 'manual-preview' ? 'Generating preview…' : 'Preview reverse mapping'}</button>
                <button className='button button--primary' disabled={busyAction !== '' || manualPreview === null} onClick={() => void replaceManual()} type='button'>{busyAction === 'manual' ? 'Saving and queuing check…' : 'Save exact bytes'}</button>
                <span className='separation-copy'>Preview and save do not apply.</span>
              </div>
              {manualPreview === null
                ? null
                : (
                    <div className='reattach-panel'>
                      <div className='section-heading'>
                        <div>
                          <p className='eyebrow'>Owned / residual</p>
                          <h3>Manual reverse preview</h3>
                        </div>
                        <span>{manualPreview.reverse.available ? 'Exact pin proven' : 'Manual-only fallback'}</span>
                      </div>
                      {manualPreview.reverse.residual_paths.length === 0
                        ? <p className='reattach-note'>Every discovered path is owned and losslessly reversible by the exact pinned capability.</p>
                        : (
                            <div className='notice notice--error'>
                              <strong>Residual paths remain manual-owned</strong>
                              <p>{manualPreview.reverse.residual_paths.join(', ')}</p>
                            </div>
                          )}
                      <details>
                        <summary>Review reversible canonical partial</summary>
                        <pre>{JSON.stringify(manualPreview.reverse.owned_partial, null, 2)}</pre>
                      </details>
                      <details open>
                        <summary>Review proposed canonical document</summary>
                        <pre>{JSON.stringify(manualPreview.reverse.proposed_canonical, null, 2)}</pre>
                      </details>
                      <small>{manualPreview.reverse.canonical_changed ? 'Saving will create a new canonical revision before queuing the exact-byte check.' : 'Canonical intent will remain unchanged; saving still creates an immutable manual candidate.'}</small>
                    </div>
                  )}
            </div>
          )
        : null}

      {candidates.length === 0
        ? (
            <div className='empty-state'>
              <strong>No startup candidates for this exact version.</strong>
              <p>
                Render a structured candidate or save exact manual bytes.
                A real binary check must finish before apply.
              </p>
            </div>
          )
        : (
            <div className='candidate-list'>
              <div className='section-heading'>
                <div>
                  <p className='eyebrow'>Immutable candidates</p>
                  <h3>Render history</h3>
                </div>
                <span>
                  {candidates.length}
                  {' '}
                  loaded
                </span>
              </div>
              <div className='entity-table-wrap'>
                <table className='data-table'>
                  <thead>
                    <tr>
                      <th>Candidate</th>
                      <th>Kind</th>
                      <th>Check state</th>
                      <th>Actions</th>
                    </tr>
                  </thead>
                  <tbody>
                    {candidates.map((candidate) => {
                      const manual = manualArtifacts?.find((item) => item.id === candidate.id);
                      return (
                        <tr key={candidate.id}>
                          <td>
                            <code>{candidate.id}</code>
                            <small className='table-subline'>
                              {candidate.sha256.slice(0, 12)}
                              … ·
                              {' '}
                              {formatTimestamp(candidate.createdAt)}
                            </small>
                          </td>
                          <td>{candidate.kind}</td>
                          <td>
                            <span className={`state-label ${stateClass(candidate.state)}`}>
                              <span aria-hidden='true' />
                              {candidate.state}
                            </span>
                          </td>
                          <td>
                            <div className='table-actions'>
                              {manual
                                ? (
                                    <button className='text-button' disabled={busyAction !== ''} onClick={() => {
                                      setManualRaw(manual.raw);
                                      setSelectedCore(manual.core_artifact_id);
                                      setManualOpenOverride(true);
                                      setMessage(`Loaded an editable copy of ${manual.id}; saving creates a new immutable candidate.`);
                                    }} type='button'>
                                      Edit copy
                                    </button>
                                  )
                                : null}
                              {manual?.state === 'stale' ? <button className='text-button' disabled={busyAction !== ''} onClick={() => void previewReattach(manual)} type='button'>Reattach</button> : null}
                              {manual ? <button className='text-button text-button--danger' disabled={busyAction !== ''} onClick={() => void discard(manual)} type='button'>Discard</button> : null}
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
              <small className='unsupported-copy'>Candidate metadata is durable across browser sessions; configuration bytes remain behind explicit secret-bearing read boundaries.</small>
            </div>
          )}

      {reattach === null
        ? null
        : (
            <div className='reattach-panel'>
              <div className='section-heading'>
                <div>
                  <p className='eyebrow'>Base / current / manual</p>
                  <h3>Three-way reattach</h3>
                </div>
                <span>
                  {reattach.conflicts.length}
                  {' '}
                  conflicts
                </span>
              </div>
              {reattach.residual_paths.length === 0
                ? <p className='reattach-note'>All discovered manual paths are owned by the pinned exact-version capability.</p>
                : (
                    <div className='notice notice--error'>
                      <strong>Residual manual paths stay manual</strong>
                      <p>{reattach.residual_paths.join(', ')}</p>
                    </div>
                  )}
              {reattach.conflicts.map((conflict) => (
                <fieldset className='conflict-card' key={conflict.path}>
                  <legend>{conflict.path}</legend>
                  <div className='conflict-values'>
                    <span>
                      <small>Base</small>
                      <code>{displayValue(conflict.base)}</code>
                    </span>
                    <label>
                      <input checked={decisions[conflict.path] === 'current'} name={`decision:${conflict.path}`} onChange={() => setDecisions({ ...decisions, [conflict.path]: 'current' })} type='radio' />
                      <span>
                        <small>Current</small>
                        <code>{displayValue(conflict.current)}</code>
                      </span>
                    </label>
                    <label>
                      <input checked={decisions[conflict.path] === 'manual'} name={`decision:${conflict.path}`} onChange={() => setDecisions({ ...decisions, [conflict.path]: 'manual' })} type='radio' />
                      <span>
                        <small>Manual</small>
                        <code>{displayValue(conflict.manual)}</code>
                      </span>
                    </label>
                  </div>
                </fieldset>
              ))}
              <details>
                <summary>Review automatically merged canonical document</summary>
                <pre>{JSON.stringify(reattach.merged, null, 2)}</pre>
              </details>
              <div className='inline-actions'>
                <button className='button button--primary' disabled={busyAction !== ''} onClick={() => void applyReattach()} type='button'>{busyAction === 'reattach-apply' ? 'Reattaching…' : 'Create reattached revision'}</button>
                <button className='button button--secondary' disabled={busyAction !== ''} onClick={() => setReattach(null)} type='button'>Cancel</button>
                <small>
                  This creates a new revision and checked candidate; it never mutates the old manual artifact.
                </small>
              </div>
            </div>
          )}
    </section>
  );
}
