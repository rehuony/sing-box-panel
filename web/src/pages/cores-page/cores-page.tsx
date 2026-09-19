import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight, RefreshCw, Search, Upload } from 'lucide-react';

import type { CatalogAsset, CoreArtifact, Task } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { waitForTask } from '@/lib/wait-for-task';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { useControlPlane } from '@/stores/control-plane.store';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
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

function trusted(asset: CatalogAsset) {
  return asset.has_api_digest && asset.has_catalog_digest
    ? asset.api_digest === asset.catalog_digest
    : asset.has_api_digest || asset.has_catalog_digest;
}
function capabilities(artifact: CoreArtifact): string[] {
  const fingerprint = artifact.feature_fingerprint;
  if (!fingerprint || typeof fingerprint !== 'object' || Array.isArray(fingerprint)) return [];
  const values = (fingerprint as { features?: unknown }).features;
  return Array.isArray(values)
    ? values.filter((value): value is string => typeof value === 'string')
    : [];
}
type TrustAction = 'quarantined' | 'revoked' | 'remove';
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
  const [confirmation, setConfirmation] = useState<{
    action: TrustAction;
    artifact: CoreArtifact;
  } | null>(null);
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
  async function changeTrust() {
    if (!confirmation || !lifecycleRef.current) return;
    const { action, artifact } = confirmation;
    const { signal } = lifecycleRef.current;
    setConfirmation(null);
    setPending({ key: artifact.id, status: 'running' });
    try {
      if (action === 'remove') await client.removeCoreArtifact(artifact.id, signal);
      else if (action === 'revoked') await client.revokeCoreArtifact(artifact.id, signal);
      else await client.quarantineCoreArtifact(artifact.id, signal);
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
    <div className='core-page'>
      <header className='core-page__heading'>
        <h1>{t('cores.title')}</h1>
      </header>
      <section className='core-library' aria-label={t('cores.title')}>
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
        <div className='core-library__toolbar'>
          <label className='core-search'>
            <Search aria-hidden='true' />
            <input
              aria-label={t('cores.filter.search')}
              placeholder={t('cores.filter.search')}
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </label>
          <Tooltip>
            <TooltipTrigger className='core-platform' render={<span tabIndex={0} role='note' />}>
              <span>
                {library.platform
                  ? `${library.platform.os} / ${library.platform.arch.toUpperCase()}`
                  : t('cores.library.platformUnknown')}
              </span>
            </TooltipTrigger>
            <TooltipContent>{t('cores.library.platformHelp')}</TooltipContent>
          </Tooltip>
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
                <th>{t('cores.library.sourceCapabilities')}</th>
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
                          <span className='core-variant'>{artifact.variant}</span>
                        </td>
                        <td>
                          <span>
                            {artifact.source_kind === 'official'
                              ? t('cores.source.official')
                              : artifact.user_source || t('cores.source.user_verified')}
                          </span>
                          <div className='core-capabilities'>
                            {capabilities(artifact).map((value) => (
                              <span key={value}>{value.replace(/^with_/, '')}</span>
                            ))}
                          </div>
                          <details className='core-evidence'>
                            <summary>{t('cores.details')}</summary>
                            <dl>
                              <dt>{t('cores.detail.installed')}</dt>
                              <dd>{new Date(artifact.created_at).toLocaleString()}</dd>
                              <dt>{t('cores.detail.archiveSha')}</dt>
                              <dd>{artifact.archive_sha256}</dd>
                              <dt>{t('cores.detail.binarySha')}</dt>
                              <dd>{artifact.binary_sha256}</dd>
                            </dl>
                            <div className='core-trust-actions'>
                              {(['quarantined', 'revoked', 'remove'] as const).map((action) => (
                                <Button
                                  key={action}
                                  variant='ghost'
                                  disabled={Boolean(pending) || enabled}
                                  onClick={() => setConfirmation({ action, artifact })}
                                >
                                  {t(
                                    `cores.action.${action === 'quarantined' ? 'quarantine' : action === 'revoked' ? 'revoke' : 'remove'}`,
                                  )}
                                </Button>
                              ))}
                            </div>
                          </details>
                        </td>
                        <td>
                          <span className={`core-status ${enabled ? 'is-running' : ''}`}>
                            {pending?.key === artifact.id
                              ? t(`cores.library.${pending.status}`)
                              : enabled
                                ? t('cores.library.enabled')
                                : t(`cores.state.${artifact.verification_state}`)}
                          </span>
                        </td>
                        <td>
                          <Button
                            variant='ghost'
                            disabled={
                              Boolean(pending)
                              || runtimeUncertain
                              || artifact.verification_state !== 'verified'
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
                        </td>
                      </tr>
                    );
                  })
                : available.slice(start, start + size).map((asset) => {
                    const exists = library.artifacts.some(
                      (artifact) =>
                        artifact.asset_id === asset.asset_id
                        && artifact.verification_state === 'verified',
                    );
                    return (
                      <tr key={asset.asset_id}>
                        <td>
                          <strong>{asset.version}</strong>
                          <span className='core-variant'>{asset.variant}</span>
                        </td>
                        <td>
                          {t('cores.source.official')}
                          <span className='core-asset-size'>
                            {(asset.size / (1024 * 1024)).toFixed(1)}
                            {' '}
                            MB
                          </span>
                        </td>
                        <td>
                          {pending?.key === `install:${asset.asset_id}`
                            ? t(`cores.library.${pending.status}`)
                            : exists
                              ? t('cores.tabs.installed')
                              : trusted(asset)
                                ? t('cores.state.verified')
                                : t('cores.startup.blocked')}
                        </td>
                        <td>
                          <Button
                            variant='ghost'
                            disabled={Boolean(pending) || !trusted(asset) || exists}
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
          <select
            aria-label={t('cores.library.pageSize')}
            value={size}
            onChange={(event) => setSize(Number(event.target.value))}
          >
            {[5, 10, 50].map((value) => (
              <option key={value} value={value}>
                {t('cores.library.perPage', { count: value })}
              </option>
            ))}
          </select>
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
            <Button variant='secondary' onClick={() => void changeTrust()}>
              {t('cores.confirm.action')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
