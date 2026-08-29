import type { HttpApiContext } from './shared';
import type { ApiClient, Task, TaskFilter, TaskPage } from '../api-client';

export function createTasksHttpApi(context: HttpApiContext) {
  const {
    baseUrl, buildQuery, fetcher, request, writeHeaders,
  } = context;
  return {
    listTasks(filter: TaskFilter = {}, signal) {
      const query = buildQuery({
        before_id: filter.beforeID,
        before_time: filter.beforeTime,
        kind: filter.kind,
        lane: filter.lane,
        limit: filter.limit ?? 50,
        status: filter.status,
      });
      return request<TaskPage>(fetcher, `${baseUrl}/tasks${query}`, {
        method: 'GET',
        signal,
      });
    },
    getTask(taskID, signal) {
      return request<Task>(fetcher, `${baseUrl}/tasks/${encodeURIComponent(taskID)}`, {
        method: 'GET', signal,
      });
    },
    cancelTask(taskID, signal) {
      return request<Task>(
        fetcher,
        `${baseUrl}/tasks/${encodeURIComponent(taskID)}/cancel`,
        { method: 'POST', headers: writeHeaders(), signal },
      );
    },

  } satisfies Partial<ApiClient>;
}
