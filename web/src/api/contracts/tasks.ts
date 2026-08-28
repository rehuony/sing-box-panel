export type TaskStatus
  = | 'queued'
    | 'running'
    | 'succeeded'
    | 'failed'
    | 'canceled'
    | 'superseded';

export interface Task {
  id: string;
  kind: string;
  attempt: number;
  payload: unknown;
  result?: unknown;
  failure?: unknown;
  created_at: string;
  generation: number;
  status: TaskStatus;
  updated_at: string;
  not_before?: string;
  idempotency_key?: string;
  cancel_requested: boolean;
  lease_expires_at?: string;
  startup_artifact_id?: string;
  activation_bundle_id?: string;
  canonical_revision_id?: string;
  lane: 'runtime' | 'maintenance';
}

export interface TaskPage {
  items: Task[];
  next?: {
    created_at: string;
    id: string;
  };
}

export interface TaskFilter {
  kind?: string;
  limit?: number;
  lane?: Task['lane'];
  status?: TaskStatus;
}
