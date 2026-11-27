import type { ReactNode } from 'react';

import { ScrollText } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@shared/components/ui/badge';
import { useDeferredValue, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@shared/components/ui/table';

import type { LogEntry, LogLevel, LogScope } from '@/api/api-client';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { RegionState } from '@/components/region-state';
import { SelectField } from '@/components/select-field';
import { LoadingState } from '@/components/loading-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { logTitle } from '@/utils/panel-log-presentation';
import { ListPagination } from '@/components/list-pagination';
import { PanelLogLevelText } from '@/components/panel-log-level';
import { CardToolbar, StripDivider, StripSearch, TitleStrip } from '@/components/title-strip';

import type { PanelLogRecord } from './panel-log-details';

import { useLogRowFocus } from './use-log-row-focus';
import { ClearLogsAction } from './clear-logs-action';
import { PanelLogDetails } from './panel-log-details';
import { LogRow, LogRowTitle, LogTime } from './log-table';
import { logContext, logTimeColumnClass, matchingEventCodes } from './panel-log-presentation';

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
  const [limit, setLimit] = useState(50);
  const [page, setPage] = useState(1);
  const enabled = usePageVisible();
  const node = 'nodeID' in scope ? scope.nodeID : undefined;
  const logs = useQuery({
    queryKey: ['panelLogs', node ?? 'panel', limit, level, query, page],
    queryFn: ({ signal }) => client.listPanelLogs({ limit, level: level ? level as LogLevel : undefined,
      search: query || undefined, searchCodes: matchingEventCodes(query), scope,
      offset: (page - 1) * limit }, signal),
    enabled, staleTime: 5000, refetchInterval: enabled ? 5000 : false, placeholderData: keepPreviousData,
  });
  // The panel's log names nodes by their current names; a node's own log needs none.
  const fleet = useQuery({ ...queries.fleetNodes(client), enabled: enabled && node === undefined });
  const nodeNames = new Map((fleet.data?.items ?? []).map(item => [item.id, item.name]));
  const nodeName = node === undefined ? (id: string) => nodeNames.get(id) : undefined;
  const result = logs.data ?? { items: [], total: 0 };
  const loading = logs.isPending || logs.isPlaceholderData;
  const pages = Math.max(1, Math.ceil(result.total / limit));
  if (!logs.isPlaceholderData && logs.data && page > pages) setPage(pages);
  const rowFocus = useLogRowFocus(result.items.map(item => item.id));
  const columns = [
    { key: 'time', className: logTimeColumnClass(result.items.map(item => item.time)) },
    { key: 'level', className: 'w-19 max-sm:w-16' },
    { key: 'summary', className: '' },
  ] as const;
  function open(item: LogEntry, trigger: HTMLButtonElement | null) {
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
          {/* The time, the level and the event fit a phone without scrolling sideways. */}
          <Table>
            <TableHeader>
              <TableRow>
                {columns.map(column => (
                  <TableHead className={column.className} key={column.key}>
                    {t(`productLogs.columns.${column.key}`)}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody onKeyDown={rowFocus.onKeyDown}>
              {result.items.map((item) => {
                const title = logTitle(item, t);
                const context = logContext(item.metadata, t, { language, nodeName });
                return (
                  <LogRow key={item.id} onOpen={trigger => open(item, trigger)}>
                    <TableCell><LogTime language={language} value={item.time} /></TableCell>
                    <TableCell className='font-[family-name:var(--font-data)] text-xs'><PanelLogLevelText level={item.level} /></TableCell>
                    <TableCell>
                      <div className='flex min-w-0 items-center gap-2'>
                        {item.source === 'security' && <Badge variant='secondary'>{t('productLogs.sources.security')}</Badge>}
                        <LogRowTitle
                          detail={context || undefined}
                          tabIndex={rowFocus.tabIndex(item.id)}
                          title={title}
                          onFocus={() => rowFocus.onFocus(item.id)}
                          onOpen={trigger => open(item, trigger)}
                        />
                      </div>
                    </TableCell>
                  </LogRow>
                );
              })}
            </TableBody>
          </Table>
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
        returnFocus={() => {
          const trigger = triggerRef.current;
          // Details a press opened leave focus out of the list.
          if (trigger === null) return false;
          return trigger.isConnected ? trigger : searchRef.current;
        }}
      />
    </>
  );
}
