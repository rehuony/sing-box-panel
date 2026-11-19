import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { CirclePlus, TriangleAlert } from 'lucide-react';
import { toast } from '@shared/components/ui/toast-manager';
import { useDeferredValue, useEffect, useState } from 'react';
import { formatDateTime, formatShortDateTime } from '@shared/lib/format-time';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { SubscriptionSource } from '@/api/api-client';

import { queries } from '@/api/queries';
import { ApiRequestError } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { usePageVisible } from '@/hooks/use-page-visible';
import { ListPagination } from '@/components/list-pagination';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { StripDivider, StripSearch } from '@/components/title-strip';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

import type { SourceDialogMode } from './subscription-source-dialog';

import { useIntervalLabel } from './use-interval-label';
import { SubscriptionContentState } from './subscription-content-state';
import { SubscriptionSourceDialog } from './subscription-source-dialog';

/** When the source last got content, and the failure of its last attempt where there was one. */
function ContentCell({ source }: { source: SubscriptionSource }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const time = formatShortDateTime(source.content_at, locale) || '—';
  return (
    <span className='flex min-w-0 flex-col items-center'>
      <span className='flex max-w-full items-center gap-1.5'>
        <span className='truncate tabular-nums' title={formatDateTime(source.content_at, locale) || undefined}>{time}</span>
        {source.last_error
          ? (
              <Tooltip>
                <TooltipTrigger render={<span className='flex flex-none text-[var(--color-warning)]' />}>
                  <TriangleAlert aria-hidden='true' className='size-4' />
                  <span className='sr-only'>
                    {t('subscriptions.sources.lastError', { code: source.last_error, time: formatDateTime(source.last_attempt_at, locale) || '—' })}
                  </span>
                </TooltipTrigger>
                <TooltipContent>{t('subscriptions.sources.lastError', { code: source.last_error, time: formatDateTime(source.last_attempt_at, locale) || '—' })}</TooltipContent>
              </Tooltip>
            )
          : null}
      </span>
      {source.content_format ? <span className='text-xs text-muted-foreground'>{t(`subscriptions.sources.formats.${source.content_format}`)}</span> : null}
    </span>
  );
}

/**
 * The sources that feed the node pool, URL or file, in one paged table edge
 * to edge on the window's card, with the dialog that adds and edits them.
 */
