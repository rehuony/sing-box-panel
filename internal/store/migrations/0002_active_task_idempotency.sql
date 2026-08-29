DROP INDEX tasks_lane_idempotency;

CREATE UNIQUE INDEX tasks_lane_idempotency
    ON tasks(lane, idempotency_key)
    WHERE idempotency_key IS NOT NULL
      AND status IN ('queued', 'running');
