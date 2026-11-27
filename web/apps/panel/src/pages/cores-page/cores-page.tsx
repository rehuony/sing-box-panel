import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { Badge } from '@shared/components/ui/badge';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { ExternalLink, Package, PackageSearch, RefreshCw, SearchX, Upload } from 'lucide-react';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@shared/components/ui/table';
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
import { RegionError, RegionState } from '@/components/region-state';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { CardToolbar, StripAction, StripDivider, StripSearch } from '@/components/title-strip';

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
  const listError = tab === 'installed' ? library.error : library.catalogError;
  const refreshLabel = t(tab === 'installed' ? 'cores.refreshInstalled' : 'cores.refreshCatalog');
  const retry = () => void (tab === 'installed' ? library.refreshInstalled() : library.refreshCatalog(false));
  const total = tab === 'installed' ? library.artifacts.length : library.catalog?.assets.length ?? 0;
  return (
    <div className='flex min-h-0 min-w-0 flex-1 flex-col'>
      <h1 className='sr-only'>{t('cores.title')}</h1>
      <Tabs
        className='flex min-h-80 flex-1 flex-col gap-0 overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'
        render={<section aria-label={t('cores.title')} />}
        value={tab}
        onValueChange={(value) => setTab(value as typeof tab)}
      >
        <CardToolbar>
          <TabsList aria-label={t('cores.title')} className='flex-none' variant='pill'>
            <TabsTrigger value='installed'>
              {t('cores.tabs.installed')}
            </TabsTrigger>
            <TabsTrigger value='catalog'>
              {t('cores.tabs.catalog')}
            </TabsTrigger>
          </TabsList>
          <StripDivider className='max-sm:hidden' />
          <StripSearch className='max-sm:order-last max-sm:basis-full' label={t('cores.filter.search')} onValueChange={setSearch} value={search} />
          <StripDivider className='max-sm:hidden' />
          <div className='flex items-center gap-0.5 max-sm:ms-auto'>
            <StripAction
              disabled={Boolean(pending) || listLoading}
              icon={<RefreshCw aria-hidden='true' className={listLoading ? 'animate-spin motion-reduce:animate-none' : undefined} data-icon='inline-start' />}
              label={refreshLabel}
              onClick={() => void (tab === 'installed'
                ? library.refreshInstalled()
                : library.refreshCatalog(true))}
            />
            <StripAction
              disabled={Boolean(pending) || !canImport}
              icon={<Upload aria-hidden='true' data-icon='inline-start' />}
              label={t('cores.import.title')}
              onClick={() => setImportOpen(true)}
            />
          </div>
        </CardToolbar>
        {listError != null && count > 0 && (
          <ErrorNotice error={listError} title={t(tab === 'installed' ? 'cores.error.installed' : 'cores.error.catalog')} />
        )}
        <TabsContent
          className='flex min-h-0 flex-1 flex-col'
          value={tab}
          aria-busy={listLoading}
        >
          <ScrollArea className='flex-1' viewportClassName='flex flex-col'>
            <Table className='min-w-160' density='comfortable'>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-1/5'>{t('cores.library.version')}</TableHead>
                  <TableHead className='w-[24%]'>{t('cores.library.source')}</TableHead>
                  {tab === 'catalog' && <TableHead className='w-[38%]'>{t('cores.library.officialLink')}</TableHead>}
                  {tab === 'installed' && <TableHead className='w-[38%]'>{t('cores.library.status')}</TableHead>}
                  <TableHead><span className='sr-only'>{t('cores.library.actions')}</span></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tab === 'installed'
                  ? installed.slice(start, start + size).map((artifact) => {
                      const enabled
                        = library.runtime?.enabled_core?.core_artifact_id === artifact.id;
                      return (
                        <TableRow key={artifact.id}>
                          <TableCell kind='wrap'>
                            <span className='font-mono text-muted-foreground'>{artifact.exact_version}</span>
                          </TableCell>
                          <TableCell kind='wrap'>
                            {t(artifact.source_kind === 'official' ? 'cores.source.official' : 'cores.source.user_verified')}
                          </TableCell>
                          <TableCell kind='wrap'>
                            <span className={enabled ? 'text-[var(--color-success-text)]' : undefined}>
                              {pending?.key === artifact.id
                                ? t(`cores.library.${pending.status}`)
                                : enabled
                                  ? t('cores.library.enabled')
                                  : runtimeUncertain
                                    ? t('cores.library.unknown')
                                    : t('cores.library.disabled')}
                            </span>
                          </TableCell>
                          <TableCell className='[&>div]:gap-2' kind='actions'>
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
                          </TableCell>
                        </TableRow>
                      );
                    })
                  : available.slice(start, start + size).map((asset) => {
                      const exists = library.artifacts.some((artifact) => artifact.asset_id === asset.asset_id);
                      return (
                        <TableRow key={asset.asset_id}>
                          <TableCell kind='wrap'>
                            <span className='font-mono text-muted-foreground'>{asset.version}</span>
                          </TableCell>
                          <TableCell kind='wrap'>
                            {t('cores.source.official')}
                          </TableCell>
                          <TableCell kind='wrap'>
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
                          </TableCell>
                          <TableCell kind='actions'>
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
                          </TableCell>
                        </TableRow>
                      );
                    })}
              </TableBody>
            </Table>
            {!count
              ? listLoading
                ? <LoadingState label={t(tab === 'installed' ? 'cores.loadingInstalled' : 'cores.loadingCatalog')} fullScreen={false} />
                : listError != null
                  ? <RegionError error={listError} reload={retry} title={t(tab === 'installed' ? 'cores.error.installed' : 'cores.error.catalog')} />
                  : (
                      <RegionState
                        icon={total > 0 ? SearchX : tab === 'installed' ? Package : PackageSearch}
                        title={t(total > 0 ? 'cores.empty.noMatch' : `cores.empty.${tab}`)}
                      />
                    )
              : null}
          </ScrollArea>
          <ListPagination
            className='border-t px-2 pb-2'
            page={page}
            pages={pages}
            pageSize={size}
            onPageChange={next => setPagination({ tab, search, size, page: next })}
            onPageSizeChange={setSize}
          />
        </TabsContent>
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
