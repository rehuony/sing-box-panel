import { useCallback, useEffect, useState } from 'react';

import type { Task, TaskPage, TaskStatus } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { PageHeading } from '@/components/page-heading';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';

import './tasks-page.css';

type LaneFilter = '' | Task['lane'];
type StatusFilter = '' | TaskStatus;

function formatTimestamp(timestamp: string): string {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(new Date(timestamp));
}

function terminal(task: Task): boolean {
  return ['succeeded', 'failed', 'canceled', 'superseded'].includes(task.status);
}

function describeFailure(task: Task): string | null {
  if (task.failure === undefined || task.failure === null) {
    return null;
  }
  if (typeof task.failure === 'string') {
    return task.failure;
  }
  try {
    return JSON.stringify(task.failure);
  } catch {
    return 'Task failure details could not be displayed.';
  }
}

export function TasksPage() {
  const client = useApiClient();
  const [lane, setLane] = useState<LaneFilter>('');
  const [status, setStatus] = useState<StatusFilter>('');
  const [tasks, setTasks] = useState<TaskPage | null>(null);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [actionError, setActionError] = useState('');
  const [cancelingID, setCancelingID] = useState<string | null>(null);
  const [expandedID, setExpandedID] = useState<string | null>(null);

  const loadTasks = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setLoadError(null);
        setTasks(
          await client.listTasks(
            {
              lane: lane === '' ? undefined : lane,
              limit: 100,
              status: status === '' ? undefined : status,
            },
            signal,
          ),
        );
      } catch (error) {
        if (
          signal?.aborted
          || (error instanceof DOMException && error.name === 'AbortError')
        ) {
          return;
        }
        setLoadError(error);
      }
    },
    [client, lane, status],
  );

  useEffect(() => {
    const controller = new AbortController();
    void loadTasks(controller.signal);
    return () => controller.abort();
  }, [loadTasks]);

  async function cancel(task: Task) {
    setCancelingID(task.id);
    setActionError('');
    try {
      await client.cancelTask(task.id);
      await loadTasks();
    } catch (error) {
      setActionError(describeRequestError(error));
    } finally {
      setCancelingID(null);
    }
  }

  return (
    <div className='page-stack'>
      <PageHeading
        action={(
          <button className='button button--secondary' onClick={() => void loadTasks()} type='button'>
            Refresh queue
          </button>
        )}
        eyebrow='Operations / durable tasks'
        summary='Runtime intents are serialized separately from maintenance work. Cancellation is cooperative and never reports success before the worker stops.'
        title='Every slow action leaves a record.'
      />

      <section className='task-console' aria-labelledby='task-queue-title'>
        <div className='task-console__toolbar'>
          <div>
            <p className='eyebrow'>Current queue</p>
            <h2 id='task-queue-title'>Tasks</h2>
          </div>
          <div className='filter-row'>
            <label>
              Lane
              <select value={lane} onChange={(event) => setLane(event.target.value as LaneFilter)}>
                <option value=''>All lanes</option>
                <option value='runtime'>Runtime</option>
                <option value='maintenance'>Maintenance</option>
              </select>
            </label>
            <label>
              Status
              <select value={status} onChange={(event) => setStatus(event.target.value as StatusFilter)}>
                <option value=''>All states</option>
                <option value='queued'>Queued</option>
                <option value='running'>Running</option>
                <option value='succeeded'>Succeeded</option>
                <option value='failed'>Failed</option>
                <option value='canceled'>Canceled</option>
                <option value='superseded'>Superseded</option>
              </select>
            </label>
          </div>
        </div>

        {actionError === ''
          ? null
          : (
              <div className='notice notice--error' role='alert'>
                <strong>Cancellation was not accepted</strong>
                <p>{actionError}</p>
              </div>
            )}
        {loadError === null ? null : <ErrorNotice error={loadError} title='Task queue is unavailable' />}
        {tasks === null && loadError === null
          ? (
              <div className='inline-loading' aria-busy='true'>Reading durable task records…</div>
            )
          : null}
        {tasks?.items.length === 0
          ? (
              <div className='empty-state'>
                <strong>No tasks match these filters.</strong>
                <p>Change the lane or status filter to inspect a different part of the queue.</p>
              </div>
            )
          : null}

        {tasks === null || tasks.items.length === 0
          ? null
          : (
              <div className='task-list'>
                {tasks.items.map((task) => {
                  const expanded = expandedID === task.id;
                  const failure = describeFailure(task);
                  return (
                    <article className={`task-row task-row--${task.status}`} key={task.id}>
                      <button
                        aria-expanded={expanded}
                        className='task-row__summary'
                        onClick={() => setExpandedID(expanded ? null : task.id)}
                        type='button'
                      >
                        <span className={`task-mark task-mark--${task.status}`} aria-hidden='true' />
                        <span className='task-row__identity'>
                          <strong>{task.kind}</strong>
                          <code>{task.id}</code>
                        </span>
                        <span className='task-row__lane'>{task.lane}</span>
                        <span className={`task-status task-status--${task.status}`}>
                          {task.cancel_requested && !terminal(task) ? 'canceling' : task.status}
                        </span>
                        <time>{formatTimestamp(task.updated_at)}</time>
                        <span className='task-row__chevron' aria-hidden='true'>⌄</span>
                      </button>
                      {expanded
                        ? (
                            <div className='task-row__detail'>
                              <dl>
                                <div>
                                  <dt>Generation</dt>
                                  <dd>{task.generation}</dd>
                                </div>
                                <div>
                                  <dt>Attempt</dt>
                                  <dd>{task.attempt}</dd>
                                </div>
                                <div>
                                  <dt>Created</dt>
                                  <dd>{formatTimestamp(task.created_at)}</dd>
                                </div>
                                <div>
                                  <dt>Revision</dt>
                                  <dd>{task.canonical_revision_id ?? 'not bound'}</dd>
                                </div>
                              </dl>
                              {failure === null
                                ? null
                                : (
                                    <div className='task-failure' role='status'>
                                      <strong>Failure detail</strong>
                                      <code>{failure}</code>
                                    </div>
                                  )}
                              <div className='inline-actions'>
                                <button
                                  className='button button--warning'
                                  disabled={terminal(task) || task.cancel_requested || cancelingID === task.id}
                                  onClick={() => void cancel(task)}
                                  type='button'
                                >
                                  {cancelingID === task.id ? 'Requesting…' : 'Request cancellation'}
                                </button>
                                {terminal(task) ? <small>Terminal tasks cannot be canceled.</small> : null}
                              </div>
                            </div>
                          )
                        : null}
                    </article>
                  );
                })}
              </div>
            )}
      </section>
    </div>
  );
}
