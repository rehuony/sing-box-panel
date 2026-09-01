import { Link } from 'react-router-dom';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Download, RefreshCw, UploadCloud } from 'lucide-react';

import type {
  CatalogAsset,
  CoreArtifact,
  StartupArtifactSummary,
} from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { useControlPlane } from '@/stores/control-plane.store';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
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

import type { CoreVersionEntry } from './core-version-rail';

import { CoreVersionRail } from './core-version-rail';
import { CoreArtifactList } from './core-artifact-list';
import { CoreArtifactDetail } from './core-artifact-detail';
import { useCoreLibraryState } from './use-core-library-state';
import { compareExactVersions, formatBytes, isExactVersion } from './core-version.utils';
import './cores-page.css';

type ArtifactAction = 'quarantined' | 'remove' | 'revoked';

interface ArtifactActionCandidate {
  action: ArtifactAction;
  artifact: CoreArtifact;
}

interface AcceptedTask {
  id: string;
  label: string;
}

function supportsTrustedInstall(asset: CatalogAsset) {
  return asset.has_api_digest && asset.has_catalog_digest
    ? asset.api_digest === asset.catalog_digest
    : asset.has_api_digest || asset.has_catalog_digest;
}

export function CoresPage() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const controlPlane = useControlPlane();
  const selectedVersion = controlPlane.viewVersion;
  const setSelectedVersion = controlPlane.setViewVersion;
  const {
    schemaArtifact,
    schemaError,
    schemaSupport,
    artifactError,
    artifacts,
    catalog,
    catalogError,
    closeInspection,
    inspectArtifact,
    inspection,
    inspectionID,
    loadArtifacts,
    loadOlder,
    loadingOlder,
    versionArtifacts,
  } = useCoreLibraryState(selectedVersion);
  const [pending, setPending] = useState<Set<string>>(() => new Set());
  const [message, setMessage] = useState('');
  const [acceptedTask, setAcceptedTask] = useState<AcceptedTask | null>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [filter, setFilter] = useState('');
  const [architecture, setArchitecture] = useState('');
  const [importOpen, setImportOpen] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [importVersion, setImportVersion] = useState('1.13.19');
  const [importDescription, setImportDescription] = useState(() => t('cores.import.defaultDescription'));
  const [importArchitecture, setImportArchitecture] = useState<'' | 'amd64' | 'arm64'>('');
  const [importVariant, setImportVariant] = useState('plain');
  const [acceptedStartupTasks, setAcceptedStartupTasks] = useState<Record<string, string>>({});
  const [actionCandidate, setActionCandidate] = useState<ArtifactActionCandidate | null>(null);

  const versions = useMemo<CoreVersionEntry[]>(() => {
    const entries = new Map<string, CoreVersionEntry>();
    const accept = (version: string) => {
      if (!entries.has(version)) entries.set(version, { catalogCount: 0, installedCount: 0, version });
      return entries.get(version)!;
    };
    if (selectedVersion !== '') accept(selectedVersion);
    for (const artifact of artifacts?.items ?? []) accept(artifact.exact_version).installedCount += 1;
    for (const asset of catalog?.assets ?? []) accept(asset.version).catalogCount += 1;
    return [...entries.values()].sort((left, right) => compareExactVersions(right.version, left.version));
  }, [artifacts, catalog, selectedVersion]);

  const selectedArtifacts = versionArtifacts.filter((artifact) =>
    (architecture === '' || artifact.arch === architecture)
    && `${artifact.arch} ${artifact.variant} ${artifact.id}`.toLowerCase().includes(filter.toLowerCase()));
  const selectedAssets = (catalog?.assets ?? []).filter((asset) =>
    asset.version === selectedVersion
    && (architecture === '' || asset.arch === architecture)
    && `${asset.arch} ${asset.variant} ${asset.name}`.toLowerCase().includes(filter.toLowerCase()));
  function setBusy(key: string, busy: boolean) {
    setPending((current) => {
      const next = new Set(current);
      if (busy) next.add(key);
      else next.delete(key);
      return next;
    });
  }

  function reportTask(taskID: string, label: string) {
    setMessage('');
    setAcceptedTask({ id: taskID, label });
  }

  function beginAction() {
    setAcceptedTask(null);
    setActionError(null);
    setMessage('');
  }

  async function refreshCatalog() {
    setBusy('refresh', true);
    beginAction();
    try {
      const task = await client.refreshCatalog(true);
      reportTask(task.id, t('cores.refresh', { defaultValue: 'Catalog refresh' }));
    } catch (error) {
      setActionError(error);
    } finally {
      setBusy('refresh', false);
    }
  }

  async function install(asset: CatalogAsset) {
    const key = `install:${asset.asset_id}`;
    setBusy(key, true);
    beginAction();
    try {
      const task = await client.installCore(asset.asset_id);
      reportTask(task.id, t('cores.install', { defaultValue: 'Install' }));
    } catch (error) {
      setActionError(error);
    } finally {
      setBusy(key, false);
    }
  }

  async function importArchive() {
    if (importFile === null || importArchitecture === '' || importDescription.trim() === '' || !isExactVersion(importVersion)) {
      setActionError(new Error(t('cores.import.invalid', { defaultValue: 'Choose a valid archive, architecture, exact version and description.' })));
      return;
    }
    setBusy('import', true);
    beginAction();
    try {
      const task = await client.importCoreArchive({
        archive: importFile,
        architecture: importArchitecture,
        exactVersion: importVersion,
        sourceDescription: importDescription.trim(),
        variant: importVariant,
      });
      reportTask(task.id, t('cores.import.title', { defaultValue: 'Import archive' }));
      setSelectedVersion(importVersion);
      setImportOpen(false);
    } catch (error) {
      setActionError(error);
    } finally {
      setBusy('import', false);
    }
  }

  async function checkStartup(startupArtifact: StartupArtifactSummary) {
    const key = `check:${startupArtifact.id}`;
    setBusy(key, true);
    beginAction();
    try {
      const task = await client.checkStartupArtifact(startupArtifact.id);
      setAcceptedStartupTasks((current) => ({ ...current, [startupArtifact.id]: task.id }));
      reportTask(task.id, t('cores.check', { defaultValue: 'Startup check' }));
    } catch (error) {
      setActionError(error);
    } finally {
      setBusy(key, false);
    }
  }

  async function confirmArtifactAction() {
    if (actionCandidate === null) return;
    const { action, artifact } = actionCandidate;
    const key = `${action}:${artifact.id}`;
    setActionCandidate(null);
    setBusy(key, true);
    beginAction();
    try {
      if (action === 'remove') await client.removeCoreArtifact(artifact.id);
      else if (action === 'revoked') await client.revokeCoreArtifact(artifact.id);
      else await client.quarantineCoreArtifact(artifact.id);
      setMessage(t('cores.artifact.changed', { defaultValue: 'Artifact state updated.' }));
      if (inspectionID === artifact.id) closeInspection();
      await loadArtifacts();
    } catch (error) {
      setActionError(error);
    } finally {
      setBusy(key, false);
    }
  }

  return (
    <div className='core-page'>
      <header className='core-page__heading'>
        <div>
          <h1>{t('cores.title', { defaultValue: 'Core Library' })}</h1>
          <p>{t('cores.subtitle', { defaultValue: 'Verified binaries with version-scoped configuration editing.' })}</p>
        </div>
        <Button onClick={() => setImportOpen(true)}>
          <UploadCloud aria-hidden='true' />
          {t('cores.import.title', { defaultValue: 'Import archive' })}
        </Button>
      </header>

      {actionError === null ? null : <ErrorNotice error={actionError} title={t('cores.error.action', { defaultValue: 'Core operation failed' })} />}
      {acceptedTask === null
        ? null
        : (
            <div className='notice notice--success' role='status'>
              <strong>{t('cores.task.title', { defaultValue: 'Task accepted' })}</strong>
              <p>
                {t('cores.task.accepted', {
                  action: acceptedTask.label,
                  defaultValue: '{{action}} accepted as task {{id}}.',
                  id: acceptedTask.id,
                })}
              </p>
              <Link className='text-link' to='/tasks'>{t('cores.openTasks', { defaultValue: 'Open tasks' })}</Link>
            </div>
          )}
      {message === '' ? null : <div className='notice notice--success' role='status'>{message}</div>}

      <div className='core-workspace'>
        <CoreVersionRail
          entries={versions}
          loadingOlder={loadingOlder}
          locale={locale}
          onLoadOlder={() => void loadOlder()}
          onSelect={setSelectedVersion}
          selectedVersion={selectedVersion}
          showLoadOlder={artifacts?.next !== undefined}
        />

        <main className='core-library'>
          <div className='core-library__toolbar'>
            <Input aria-label={t('cores.filter.search', { defaultValue: 'Filter artifacts' })} onChange={(event) => setFilter(event.target.value)} placeholder={t('cores.filter.search', { defaultValue: 'Filter artifacts' })} value={filter} />
            <select aria-label={t('cores.filter.architecture', { defaultValue: 'Architecture' })} onChange={(event) => setArchitecture(event.target.value)} value={architecture}>
              <option value=''>{t('cores.filter.allArchitectures', { defaultValue: 'All architectures' })}</option>
              <option value='amd64'>amd64</option>
              <option value='arm64'>arm64</option>
            </select>
            <span className={`support-pill support-pill--${schemaSupport?.structured ? 'supported' : 'unavailable'}`}>
              {schemaSupport?.structured
                ? t('cores.schema.structured', { defaultValue: 'Structured Schema available' })
                : schemaError !== undefined
                  ? t('cores.schema.error', { defaultValue: 'Schema lookup failed' })
                  : schemaArtifact !== undefined && schemaSupport === undefined
                    ? t('cores.schema.loading', { defaultValue: 'Checking Schema…' })
                    : t('cores.schema.jsonOnly', { defaultValue: 'JSON only' })}
            </span>
          </div>

          <Tabs defaultValue='installed'>
            <TabsList variant='line'>
              <TabsTrigger value='installed'>{t('cores.tabs.installed', { defaultValue: 'Installed' })}</TabsTrigger>
              <TabsTrigger value='catalog'>{t('cores.tabs.catalog', { defaultValue: 'Catalog' })}</TabsTrigger>
            </TabsList>
            <TabsContent value='installed'>
              {artifactError === null
                ? null
                : (
                    <ErrorNotice
                      error={artifactError}
                      title={t('cores.error.installed', { defaultValue: 'Installed artifacts are unavailable' })}
                    />
                  )}
              {artifacts === null
                ? <div className='inline-loading'>{t('cores.loading', { defaultValue: 'Loading artifacts…' })}</div>
                : null}
              {artifacts !== null && selectedArtifacts.length === 0 ? <div className='empty-state'>{t('cores.empty.installed', { defaultValue: 'No installed artifacts match.' })}</div> : null}
              <CoreArtifactList
                artifacts={selectedArtifacts}
                onInspect={(artifact) => void inspectArtifact(artifact)}
              />
            </TabsContent>
            <TabsContent value='catalog'>
              <div className='catalog-actions'>
                <Button disabled={pending.has('refresh')} onClick={() => void refreshCatalog()} variant='outline'>
                  <RefreshCw aria-hidden='true' />
                  {t('cores.refresh', { defaultValue: 'Refresh catalog' })}
                </Button>
              </div>
              {catalogError === null ? null : <ErrorNotice error={catalogError} title={t('cores.error.catalog', { defaultValue: 'Catalog is unavailable' })} />}
              {catalog !== null && selectedAssets.length === 0 ? <div className='empty-state'>{t('cores.empty.catalog', { defaultValue: 'No catalog artifacts match.' })}</div> : null}
              <div className='core-card-list'>
                {selectedAssets.map((asset) => {
                  const trusted = supportsTrustedInstall(asset);
                  return (
                    <article className='core-card' key={asset.asset_id}>
                      <span className={`core-card__state ${trusted ? 'core-card__state--verified' : 'core-card__state--quarantined'}`} aria-hidden='true' />
                      <div>
                        <strong>
                          {asset.arch}
                          {' '}
                          ·
                          {' '}
                          {asset.variant}
                        </strong>
                        <span>{asset.name}</span>
                        <code>{formatBytes(asset.size, locale, t('cores.value.unknownSize'))}</code>
                      </div>
                      <Button disabled={!trusted || pending.size > 0} onClick={() => void install(asset)}>
                        <Download aria-hidden='true' />
                        {pending.has(`install:${asset.asset_id}`) ? t('cores.queueing', { defaultValue: 'Queueing…' }) : t('cores.install', { defaultValue: 'Install' })}
                      </Button>
                    </article>
                  );
                })}
              </div>
            </TabsContent>
          </Tabs>
        </main>
      </div>

      <Dialog open={importOpen} onOpenChange={setImportOpen}>
        <DialogContent className='core-import-dialog'>
          <DialogHeader>
            <DialogTitle>{t('cores.import.title', { defaultValue: 'Import archive' })}</DialogTitle>
            <DialogDescription>{t('cores.import.description', { defaultValue: 'The archive and exact binary version are verified before installation.' })}</DialogDescription>
          </DialogHeader>
          <div className='core-import-form'>
            <label>
              {t('cores.import.archive', { defaultValue: 'Archive' })}
              <Input accept='.tar.gz,.tgz,.zip' onChange={(event) => setImportFile(event.target.files?.[0] ?? null)} type='file' />
            </label>
            <label>
              {t('cores.import.version', { defaultValue: 'Exact version' })}
              <Input onChange={(event) => setImportVersion(event.target.value)} value={importVersion} />
            </label>
            <label>
              {t('cores.import.descriptionLabel', { defaultValue: 'Source description' })}
              <Input onChange={(event) => setImportDescription(event.target.value)} value={importDescription} />
            </label>
            <label>
              {t('cores.filter.architecture', { defaultValue: 'Architecture' })}
              <select onChange={(event) => setImportArchitecture(event.target.value as '' | 'amd64' | 'arm64')} value={importArchitecture}>
                <option value=''>{t('cores.import.selectArchitecture', { defaultValue: 'Select architecture' })}</option>
                <option value='amd64'>amd64</option>
                <option value='arm64'>arm64</option>
              </select>
            </label>
            <label>
              {t('cores.import.variant', { defaultValue: 'Variant' })}
              <Input onChange={(event) => setImportVariant(event.target.value)} value={importVariant} />
            </label>
          </div>
          <DialogFooter><Button disabled={pending.has('import')} onClick={() => void importArchive()}>{t('cores.import.submit', { defaultValue: 'Upload and verify' })}</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <CoreArtifactDetail
        inspection={inspection}
        inspectionID={inspectionID}
        locale={locale}
        onCheckStartup={(startup) => void checkStartup(startup)}
        onClose={closeInspection}
        onRequestAction={(action, artifact) => setActionCandidate({ action, artifact })}
        pending={pending}
        acceptedStartupTasks={acceptedStartupTasks}
      />

      <AlertDialog open={actionCandidate !== null} onOpenChange={(open) => {
        if (!open) setActionCandidate(null);
      }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('cores.confirm.title', { defaultValue: 'Change artifact trust?' })}</AlertDialogTitle>
            <AlertDialogDescription>{t('cores.confirm.description', { defaultValue: 'This changes whether the artifact can be selected for future runtime work.' })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('cores.confirm.cancel', { defaultValue: 'Cancel' })}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void confirmArtifactAction()} variant='destructive'>{t('cores.confirm.action', { defaultValue: 'Confirm' })}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
