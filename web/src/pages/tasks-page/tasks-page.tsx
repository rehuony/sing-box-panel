import { useTranslation } from 'react-i18next';
import { useCallback, useEffect, useRef, useState } from 'react';
import { CheckCircle2, Clock3, RefreshCw, XCircle } from 'lucide-react';

import type { Task, TaskPage, TaskStatus } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { useApiClient } from '@/api/api-client-context';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';

import './tasks-page.css';

type LaneFilter = '' | Task['lane'];
type StatusFilter = '' | TaskStatus;

function terminal(task: Task) {
  return ['succeeded', 'failed', 'canceled', 'superseded'].includes(task.status);
}

function formatTimestamp(timestamp: string, locale: string) {
  return new Intl.DateTimeFormat(locale, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(timestamp));
}

function formatTaskData(value: unknown) {
  if (value === undefined || value === null) return null;
  if (typeof value === 'string') return value;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function StatusIcon({ status }: { status: TaskStatus }) {
  if (status === 'succeeded') return <CheckCircle2 aria-hidden='true' />;
  if (status === 'failed' || status === 'canceled') return <XCircle aria-hidden='true' />;
  return <Clock3 aria-hidden='true' />;
}

export function TasksPage() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const [lane, setLane] = useState<LaneFilter>('');
  const [status, setStatus] = useState<StatusFilter>('');
  const [kindInput, setKindInput] = useState('');
  const [kind, setKind] = useState('');
  const [tasks, setTasks] = useState<TaskPage | null>(null);
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [detailError, setDetailError] = useState('');
  const [actionMessage, setActionMessage] = useState('');
  const [refreshing, setRefreshing] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [canceling, setCanceling] = useState(false);
  const [confirmCancel, setConfirmCancel] = useState(false);
  const generationRef = useRef(0);
  const detailControllerRef = useRef<AbortController | null>(null);
  const cancelControllerRef = useRef<AbortController | null>(null);
  const cancelGenerationRef = useRef(0);
  const selectedTaskRef = useRef<Task | null>(null);
  const loadingOlderRef = useRef(false);

  useEffect(() => {
    const timer = window.setTimeout(setKind, 300, kindInput);
    return () => window.clearTimeout(timer);
  }, [kindInput]);

  const loadTasks = useCallback(async (
    signal?: AbortSignal,
    cursor?: NonNullable<TaskPage['next']>,
    append = false,
  ) => {
    const generation = ++generationRef.current;
    if (!append) setTasks(null);
    try {
      setLoadError(null);
      const page = await client.listTasks({
        beforeID: cursor?.id,
        beforeTime: cursor?.created_at,
        kind: kind.trim() === '' ? undefined : kind.trim() as Task['kind'],
        lane: lane === '' ? undefined : lane,
        limit: 100,
        status: status === '' ? undefined : status,
      }, signal);
      if (signal?.aborted || generation !== generationRef.current) return;
      setTasks((current) => append && current !== null
        ? { items: [...current.items, ...page.items], next: page.next }
        : page);
    } catch (error) {
      if (signal?.aborted || generation !== generationRef.current) return;
      setLoadError(error);
    }
  }, [client, kind, lane, status]);

  useEffect(() => {
    const controller = new AbortController();
    void loadTasks(controller.signal);
    return () => controller.abort();
  }, [loadTasks]);

  useEffect(() => () => {
    detailControllerRef.current?.abort();
    cancelControllerRef.current?.abort();
  }, []);

  async function openDetail(task: Task) {
    detailControllerRef.current?.abort();
    cancelControllerRef.current?.abort();
    cancelControllerRef.current = null;
    cancelGenerationRef.current += 1;
    const controller = new AbortController();
    detailControllerRef.current = controller;
    selectedTaskRef.current = task;
    setSelectedTask(task);
    setDetailLoading(true);
    setDetailError('');
    setCanceling(false);
    setConfirmCancel(false);
    try {
      const detail = await client.getTask(task.id, controller.signal);
      if (!controller.signal.aborted && selectedTaskRef.current?.id === task.id) {
        selectedTaskRef.current = detail;
        setSelectedTask(detail);
        setTasks((current) => current === null
          ? current
          : {
              ...current,
              items: current.items.map((item) => item.id === detail.id ? detail : item),
            });
      }
    } catch (error) {
      if (!controller.signal.aborted && selectedTaskRef.current?.id === task.id) {
        setDetailError(describeRequestError(error));
      }
    } finally {
      if (!controller.signal.aborted && selectedTaskRef.current?.id === task.id) {
        setDetailLoading(false);
      }
    }
  }

  async function refresh() {
    setRefreshing(true);
    try {
      await loadTasks();
      const currentSelection = selectedTaskRef.current;
      if (currentSelection !== null) await openDetail(currentSelection);
    } finally {
      setRefreshing(false);
    }
  }

  async function cancelSelected() {
    const task = selectedTaskRef.current;
    if (task === null) return;
    cancelControllerRef.current?.abort();
    const controller = new AbortController();
    const generation = ++cancelGenerationRef.current;
    cancelControllerRef.current = controller;
    setCanceling(true);
    setDetailError('');
    try {
      const updated = await client.cancelTask(task.id, controller.signal);
      if (
        controller.signal.aborted
        || generation !== cancelGenerationRef.current
        || selectedTaskRef.current?.id !== task.id
      ) {
        return;
      }
      selectedTaskRef.current = updated;
      setSelectedTask(updated);
      setActionMessage(updated.cancel_requested && !terminal(updated)
        ? t('tasks.cancel.pending', { defaultValue: 'Cancellation requested. The worker will report the final state.' })
        : t('tasks.cancel.complete', {
            defaultValue: 'Task is now {{status}}.',
            status: t(`telemetry.taskStatus.${updated.status}`, { defaultValue: updated.status }),
          }));
      await loadTasks();
    } catch (error) {
      if (
        !controller.signal.aborted
        && generation === cancelGenerationRef.current
        && selectedTaskRef.current?.id === task.id
      ) {
        setDetailError(describeRequestError(error));
      }
    } finally {
      if (
        !controller.signal.aborted
        && generation === cancelGenerationRef.current
        && selectedTaskRef.current?.id === task.id
      ) {
        cancelControllerRef.current = null;
        setCanceling(false);
        setConfirmCancel(false);
      }
    }
  }

  async function loadOlder() {
    if (tasks?.next === undefined || loadingOlderRef.current) return;
    loadingOlderRef.current = true;
    setLoadingOlder(true);
    try {
      await loadTasks(undefined, tasks.next, true);
    } finally {
      loadingOlderRef.current = false;
      setLoadingOlder(false);
    }
  }

  return (
    <div className='tasks-page'>
      <header className='tasks-page__heading'>
        <div>
          <h1>{t('tasks.title', { defaultValue: 'Task history' })}</h1>
          <p>{t('tasks.subtitle', { defaultValue: 'Durable operator actions and their worker-reported outcomes.' })}</p>
        </div>
        <Button disabled={refreshing} onClick={() => void refresh()} variant='outline'>
          <RefreshCw aria-hidden='true' />
          {refreshing
            ? t('tasks.refreshing', { defaultValue: 'Refreshing…' })
            : t('tasks.refresh', { defaultValue: 'Refresh' })}
        </Button>
      </header>

      <section className='tasks-console' aria-labelledby='task-list-title'>
        <div className='tasks-filter-row'>
          <h2 id='task-list-title'>{t('tasks.list.title', { defaultValue: 'Operations' })}</h2>
          <label>
            <span>{t('tasks.filter.kind', { defaultValue: 'Kind' })}</span>
            <Input onChange={(event) => setKindInput(event.target.value)} placeholder={t('tasks.filter.kindPlaceholder', { defaultValue: 'All kinds' })} value={kindInput} />
          </label>
          <label>
            <span>{t('tasks.filter.lane', { defaultValue: 'Lane' })}</span>
            <select onChange={(event) => setLane(event.target.value as LaneFilter)} value={lane}>
              <option value=''>{t('tasks.filter.allLanes', { defaultValue: 'All lanes' })}</option>
              <option value='runtime'>{t('tasks.lane.runtime', { defaultValue: 'Runtime' })}</option>
              <option value='maintenance'>{t('tasks.lane.maintenance', { defaultValue: 'Maintenance' })}</option>
            </select>
          </label>
          <label>
            <span>{t('tasks.filter.status', { defaultValue: 'Status' })}</span>
            <select onChange={(event) => setStatus(event.target.value as StatusFilter)} value={status}>
              <option value=''>{t('tasks.filter.allStates', { defaultValue: 'All states' })}</option>
              {(['queued', 'running', 'succeeded', 'failed', 'canceled', 'superseded'] as const).map((value) => (
                <option key={value} value={value}>{t(`telemetry.taskStatus.${value}`, { defaultValue: value })}</option>
              ))}
            </select>
          </label>
        </div>

        {actionMessage === '' ? null : <div className='notice notice--success' role='status'>{actionMessage}</div>}
        {loadError === null ? null : <ErrorNotice error={loadError} title={t('tasks.error.load', { defaultValue: 'Task history is unavailable' })} />}
        {tasks === null && loadError === null ? <div className='inline-loading'>{t('tasks.loading', { defaultValue: 'Loading tasks…' })}</div> : null}
        {tasks?.items.length === 0 ? <div className='empty-state'>{t('tasks.empty', { defaultValue: 'No tasks match these filters.' })}</div> : null}

        {tasks === null
          ? null
          : (
              <div className='tasks-list'>
                {tasks.items.map((task) => (
                  <button className='task-row' key={task.id} onClick={() => void openDetail(task)} type='button'>
                    <span className={`task-row__state task-row__state--${task.status}`}><StatusIcon status={task.status} /></span>
                    <span className='task-row__identity'>
                      <strong>{task.kind}</strong>
                      <code>{task.id}</code>
                    </span>
                    <span className='task-row__meta'>{t(`tasks.lane.${task.lane}`, { defaultValue: task.lane })}</span>
                    <span className={`task-status task-status--${task.status}`}>{t(`telemetry.taskStatus.${task.status}`, { defaultValue: task.status })}</span>
                    <time dateTime={task.updated_at}>{formatTimestamp(task.updated_at, locale)}</time>
                  </button>
                ))}
              </div>
            )}

        {tasks?.next === undefined
          ? null
          : (
              <footer className='tasks-console__footer'>
                <Button disabled={loadingOlder} onClick={() => void loadOlder()} variant='outline'>
                  {loadingOlder
                    ? t('tasks.loadingOlder', { defaultValue: 'Loading older tasks…' })
                    : t('tasks.loadOlder', { defaultValue: 'Load older tasks' })}
                </Button>
              </footer>
            )}
      </section>

      <Sheet open={selectedTask !== null} onOpenChange={(open) => {
        if (!open) {
          detailControllerRef.current?.abort();
          cancelControllerRef.current?.abort();
          cancelControllerRef.current = null;
          cancelGenerationRef.current += 1;
          selectedTaskRef.current = null;
          setSelectedTask(null);
          setDetailLoading(false);
          setCanceling(false);
          setConfirmCancel(false);
        }
      }}>
        <SheetContent className='task-sheet' side='right'>
          <SheetHeader>
            <SheetTitle>{selectedTask?.kind ?? t('tasks.detail.title', { defaultValue: 'Task details' })}</SheetTitle>
            <SheetDescription>{selectedTask?.id}</SheetDescription>
          </SheetHeader>
          {detailLoading ? <div className='inline-loading'>{t('tasks.detail.loading', { defaultValue: 'Loading exact task…' })}</div> : null}
          {detailError === '' ? null : <div className='notice notice--error' role='alert'>{detailError}</div>}
          {selectedTask === null
            ? null
            : (
                <div className='task-sheet__body'>
                  <dl>
                    <div>
                      <dt>{t('tasks.detail.status', { defaultValue: 'Status' })}</dt>
                      <dd>{t(`telemetry.taskStatus.${selectedTask.status}`, { defaultValue: selectedTask.status })}</dd>
                    </div>
                    <div>
                      <dt>{t('tasks.detail.lane', { defaultValue: 'Lane' })}</dt>
                      <dd>{t(`tasks.lane.${selectedTask.lane}`, { defaultValue: selectedTask.lane })}</dd>
                    </div>
                    <div>
                      <dt>{t('tasks.detail.attempt', { defaultValue: 'Attempt' })}</dt>
                      <dd>{new Intl.NumberFormat(locale).format(selectedTask.attempt)}</dd>
                    </div>
                    <div>
                      <dt>{t('tasks.detail.generation', { defaultValue: 'Generation' })}</dt>
                      <dd>{new Intl.NumberFormat(locale).format(selectedTask.generation)}</dd>
                    </div>
                    <div>
                      <dt>{t('tasks.detail.created', { defaultValue: 'Created' })}</dt>
                      <dd>{formatTimestamp(selectedTask.created_at, locale)}</dd>
                    </div>
                    <div>
                      <dt>{t('tasks.detail.updated', { defaultValue: 'Updated' })}</dt>
                      <dd>{formatTimestamp(selectedTask.updated_at, locale)}</dd>
                    </div>
                  </dl>
                  {([['payload', selectedTask.payload], ['result', selectedTask.result], ['failure', selectedTask.failure]] as const).map(([label, value]) => {
                    const formatted = formatTaskData(value);
                    return formatted === null
                      ? null
                      : (
                          <section key={label}>
                            <h3>{t(`tasks.detail.${label}`, { defaultValue: label })}</h3>
                            <pre>{formatted}</pre>
                          </section>
                        );
                  })}
                </div>
              )}
          <SheetFooter>
            {selectedTask === null || terminal(selectedTask) || selectedTask.cancel_requested
              ? <small>{selectedTask?.cancel_requested ? t('tasks.cancel.alreadyPending', { defaultValue: 'Cancellation is already pending with the worker.' }) : t('tasks.cancel.terminal', { defaultValue: 'Terminal tasks cannot be canceled.' })}</small>
              : confirmCancel
                ? (
                    <div className='task-sheet__confirm'>
                      <Button autoFocus onClick={() => setConfirmCancel(false)} variant='outline'>{t('tasks.cancel.keep', { defaultValue: 'Keep task' })}</Button>
                      <Button disabled={canceling} onClick={() => void cancelSelected()} variant='destructive'>{t('tasks.cancel.confirm', { defaultValue: 'Confirm cancellation' })}</Button>
                    </div>
                  )
                : <Button onClick={() => setConfirmCancel(true)} variant='destructive'>{t('tasks.cancel.request', { defaultValue: 'Request cancellation' })}</Button>}
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  );
}
