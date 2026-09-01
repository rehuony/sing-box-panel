import type { TFunction } from 'i18next';

import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import type { ConfigurationPreview, CoreArtifact, MonitoringTier, RuntimeHistoryPage, RuntimeStatus, StartupArtifactPage, StartupArtifactSummary, Task } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
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

import { ActivationHistory } from './activation-history';
import { encodeCanonicalValue } from './use-canonical-configuration';

export interface StartupWorkflowProps {
  exactVersion: string;
  mutationsDisabled: boolean;
}

function compact(value: string): string {
  return value.length > 18 ? `${value.slice(0, 15)}…` : value;
}

function formatTimestamp(timestamp: string | undefined, locale: string): string {
  if (timestamp === undefined) return '—';
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' })
    .format(new Date(timestamp));
}

function describeAcceptedTask(task: Task, action: string, t: TFunction): string {
  return t('configuration.deploy.task.accepted', { action, id: task.id });
}

export function StartupWorkflow({ exactVersion, mutationsDisabled }: StartupWorkflowProps) {
  const client = useApiClient();
  const { i18n, t } = useTranslation();
  const [cores, setCores] = useState<CoreArtifact[]>([]);
  const [selectedCore, setSelectedCore] = useState('');
  const [previewResult, setPreviewResult] = useState<{
    coreArtifactID: string;
    value: ConfigurationPreview;
  } | null>(null);
  const [candidates, setCandidates] = useState<StartupArtifactSummary[]>([]);
  const [candidateNext, setCandidateNext] = useState<StartupArtifactPage['next']>();
  const [candidateCoreArtifactID, setCandidateCoreArtifactID] = useState('');
  const [selectedCandidate, setSelectedCandidate] = useState('');
  const [monitoringTier, setMonitoringTier] = useState<MonitoringTier>('process_only');
  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [error, setError] = useState<unknown>(null);
  const [message, setMessage] = useState('');
  const [applyConfirmation, setApplyConfirmation] = useState<{
    monitoringTier: MonitoringTier;
    startupArtifactID: string;
  } | null>(null);
  const [rollbackConfirmationOpen, setRollbackConfirmationOpen] = useState(false);
  const [rollbackEvidence, setRollbackEvidence] = useState<RuntimeStatus | null>(null);
  const [rollbackConfirmation, setRollbackConfirmation] = useState('');
  const [activationHistory, setActivationHistory] = useState<RuntimeHistoryPage | null>(null);
  const [activationHistoryError, setActivationHistoryError] = useState<unknown>(null);
  const [activationHistoryLoading, setActivationHistoryLoading] = useState(true);
  const [loadingOlderCandidates, setLoadingOlderCandidates] = useState(false);
  const candidateRequestRef = useRef(0);
  const activationHistoryRequestRef = useRef(0);
  const loadingOlderCandidatesRef = useRef(false);

  const loadCandidates = useCallback(async (
    coreArtifactID: string,
    signal?: AbortSignal,
    cursor?: NonNullable<StartupArtifactPage['next']>,
    append = false,
  ) => {
    const request = candidateRequestRef.current + 1;
    candidateRequestRef.current = request;
    if (coreArtifactID === '') {
      return;
    }
    const page = await client.listStartupArtifacts({
      beforeID: cursor?.id,
      beforeTime: cursor?.created_at,
      coreArtifactID,
      limit: 100,
    }, signal);
    if (!signal?.aborted && candidateRequestRef.current === request) {
      setCandidateCoreArtifactID(coreArtifactID);
      setCandidates((current) => append ? [...current, ...page.items] : page.items);
      setCandidateNext(page.next);
      if (!append) {
        setSelectedCandidate((current) => page.items.some((item) => item.id === current) ? current : '');
      }
    }
  }, [client]);

  async function loadOlderCandidatesPage() {
    if (candidateNext === undefined || selectedCore === '' || loadingOlderCandidatesRef.current) return;
    loadingOlderCandidatesRef.current = true;
    setLoadingOlderCandidates(true);
    try {
      await loadCandidates(selectedCore, undefined, candidateNext, true);
      setError(null);
    } catch (loadError) {
      setError(loadError);
    } finally {
      loadingOlderCandidatesRef.current = false;
      setLoadingOlderCandidates(false);
    }
  }

  async function refreshCandidates() {
    if (selectedCore === '') return;
    try {
      await loadCandidates(selectedCore);
      setError(null);
    } catch (loadError) {
      setError(loadError);
    }
  }

  const loadActivationHistory = useCallback(async (signal?: AbortSignal) => {
    const request = activationHistoryRequestRef.current + 1;
    activationHistoryRequestRef.current = request;
    setActivationHistoryLoading(true);
    try {
      const page = await client.getRuntimeHistory({ limit: 100 }, signal);
      if (signal?.aborted || activationHistoryRequestRef.current !== request) return;
      setActivationHistory(page);
      setActivationHistoryError(null);
    } catch (loadError) {
      if (signal?.aborted || activationHistoryRequestRef.current !== request) return;
      setActivationHistoryError(loadError);
    } finally {
      if (!signal?.aborted && activationHistoryRequestRef.current === request) {
        setActivationHistoryLoading(false);
      }
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
    const controller = new AbortController();
    void loadActivationHistory(controller.signal);
    return () => controller.abort();
  }, [loadActivationHistory]);

  useEffect(() => {
    if (selectedCore === '') return;
    const controller = new AbortController();
    void Promise.all([
      loadCandidates(selectedCore, controller.signal),
      client.previewConfiguration({ coreArtifactID: selectedCore }, controller.signal),
    ]).then(([, result]) => {
      if (controller.signal.aborted) return;
      setError(null);
      setPreviewResult({ coreArtifactID: selectedCore, value: result });
    }).catch((loadError: unknown) => {
      if (!controller.signal.aborted) setError(loadError);
    });
    return () => controller.abort();
  }, [client, loadCandidates, selectedCore]);

  const preview = previewResult?.coreArtifactID === selectedCore ? previewResult.value : null;
  const support = preview?.support ?? null;
  const readyCandidates = useMemo(
    () => candidateCoreArtifactID === selectedCore
      ? candidates.filter((item) => item.state === 'ready')
      : [],
    [candidateCoreArtifactID, candidates, selectedCore],
  );
  const selectedReadyCandidate = readyCandidates.find((item) => item.id === selectedCandidate) ?? null;
  async function compile() {
    if (mutationsDisabled || selectedCore === '' || preview === null) return;
    setBusyAction('compile');
    setError(null);
    setMessage('');
    try {
      const result = await client.compileConfiguration({
        coreArtifactID: selectedCore,
      });
      setMessage(t('configuration.deploy.compiled', {
        id: result.artifact.id,
        progress: describeAcceptedTask(result.task, t('configuration.deploy.action.validation'), t),
      }));
      await loadCandidates(selectedCore);
      setSelectedCandidate(result.artifact.id);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setBusyAction('');
    }
  }

  async function apply() {
    if (applyConfirmation === null || mutationsDisabled) return;
    setBusyAction('apply');
    setError(null);
    setMessage('');
    try {
      const result = await client.activateStartupArtifact(
        applyConfirmation.startupArtifactID,
        applyConfirmation.monitoringTier,
      );
      setMessage(describeAcceptedTask(
        result.task,
        t('configuration.deploy.action.activation', { id: result.activation.activation_bundle_id }),
        t,
      ));
      setApplyConfirmation(null);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setBusyAction('');
    }
  }

  function changeCore(coreArtifactID: string) {
    setSelectedCore(coreArtifactID);
    setPreviewResult(null);
    setCandidates([]);
    setCandidateNext(undefined);
    setCandidateCoreArtifactID('');
    setSelectedCandidate('');
  }

  async function checkCandidate(candidate: StartupArtifactSummary) {
    if (mutationsDisabled) return;
    setBusyAction(`check:${candidate.id}`);
    setError(null);
    setMessage('');
    try {
      const task = await client.checkStartupArtifact(candidate.id);
      setMessage(describeAcceptedTask(
        task,
        t('configuration.deploy.action.candidateCheck', { id: candidate.id }),
        t,
      ));
    } catch (actionError) {
      setError(actionError);
    } finally {
      setBusyAction('');
    }
  }

  async function prepareRollback() {
    setBusyAction('rollback-evidence');
    setError(null);
    setMessage('');
    try {
      const status = await client.getRuntimeStatus();
      if (status.rollback_bundle_id === undefined) {
        throw new Error(t('configuration.deploy.rollback.unavailable'));
      }
      setRollbackEvidence(status);
      setRollbackConfirmation('');
      setRollbackConfirmationOpen(true);
    } catch (actionError) {
      setError(actionError);
    } finally {
      setBusyAction('');
    }
  }

  async function rollback() {
    const rollbackBundleID = rollbackEvidence?.rollback_bundle_id;
    if (mutationsDisabled || rollbackBundleID === undefined || rollbackConfirmation !== rollbackBundleID) return;
    setBusyAction('rollback');
    setError(null);
    setMessage('');
    try {
      const task = await client.rollbackRuntime(rollbackBundleID);
      setMessage(describeAcceptedTask(
        task,
        t('configuration.deploy.action.rollback', { id: rollbackBundleID }),
        t,
      ));
      setRollbackConfirmationOpen(false);
      setRollbackEvidence(null);
      setRollbackConfirmation('');
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
          <h2 id='startup-workflow-title'>{t('configuration.deploy.title')}</h2>
        </div>
        <button className='button button--secondary' disabled={selectedCore === '' || loading} onClick={() => void refreshCandidates()} type='button'>{t('configuration.deploy.refreshCandidates')}</button>
      </div>

      {error === null ? null : <ErrorNotice error={error} title={t('configuration.deploy.error')} />}
      {message === ''
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>{t('configuration.deploy.accepted')}</strong>
              <p>{message}</p>
              <Link className='text-link' to='/tasks'>{t('configuration.deploy.openTasks')}</Link>
            </div>
          )}

      <div className='form-grid'>
        <div className='field-group'>
          <label htmlFor='startup-core'>{t('configuration.deploy.core.label')}</label>
          <select disabled={loading || cores.length === 0} id='startup-core' onChange={(event) => changeCore(event.target.value)} value={selectedCore}>
            {cores.length === 0
              ? (
                  <option value=''>
                    {t('configuration.deploy.core.noVerified', {
                      version: exactVersion || t('configuration.deploy.core.selectedVersion'),
                    })}
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
          <span className='field-label'>{t('configuration.deploy.schemaCapability.label')}</span>
          <strong>{support?.structured === true ? t('configuration.deploy.schemaCapability.structured') : t('configuration.deploy.schemaCapability.unavailable')}</strong>
          <small>{support?.structured === false ? support.reason : t('configuration.deploy.schemaCapability.matched')}</small>
        </div>
      </div>

      {preview === null
        ? null
        : (
            <div className='projection-preview'>
              <div className='section-heading'>
                <div>
                  <h3>
                    {t('configuration.revision', {
                      sequence: new Intl.NumberFormat(i18n.language).format(preview.canonical_revision.sequence),
                    })}
                  </h3>
                </div>
                <span>{preview.support.exact_version}</span>
              </div>
              <dl className='detail-list' aria-label={t('configuration.deploy.previewEvidence')}>
                <div>
                  <dt>{t('configuration.deploy.evidence.canonicalRevision')}</dt>
                  <dd><code>{preview.canonical_revision.id}</code></dd>
                </div>
                <div>
                  <dt>{t('configuration.deploy.evidence.canonicalDigest')}</dt>
                  <dd><code>{preview.canonical_revision.sha256}</code></dd>
                </div>
                <div>
                  <dt>{t('configuration.deploy.evidence.coreArtifact')}</dt>
                  <dd><code>{preview.core_artifact.id}</code></dd>
                </div>
                <div>
                  <dt>{t('configuration.deploy.evidence.binaryDigest')}</dt>
                  <dd><code>{preview.core_artifact.binary_sha256}</code></dd>
                </div>
                <div>
                  <dt>{t('configuration.deploy.evidence.schemaCapability')}</dt>
                  <dd>{preview.support.structured ? t('configuration.deploy.schemaCapability.structured') : t('configuration.deploy.schemaCapability.unavailable')}</dd>
                </div>
              </dl>
              <pre className='configuration-entity-json'>{encodeCanonicalValue(preview.config, 2)}</pre>
              <button className='button button--primary' disabled={mutationsDisabled || busyAction !== ''} onClick={() => void compile()} type='button'>
                {busyAction === 'compile' ? t('configuration.deploy.compiling') : t('configuration.deploy.compile')}
              </button>
            </div>
          )}

      <div className='form-grid'>
        <div className='field-group'>
          <label htmlFor='startup-candidate'>{t('configuration.deploy.candidate.label')}</label>
          <select id='startup-candidate' onChange={(event) => setSelectedCandidate(event.target.value)} value={selectedReadyCandidate?.id ?? ''}>
            <option value=''>{t('configuration.deploy.candidate.select')}</option>
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
          <label htmlFor='monitoring-tier'>{t('configuration.deploy.monitoring.label')}</label>
          <select id='monitoring-tier' onChange={(event) => setMonitoringTier(event.target.value as MonitoringTier)} value={monitoringTier}>
            <option value='process_only'>{t('configuration.deploy.monitoring.processOnly')}</option>
            <option value='limited'>{t('configuration.deploy.monitoring.limited')}</option>
          </select>
        </div>
        <button
          className='button button--primary'
          disabled={mutationsDisabled || selectedReadyCandidate === null || busyAction !== ''}
          onClick={() => selectedReadyCandidate !== null && setApplyConfirmation({
            monitoringTier,
            startupArtifactID: selectedReadyCandidate.id,
          })}
          type='button'
        >
          {busyAction === 'apply' ? t('configuration.deploy.queueingApply') : t('configuration.deploy.apply')}
        </button>
      </div>

      <section aria-labelledby='startup-artifact-evidence-title'>
        <div className='section-heading'>
          <h3 id='startup-artifact-evidence-title'>{t('configuration.deploy.artifacts.title')}</h3>
          <span>{t('configuration.deploy.artifacts.loaded', { count: candidates.length })}</span>
        </div>
        {candidates.length === 0
          ? <p className='source-note'>{t('configuration.deploy.artifacts.empty')}</p>
          : (
              <div className='revision-history__list'>
                {candidates.map((candidate) => (
                  <article className='revision-card' key={candidate.id}>
                    <div>
                      <strong><code>{candidate.id}</code></strong>
                      <span className={`state-label state-label--${candidate.state === 'ready' ? 'success' : candidate.state === 'failed' ? 'danger' : 'warning'}`}>
                        {t(`configuration.deploy.artifacts.state.${candidate.state}`)}
                      </span>
                      <span>{formatTimestamp(candidate.created_at, i18n.language)}</span>
                    </div>
                    <dl className='detail-list'>
                      <div>
                        <dt>{t('configuration.deploy.evidence.canonicalRevision')}</dt>
                        <dd><code>{candidate.canonical_revision_id}</code></dd>
                      </div>
                      <div>
                        <dt>{t('configuration.deploy.evidence.coreArtifact')}</dt>
                        <dd><code>{candidate.core_artifact_id}</code></dd>
                      </div>
                      <div>
                        <dt>{t('configuration.deploy.evidence.configDigest')}</dt>
                        <dd><code>{candidate.config_sha256}</code></dd>
                      </div>
                      <div>
                        <dt>{t('configuration.deploy.evidence.checkedAt')}</dt>
                        <dd>{formatTimestamp(candidate.checked_at, i18n.language)}</dd>
                      </div>
                    </dl>
                    {candidate.state === 'pending'
                      ? (
                          <button className='button button--secondary' disabled={mutationsDisabled || busyAction !== ''} onClick={() => void checkCandidate(candidate)} type='button'>
                            {busyAction === `check:${candidate.id}` ? t('configuration.deploy.queueing') : t('configuration.deploy.artifacts.check', { id: candidate.id })}
                          </button>
                        )
                      : null}
                  </article>
                ))}
              </div>
            )}
        {candidateNext === undefined
          ? null
          : (
              <Button
                aria-busy={loadingOlderCandidates}
                disabled={loadingOlderCandidates}
                onClick={() => void loadOlderCandidatesPage()}
                type='button'
                variant='outline'
              >
                {loadingOlderCandidates
                  ? t('configuration.deploy.artifacts.loadingOlder')
                  : t('configuration.deploy.artifacts.loadOlder')}
              </Button>
            )}
      </section>

      <ActivationHistory
        error={activationHistoryError}
        loading={activationHistoryLoading}
        onRefresh={loadActivationHistory}
        page={activationHistory}
      />

      <div className='inline-actions' aria-label={t('configuration.deploy.rollback.label')}>
        <button className='button button--secondary' disabled={busyAction !== ''} onClick={() => void prepareRollback()} type='button'>
          {busyAction === 'rollback-evidence' ? t('configuration.deploy.rollback.loading') : t('configuration.deploy.rollback.open')}
        </button>
      </div>

      <AlertDialog
        onOpenChange={(open) => {
          if (!open) setApplyConfirmation(null);
        }}
        open={applyConfirmation !== null}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('configuration.deploy.confirmApply.title')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('configuration.deploy.confirmApply.description', { id: applyConfirmation?.startupArtifactID ?? '' })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction disabled={mutationsDisabled || applyConfirmation === null || busyAction !== ''} onClick={() => void apply()}>
              {t('configuration.deploy.confirmApply.action', { id: applyConfirmation?.startupArtifactID ?? '' })}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        onOpenChange={(open) => {
          setRollbackConfirmationOpen(open);
          if (!open) {
            setRollbackConfirmation('');
            setRollbackEvidence(null);
          }
        }}
        open={rollbackConfirmationOpen}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('configuration.deploy.rollback.title')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('configuration.deploy.rollback.description', { id: rollbackEvidence?.rollback_bundle_id ?? '' })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className='field-group'>
            <label htmlFor='rollback-bundle-confirmation'>{t('configuration.deploy.rollback.confirmLabel')}</label>
            <input
              autoComplete='off'
              id='rollback-bundle-confirmation'
              onChange={(event) => setRollbackConfirmation(event.target.value)}
              spellCheck={false}
              value={rollbackConfirmation}
            />
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction
              disabled={mutationsDisabled || busyAction !== '' || rollbackConfirmation !== rollbackEvidence?.rollback_bundle_id}
              onClick={() => void rollback()}
              variant='destructive'
            >
              {busyAction === 'rollback' ? t('configuration.deploy.queueing') : t('configuration.deploy.rollback.action')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
