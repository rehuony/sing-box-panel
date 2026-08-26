import { useCallback, useEffect, useState } from 'react';

import type { LogEntry, LogLevel, LogSource } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { ActionError } from '@/components/action-error';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

export interface LogsPanelProps {
  refreshKey: number;
}

function formatLogTime(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: '2-digit', minute: '2-digit', second: '2-digit',
  }).format(new Date(value));
}

export function LogsPanel({ refreshKey }: LogsPanelProps) {
  const client = useApiClient();
  const [logs, setLogs] = useState<LogEntry[] | null>(null);
  const [selected, setSelected] = useState<LogEntry | null>(null);
  const [source, setSource] = useState<LogSource | ''>('');
  const [level, setLevel] = useState<LogLevel | ''>('');
  const [loadError, setLoadError] = useState<unknown>(null);
  const [detailError, setDetailError] = useState('');

  const load = useCallback(async (signal?: AbortSignal) => {
    try {
      setLoadError(null);
      const page = await client.listLogs({
        source: source || undefined,
        level: level || undefined,
        limit: 50,
      }, signal);
      if (!signal?.aborted) setLogs(page.items);
    } catch (error) {
      if (!signal?.aborted) setLoadError(error);
    }
  }, [client, level, source]);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => controller.abort();
  }, [load, refreshKey]);

  async function inspect(entry: LogEntry) {
    setDetailError('');
    try {
      setSelected(await client.getLog(entry.id));
    } catch (error) {
      setDetailError(describeRequestError(error));
    }
  }

  return (
    <section className='logs-panel' aria-labelledby='durable-logs-title'>
      <div className='section-heading'>
        <div>
          <p className='eyebrow'>Sanitized durable events</p>
          <h2 id='durable-logs-title'>Logs</h2>
        </div>
        <span>
          {logs?.length ?? 0}
          {' '}
          shown
        </span>
      </div>
      <div className='log-filters' role='group' aria-label='Log filters'>
        <label>
          Source
          <select aria-label='Log source' onChange={(event) => setSource(event.target.value as LogSource | '')} value={source}>
            <option value=''>All sources</option>
            <option value='panel'>Panel</option>
            <option value='core'>Core</option>
            <option value='task'>Task</option>
            <option value='security'>Security</option>
          </select>
        </label>
        <label>
          Level
          <select aria-label='Log level' onChange={(event) => setLevel(event.target.value as LogLevel | '')} value={level}>
            <option value=''>All levels</option>
            <option value='trace'>Trace</option>
            <option value='debug'>Debug</option>
            <option value='info'>Info</option>
            <option value='warn'>Warn</option>
            <option value='error'>Error</option>
            <option value='fatal'>Fatal</option>
          </select>
        </label>
        <button className='button button--secondary button--small' onClick={() => void load()} type='button'>Refresh logs</button>
      </div>
      {loadError === null ? null : <ErrorNotice error={loadError} title='Durable log API unavailable' />}
      <ActionError message={detailError} title='Log detail failed' />
      {logs === null ? <div className='inline-loading' aria-busy='true'>Loading durable events…</div> : null}
      {logs?.length === 0
        ? (
            <div className='empty-state'>
              <strong>No matching events.</strong>
              <p>The panel does not synthesize routine log entries.</p>
            </div>
          )
        : null}
      {logs && logs.length > 0
        ? (
            <ol className='log-list'>
              {logs.map((entry) => (
                <li key={entry.id}>
                  <button aria-pressed={selected?.id === entry.id} onClick={() => void inspect(entry)} type='button'>
                    <span className={`log-level log-level--${entry.level}`}>{entry.level}</span>
                    <span>
                      <strong>{entry.message}</strong>
                      <small>
                        {entry.source}
                        {' '}
                        ·
                        {' '}
                        {entry.code}
                      </small>
                    </span>
                    <time dateTime={entry.time}>{formatLogTime(entry.time)}</time>
                  </button>
                </li>
              ))}
            </ol>
          )
        : null}
      {selected === null
        ? null
        : (
            <div className='log-detail' aria-live='polite'>
              <div className='section-heading'>
                <div>
                  <p className='eyebrow'>Selected event</p>
                  <h3>{selected.code}</h3>
                </div>
                <span>{selected.id}</span>
              </div>
              <p>{selected.message}</p>
              <dl>
                <div>
                  <dt>Source</dt>
                  <dd>{selected.source}</dd>
                </div>
                <div>
                  <dt>Level</dt>
                  <dd>{selected.level}</dd>
                </div>
                <div>
                  <dt>Recorded</dt>
                  <dd>{new Date(selected.time).toLocaleString()}</dd>
                </div>
              </dl>
              <details>
                <summary>Sanitized metadata</summary>
                <pre>{JSON.stringify(selected.metadata, null, 2)}</pre>
              </details>
            </div>
          )}
    </section>
  );
}
