import { useTranslation } from 'react-i18next';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { useDeferredValue, useEffect, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';

import type { LogLevel, PanelLogPage } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { SelectField } from '@/components/select-field';
import { ToolbarActions } from '@/components/workspace-toolbar';

import { PanelLogDetail } from './panel-log-detail';

export function PanelLogsPanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
} = {}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [params] = useSearchParams();
  const location = useLocation();
  const navigate = useNavigate();
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search);
  const [level, setLevel] = useState('');
  const [limit, setLimit] = useState(10);
  const [cursors, setCursors] = useState<NonNullable<PanelLogPage['next']>[]>([]);
  const [result, setResult] = useState<PanelLogPage>({ items: [] });
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(true);
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
    const next = new URLSearchParams(params);
    next.delete('tab');
    if (id) next.set('task', id);
    else next.delete('task');
    void navigate({ pathname: location.pathname, search: next.size ? `?${next}` : '', hash: '#logs-panel' }, { state: location.state });
  }
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
              setCursors([]);
            }}
          />
          <div className='log-toolbar__filters'>
            <SelectField
              aria-label={t('productLogs.level')}
              value={level}
              onValueChange={(value) => {
                setLevel(value);
                setCursors([]);
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
                  {item.source === 'task'
                    ? t(`tasks.kind.${item.code}`, { defaultValue: item.message })
                    : item.message}
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
      <div className='log-pagination'>
        <SelectField
          aria-label={t('productLogs.pageSize')}
          value={limit}
          onValueChange={(value) => {
            setLimit(value);
            setCursors([]);
          }}
          items={[5, 10, 50].map((value) => ({ value, label: t('productLogs.perPage', { count: value }) }))}
        />
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
        key={taskID ?? 'closed'}
        taskID={taskID}
        onClose={() => selectTask(null)}
        onTaskChange={selectTask}
      />
    </div>
  );
}
