import type { ApiClient, Task } from '@/api/api-client';

// Only observes the accepted task; aborting observation never cancels server work.
export async function waitForTask(
  client: ApiClient,
  initial: Task,
  signal: AbortSignal,
  onUpdate?: (task: Task) => void,
): Promise<Task> {
  let task = initial;
  while (!signal.aborted) {
    onUpdate?.(task);
    if (['succeeded', 'failed', 'canceled', 'superseded'].includes(task.status)) return task;
    await new Promise<void>((resolve, reject) => {
      let timer: ReturnType<typeof setTimeout>;
      const cancel = () => {
        clearTimeout(timer);
        reject(signal.reason ?? new DOMException('Aborted', 'AbortError'));
      };
      timer = setTimeout(() => {
        signal.removeEventListener('abort', cancel);
        resolve();
      }, 1000);
      signal.addEventListener('abort', cancel, { once: true });
    });
    task = await client.getTask(task.id, signal);
  }
  throw signal.reason ?? new DOMException('Aborted', 'AbortError');
}