export function SubscriptionSourcePanel({ active = true, onStarted, startCreating = false, toolbarTarget }: {
  active?: boolean;
  /** Opens the dialog that adds a source as the panel appears. */
  startCreating?: boolean;
  /** Says the request to add a source was taken. */
  onStarted?: () => void;
  toolbarTarget?: HTMLElement | null;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const visible = usePageVisible();
  const intervalLabel = useIntervalLabel();
  const enabled = active && visible;
  const list = useQuery({ ...queries.sources(client), enabled, refetchInterval: enabled ? 15_000 : false });
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search.trim().toLowerCase());
  const [size, setSize] = useState(10);
  const [page, setPage] = useState(1);
  const [dialog, setDialog] = useState<SourceDialogMode | null>(() => startCreating ? { kind: 'create' } : null);
  const [deleting, setDeleting] = useState<SubscriptionSource | null>(null);
  const [refreshing, setRefreshing] = useState<ReadonlySet<string>>(() => new Set());
  useEffect(() => {
    if (startCreating) onStarted?.();
  }, [onStarted, startCreating]);

  const sources = list.data ?? [];
  const matching = sources.filter(source => query === '' || [source.name, source.url ?? ''].some(value => value.toLowerCase().includes(query)));
  const pages = Math.max(1, Math.ceil(matching.length / size));
  const current = Math.min(page, pages);
  const rows = matching.slice((current - 1) * size, current * size);

  async function refresh(source: SubscriptionSource) {
    if (refreshing.has(source.id)) return;
    setRefreshing(ids => new Set(ids).add(source.id));
    try {
      const result = await client.refreshSubscriptionSource(source.id);
      toast.add({ title: t('subscriptions.sources.refreshed', { count: result.node_count, name: source.name }), type: 'success' });
    } catch (error) {
      toast.add({ title: t('subscriptions.sources.refreshFailed', { name: source.name }), description: describeRequestError(error), type: 'error' });
    } finally {
      setRefreshing((ids) => {
        const next = new Set(ids);
        next.delete(source.id);
        return next;
      });
      void cache.invalidateQueries({ queryKey: ['sources'] });
    }
  }

  const remove = useMutation({
    mutationFn: (source: SubscriptionSource) => client.deleteSubscriptionSource(source.id, source.updated_at),
    onSuccess: () => {
      setDeleting(null);
      toast.add({ title: t('subscriptions.sources.deleted'), type: 'success' });
    },
    onError: (error) => {
      const stale = error instanceof ApiRequestError && error.status === 412;
      toast.add({ title: stale ? t('subscriptions.sources.deleteStale') : describeRequestError(error), type: stale ? 'warning' : 'error' });
      if (stale) setDeleting(null);
    },
    onSettled: () => cache.invalidateQueries({ queryKey: ['sources'] }),
  });

  let state = null;
  if (rows.length === 0) {
    if (list.isPending) {
      state = <SubscriptionContentState loading />;
    } else if (list.data === undefined) {
      state = (
        <SubscriptionContentState kind='preview' title={t('subscriptions.sources.loadFailed')}>
          <Button onClick={() => void list.refetch()} variant='outline'>{t('shell.error.retry')}</Button>
        </SubscriptionContentState>
      );
    } else if (sources.length === 0) {
      state = (
        <SubscriptionContentState kind='sources' title={t('subscriptions.sources.empty')}>
          <Button onClick={() => setDialog({ kind: 'create' })} variant='outline'>
            <CirclePlus aria-hidden='true' data-icon='inline-start' />
            {t('subscriptions.sources.add')}
          </Button>
        </SubscriptionContentState>
      );
    } else {
      state = <SubscriptionContentState kind='search' title={t('subscriptions.sources.noMatches')} />;
    }
  }

  return (
    <section aria-label={t('subscriptions.tabs.sources')} className='flex h-full min-h-0 min-w-0 flex-col'>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='flex w-full min-w-0 flex-1 items-center gap-2'>
          <StripSearch
            label={t('subscriptions.sources.search')}
            onValueChange={(value) => {
              setSearch(value);
              setPage(1);
            }}
            value={search}
          />
          <StripDivider />
          <Button
            aria-label={t('subscriptions.sources.add')}
            className='max-sm:w-(--control-height-sm) max-sm:px-0'
            onClick={() => setDialog({ kind: 'create' })}
            size='sm'
            variant='flat'
          >
            <CirclePlus aria-hidden='true' data-icon='inline-start' />
            <span className='max-sm:sr-only'>{t('subscriptions.sources.add')}</span>
          </Button>
        </div>
      </ToolbarActions>
      {list.error && list.data !== undefined ? <ErrorNotice error={list.error} title={t('subscriptions.sources.loadFailed')} /> : null}
      <div aria-busy={list.isPending || undefined} className='flex min-h-0 flex-1 flex-col overflow-auto *:[table]:flex-none'>
        <table className='workspace-table w-full min-w-224 table-fixed border-collapse [&_td]:px-4 [&_td]:text-center [&_td:first-child]:text-left [&_tbody_tr]:border-b [&_th]:sticky [&_th]:top-0 [&_th]:z-1 [&_th]:bg-[var(--color-surface-raised)] [&_th]:px-4 [&_th]:text-center [&_th]:text-muted-foreground [&_th:first-child]:text-left [&_th:nth-child(1)]:w-[30%] [&_th:nth-child(2)]:w-[16%] [&_th:nth-child(3)]:w-[10%] [&_th:nth-child(4)]:w-[18%] [&_th:nth-child(5)]:w-[26%]'>
          <thead>
            <tr>
              <th>{t('subscriptions.sources.columns.name')}</th>
              <th>{t('subscriptions.sources.columns.kind')}</th>
              <th>{t('subscriptions.sources.columns.nodes')}</th>
              <th>{t('subscriptions.sources.columns.content')}</th>
              <th>{t('subscriptions.sources.columns.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(source => (
              <tr key={source.id}>
                <td>
                  <span className='flex min-w-0 flex-col'>
                    <span className='truncate' title={source.name}>{source.name}</span>
                    {source.url ? <span className='truncate font-[family-name:var(--font-data)] text-xs text-muted-foreground' title={source.url}>{source.url}</span> : null}
                  </span>
                </td>
                <td>
                  <span className='flex min-w-0 flex-col'>
                    <span>{t(`subscriptions.sources.kind.${source.kind}`)}</span>
                    {source.kind === 'url' ? <span className='truncate text-xs text-muted-foreground'>{intervalLabel(source.refresh_minutes ?? 0)}</span> : null}
                  </span>
                </td>
                <td className='tabular-nums'>{source.node_count}</td>
                <td><ContentCell source={source} /></td>
                <td>
                  <div className='inline-flex flex-wrap justify-center gap-2'>
                    <Button onClick={() => setDialog({ kind: 'edit', source })} size='sm' variant='outline'>{t('subscriptions.sources.edit')}</Button>
                    {source.kind === 'url'
                      ? (
                          <Button aria-busy={refreshing.has(source.id) || undefined} onClick={() => void refresh(source)} size='sm' variant='outline'>
                            {t('subscriptions.sources.refresh')}
                          </Button>
                        )
                      : <Button onClick={() => setDialog({ kind: 'content', source })} size='sm' variant='outline'>{t('subscriptions.sources.replace')}</Button>}
                    <Button onClick={() => setDeleting(source)} size='sm' variant='destructive'>{t('subscriptions.sources.delete')}</Button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {state}
      </div>
      <ListPagination
        className='border-t px-2 pb-2'
        onPageChange={setPage}
        onPageSizeChange={(value) => {
          setSize(value);
          setPage(1);
        }}
        page={current}
        pageSize={size}
        pages={pages}
      />
      {dialog ? <SubscriptionSourceDialog mode={dialog} onClose={() => setDialog(null)} /> : null}
      <Dialog open={deleting !== null} onOpenChange={(open) => {
        if (!open && !remove.isPending) setDeleting(null);
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.sources.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {deleting ? t('subscriptions.sources.deletePrompt', { count: deleting.node_count, name: deleting.name }) : null}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button onClick={() => setDeleting(null)} variant='outline'>{t('common.cancel')}</Button>
            <Button aria-busy={remove.isPending || undefined} onClick={() => deleting && !remove.isPending && remove.mutate(deleting)} variant='destructive'>
              {t('subscriptions.sources.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}
