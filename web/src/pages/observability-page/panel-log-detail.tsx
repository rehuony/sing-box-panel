import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';

import type { PanelLog, Task } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { ErrorNotice } from '@/components/error-notice';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';

export function PanelLogDetail({
  entry,
  taskID,
  onClose,
  onTaskChange,
}: {
  entry: PanelLog | null;
  taskID: string | null;
  onClose: () => void;
  onTaskChange: (id: string) => void;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [task, setTask] = useState<Task | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const operationRef = useRef<AbortController | null>(null);
  useEffect(() => () => operationRef.current?.abort(), []);
  useEffect(() => {
    if (!taskID) return;
    const abort = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    async function load(reset = false) {
      let terminal = false;
      if (reset) {
        setTask(null);
        setError(null);
      }
      try {
        const result = await client.getTask(taskID!, abort.signal);
        if (!abort.signal.aborted) {
          setTask(result);
          setError(null);
          terminal = !['queued', 'running'].includes(result.status);
        }
      } catch (reason) {
        if (!abort.signal.aborted) setError(reason);
      } finally {
        if (!abort.signal.aborted && !terminal) timer = setTimeout(() => void load(), 2000);
      }
    }
    void load(true);
    return () => {
      abort.abort();
      clearTimeout(timer);
    };
  }, [client, taskID]);
  const canRetry
    = task
      && ['failed', 'canceled'].includes(task.status)
      && ['catalog-refresh', 'core-install', 'subscription-source-refresh'].includes(task.kind);
  const canCancel = task && ['queued', 'running'].includes(task.status) && !task.cancel_requested;
  async function act(retry: boolean) {
    if (!task) return;
    const controller = new AbortController();
    operationRef.current?.abort();
    operationRef.current = controller;
    setBusy(true);
    setError(null);
    try {
      const next = await (retry
        ? client.retryTask(task.id, controller.signal)
        : client.cancelTask(task.id, controller.signal));
      if (controller.signal.aborted) return;
      setTask(next);
      onTaskChange(next.id);
      toast.add({
        title: t(retry ? 'productLogs.retried' : 'productLogs.cancelRequested'),
        type: 'success',
      });
    } catch (reason) {
      if (!controller.signal.aborted) setError(reason);
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  }
  return (
    <Dialog
      open={!!entry || !!taskID}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className='panel-log-dialog'>
        <DialogHeader>
          <DialogTitle>{t('productLogs.details')}</DialogTitle>
        </DialogHeader>
        {error != null && <ErrorNotice error={error} title={t('productLogs.unavailable')} />}
        {taskID && !task && !error && <p>{t('productLogs.loading')}</p>}
        {task && (
          <>
            <dl className='panel-log-detail'>
              <div>
                <dt>{t('productLogs.message')}</dt>
                <dd>{t(`tasks.kind.${task.kind}`, { defaultValue: task.kind })}</dd>
              </div>
              <div>
                <dt>{t('productLogs.status')}</dt>
                <dd>{t(`telemetry.taskStatus.${task.status}`)}</dd>
              </div>
              <div>
                <dt>{t('tasks.detail.updated')}</dt>
                <dd>{new Date(task.updated_at).toLocaleString()}</dd>
              </div>
              <div>
                <dt>{t('tasks.detail.attempt')}</dt>
                <dd>{task.attempt}</dd>
              </div>
            </dl>
            <p role='status'>
              {t(task.cancel_requested && ['queued', 'running'].includes(task.status)
                ? 'tasks.cancel.pending'
                : `productLogs.summary.${task.status}`)}
            </p>
            <div className='panel-log-actions'>
              {canCancel && (
                <Button
                  disabled={busy}
                  variant='secondary'
                  onClick={() => {
                    void act(false);
                  }}
                >
                  {t('tasks.cancel.request')}
                </Button>
              )}
              {canRetry && (
                <Button
                  disabled={busy}
                  variant='secondary'
                  onClick={() => {
                    void act(true);
                  }}
                >
                  {t('productLogs.retry')}
                </Button>
              )}
            </div>
          </>
        )}
        {entry && !taskID && <p>{entry.message}</p>}
      </DialogContent>
    </Dialog>
  );
}
