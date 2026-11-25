import type { ReactNode } from 'react';

import { Activity } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { useDeferredValue, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { keepPreviousData, useQuery } from '@tanstack/react-query';

import type { PanelLogRecord } from '@/components/panel-log';
import type { NodeRuntimeTransition, RuntimeTransitionState } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { RegionState } from '@/components/region-state';
import { SelectField } from '@/components/select-field';
import { LoadingState } from '@/components/loading-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { ListPagination } from '@/components/list-pagination';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';
import { CardToolbar, StripDivider, StripSearch } from '@/components/title-strip';
import { matchingEventCodes, PanelLogDetails, RuntimeStateBadge } from '@/components/panel-log';

const columns = [
  { key: 'time', className: 'w-48' },
  { key: 'state', className: 'w-25' },
  { key: 'reason', className: '' },
  { key: 'actions', className: 'w-30' },
] as const;

const states = ['running', 'stopped', 'failed', 'unknown'] as const satisfies readonly RuntimeTransitionState[];

/**
 * The node's runtime states, newest first, in the log's table and pagination,
 * filtered by state, under a toolbar that lead, the window's tabs, starts.
 */
export function RuntimeStatesView({ lead, nodeID }: { lead: ReactNode; nodeID: string }) {
  const { t, i18n } = useTranslation();
  const language = i18n.resolvedLanguage;
  const [selected, setSelected] = useState<PanelLogRecord | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const client = useApiClient();
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search.trim());
  const [state, setState] = useState('');
  const [limit, setLimit] = useState(10);
  const [page, setPage] = useState(1);
  const enabled = usePageVisible();
  const history = useQuery({
    queryKey: ['fleetRuntimeHistory', nodeID, limit, state, query, page],
    queryFn: ({ signal }) => client.listFleetRuntimeHistory({
      limit, state: state ? state as RuntimeTransitionState : undefined,
      search: query || undefined, searchCodes: matchingEventCodes(query), nodeID,
      offset: (page - 1) * limit,
    }, signal),
    enabled, staleTime: 5000, refetchInterval: enabled ? 5000 : false, placeholderData: keepPreviousData,
  });
  const result = history.data ?? { items: [], total: 0 };
  const loading = history.isPending || history.isPlaceholderData;
  const pages = Math.max(1, Math.ceil(result.total / limit));
  if (!history.isPlaceholderData && history.data && page > pages) setPage(pages);
  function open(item: NodeRuntimeTransition, trigger: HTMLButtonElement) {
    triggerRef.current = trigger;
    setSelected({ kind: 'state', transition: structuredClone(item) });
    setDetailsOpen(true);
  }
  return (
    <>
      {history.error != null && <ErrorNotice error={history.error} title={t('productLogs.statesUnavailable')} />}
      <div className='flex min-h-80 flex-1 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'>
        <CardToolbar>
          {lead}
          <StripDivider className='max-sm:hidden' />
          <SelectField
            active={state !== ''}
            aria-label={t('productLogs.state')}
            className='flex-none'
            variant='flat'
            value={state}
            onValueChange={(value) => {
              setState(value);
              setPage(1);
            }}
            items={[{ value: '', label: t('productLogs.allStates') }, ...states.map(value => ({ value, label: t(`productLogs.states.${value}`) }))]}
          />
          <StripDivider className='max-sm:hidden' />
          <StripSearch
            ref={searchRef}
            className='max-sm:order-last max-sm:basis-full'
            label={t('productLogs.searchStates')}
            value={search}
            onValueChange={(value) => {
              setSearch(value);
              setPage(1);
            }}
          />
        </CardToolbar>
        <ScrollArea aria-busy={loading} className='flex-1' viewportClassName='flex flex-col'>
          <table className='workspace-table w-full min-w-195 flex-none table-fixed border-collapse [&_td]:px-4 [&_th]:px-4 [&_th]:text-left [&_th]:text-muted-foreground [&_td:last-child]:text-center [&_th:last-child]:text-center'>
            <thead>
              <tr>
                {columns.map(column => (
                  <th className={`sticky top-0 z-1 bg-[var(--color-surface-raised)] ${column.className}`} scope='col' key={column.key}>
                    {t(`productLogs.stateColumns.${column.key}`)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.items.map((item) => {
                const title = logTitle({ code: item.reason }, t);
                return (
                  <tr className='border-b' key={item.id}>
                    <td className='truncate text-muted-foreground'><time dateTime={item.occurred_at}>{formatLogTime(item.occurred_at, language)}</time></td>
                    <td><RuntimeStateBadge state={item.state} /></td>
                    <td><div className='truncate' title={title}>{title}</div></td>
                    <td>
                      <Button variant='ghost' size='sm' aria-label={`${t('productLogs.details')}: ${title}`} onClick={event => open(item, event.currentTarget)}>
                        {t('productLogs.details')}
                      </Button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          {!result.items.length && (loading
            ? <LoadingState label={t('productLogs.loading')} fullScreen={false} />
            : <RegionState icon={Activity} title={t('productLogs.empty')} />)}
        </ScrollArea>
        <ListPagination
          className='border-t px-2 pb-2'
          page={page}
          pages={pages}
          pageSize={limit}
          onPageChange={setPage}
          onPageSizeChange={value => {
            setLimit(value);
            setPage(1);
          }}
        />
      </div>
      <PanelLogDetails
        item={selected}
        open={detailsOpen}
        onOpenChange={setDetailsOpen}
        onClosed={() => setSelected(null)}
        returnFocus={() => triggerRef.current?.isConnected ? triggerRef.current : searchRef.current}
      />
    </>
  );
}
