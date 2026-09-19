import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { useDeferredValue, useEffect, useState } from 'react';

import type { LogLevel, PanelLog, PanelLogPage } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';

import { PanelLogDetail } from './panel-log-detail';

export function PanelLogsPanel() {
  const { t } = useTranslation();
  const client = useApiClient();
  const [params, setParams] = useSearchParams();
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search);
  const [level, setLevel] = useState('');
  const [limit, setLimit] = useState(10);
  const [cursors, setCursors] = useState<NonNullable<PanelLogPage['next']>[]>([]);
  const [result, setResult] = useState<PanelLogPage>({ items: [] });
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
  const [entry, setEntry] = useState<PanelLog | null>(null);
  const taskID = params.get('task');
  const cursor = cursors.at(-1);
  useEffect(() => {
    const abort = new AbortController();
    let inFlight = false;
    async function load(initial = false) {
      if (initial) setLoading(true);
      if (inFlight) return;
      inFlight = true;
      try {
        const page = await client.listPanelLogs(
          {
            limit,
            level: level ? (level as LogLevel) : undefined,
            search: query || undefined,
            beforeTime: cursor?.time,
            beforeID: cursor?.id,
          },
          abort.signal,
        );
        if (!abort.signal.aborted) {
          setResult(page);
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
  }, [client, limit, level, query, cursor]);
  function selectTask(id: string | null) {
    setParams((next) => {
      next.set('tab', 'panel');
      if (id) next.set('task', id);
      else next.delete('task');
      return next;
    });
  }
  return (
    <div className='log-workspace'>
      <div className='log-toolbar'>
        <Input
          aria-label={t('productLogs.searchPanel')}
          placeholder={t('productLogs.searchPanel')}
          value={search}
          onChange={(event) => {
            setSearch(event.target.value);
            setCursors([]);
          }}
        />
        <div className='log-toolbar__filters'>
          <select
            aria-label={t('productLogs.level')}
            value={level}
            onChange={(event) => {
              setLevel(event.target.value);
              setCursors([]);
            }}
          >
            <option value=''>ALL</option>
            {['trace', 'debug', 'info', 'warn', 'error', 'fatal'].map((value) => (
              <option key={value} value={value}>
                {value.toUpperCase()}
              </option>
            ))}
          </select>
        </div>
      </div>
      {error != null && <ErrorNotice error={error} title={t('productLogs.unavailable')} />}
      <div className='panel-log-scroll' aria-busy={loading}>
        <table className='panel-log-table'>
          <thead>
            <tr>
              {['time', 'message', 'status', 'source', 'actions'].map((key) => (
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
                  {item.source === 'task'
                    ? t(`tasks.kind.${item.code}`, { defaultValue: item.message })
                    : item.message}
                </td>
                <td>
                  <span className={`panel-log-state panel-log-state--${item.status || item.level}`}>
                    {item.status
                      ? t(`telemetry.taskStatus.${item.status}`, { defaultValue: item.status })
                      : item.level.toUpperCase()}
                  </span>
                </td>
                <td>{t(`productLogs.sources.${item.source}`)}</td>
                <td>
                  <Button
                    variant='ghost'
                    onClick={() => {
                      setEntry(item);
                      selectTask(item.task_id ?? null);
                    }}
                  >
                    {t('productLogs.details')}
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!result.items.length && (
          <p className='log-empty'>{t(loading ? 'productLogs.loading' : 'productLogs.empty')}</p>
        )}
      </div>
      <div className='log-pagination'>
        <select
          aria-label={t('productLogs.pageSize')}
          value={limit}
          onChange={(event) => {
            setLimit(Number(event.target.value));
            setCursors([]);
          }}
        >
          {[5, 10, 50].map((size) => (
            <option key={size} value={size}>
              {t('productLogs.perPage', { count: size })}
            </option>
          ))}
        </select>
        <div>
          <Button
            aria-label={t('pagination.previous')}
            variant='ghost'
            size='icon'
            disabled={!cursors.length || loading}
            onClick={() => setCursors((previous) => previous.slice(0, -1))}
          >
            <ChevronLeft />
          </Button>
          <span aria-current='page'>{cursors.length + 1}</span>
          <Button
            aria-label={t('pagination.next')}
            variant='ghost'
            size='icon'
            disabled={!result.next || loading}
            onClick={() => {
              if (result.next) setCursors((previous) => [...previous, result.next!]);
            }}
          >
            <ChevronRight />
          </Button>
        </div>
      </div>
      <PanelLogDetail
        key={taskID ?? entry?.id ?? 'closed'}
        entry={entry}
        taskID={taskID}
        onClose={() => {
          setEntry(null);
          selectTask(null);
        }}
        onTaskChange={selectTask}
      />
    </div>
  );
}
