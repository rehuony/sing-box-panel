import type { Task, TaskStatus } from '../generated';

export type { Task, TaskPage, TaskStatus } from '../generated';

export interface TaskFilter {
  limit?: number;
  beforeID?: string;
  beforeTime?: string;
  kind?: Task['kind'];
  lane?: Task['lane'];
  status?: TaskStatus;
}
