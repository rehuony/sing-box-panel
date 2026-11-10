import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Badge } from '@shared/components/ui/badge';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { ExternalLink, RefreshCw, Search, Upload } from 'lucide-react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

import type { CoreArtifact } from '@/api/api-client';

import { nodePath } from '@/utils/node-path';
import { useHashTab } from '@/hooks/use-hash-tab';
import { ApiRequestError } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { useCurrentNode } from '@/stores/current-node.store';
import { ListPagination } from '@/components/list-pagination';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { WorkspaceToolbar, WorkspaceToolbarActions } from '@/components/workspace-toolbar';

import { CoreImportDialog } from './core-import-dialog';
import { useVersionLibrary } from './use-version-library';

export function CoresPage() {
  const { t } = useTranslation();
  const node = useCurrentNode();
  const navigate = useNavigate();
  const client = useApiClient();
  const library = useVersionLibrary();
  const [tab, setTab] = useHashTab('cores-', ['installed', 'catalog'] as const, 'installed');
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
  if (pagination.tab !== tab || pagination.search !== search || pagination.size !== size || pagination.page !== page) {
    setPagination({ tab, search, size, page });
  }
  const start = (page - 1) * size;
  const arch = library.platform?.arch;
  const canImport = library.platform?.os === 'linux' && (arch === 'arm64' || arch === 'amd64');
  async function run(
    key: string,
    action: (signal: AbortSignal) => Promise<unknown>,
  ): Promise<boolean> {
    if (pending || !lifecycleRef.current) return false;
    const { signal } = lifecycleRef.current;
    setPending({ key, status: 'running' });
    try {
      await action(signal);
      if (signal.aborted) return false;
      await library.refreshInstalled();
      toast.add({ title: t('cores.library.completed'), type: 'success' });
      return true;
    } catch (reason) {
      if (!signal.aborted) {
        toast.add({
          title: describeRequestError(reason),
          type: 'error',
          ...(reason instanceof ApiRequestError && reason.code === 'configuration_not_saved'
            ? {
                actionProps: {
                  children: t('cores.action.configure'),
                  onClick: () => void navigate(nodePath(node.id, 'configuration')),
                },
              }
            : {}),
        });
      }
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
        await library.refreshInstalled();
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
  const listLoading = tab === 'installed' ? library.installedLoading : library.catalogLoading;
  const refreshLabel = t(tab === 'installed' ? 'cores.refreshInstalled' : 'cores.refreshCatalog');
  return (
    <div className='flex min-h-0 min-w-0 flex-1 flex-col'>
      <h1 className='sr-only'>{t('cores.title')}</h1>
      <Tabs
        className='flex min-h-0 flex-1 flex-col gap-[var(--workspace-content-gap)]'
        render={<section aria-label={t('cores.title')} />}
        value={tab}
        onValueChange={(value) => setTab(value as typeof tab)}
      >
        <WorkspaceToolbar>
          <TabsList aria-label={t('cores.title')}>
            <TabsTrigger value='installed'>
              {t('cores.tabs.installed')}
            </TabsTrigger>
            <TabsTrigger value='catalog'>
              {t('cores.tabs.catalog')}
            </TabsTrigger>
          </TabsList>
          <WorkspaceToolbarActions className='gap-1 sm:gap-2'>
            <Button
              variant='ghost'
              size='icon-sm'
              aria-label={refreshLabel}
              title={refreshLabel}
              disabled={Boolean(pending) || listLoading}
              onClick={() => void (tab === 'installed'
                ? library.refreshInstalled()
                : library.refreshCatalog(true))}
            >
              <RefreshCw className={listLoading ? 'animate-spin motion-reduce:animate-none' : undefined} />
            </Button>
            <Button
              variant='ghost'
              size='icon-sm'
              aria-label={t('cores.import.title')}
              title={t('cores.import.title')}
              disabled={Boolean(pending) || !canImport}
              onClick={() => setImportOpen(true)}
            >
              <Upload />
            </Button>
            <div className='relative min-w-0 flex-1 sm:w-60 sm:flex-none'>
              <Search aria-hidden='true' className='pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground' />
              <Input
                aria-label={t('cores.filter.search')}
                className='h-(--control-height-sm) min-h-(--control-height-sm) pl-8'
                placeholder={t('cores.filter.search')}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </div>
          </WorkspaceToolbarActions>
        </WorkspaceToolbar>
        {library.error != null && (
          <ErrorNotice error={library.error} title={t('cores.error.installed')} />
        )}
        {tab === 'catalog' && library.catalogError != null && (
          <ErrorNotice error={library.catalogError} title={t('cores.error.catalog')} />
        )}
        <TabsContent
          className='flex min-h-80 flex-1 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'
          value={tab}
          aria-busy={listLoading}
        >
          <ScrollArea className='flex-1' viewportClassName='flex flex-col'>
            <table className='workspace-table w-full min-w-160 table-fixed border-collapse [&_td]:px-4 [&_td]:wrap-anywhere [&_th]:sticky [&_th]:top-0 [&_th]:z-1 [&_th]:bg-[var(--color-surface-raised)] [&_th]:px-4 [&_th]:text-left [&_th]:text-muted-foreground [&_tbody_tr]:border-b [&_td:last-child]:text-center [&_th:last-child]:text-center'>
              <thead>
                <tr>
                  <th className='w-1/5'>{t('cores.library.version')}</th>
                  <th className='w-[24%]'>{t('cores.library.source')}</th>
                  {tab === 'catalog' && <th className='w-[38%]'>{t('cores.library.officialLink')}</th>}
                  {tab === 'installed' && <th className='w-[38%]'>{t('cores.library.status')}</th>}
                  <th>{t('cores.library.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {tab === 'installed'
                  ? installed.slice(start, start + size).map((artifact) => {
                      const enabled
                        = library.runtime?.enabled_core?.core_artifact_id === artifact.id;
                      return (
                        <tr key={artifact.id}>
                          <td>
                            <strong className='font-mono text-muted-foreground'>{artifact.exact_version}</strong>
                          </td>
                          <td>
                            {t(artifact.source_kind === 'official' ? 'cores.source.official' : 'cores.source.user_verified')}
                          </td>
                          <td>
                            <span className={enabled ? 'text-[var(--color-success-text)]' : undefined}>
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
                            <div className='flex items-center justify-center gap-2'>
                              <Button
                                size='sm'
                                variant={enabled ? 'destructive' : 'default'}
                                title={runtimeUncertain ? t('cores.library.unknown') : undefined}
                                disabled={
                                  Boolean(pending)
                                  || runtimeUncertain
                                }
                                onClick={() =>
                                  void run(artifact.id, (signal) =>
                                    enabled
                                      ? client.disableCore(artifact.id, signal)
                                      : client.enableCore(artifact.id, signal),
                                  )
                                }
                              >
                                {t(enabled ? 'cores.library.disable' : 'cores.library.enable')}
                              </Button>
                              <Button
                                size='sm'
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
                      const exists = library.artifacts.some((artifact) => artifact.asset_id === asset.asset_id);
                      return (
                        <tr key={asset.asset_id}>
                          <td>
                            <strong className='font-mono text-muted-foreground'>{asset.version}</strong>
                          </td>
                          <td>
                            {t('cores.source.official')}
                          </td>
                          <td>
                            <Badge
                              className='max-w-full'
                              title={asset.name}
                              render={(
                                <a
                                  href={`https://github.com/SagerNet/sing-box/releases/tag/v${encodeURIComponent(asset.version)}`}
                                  target='_blank'
                                  rel='noopener noreferrer'
                                />
                              )}
                            >
                              <span className='truncate'>{asset.name}</span>
                              <ExternalLink aria-hidden='true' data-icon='inline-end' />
                            </Badge>
                          </td>
                          <td>
                            <Button
                              size='sm'
                              variant='default'
                              disabled={exists || Boolean(pending)}
                              onClick={() =>
                                void run(`install:${asset.asset_id}`, (signal) =>
                                  client.installCore(asset.asset_id, signal),
                                )
                              }
                            >
                              {pending?.key === `install:${asset.asset_id}`
                                ? t(`cores.library.${pending.status}`)
                                : t(exists ? 'cores.library.installed' : 'cores.library.download')}
                            </Button>
                          </td>
                        </tr>
                      );
                    })}
              </tbody>
            </table>
            {!count
              ? listLoading
                ? <LoadingState label={t(tab === 'installed' ? 'cores.loadingInstalled' : 'cores.loadingCatalog')} fullScreen={false} />
                : <div className='grid min-h-40 flex-1 place-content-center justify-items-center gap-2.5 px-5 py-6 text-center text-sm text-muted-foreground' role='status'>{t(`cores.empty.${tab}`)}</div>
              : null}
          </ScrollArea>
        </TabsContent>
        <ListPagination
          page={page}
          pages={pages}
          pageSize={size}
          disabled={listLoading || count === 0}
          onPageChange={next => setPagination({ tab, search, size, page: next })}
          onPageSizeChange={setSize}
        />
      </Tabs>
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
            <Button variant='outline' onClick={() => setConfirmation(null)}>
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
