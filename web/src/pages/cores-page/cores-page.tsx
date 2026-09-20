import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight, RefreshCw, Search, Upload } from 'lucide-react';

import type { CatalogAsset, CoreArtifact, Task } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { waitForTask } from '@/lib/wait-for-task';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { useControlPlane } from '@/stores/control-plane.store';
import { WorkspaceToolbar } from '@/components/workspace-toolbar';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { CoreImportDialog } from './core-import-dialog';
import { useVersionLibrary } from './use-version-library';
import './cores-page.css';

function hasDownloadChecksum(asset: CatalogAsset) {
  return asset.has_api_digest && asset.has_catalog_digest
    ? asset.api_digest === asset.catalog_digest
    : asset.has_api_digest || asset.has_catalog_digest;
}
export function CoresPage() {
  const { t } = useTranslation();
  const client = useApiClient();
  const control = useControlPlane();
  const library = useVersionLibrary();
  const [tab, setTab] = useState<'installed' | 'catalog'>('installed');
  const [search, setSearch] = useState('');
  const [size, setSize] = useState(10);
  const [pagination, setPagination] = useState({ tab, search, size, page: 1 });
  const [pending, setPending] = useState<{ key: string; status: string } | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [confirmation, setConfirmation] = useState<CoreArtifact | null>(null);
  const lifecycleRef = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    lifecycleRef.current = controller;
    return () => controller.abort();
  }, []);
  const installed = useMemo(
    () =>
      library.artifacts.filter((value) =>
        `${value.exact_version} ${value.variant} ${value.user_source ?? ''} ${value.source_kind}`
          .toLowerCase()
          .includes(search.toLowerCase()),
      ),
    [library.artifacts, search],
  );
  const available = useMemo(
    () =>
      (library.catalog?.assets ?? []).filter((value) =>
        `${value.version} ${value.name} ${value.variant}`
          .toLowerCase()
          .includes(search.toLowerCase()),
      ),
    [library.catalog, search],
  );
  const count = tab === 'installed' ? installed.length : available.length;
  const pages = Math.max(1, Math.ceil(count / size));
  const page = Math.min(
    pagination.tab === tab && pagination.search === search && pagination.size === size
      ? pagination.page
      : 1,
    pages,
  );
  const start = (page - 1) * size;
  const arch = library.platform?.arch;
  const canImport = library.platform?.os === 'linux' && (arch === 'arm64' || arch === 'amd64');
  async function run(
    key: string,
    action: (signal: AbortSignal) => Promise<Task>,
  ): Promise<boolean> {
    if (pending || !lifecycleRef.current) return false;
    const { signal } = lifecycleRef.current;
    setPending({ key, status: 'queued' });
    try {
      const task = await action(signal);
      const result = await waitForTask(client, task, signal, (value) =>
        setPending({ key, status: value.status }),
      );
      if (signal.aborted) return false;
      if (result.status !== 'succeeded') {
        throw new Error(
          t('cores.library.operationFailed', { id: result.id, status: result.status }),
        );
      }
      await Promise.all([library.load(), control.refresh(signal)]);
      toast.add({ title: t('cores.library.completed'), type: 'success' });
      return true;
    } catch (reason) {
      if (!signal.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
      return false;
    } finally {
      if (!signal.aborted) setPending(null);
    }
  }
  async function removeArtifact() {
    if (!confirmation || !lifecycleRef.current) return;
    const artifact = confirmation;
    const { signal } = lifecycleRef.current;
    setConfirmation(null);
    setPending({ key: artifact.id, status: 'running' });
    try {
      await client.removeCoreArtifact(artifact.id, signal);
      if (!signal.aborted) {
        await library.load();
        toast.add({ title: t('cores.artifact.changed'), type: 'success' });
      }
    } catch (reason) {
      if (!signal.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!signal.aborted) setPending(null);
    }
  }
  const runtimeUncertain
    = !library.runtime
      || ['stale', 'inspection_unavailable'].includes(library.runtime.observation_state);
  return (
    <div className='core-page panel-page'>
      <header className='core-page__heading panel-page-heading'>
        <h1>{t('cores.title')}</h1>
      </header>
      <section className='core-library' aria-label={t('cores.title')}>
        <WorkspaceToolbar>
          <div className='core-tabs' role='tablist' aria-label={t('cores.title')}>
            <Button
              role='tab'
              aria-selected={tab === 'installed'}
              variant={tab === 'installed' ? 'secondary' : 'ghost'}
              onClick={() => setTab('installed')}
            >
              {t('cores.tabs.installed')}
            </Button>
            <Button
              role='tab'
              aria-selected={tab === 'catalog'}
              variant={tab === 'catalog' ? 'secondary' : 'ghost'}
              onClick={() => setTab('catalog')}
            >
              {t('cores.tabs.catalog')}
            </Button>
          </div>
          <div className='core-library__toolbar workspace-toolbar__actions'>
            <label className='core-search'>
              <Search aria-hidden='true' />
              <input
                aria-label={t('cores.filter.search')}
                placeholder={t('cores.filter.search')}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </label>
            <Button
              variant='ghost'
              size='icon'
              aria-label={t('cores.refresh')}
              disabled={Boolean(pending)}
              onClick={() => void run('refresh', (signal) => client.refreshCatalog(true, signal))}
            >
              <RefreshCw />
            </Button>
            <Button
              variant='ghost'
              size='icon'
              aria-label={t('cores.import.title')}
              disabled={Boolean(pending) || !canImport}
              onClick={() => setImportOpen(true)}
            >
              <Upload />
            </Button>
          </div>
        </WorkspaceToolbar>
        {library.error != null && (
          <ErrorNotice error={library.error} title={t('cores.error.installed')} />
        )}
        {tab === 'catalog' && library.catalogError != null && (
          <ErrorNotice error={library.catalogError} title={t('cores.error.catalog')} />
        )}
        <div
          className='core-list-scroll'
          role='tabpanel'
          aria-label={t(`cores.tabs.${tab}`)}
          aria-busy={library.loading}
        >
          <table className='core-table'>
            <thead>
              <tr>
                <th>{t('cores.library.version')}</th>
                <th>{t('cores.library.source')}</th>
                <th>{t('cores.library.status')}</th>
                <th>{t('cores.library.actions')}</th>
              </tr>
            </thead>
            <tbody>
              {tab === 'installed'
                ? installed.slice(start, start + size).map((artifact) => {
                    const enabled
                      = library.runtime?.observation_state === 'running'
                        && library.runtime.running?.core_artifact_id === artifact.id;
                    return (
                      <tr key={artifact.id}>
                        <td>
                          <strong>{artifact.exact_version}</strong>
                        </td>
                        <td>
                          {t(artifact.source_kind === 'official' ? 'cores.source.official' : 'cores.source.user_verified')}
                        </td>
                        <td>
                          <span className={`core-status ${enabled ? 'is-running' : ''}`}>
                            {pending?.key === artifact.id
                              ? t(`cores.library.${pending.status}`)
                              : enabled
                                ? t('cores.library.enabled')
                                : runtimeUncertain
                                  ? t('cores.library.unknown')
                                  : t('cores.library.disabled')}
                          </span>
                        </td>
                        <td>
                          <div className='core-row-actions'>
                            <Button
                              variant={enabled ? 'destructive' : 'default'}
                              title={runtimeUncertain ? t('cores.library.unknown') : undefined}
                              disabled={
                                Boolean(pending)
                                || runtimeUncertain
                              }
                              onClick={() =>
                                void run(artifact.id, (signal) =>
                                  enabled
                                    ? client.stopRuntime(signal)
                                    : client.enableCore(artifact.id, signal),
                                )
                              }
                            >
                              {t(enabled ? 'cores.library.disable' : 'cores.library.enable')}
                            </Button>
                            <Button
                              variant='destructive'
                              disabled={Boolean(pending) || enabled || runtimeUncertain}
                              onClick={() => setConfirmation(artifact)}
                            >
                              {t('cores.action.remove')}
                            </Button>
                          </div>
                        </td>
                      </tr>
                    );
                  })
                : available.slice(start, start + size).map((asset) => {
                    const downloaded = library.artifacts.find((artifact) => artifact.asset_id === asset.asset_id);
                    const exists = Boolean(downloaded);
                    const enabled = library.runtime?.observation_state === 'running'
                      && downloaded?.id === library.runtime.running?.core_artifact_id;
                    return (
                      <tr key={asset.asset_id}>
                        <td>
                          <strong>{asset.version}</strong>
                        </td>
                        <td>
                          {t('cores.source.official')}

                        </td>
                        <td>
                          {pending?.key === `install:${asset.asset_id}`
                            ? t(`cores.library.${pending.status}`)
                            : !downloaded
                                ? t('cores.library.notInstalled')
                                : enabled
                                  ? t('cores.library.enabled')
                                  : runtimeUncertain
                                    ? t('cores.library.unknown')
                                    : t('cores.library.disabled')}
                        </td>
                        <td>
                          <Button
                            variant='default'
                            title={!hasDownloadChecksum(asset) ? t('cores.installUnavailable') : undefined}
                            disabled={Boolean(pending) || !hasDownloadChecksum(asset) || exists}
                            onClick={() =>
                              void run(`install:${asset.asset_id}`, (signal) =>
                                client.installCore(asset.asset_id, signal),
                              )
                            }
                          >
                            {t('cores.library.download')}
                          </Button>
                        </td>
                      </tr>
                    );
                  })}
            </tbody>
          </table>
          {!count && (
            <p className='core-empty'>
              {library.loading ? t('cores.loading') : t(`cores.empty.${tab}`)}
            </p>
          )}
        </div>
        <footer className='core-pagination'>
          <SelectField
            aria-label={t('cores.library.pageSize')}
            value={size}
            onValueChange={(value) => {
              setSize(value);
            }}
            items={[5, 10, 50].map((value) => ({ value, label: t('cores.library.perPage', { count: value }) }))}
          />
          <div>
            <Button
              variant='ghost'
              size='icon'
              aria-label={t('cores.library.previous')}
              disabled={page === 1}
              onClick={() => setPagination({ tab, search, size, page: page - 1 })}
            >
              <ChevronLeft />
            </Button>
            <span aria-current='page'>{page}</span>
            <Button
              variant='ghost'
              size='icon'
              aria-label={t('cores.library.next')}
              disabled={page >= pages}
              onClick={() => setPagination({ tab, search, size, page: page + 1 })}
            >
              <ChevronRight />
            </Button>
          </div>
        </footer>
      </section>
      {importOpen && canImport && (
        <CoreImportDialog
          architecture={arch}
          onClose={() => setImportOpen(false)}
          onImport={(input) => run('import', (signal) => client.importCoreArchive(input, signal))}
        />
      )}
      <Dialog open={Boolean(confirmation)} onOpenChange={(open) => !open && setConfirmation(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('cores.confirm.title')}</DialogTitle>
            <DialogDescription>{t('cores.confirm.description')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant='secondary' onClick={() => setConfirmation(null)}>
              {t('cores.confirm.cancel')}
            </Button>
            <Button variant='destructive' onClick={() => void removeArtifact()}>
              {t('cores.confirm.action')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
