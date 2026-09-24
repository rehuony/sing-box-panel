import { useTranslation } from 'react-i18next';
import { useDeferredValue, useEffect, useRef, useState } from 'react';

import type { LogLevel, PanelLog, PanelLogPage } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { ListPagination } from '@/components/list-pagination';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { Empty, EmptyHeader, EmptyTitle } from '@/components/ui/empty';

import { PanelLogLevel } from './panel-log-level';
import { PanelLogDetails } from './panel-log-details';
import { formatLogTime, logTitle, matchingEventCodes } from './panel-log-presentation';

export function PanelLogsPanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
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
  const [result, setResult] = useState<PanelLogPage>({ items: [], total: 0 });
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const pages = Math.max(1, Math.ceil(result.total / limit));
  useEffect(() => {
    const abort = new AbortController();
    let inFlight = false;
    async function load(initial = false) {
      if (initial) setLoading(true);
      if (inFlight) return;
      inFlight = true;
      try {
        const response = await client.listPanelLogs(
          {
            limit,
            level: level ? (level as LogLevel) : undefined,
            search: query || undefined,
            searchCodes: matchingEventCodes(query),
            offset: (page - 1) * limit,
          },
          abort.signal,
        );
        if (!abort.signal.aborted) {
          setResult(response);
          setPage(current => Math.min(current, Math.max(1, Math.ceil(response.total / limit))));
          setError(null);
        }
      } catch (reason) {
        if (!abort.signal.aborted) setError(reason);
      } finally {
        inFlight = false;
        if (!abort.signal.aborted) setLoading(false);
      }
    }
    void load(true);
    const timer = setInterval(() => {
      void load();
    }, 5000);
    return () => {
      abort.abort();
      clearInterval(timer);
    };
  }, [client, limit, level, query, page]);
  return (
    <div className='log-workspace'>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='log-toolbar workspace-toolbar-content'>
          <Input
            ref={searchRef}
            aria-label={t('productLogs.searchPanel')}
            placeholder={t('productLogs.searchPanel')}
            value={search}
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
          />
          <div className='log-toolbar__filters'>
            <SelectField
              aria-label={t('productLogs.level')}
              value={level}
              onValueChange={(value) => {
                setLevel(value);
                setPage(1);
              }}
              items={[{ value: '', label: t('productLogs.allLevels') }, ...['trace', 'debug', 'info', 'warn', 'error', 'fatal'].map((value) => ({ value, label: value.toUpperCase() }))]}
            />
          </div>
        </div>
      </ToolbarActions>
      {error != null && <ErrorNotice error={error} title={t('productLogs.unavailable')} />}
      <div className='panel-log-scroll' aria-busy={loading}>
        <table className='workspace-table panel-log-table'>
          <thead>
            <tr>
              {['time', 'level', 'summary', 'source', 'actions'].map((key) => (
                <th scope='col' key={key}>{t(`productLogs.columns.${key}`)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {result.items.map((item) => {
              const title = logTitle(item, t);
              return (
                <tr key={item.id}>
                  <td><time dateTime={item.time}>{formatLogTime(item.time, language)}</time></td>
                  <td><PanelLogLevel level={item.level} /></td>
                  <td>
                    <div className='panel-log-summary truncate' title={title}>{title}</div>
                  </td>
                  <td>{t(`productLogs.sources.${item.source}`, { defaultValue: item.source })}</td>
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
        {!result.items.length && (
          <Empty role='status'><EmptyHeader><EmptyTitle>{t(loading ? 'productLogs.loading' : 'productLogs.empty')}</EmptyTitle></EmptyHeader></Empty>
        )}
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
    </div>
  );
}
