import { Link } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useState } from 'react';

import type { ConfigurationAdapterSupport, ConfigurationPreview, CoreArtifact, MonitoringTier, StartupArtifactSummary } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';

export interface StartupWorkflowProps {
  exactVersion: string;
}

function compact(value: string): string {
  return value.length > 18 ? `${value.slice(0, 15)}…` : value;
}

export function StartupWorkflow({ exactVersion }: StartupWorkflowProps) {
  const client = useApiClient();
  const [cores, setCores] = useState<CoreArtifact[]>([]);
  const [selectedCore, setSelectedCore] = useState('');
  const [supportResult, setSupportResult] = useState<{
    coreArtifactID: string;
    value: ConfigurationAdapterSupport;
  } | null>(null);
  const [previewResult, setPreviewResult] = useState<{
    coreArtifactID: string;
    value: ConfigurationPreview;
  } | null>(null);
  const [candidates, setCandidates] = useState<StartupArtifactSummary[]>([]);
  const [candidateCoreArtifactID, setCandidateCoreArtifactID] = useState('');
  const [selectedCandidate, setSelectedCandidate] = useState('');
  const [acceptedIgnoredDigest, setAcceptedIgnoredDigest] = useState('');
  const [monitoringTier, setMonitoringTier] = useState<MonitoringTier>('process_only');
  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [message, setMessage] = useState('');

  const loadCandidates = useCallback(async (coreArtifactID: string, signal?: AbortSignal) => {
    setCandidates([]);
    setCandidateCoreArtifactID('');
    setSelectedCandidate('');
    if (coreArtifactID === '') {
      return;
    }
    const page = await client.listStartupArtifacts({ coreArtifactID, limit: 100 }, signal);
    if (!signal?.aborted) {
      setCandidateCoreArtifactID(coreArtifactID);
      setCandidates(page.items);
    }
  }, [client]);

  useEffect(() => {
    const controller = new AbortController();
    void client.listCoreArtifacts({ exactVersion, verificationState: 'verified', limit: 200 }, controller.signal)
      .then((page) => {
        if (controller.signal.aborted) return;
        setCores(page.items);
        setSelectedCore((current) => page.items.some((item) => item.id === current) ? current : page.items[0]?.id ?? '');
      })
      .catch((loadError: unknown) => {
        if (!controller.signal.aborted) setError(loadError);
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [client, exactVersion]);

  useEffect(() => {
    if (selectedCore === '') return;
    const controller = new AbortController();
    void Promise.all([
      client.getConfigurationSupport(selectedCore, controller.signal),
      loadCandidates(selectedCore, controller.signal),
    ]).then(([resolved]) => {
      if (controller.signal.aborted) return;
      setError(null);
      setSupportResult({ coreArtifactID: selectedCore, value: resolved });
      if (!resolved.supported) return;
      return client.previewConfiguration({ coreArtifactID: selectedCore }, controller.signal)
        .then((result) => {
          if (!controller.signal.aborted) {
            setPreviewResult({ coreArtifactID: selectedCore, value: result });
          }
        });
    }).catch((loadError: unknown) => {
      if (!controller.signal.aborted) setError(loadError);
    });
    return () => controller.abort();
  }, [client, loadCandidates, selectedCore]);

  const support = supportResult?.coreArtifactID === selectedCore ? supportResult.value : null;
  const preview = previewResult?.coreArtifactID === selectedCore ? previewResult.value : null;
  const readyCandidates = useMemo(
    () => candidateCoreArtifactID === selectedCore
      ? candidates.filter((item) => item.state === 'ready')
      : [],
    [candidateCoreArtifactID, candidates, selectedCore],
  );
  const selectedReadyCandidate = readyCandidates.find((item) => item.id === selectedCandidate) ?? null;
  const hasIgnored = preview?.diagnostics.some((item) => item.class === 'ignored') === true;
  const ignoredAccepted = preview?.ignored_digest !== undefined
    && acceptedIgnoredDigest === preview.ignored_digest;

  async function compile() {
    if (selectedCore === '' || preview === null) return;
    setBusyAction('compile');
    setError(null);
    setMessage('');
    try {
      const result = await client.compileConfiguration({
        coreArtifactID: selectedCore,
        acceptedIgnoredDigest: hasIgnored && ignoredAccepted ? preview.ignored_digest : undefined,
      });
      setMessage(`Compiled ${result.artifact.id}; validation is queued as ${result.task.id}.`);
      await loadCandidates(selectedCore);
      setSelectedCandidate(result.artifact.id);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setBusyAction('');
    }
  }

  async function apply() {
    if (selectedReadyCandidate === null) return;
    setBusyAction('apply');
    setError(null);
    setMessage('');
    try {
      const result = await client.activateStartupArtifact(selectedReadyCandidate.id, monitoringTier);
      setMessage(`Activation ${result.activation.activation_bundle_id} is queued as ${result.task.id}.`);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setBusyAction('');
    }
  }

  function changeCore(coreArtifactID: string) {
    setSelectedCore(coreArtifactID);
    setSupportResult(null);
    setPreviewResult(null);
    setCandidates([]);
    setCandidateCoreArtifactID('');
    setSelectedCandidate('');
    setAcceptedIgnoredDigest('');
  }

  async function lifecycle(operation: 'start' | 'stop' | 'restart' | 'rollback') {
    setBusyAction(operation);
    setError(null);
    setMessage('');
    try {
      const task = operation === 'start'
        ? await client.startRuntime()
        : operation === 'stop'
          ? await client.stopRuntime()
          : operation === 'restart'
            ? await client.restartRuntime()
            : await client.rollbackRuntime();
      setMessage(`${operation} is queued as ${task.id}.`);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setBusyAction('');
    }
  }

  return (
    <section className='startup-workflow' aria-labelledby='startup-workflow-title'>
      <div className='section-heading'>
        <div>
          <p className='eyebrow'>Exact adapter / immutable startup bytes</p>
          <h2 id='startup-workflow-title'>Preview, compile, check and apply</h2>
        </div>
        <button className='button button--secondary' disabled={selectedCore === '' || loading} onClick={() => void loadCandidates(selectedCore)} type='button'>Refresh candidates</button>
      </div>

      {error === null ? null : <ErrorNotice error={error} title='Configuration workflow failed' />}
      {message === ''
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>Operation accepted</strong>
              <p>{message}</p>
              <Link className='text-link' to='/tasks'>Open task queue</Link>
            </div>
          )}

      <div className='form-grid'>
        <div className='field-group'>
          <label htmlFor='startup-core'>Verified core artifact</label>
          <select disabled={loading || cores.length === 0} id='startup-core' onChange={(event) => changeCore(event.target.value)} value={selectedCore}>
            {cores.length === 0
              ? (
                  <option value=''>
                    No verified
                    {exactVersion || 'selected-version'}
                    {' '}
                    artifact
                  </option>
                )
              : null}
            {cores.map((core) => (
              <option key={core.id} value={core.id}>
                {core.exact_version}
                {' '}
                ·
                {' '}
                {core.arch}
                {' '}
                ·
                {' '}
                {compact(core.binary_sha256)}
              </option>
            ))}
          </select>
        </div>
        <div className='field-group'>
          <span className='field-label'>Adapter</span>
          <strong>{support?.supported === true ? `${support.adapter_id}@${support.adapter_revision}` : 'Unavailable'}</strong>
          <small>{support?.supported === false ? support.reason : 'Support is matched against the complete installed binary profile.'}</small>
        </div>
      </div>

      {preview === null
        ? null
        : (
            <div className='projection-preview'>
              <div className='section-heading'>
                <div>
                  <p className='eyebrow'>Projection preview</p>
                  <h3>
                    Revision #
                    {preview.canonical_revision.sequence}
                  </h3>
                </div>
                <span>
                  {preview.diagnostics.length}
                  {' '}
                  diagnostics
                </span>
              </div>
              {preview.diagnostics.length === 0
                ? <p className='source-note'>Every configured field is accepted without a projection diagnostic.</p>
                : (
                    <ul className='diagnostic-list'>
                      {preview.diagnostics.map((diagnostic) => (
                        <li key={`${diagnostic.path}:${diagnostic.code}`}>
                          <span className={`state-label state-label--${diagnostic.class === 'ignored' ? 'warning' : 'success'}`}>{diagnostic.class}</span>
                          <code>{diagnostic.path}</code>
                          <strong>{diagnostic.code}</strong>
                          <p>{diagnostic.message}</p>
                        </li>
                      ))}
                    </ul>
                  )}
              {hasIgnored
                ? (
                    <label className='checkbox-field' htmlFor='accept-ignored-fields'>
                      <input
                        checked={ignoredAccepted}
                        id='accept-ignored-fields'
                        onChange={(event) => setAcceptedIgnoredDigest(
                          event.target.checked ? preview.ignored_digest ?? '' : '',
                        )}
                        type='checkbox'
                      />
                      <span>
                        I understand these fields remain in global history but are ignored by
                        {exactVersion}
                        .
                      </span>
                    </label>
                  )
                : null}
              <button className='button button--primary' disabled={busyAction !== '' || (hasIgnored && !ignoredAccepted)} onClick={() => void compile()} type='button'>
                {busyAction === 'compile' ? 'Compiling…' : 'Compile and queue check'}
              </button>
            </div>
          )}

      <div className='form-grid'>
        <div className='field-group'>
          <label htmlFor='startup-candidate'>Ready startup candidate</label>
          <select id='startup-candidate' onChange={(event) => setSelectedCandidate(event.target.value)} value={selectedReadyCandidate?.id ?? ''}>
            <option value=''>Select a checked candidate</option>
            {readyCandidates.map((candidate) => (
              <option key={candidate.id} value={candidate.id}>
                {candidate.id}
                {' '}
                ·
                {' '}
                {compact(candidate.config_sha256)}
              </option>
            ))}
          </select>
        </div>
        <div className='field-group'>
          <label htmlFor='monitoring-tier'>Monitoring tier</label>
          <select id='monitoring-tier' onChange={(event) => setMonitoringTier(event.target.value as MonitoringTier)} value={monitoringTier}>
            <option value='process_only'>Process only</option>
            <option value='limited'>Limited Clash API metrics</option>
          </select>
        </div>
        <button className='button button--primary' disabled={selectedReadyCandidate === null || busyAction !== ''} onClick={() => void apply()} type='button'>
          {busyAction === 'apply' ? 'Queueing apply…' : 'Apply ready candidate'}
        </button>
      </div>

      <div className='inline-actions' aria-label='Runtime lifecycle'>
        {(['start', 'stop', 'restart', 'rollback'] as const).map((operation) => (
          <button className='button button--secondary' disabled={busyAction !== ''} key={operation} onClick={() => void lifecycle(operation)} type='button'>
            {busyAction === operation ? 'Queueing…' : operation[0].toUpperCase() + operation.slice(1)}
          </button>
        ))}
      </div>
    </section>
  );
}
