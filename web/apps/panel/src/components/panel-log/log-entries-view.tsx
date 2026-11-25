import type { ReactNode } from 'react';

import { ScrollText } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { useDeferredValue, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { keepPreviousData, useQuery } from '@tanstack/react-query';

import type { LogEntry, LogLevel, LogScope } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { RegionState } from '@/components/region-state';
import { SelectField } from '@/components/select-field';
import { LoadingState } from '@/components/loading-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { PanelLogLevel } from '@/components/panel-log-level';
import { ListPagination } from '@/components/list-pagination';
import { formatLogTime, logTitle } from '@/utils/panel-log-presentation';
import { CardToolbar, StripDivider, StripSearch, TitleStrip } from '@/components/title-strip';

import type { PanelLogRecord } from './panel-log-details';

import { ClearLogsAction } from './clear-logs-action';
import { PanelLogDetails } from './panel-log-details';
import { matchingEventCodes } from './panel-log-presentation';

const panelColumns = [
  { key: 'time', className: 'w-48' },
  { key: 'level', className: 'w-25' },
  { key: 'summary', className: '' },
  { key: 'source', className: 'w-32' },
  { key: 'actions', className: 'w-30' },
] as const;

// A node's events all come from the panel's work on it, so they need no source.
const nodeColumns = panelColumns.filter(column => column.key !== 'source');

/**
 * The panel's log in one scope, filtered by level: the panel's own entries
 * in the logs window, under its title strip, or one node's events in that
 * node's logs window, under a toolbar at the top of the card that lead,
 * the window's tabs, starts. Clearing clears only the scope.
 */
export function LogEntriesView({ lead, scope }: { lead?: ReactNode; scope: LogScope }) {
  const { t, i18n } = useTranslation();
  const language = i18n.resolvedLanguage;
  const [selected, setSelected] = useState<PanelLogRecord | null>(null);
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
  const node = 'nodeID' in scope ? scope.nodeID : undefined;
  const columns = node === undefined ? panelColumns : nodeColumns;
  const logs = useQuery({
    queryKey: ['panelLogs', node ?? 'panel', limit, level, query, page],
    queryFn: ({ signal }) => client.listPanelLogs({ limit, level: level ? level as LogLevel : undefined,
      search: query || undefined, searchCodes: matchingEventCodes(query), scope,
      offset: (page - 1) * limit }, signal),
    enabled, staleTime: 5000, refetchInterval: enabled ? 5000 : false, placeholderData: keepPreviousData,
  });
  const result = logs.data ?? { items: [], total: 0 };
  const loading = logs.isPending || logs.isPlaceholderData;
  const pages = Math.max(1, Math.ceil(result.total / limit));
  if (!logs.isPlaceholderData && logs.data && page > pages) setPage(pages);
  function open(item: LogEntry, trigger: HTMLButtonElement) {
    triggerRef.current = trigger;
    setSelected({ entry: structuredClone(item), kind: 'log' });
    setDetailsOpen(true);
  }
  const controls = (
    <>
      {lead}
      {lead != null && <StripDivider className='max-sm:hidden' />}
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
      <StripDivider className={lead != null ? 'max-sm:hidden' : undefined} />
      {/* In a node's window the search takes its own last line on a phone, as in the core log. */}
      <StripSearch
        ref={searchRef}
        className={lead != null ? 'max-sm:order-last max-sm:basis-full' : undefined}
        label={t(node === undefined ? 'productLogs.searchPanel' : 'productLogs.searchEvents')}
        value={search}
        onValueChange={(value) => {
          setSearch(value);
          setPage(1);
        }}
      />
      <StripDivider className={lead != null ? 'max-sm:hidden' : undefined} />
      <ClearLogsAction className={lead != null ? 'max-sm:ms-auto' : undefined} scope={scope} />
    </>
  );
  return (
    <>
      {lead == null && <TitleStrip>{controls}</TitleStrip>}
      {logs.error != null && <ErrorNotice error={logs.error} title={t('productLogs.unavailable')} />}
      <div className='flex min-h-80 flex-1 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'>
        {lead != null && <CardToolbar>{controls}</CardToolbar>}
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
                    {node === undefined && <td className='truncate text-muted-foreground'>{t(`productLogs.sources.${item.source}`, { defaultValue: item.source })}</td>}
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
    </>
  );
}
