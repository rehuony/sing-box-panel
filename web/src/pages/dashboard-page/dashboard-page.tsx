import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import type { Task, TaskPage } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { PageHeading } from '@/components/page-heading';
import { useControlPlane } from '@/stores/control-plane.store';

import './dashboard-page.css';

function formatTimestamp(timestamp: string): string {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(timestamp));
}

function taskStatusLabel(task: Task): string {
  if (task.cancel_requested && (task.status === 'queued' || task.status === 'running')) {
    return 'canceling';
  }
  return task.status;
}

export function DashboardPage() {
  const client = useApiClient();
  const controlPlane = useControlPlane();
  const [taskPage, setTaskPage] = useState<TaskPage | null>(null);
  const [taskError, setTaskError] = useState<unknown>(null);

  const loadTasks = useCallback(
    async (signal?: AbortSignal) => {
      try {
        setTaskError(null);
        setTaskPage(await client.listTasks({ limit: 5 }, signal));
      } catch (error) {
        if (
          signal?.aborted ||
          (error instanceof DOMException && error.name === 'AbortError')
        ) {
          return;
        }
        setTaskError(error);
      }
    },
    [client],
  );

  useEffect(() => {
    const controller = new AbortController();
    void loadTasks(controller.signal);
    return () => controller.abort();
  }, [loadTasks]);

  if (controlPlane.status !== 'ready') {
    return null;
  }

  const { context } = controlPlane;
  const runningVersion = context.running?.exactVersion ?? 'Stopped';

  return (
    <div className="page-stack">
      <PageHeading
        action={
          <button
            className="button button--secondary"
            onClick={() => {
              void controlPlane.refresh();
              void loadTasks();
            }}
            type="button"
          >
            Refresh state
          </button>
        }
        eyebrow="Operations / overview"
        summary="The saved revision and the running core remain separate until an immutable bundle is applied."
        title="One machine. One exact state."
      />

      <section className="status-ledger" aria-label="Current panel status">
        <div className="status-ledger__row status-ledger__row--running">
          <span className="status-ledger__key">RUN</span>
          <div>
            <span>Runtime identity</span>
            <strong>{runningVersion}</strong>
          </div>
          <span
            className={`state-label ${
              context.running === null
                ? 'state-label--neutral'
                : 'state-label--success'
            }`}
          >
            <span aria-hidden="true" />
            {context.running === null ? 'Stopped' : 'Observed live'}
          </span>
          <small>
            {context.running?.artifactName ?? 'No core process is currently active.'}
          </small>
        </div>

        <div className="status-ledger__row">
          <span className="status-ledger__key">CFG</span>
          <div>
            <span>Canonical configuration</span>
            <strong>Revision #{context.canonical.revision}</strong>
          </div>
          <span
            className={`state-label ${
              context.canonical.hasUnappliedChanges
                ? 'state-label--warning'
                : 'state-label--neutral'
            }`}
          >
            <span aria-hidden="true" />
            {context.canonical.hasUnappliedChanges ? 'Pending apply' : 'In sync'}
          </span>
          <small>Saved {formatTimestamp(context.canonical.savedAt)}</small>
        </div>

        <div className="status-ledger__row">
          <span className="status-ledger__key">BND</span>
          <div>
            <span>Applied bundle</span>
            <strong>
              {context.applied === null
                ? 'No bundle'
                : context.applied.bundle}
            </strong>
          </div>
          <span className="state-label state-label--neutral">
            <span aria-hidden="true" /> Immutable
          </span>
          <small>
            {context.applied === null
              ? 'Prepare and check a bundle before applying.'
              : `Revision #${context.applied.revision} · ${formatTimestamp(
                  context.applied.appliedAt,
                )}`}
          </small>
        </div>
      </section>

      <div className="overview-columns">
        <section className="operation-panel" aria-labelledby="recent-tasks-title">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Durable queue</p>
              <h2 id="recent-tasks-title">Recent tasks</h2>
            </div>
            <Link className="text-link" to="/tasks">
              Open queue
            </Link>
          </div>

          {taskError === null ? null : <ErrorNotice error={taskError} />}
          {taskPage?.items.length === 0 ? (
            <div className="empty-state">
              <strong>No tasks have been recorded.</strong>
              <p>Catalog refreshes, checks and runtime intents will appear here.</p>
            </div>
          ) : null}
          {taskPage === null || taskPage.items.length === 0 ? null : (
            <ul className="task-peek">
              {taskPage.items.map((task) => (
                <li key={task.id}>
                  <span className={`task-mark task-mark--${task.status}`} />
                  <div>
                    <strong>{task.kind}</strong>
                    <span>{task.lane} lane · attempt {task.attempt}</span>
                  </div>
                  <span className="mono-label">{taskStatusLabel(task)}</span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="operation-panel" aria-labelledby="next-action-title">
          <div className="section-heading">
            <div>
              <p className="eyebrow">Safe next move</p>
              <h2 id="next-action-title">
                {context.canonical.hasUnappliedChanges
                  ? 'Review saved changes'
                  : 'Choose the exact core view'}
              </h2>
            </div>
          </div>
          <p className="operation-panel__copy">
            {context.canonical.hasUnappliedChanges
              ? 'The canonical revision is newer than the applied bundle. Runtime actions remain separate from editing.'
              : 'Capability and configuration forms are always resolved against an exact sing-box version.'}
          </p>
          <div className="inline-actions">
            <Link className="button button--primary" to="/configuration">
              Open configuration
            </Link>
            <Link className="button button--secondary" to="/cores">
              Inspect versions
            </Link>
          </div>
        </section>
      </div>
    </div>
  );
}
