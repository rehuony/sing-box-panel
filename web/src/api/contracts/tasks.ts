import type { Task, TaskStatus } from '../generated';

export type { Task, TaskPage, TaskStatus } from '../generated';

export interface TaskFilter {
  limit?: number;
  kind?: Task['kind'];
  lane?: Task['lane'];
  status?: TaskStatus;
}
