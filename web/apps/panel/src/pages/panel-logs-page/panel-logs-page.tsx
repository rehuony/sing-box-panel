import { ScrollText, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import { Button } from '@shared/components/ui/button';
import { useDeferredValue, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { keepPreviousData, useQuery } from '@tanstack/react-query';

import type { LogLevel, PanelLog } from '@/api/api-client';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { RegionState } from '@/components/region-state';
import { SelectField } from '@/components/select-field';
import { LoadingState } from '@/components/loading-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { PanelLogLevel } from '@/components/panel-log-level';
import { ListPagination } from '@/components/list-pagination';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';
import { StripDivider, StripSearch, TitleStrip } from '@/components/title-strip';

import { PanelLogDetails } from './panel-log-details';
import { matchingEventCodes } from './panel-log-presentation';

const columns = [
  { key: 'time', className: 'w-48' },
  { key: 'level', className: 'w-25' },
  { key: 'summary', className: '' },
  { key: 'source', className: 'w-32' },
  { key: 'actions', className: 'w-30' },
] as const;

/** The panel's own log: operations, security events and node activity across the fleet. */
export function PanelLogsPage() {
  const { t, i18n } = useTranslation();
  const language = i18n.resolvedLanguage;
  const [selected, setSelected] = useState<PanelLog | null>(null);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const client = useApiClient();
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search.trim());
  const [level, setLevel] = useState('');
  const [limit, setLimit] = useState(10);
  const [page, setPage] = useState(1);
  const enabled = usePageVisible();
  // A node's overview opens its entries here; the filter stays in the address,
  // so it can be shared and cleared.
  const [params, setParams] = useSearchParams();
  const nodeID = params.get('node') ?? '';
  const nodeName = useQuery({
    ...queries.fleetStatus(client),
    enabled: nodeID !== '',
    select: status => status.items.find(item => item.node.id === nodeID)?.node.name,
  }).data ?? nodeID;
  const logs = useQuery({
    queryKey: ['panelLogs', limit, level, query, nodeID, page],
    queryFn: ({ signal }) => client.listPanelLogs({ limit, level: level ? level as LogLevel : undefined,
      search: query || undefined, searchCodes: matchingEventCodes(query), nodeID: nodeID || undefined,
      offset: (page - 1) * limit }, signal),
    enabled, staleTime: 5000, refetchInterval: enabled ? 5000 : false, placeholderData: keepPreviousData,
  });
  const result = logs.data ?? { items: [], total: 0 };
  const loading = logs.isPending || logs.isPlaceholderData;
  const pages = Math.max(1, Math.ceil(result.total / limit));
  if (!logs.isPlaceholderData && logs.data && page > pages) setPage(pages);
  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)]'>
      <h1 className='sr-only'>{t('nav.logs')}</h1>
      <TitleStrip className='flex-nowrap'>
        {nodeID
          ? (
              <>
                <Button
                  aria-label={t('productLogs.clearNode', { name: nodeName })}
                  className='max-w-48 min-w-0 flex-none bg-accent text-foreground'
                  onClick={() => {
                    setParams(current => {
                      const next = new URLSearchParams(current);
                      next.delete('node');
                      return next;
                    }, { replace: true });
                    setPage(1);
                  }}
                  size='sm'
                  type='button'
                  variant='flat'
                >
                  <span className='truncate'>{nodeName}</span>
                  <X aria-hidden='true' data-icon='inline-end' />
                </Button>
                <StripDivider />
              </>
            )
          : null}
        <StripSearch
          ref={searchRef}
          label={t('productLogs.searchPanel')}
          value={search}
          onValueChange={(value) => {
            setSearch(value);
            setPage(1);
          }}
        />
        <StripDivider />
        <SelectField
          active={level !== ''}
          aria-label={t('productLogs.level')}
          className='flex-none'
          variant='flat'
          value={level}
          onValueChange={(value) => {
            setLevel(value);
            setPage(1);
          }}
          items={[{ value: '', label: t('productLogs.allLevels') }, ...['trace', 'debug', 'info', 'warn', 'error', 'fatal'].map((value) => ({ value, label: value.toUpperCase() }))]}
        />
      </TitleStrip>
      {logs.error != null && <ErrorNotice error={logs.error} title={t('productLogs.unavailable')} />}
      <div className='flex min-h-80 flex-1 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'>
        <ScrollArea aria-busy={loading} className='flex-1' viewportClassName='flex flex-col'>
          <table className='workspace-table w-full min-w-195 flex-none table-fixed border-collapse [&_td]:px-4 [&_th]:px-4 [&_th]:text-left [&_th]:text-muted-foreground [&_td:last-child]:text-center [&_th:last-child]:text-center'>
            <thead>
              <tr>
                {columns.map(column => (
                  <th className={`sticky top-0 z-1 bg-[var(--color-surface-raised)] ${column.className}`} scope='col' key={column.key}>
                    {t(`productLogs.columns.${column.key}`)}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {result.items.map((item) => {
                const title = logTitle(item, t);
                return (
                  <tr className='border-b' key={item.id}>
                    <td className='truncate text-muted-foreground'><time dateTime={item.time}>{formatLogTime(item.time, language)}</time></td>
                    <td><PanelLogLevel level={item.level} /></td>
                    <td><div className='truncate' title={title}>{title}</div></td>
                    <td className='truncate text-muted-foreground'>{t(`productLogs.sources.${item.source}`, { defaultValue: item.source })}</td>
                    <td>
                      <Button variant='ghost' size='sm' aria-label={`${t('productLogs.details')}: ${title}`} onClick={(event) => {
                        triggerRef.current = event.currentTarget;
                        setSelected(structuredClone(item));
                        setDetailsOpen(true);
                      }}>
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
            : <RegionState icon={ScrollText} title={t('productLogs.empty')} />)}
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
    </section>
  );
}
