import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { toast } from '@shared/components/ui/toast-manager';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { EM_DASH, formatBytes } from '@shared/lib/telemetry-format';
import { buttonVariants } from '@shared/components/ui/button-variants';
import { formatDateTime, formatShortDateTime } from '@shared/lib/format-time';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@shared/components/ui/table';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';
import { Download, ExternalLink, MoreHorizontal, Package, PackageSearch, Power, PowerOff, RefreshCw, SearchX, Trash2, Upload } from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@shared/components/ui/dropdown-menu';

import type { CoreArtifact } from '@/api/api-client';

import { useHashTab } from '@/hooks/use-hash-tab';
import { IconAction } from '@/components/icon-action';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { ListPagination } from '@/components/list-pagination';
import { RegionError, RegionState } from '@/components/region-state';
import { describeRequestError, ErrorNotice, toastFailure } from '@/components/error-notice';
import { CardToolbar, StripAction, StripDivider, StripSearch } from '@/components/title-strip';

import { CoreImportDialog } from './core-import-dialog';
import { useVersionLibrary } from './use-version-library';

/** A version's state in its list: what the catalog has installed, or whether an installed one runs. */
type VersionState = 'installed' | 'notInstalled' | 'enabled' | 'disabled' | 'unknown' | 'running';

/** A version, the row's identity, in the data face. */
const versionCell = 'font-[family-name:var(--font-data)] text-compact font-medium tabular-nums';

const stateTone: Record<VersionState, string> = {
  installed: 'text-[var(--color-success-text)]',
  enabled: 'text-[var(--color-success-text)]',
  notInstalled: 'text-muted-foreground',
  disabled: 'text-muted-foreground',
  unknown: 'text-[var(--color-warning-text)]',
  running: 'text-muted-foreground',
};

/** A version's state as a dot and a word; work in progress turns the dot into a spinner. */
function VersionStatus({ state }: { state: VersionState }) {
  const { t } = useTranslation();
  return (
    <span className={cn('inline-flex items-center gap-1.5 text-xs font-medium whitespace-nowrap', stateTone[state])}>
      {state === 'running'
        ? <Spinner aria-hidden='true' className='size-3' role={undefined} />
        : <span aria-hidden='true' className='size-1.5 flex-none rounded-full bg-current' />}
      {t(`cores.library.${state}`)}
    </span>
  );
}

