import { Link } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useState } from 'react';

import type {
  CatalogAsset,
  CatalogAssetList,
  ConfigurationAdapterSupport,
  CoreArtifact,
  CoreArtifactCursor,
  CoreArtifactPage,
} from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { PageHeading } from '@/components/page-heading';
import { useControlPlane } from '@/stores/control-plane.store';

import {
  compareExactVersions,
  formatBytes,
  isExactVersion,
} from './core-version.utils';
import './cores-page.css';

interface VersionEntry {
  version: string;
  catalogCount: number;
  installedCount: number;
}

type VerificationRestriction = Extract<
  CoreArtifact['verification_state'],
  'quarantined' | 'revoked'
>;

function supportsTrustedInstall(asset: CatalogAsset): boolean {
  if (asset.has_api_digest && asset.has_catalog_digest) {
    return asset.api_digest === asset.catalog_digest;
  }
  return asset.has_api_digest || asset.has_catalog_digest;
}

function compactDigest(digest: string): string {
  return digest.length > 18 ? `${digest.slice(0, 15)}…` : digest;
}

export function CoresPage() {
  const client = useApiClient();
  const controlPlane = useControlPlane();
  const selectedVersion = controlPlane.viewVersion;
  const setSelectedVersion = controlPlane.setViewVersion;
  const [catalog, setCatalog] = useState<CatalogAssetList | null>(null);
  const [catalogError, setCatalogError] = useState<unknown>(null);
  const [artifacts, setArtifacts] = useState<CoreArtifactPage | null>(null);
  const [artifactError, setArtifactError] = useState<unknown>(null);
  const [adapterResult, setAdapterResult] = useState<{
    artifactID: string;
    support?: ConfigurationAdapterSupport;
    error?: unknown;
  } | null>(null);
  const [pendingAction, setPendingAction] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState('');
  const [actionError, setActionError] = useState<unknown>(null);
  const [removeCandidate, setRemoveCandidate] = useState<string | null>(null);
  const [verificationCandidate, setVerificationCandidate] = useState<{
    artifactID: string;
    state: VerificationRestriction;
  } | null>(null);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importVersion, setImportVersion] = useState('1.13.19');
  const [importDescription, setImportDescription] = useState('browser upload');

  const loadCatalog = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setCatalogError(null);
        setCatalog(await client.listCatalogAssets({}, signal));
      } catch (error) {
        if (
          signal?.aborted
          || (error instanceof DOMException && error.name === 'AbortError')
        ) {
          return;
        }
        setCatalogError(error);
      }
    },
    [client],
  );

  const loadArtifacts = useCallback(
    async (
      signal?: AbortSignal,
      cursor?: CoreArtifactCursor,
      append = false,
    ) => {
      try {
        setArtifactError(null);
        const page = await client.listCoreArtifacts(
          {
            beforeID: cursor?.id,
            beforeTime: cursor?.created_at,
            limit: 200,
          },
          signal,
        );
        setArtifacts((current) =>
          append && current !== null
            ? { items: [...current.items, ...page.items], next: page.next }
            : page,
        );
      } catch (error) {
        if (
          signal?.aborted
          || (error instanceof DOMException && error.name === 'AbortError')
        ) {
          return;
        }
        setArtifactError(error);
      }
    },
    [client],
  );

  useEffect(() => {
    const controller = new AbortController();
    void Promise.all([
      loadCatalog(controller.signal),
      loadArtifacts(controller.signal),
    ]);
    return () => controller.abort();
  }, [loadArtifacts, loadCatalog]);

  useEffect(() => {
    const artifact = artifacts?.items.find((item) =>
      item.exact_version === selectedVersion && item.verification_state === 'verified');
    if (artifact === undefined) return;
    const controller = new AbortController();
    void client.getConfigurationSupport(artifact.id, controller.signal)
      .then((support) => {
        if (!controller.signal.aborted) setAdapterResult({ artifactID: artifact.id, support });
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setAdapterResult({ artifactID: artifact.id, error });
      });
    return () => controller.abort();
  }, [artifacts, client, selectedVersion]);

  const versions = useMemo<VersionEntry[]>(() => {
    const counts = new Map<string, VersionEntry>();
    function accept(version: string) {
      if (!counts.has(version)) {
        counts.set(version, { version, catalogCount: 0, installedCount: 0 });
      }
      return counts.get(version)!;
    }
    if (selectedVersion !== '') {
      accept(selectedVersion);
    }
    if (
      controlPlane.status === 'ready'
      && controlPlane.context.running !== null
      && isExactVersion(controlPlane.context.running.exactVersion)
    ) {
      accept(controlPlane.context.running.exactVersion);
    }
    for (const artifact of artifacts?.items ?? []) {
      accept(artifact.exact_version).installedCount += 1;
    }
    for (const asset of catalog?.assets ?? []) {
      accept(asset.version).catalogCount += 1;
    }
    return [...counts.values()].sort((left, right) =>
      compareExactVersions(right.version, left.version),
    );
  }, [artifacts, catalog, controlPlane, selectedVersion]);

  const selectedAssets = (catalog?.assets ?? []).filter(
    (asset) => asset.version === selectedVersion,
  );
  const selectedArtifacts = (artifacts?.items ?? []).filter(
    (artifact) => artifact.exact_version === selectedVersion,
  );
  const adapterArtifact = selectedArtifacts.find((item) => item.verification_state === 'verified');
  const activeAdapterResult = adapterResult?.artifactID === adapterArtifact?.id ? adapterResult : null;
  const adapterSupport = activeAdapterResult?.support ?? null;
  const adapterError = activeAdapterResult?.error ?? null;
  const runningVersion
    = controlPlane.status === 'ready'
      ? controlPlane.context.running?.exactVersion
      : undefined;
  const selectedLabel = selectedVersion === '' ? 'No version selected' : selectedVersion;

  async function refreshCatalog() {
    setPendingAction('refresh');
    setActionError(null);
    setActionMessage('');
    try {
      const task = await client.refreshCatalog(true);
      setActionMessage(
        `Catalog refresh queued as ${task.id}. The rail updates after the task succeeds.`,
      );
    } catch (error) {
      setActionError(error);
    } finally {
      setPendingAction(null);
    }
  }

  async function install(asset: CatalogAsset) {
    const action = `install:${asset.asset_id}`;
    setPendingAction(action);
    setActionError(null);
    setActionMessage('');
    try {
      const task = await client.installCore(asset.asset_id);
      setActionMessage(
        `Verified install queued as ${task.id}. Track checksum and version checks in Tasks.`,
      );
    } catch (error) {
      setActionError(error);
    } finally {
      setPendingAction(null);
    }
  }

  async function removeArtifact(artifact: CoreArtifact) {
    setPendingAction(`remove:${artifact.id}`);
    setActionError(null);
    setActionMessage('');
    try {
      await client.removeCoreArtifact(artifact.id);
      setActionMessage(`Removed ${artifact.exact_version} artifact metadata.`);
      setRemoveCandidate(null);
      await loadArtifacts();
    } catch (error) {
      setActionError(error);
    } finally {
      setPendingAction(null);
    }
  }

  async function restrictArtifact(
    artifact: CoreArtifact,
    verificationState: VerificationRestriction,
  ) {
    const action = `${verificationState}:${artifact.id}`;
    setPendingAction(action);
    setActionError(null);
    setActionMessage('');
    try {
      if (verificationState === 'revoked') {
        await client.revokeCoreArtifact(artifact.id);
      } else {
        await client.quarantineCoreArtifact(artifact.id);
      }
      setActionMessage(
        `${artifact.exact_version} is ${verificationState}. A currently running child was not stopped; future checks, activations, starts and rollbacks cannot use these bytes.`,
      );
      setVerificationCandidate(null);
      await loadArtifacts();
    } catch (error) {
      setActionError(error);
    } finally {
      setPendingAction(null);
    }
  }

  async function loadOlderArtifacts() {
    if (artifacts?.next === undefined) return;
    setPendingAction('load-older-artifacts');
    try {
      await loadArtifacts(undefined, artifacts.next, true);
    } finally {
      setPendingAction(null);
    }
  }

  async function importArchive() {
    if (importFile === null || !isExactVersion(importVersion) || importDescription.trim() === '') {
      setActionError(new Error('Choose an archive and provide an exact stable version and description.'));
      return;
    }
    if (importFile.size < 1 || importFile.size > 128 * 1024 * 1024) {
      setActionError(new Error('The core archive must be between 1 byte and 128 MiB.'));
      return;
    }
    setPendingAction('import');
    setActionError(null);
    try {
      const task = await client.importCoreArchive({
        archive: importFile,
        sourceDescription: importDescription.trim(),
        exactVersion: importVersion,
        architecture: 'arm64',
        variant: 'plain',
      });
      setActionMessage(`Private core upload queued as ${task.id}.`);
      setSelectedVersion(importVersion);
    } catch (error) {
      setActionError(error);
    } finally {
      setPendingAction(null);
    }
  }

  return (
    <div className='page-stack core-page'>
      <PageHeading
        action={(
          <div className='version-picker'>
            <label htmlFor='selected-core-version'>Selected exact version</label>
            <select
              id='selected-core-version'
              onChange={(event) => setSelectedVersion(event.target.value)}
              value={selectedVersion}
            >
              <option disabled value=''>Select exact version</option>
              {versions.map((entry) => (
                <option key={entry.version} value={entry.version}>
                  {entry.version}
                </option>
              ))}
            </select>
          </div>
        )}
        eyebrow='Cores / version track'
        summary='Running identity, installed bytes and upstream catalog entries stay distinct. Every operation names an exact stable version.'
        title='Follow the version rail.'
      />

      {actionError === null
        ? null
        : (
            <ErrorNotice error={actionError} title='Core operation was not queued' />
          )}
      {actionMessage === ''
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>Durable task accepted</strong>
              <p>{actionMessage}</p>
              <Link className='text-link' to='/tasks'>Open task queue</Link>
            </div>
          )}

      <section className='adapter-strip' aria-labelledby='core-upload-title'>
        <div>
          <p className='eyebrow'>Private local import</p>
          <h2 id='core-upload-title'>Upload a verified Linux arm64 archive</h2>
          <small>
            The browser computes SHA-256; the server stages at most 128 MiB in a
            mode-0700 private directory and verifies the exact binary version.
          </small>
        </div>
        <div className='form-grid'>
          <div className='field-group'>
            <label htmlFor='core-import-file'>Archive</label>
            <input accept='.tar.gz,.tgz,.zip' id='core-import-file' onChange={(event) => setImportFile(event.target.files?.[0] ?? null)} type='file' />
          </div>
          <div className='field-group'>
            <label htmlFor='core-import-version'>Exact version</label>
            <input id='core-import-version' onChange={(event) => setImportVersion(event.target.value)} value={importVersion} />
          </div>
          <div className='field-group'>
            <label htmlFor='core-import-description'>Source description</label>
            <input id='core-import-description' onChange={(event) => setImportDescription(event.target.value)} value={importDescription} />
          </div>
          <button className='button button--primary' disabled={pendingAction !== null} onClick={() => void importArchive()} type='button'>
            {pendingAction === 'import' ? 'Hashing and queueing…' : 'Upload core archive'}
          </button>
        </div>
      </section>

      <div className='core-layout'>
        <aside className='version-rail-panel' aria-labelledby='version-rail-title'>
          <div className='section-heading'>
            <div>
              <p className='eyebrow'>Exact identities</p>
              <h2 id='version-rail-title'>Version rail</h2>
            </div>
            <span>
              {versions.length}
              {' '}
              versions
            </span>
          </div>
          {versions.length === 0
            ? (
                <div className='empty-state empty-state--compact'>
                  <strong>No versions discovered.</strong>
                  <p>Refresh the official catalog to begin.</p>
                </div>
              )
            : (
                <ol className='version-rail'>
                  {versions.map((entry) => {
                    const isRunning = entry.version === runningVersion;
                    const isSelected = entry.version === selectedVersion;
                    return (
                      <li
                        className={`${isRunning ? 'is-running' : ''} ${
                          isSelected ? 'is-selected' : ''
                        }`}
                        key={entry.version}
                      >
                        <button
                          aria-current={isSelected ? 'true' : undefined}
                          onClick={() => setSelectedVersion(entry.version)}
                          type='button'
                        >
                          <span className='version-rail__node' aria-hidden='true' />
                          <strong>{entry.version}</strong>
                          <span className='version-rail__counts'>
                            {entry.installedCount > 0
                              ? `${entry.installedCount} installed`
                              : 'not installed'}
                            {' · '}
                            {entry.catalogCount > 0
                              ? `${entry.catalogCount} catalog`
                              : 'catalog absent'}
                          </span>
                        </button>
                        <div className='version-rail__tags'>
                          {isRunning ? <span className='rail-tag rail-tag--running'>running</span> : null}
                          {isSelected ? <span className='rail-tag rail-tag--selected'>selected</span> : null}
                        </div>
                      </li>
                    );
                  })}
                </ol>
              )}
          {artifacts?.next === undefined
            ? null
            : (
                <button
                  className='button button--secondary'
                  disabled={pendingAction === 'load-older-artifacts'}
                  onClick={() => void loadOlderArtifacts()}
                  type='button'
                >
                  {pendingAction === 'load-older-artifacts'
                    ? 'Loading older artifacts…'
                    : 'Load older installed artifacts'}
                </button>
              )}
        </aside>

        <div className='core-detail-stack'>
          <section className='adapter-strip' aria-labelledby='adapter-title'>
            <div>
              <p className='eyebrow'>Compiled exact-version adapter</p>
              <h2 id='adapter-title'>{selectedLabel}</h2>
            </div>
            {adapterError === null
              ? (
                  <div className='adapter-strip__state'>
                    <span className={`support-pill support-pill--${adapterSupport?.supported === true ? 'supported' : 'unavailable'}`}>
                      <span aria-hidden='true' className='support-pill__mark' />
                      {adapterSupport?.supported === true
                        ? `${adapterSupport.adapter_id}@${adapterSupport.adapter_revision}`
                        : 'Adapter unavailable'}
                    </span>
                    <small>
                      {selectedVersion === ''
                        ? 'Select one exact catalog or installed version on the rail.'
                        : !selectedArtifacts.some((item) => item.verification_state === 'verified')
                            ? 'Install a verified artifact before resolving its complete binary profile.'
                            : adapterSupport === null
                              ? 'Resolving the exact binary profile…'
                              : adapterSupport.supported
                                ? 'Projection is compiled into this panel build and matches the full binary profile.'
                                : adapterSupport.reason ?? 'This binary may be installed and inspected, but cannot be compiled or run.'}
                    </small>
                  </div>
                )
              : (
                  <ErrorNotice error={adapterError} title='Adapter support could not be resolved' />
                )}
          </section>

          <section className='core-section' aria-labelledby='installed-title'>
            <div className='section-heading'>
              <div>
                <p className='eyebrow'>Immutable local bytes</p>
                <h2 id='installed-title'>
                  Installed for
                  {selectedLabel}
                </h2>
              </div>
              <span>
                {selectedArtifacts.length}
                {' '}
                artifacts
              </span>
            </div>
            {artifactError === null
              ? null
              : (
                  <ErrorNotice error={artifactError} title='Installed artifacts are unavailable' />
                )}
            {artifacts === null && artifactError === null
              ? (
                  <div className='inline-loading' aria-busy='true'>Loading installed bytes…</div>
                )
              : null}
            {artifacts !== null && selectedArtifacts.length === 0
              ? (
                  <div className='empty-state'>
                    <strong>{selectedVersion === '' ? 'Select a version to inspect local artifacts.' : `${selectedVersion} is not installed.`}</strong>
                    <p>Choose a trusted catalog artifact below. Installation runs as a durable maintenance task.</p>
                  </div>
                )
              : null}
            {selectedArtifacts.length > 0
              ? (
                  <div className='artifact-list'>
                    {selectedArtifacts.map((artifact) => (
                      <article key={artifact.id}>
                        <div>
                          <strong>
                            {artifact.arch}
                            {' '}
                            ·
                            {' '}
                            {artifact.variant}
                          </strong>
                          <span>
                            {artifact.source_kind.replace('_', ' ')}
                            {' '}
                            ·
                            {' '}
                            {artifact.verification_state}
                          </span>
                          <code>{compactDigest(artifact.archive_sha256)}</code>
                        </div>
                        {verificationCandidate?.artifactID === artifact.id
                          ? (
                              <div className='inline-actions'>
                                <button
                                  className={verificationCandidate.state === 'revoked' ? 'button button--danger' : 'button button--secondary'}
                                  disabled={pendingAction === `${verificationCandidate.state}:${artifact.id}`}
                                  onClick={() => void restrictArtifact(artifact, verificationCandidate.state)}
                                  type='button'
                                >
                                  {pendingAction === `${verificationCandidate.state}:${artifact.id}`
                                    ? 'Saving…'
                                    : `Confirm ${verificationCandidate.state}`}
                                </button>
                                <button className='text-button' onClick={() => setVerificationCandidate(null)} type='button'>
                                  Keep current trust
                                </button>
                              </div>
                            )
                          : removeCandidate === artifact.id
                            ? (
                                <div className='inline-actions'>
                                  <button
                                    className='button button--danger'
                                    disabled={pendingAction === `remove:${artifact.id}`}
                                    onClick={() => void removeArtifact(artifact)}
                                    type='button'
                                  >
                                    {pendingAction === `remove:${artifact.id}` ? 'Removing…' : 'Confirm remove'}
                                  </button>
                                  <button className='text-button' onClick={() => setRemoveCandidate(null)} type='button'>
                                    Keep
                                  </button>
                                </div>
                              )
                            : (
                                <div className='inline-actions'>
                                  {artifact.verification_state === 'verified'
                                    ? (
                                        <button
                                          className='text-button'
                                          onClick={() => setVerificationCandidate({ artifactID: artifact.id, state: 'quarantined' })}
                                          type='button'
                                        >
                                          Quarantine
                                        </button>
                                      )
                                    : null}
                                  {artifact.verification_state !== 'revoked'
                                    ? (
                                        <button
                                          className='text-button text-button--danger'
                                          onClick={() => setVerificationCandidate({ artifactID: artifact.id, state: 'revoked' })}
                                          type='button'
                                        >
                                          Revoke
                                        </button>
                                      )
                                    : null}
                                  <button
                                    className='text-button text-button--danger'
                                    onClick={() => setRemoveCandidate(artifact.id)}
                                    type='button'
                                  >
                                    Remove
                                  </button>
                                </div>
                              )}
                      </article>
                    ))}
                  </div>
                )
              : null}
          </section>

          <section className='core-section' aria-labelledby='catalog-title'>
            <div className='section-heading'>
              <div>
                <p className='eyebrow'>Official stable releases</p>
                <h2 id='catalog-title'>
                  Catalog for
                  {selectedLabel}
                </h2>
              </div>
              <button
                className='button button--secondary'
                disabled={pendingAction === 'refresh'}
                onClick={() => void refreshCatalog()}
                type='button'
              >
                {pendingAction === 'refresh' ? 'Queueing…' : 'Refresh catalog'}
              </button>
            </div>
            {catalogError === null
              ? null
              : (
                  <ErrorNotice error={catalogError} title='Catalog is not available yet' />
                )}
            {catalog === null && catalogError === null
              ? (
                  <div className='inline-loading' aria-busy='true'>Loading release catalog…</div>
                )
              : null}
            {catalog !== null && selectedAssets.length === 0
              ? (
                  <div className='empty-state'>
                    <strong>{selectedVersion === '' ? 'Select a version on the rail.' : `No catalog artifact for ${selectedVersion}.`}</strong>
                    <p>Refresh the official stable catalog, or import a verified binary with the CLI.</p>
                  </div>
                )
              : null}
            {selectedAssets.length > 0
              ? (
                  <div className='entity-table-wrap'>
                    <table className='data-table catalog-table'>
                      <thead>
                        <tr>
                          <th>Platform</th>
                          <th>Archive</th>
                          <th>Evidence</th>
                          <th><span className='visually-hidden'>Actions</span></th>
                        </tr>
                      </thead>
                      <tbody>
                        {selectedAssets.map((asset) => {
                          const installable = supportsTrustedInstall(asset);
                          return (
                            <tr key={asset.asset_id}>
                              <td>
                                <strong>{asset.arch}</strong>
                                <br />
                                <span>{asset.variant}</span>
                              </td>
                              <td>
                                <code>{asset.name}</code>
                                <br />
                                <span>{formatBytes(asset.size)}</span>
                              </td>
                              <td>
                                <span className={`state-label ${installable ? 'state-label--success' : 'state-label--warning'}`}>
                                  <span aria-hidden='true' />
                                  {installable ? 'Trusted digest' : 'Digest missing'}
                                </span>
                              </td>
                              <td>
                                <button
                                  className='button button--small button--primary'
                                  disabled={!installable || pendingAction !== null}
                                  onClick={() => void install(asset)}
                                  title={installable ? undefined : 'Installation requires trusted SHA-256 evidence.'}
                                  type='button'
                                >
                                  {pendingAction === `install:${asset.asset_id}` ? 'Queueing…' : 'Install'}
                                </button>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                )
              : null}
            {catalog?.refreshed_at
              ? (
                  <p className='source-note'>
                    Snapshot refreshed
                    {' '}
                    {new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(catalog.refreshed_at))}
                  </p>
                )
              : null}
          </section>

          <section className='runtime-contract-note'>
            <div>
              <p className='eyebrow'>Runtime boundary</p>
              <strong>Installing does not activate a core.</strong>
              <p>
                Create and check immutable startup bytes,
                then apply the ready candidate from the configuration workflow.
              </p>
            </div>
            <Link className='button button--secondary' to='/configuration'>Open startup workflow</Link>
          </section>
        </div>
      </div>
    </div>
  );
}
