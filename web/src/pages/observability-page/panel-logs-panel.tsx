import { useTranslation } from 'react-i18next';
import { useDeferredValue, useEffect, useState } from 'react';

import type { LogLevel, PanelLogPage } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { ListPagination } from '@/components/list-pagination';
import { ToolbarActions } from '@/components/workspace-toolbar';

export function PanelLogsPanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search);
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
              items={[{ value: '', label: 'ALL' }, ...['trace', 'debug', 'info', 'warn', 'error', 'fatal'].map((value) => ({ value, label: value.toUpperCase() }))]}
            />
          </div>
        </div>
      </ToolbarActions>
      {error != null && <ErrorNotice error={error} title={t('productLogs.unavailable')} />}
      <div className='panel-log-scroll' aria-busy={loading}>
        <table className='workspace-table panel-log-table'>
          <thead>
            <tr>
              {['time', 'message', 'level', 'source'].map((key) => (
                <th key={key}>{t(`productLogs.${key}`)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {result.items.map((item) => (
              <tr key={item.id}>
                <td>
                  <time dateTime={item.time}>{new Date(item.time).toLocaleString()}</time>
                </td>
                <td>
                  {item.message}
                </td>
                <td>
                  <span className={`panel-log-state panel-log-state--${item.level}`}>
                    {item.level.toUpperCase()}
                  </span>
                </td>
                <td>{t(`productLogs.sources.${item.source}`)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {!result.items.length && (
          <p className='log-empty'>{t(loading ? 'productLogs.loading' : 'productLogs.empty')}</p>
        )}
      </div>
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