export function CoresPage() {
  const { t, i18n } = useTranslation();
  const locale = i18n.resolvedLanguage ?? 'en';
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
        toast.add({ title: t('cores.library.actionFailed'), description: describeRequestError(reason), type: 'error' });
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
      if (!signal.aborted) toastFailure(t('cores.library.removeFailed'), reason);
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
          <ScrollArea className='flex-1' contentClassName='flex flex-col'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-36 max-sm:w-28'>{t('cores.library.version')}</TableHead>
                  <TableHead className='w-32 max-sm:w-28'>{t('cores.library.status')}</TableHead>
                  {tab === 'catalog'
                    ? (
                        <>
                          <TableHead className='max-sm:hidden'>{t('cores.library.file')}</TableHead>
                          <TableHead className='w-28 text-end max-sm:hidden'>{t('cores.library.size')}</TableHead>
                        </>
                      )
                    : (
                        <>
                          <TableHead className='max-sm:hidden'>{t('cores.library.source')}</TableHead>
                          <TableHead className='w-44 max-sm:hidden'>{t('cores.library.installedAt')}</TableHead>
                        </>
                      )}
                  <TableHead className='w-28 max-sm:w-24 max-sm:ps-0'><span className='sr-only'>{t('cores.library.actions')}</span></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tab === 'installed'
                  ? installed.slice(start, start + size).map((artifact) => {
                      const enabled
                        = library.runtime?.enabled_core?.core_artifact_id === artifact.id;
                      let state: VersionState = 'disabled';
                      if (pending?.key === artifact.id) state = 'running';
                      else if (enabled) state = 'enabled';
                      else if (runtimeUncertain) state = 'unknown';
                      return (
                        <TableRow key={artifact.id}>
                          <TableCell className={versionCell}>{artifact.exact_version}</TableCell>
                          <TableCell><VersionStatus state={state} /></TableCell>
                          <TableCell className='text-[var(--color-text-secondary)] max-sm:hidden'>
                            {t(artifact.source_kind === 'official' ? 'cores.source.official' : 'cores.source.user_verified')}
                          </TableCell>
                          <TableCell className='max-sm:hidden' kind='data' title={formatDateTime(artifact.created_at, locale)}>
                            {formatShortDateTime(artifact.created_at, locale) || EM_DASH}
                          </TableCell>
                          <TableCell className='max-sm:ps-0' kind='actions'>
                            <IconAction
                              disabled={Boolean(pending) || runtimeUncertain}
                              label={t(enabled ? 'cores.library.disable' : 'cores.library.enable')}
                              onClick={() =>
                                void run(artifact.id, (signal) =>
                                  enabled
                                    ? client.disableCore(artifact.id, signal)
                                    : client.enableCore(artifact.id, signal),
                                )}
                            >
                              {enabled ? <PowerOff aria-hidden='true' /> : <Power aria-hidden='true' />}
                            </IconAction>
                            <DropdownMenu>
                              <DropdownMenuTrigger
                                render={(
                                  <IconAction
                                    className='data-popup-open:bg-[var(--color-hover)] data-popup-open:text-foreground'
                                    label={t('cores.library.more', { version: artifact.exact_version })}
                                  />
                                )}
                              >
                                <MoreHorizontal aria-hidden='true' />
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align='end' className='w-auto min-w-40'>
                                <DropdownMenuGroup>
                                  <DropdownMenuItem
                                    disabled={Boolean(pending) || enabled || runtimeUncertain}
                                    variant='destructive'
                                    onClick={() => setConfirmation(artifact)}
                                  >
                                    <Trash2 aria-hidden='true' />
                                    {t('cores.action.remove')}
                                  </DropdownMenuItem>
                                </DropdownMenuGroup>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </TableCell>
                        </TableRow>
                      );
                    })
                  : available.slice(start, start + size).map((asset) => {
                      const exists = library.artifacts.some((artifact) => artifact.asset_id === asset.asset_id);
                      const key = `install:${asset.asset_id}`;
                      let state: VersionState = exists ? 'installed' : 'notInstalled';
                      if (pending?.key === key) state = 'running';
                      const release = t('cores.library.release', { version: asset.version });
                      return (
                        <TableRow key={asset.asset_id}>
                          <TableCell className={versionCell}>{asset.version}</TableCell>
                          <TableCell><VersionStatus state={state} /></TableCell>
                          <TableCell className='max-sm:hidden' kind='data' title={asset.name}>{asset.name}</TableCell>
                          <TableCell className='max-sm:hidden' kind='number'>{formatBytes(asset.size, locale)}</TableCell>
                          <TableCell className='max-sm:ps-0' kind='actions'>
                            {exists
                              ? null
                              : (
                                  <IconAction
                                    disabled={Boolean(pending)}
                                    label={t('cores.library.download')}
                                    onClick={() =>
                                      void run(key, (signal) => client.installCore(asset.asset_id, signal))}
                                  >
                                    <Download aria-hidden='true' />
                                  </IconAction>
                                )}
                            <Tooltip>
                              <TooltipTrigger
                                render={(
                                  <a
                                    aria-label={release}
                                    className={buttonVariants({ size: 'icon-sm', variant: 'ghost' })}
                                    href={`https://github.com/SagerNet/sing-box/releases/tag/v${encodeURIComponent(asset.version)}`}
                                    rel='noopener noreferrer'
                                    target='_blank'
                                  />
                                )}
                              >
                                <ExternalLink aria-hidden='true' />
                              </TooltipTrigger>
                              <TooltipContent>{release}</TooltipContent>
                            </Tooltip>
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
