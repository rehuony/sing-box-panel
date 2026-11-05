import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { useDeferredValue, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { Empty, EmptyHeader, EmptyTitle } from '@shared/components/ui/empty';

import type { LogLevel, PanelLog } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { LoadingState } from '@/components/loading-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { PanelLogLevel } from '@/components/panel-log-level';
import { ListPagination } from '@/components/list-pagination';
import { ShellNavigationTrigger } from '@/components/app-shell';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';

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
  const logs = useQuery({
    queryKey: ['panelLogs', limit, level, query, page],
    queryFn: ({ signal }) => client.listPanelLogs({ limit, level: level ? level as LogLevel : undefined,
      search: query || undefined, searchCodes: matchingEventCodes(query), offset: (page - 1) * limit }, signal),
    enabled, staleTime: 5000, refetchInterval: enabled ? 5000 : false, placeholderData: keepPreviousData,
  });
  const result = logs.data ?? { items: [], total: 0 };
  const loading = logs.isPending || logs.isPlaceholderData;
  const pages = Math.max(1, Math.ceil(result.total / limit));
  if (!logs.isPlaceholderData && logs.data && page > pages) setPage(pages);
  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)]'>
      <h1 className='sr-only'>{t('nav.logs')}</h1>
      <div className='flex flex-wrap items-center gap-3'>
        <ShellNavigationTrigger />
        <Input
          ref={searchRef}
          aria-label={t('productLogs.searchPanel')}
          className='min-w-0 flex-1 sm:w-70 sm:flex-none'
          placeholder={t('productLogs.searchPanel')}
          value={search}
          onChange={(event) => {
            setSearch(event.target.value);
            setPage(1);
          }}
        />
        <SelectField
          aria-label={t('productLogs.level')}
          className='min-w-28'
          value={level}
          onValueChange={(value) => {
            setLevel(value);
            setPage(1);
          }}
          items={[{ value: '', label: t('productLogs.allLevels') }, ...['trace', 'debug', 'info', 'warn', 'error', 'fatal'].map((value) => ({ value, label: value.toUpperCase() }))]}
        />
      </div>
      {logs.error != null && <ErrorNotice error={logs.error} title={t('productLogs.unavailable')} />}
      <div className='flex min-h-80 flex-1 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-card'>
        <ScrollArea aria-busy={loading} className='flex-1' viewportClassName='flex flex-col'>
          <table className='workspace-table w-full min-w-195 flex-none table-fixed border-collapse'>
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
            : <Empty role='status'><EmptyHeader><EmptyTitle>{t('productLogs.empty')}</EmptyTitle></EmptyHeader></Empty>)}
        </ScrollArea>
      </div>
      <PanelLogDetails
        item={selected}
        open={detailsOpen}
        onOpenChange={setDetailsOpen}
        onClosed={() => setSelected(null)}
        returnFocus={() => triggerRef.current?.isConnected ? triggerRef.current : searchRef.current}
      />
      <ListPagination
        page={page}
        pages={pages}
        pageSize={limit}
        disabled={loading || result.total === 0}
        onPageChange={setPage}
        onPageSizeChange={value => {
          setLimit(value);
          setPage(1);
        }}
      />
    </section>
  );
}
