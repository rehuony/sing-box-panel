// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) ClaimTask(ctx context.Context, input ClaimTaskInput) (*Task, error) {
	if input.Lane != TaskLaneRuntime && input.Lane != TaskLaneMaintenance {
		return nil, fmt.Errorf("invalid task lane %q", input.Lane)
	}
	if strings.TrimSpace(input.LeaseOwner) == "" {
		return nil, errors.New("task lease owner is empty")
	}
	if input.Now.IsZero() {
		return nil, errors.New("task claim time is zero")
	}
	if input.LeaseDuration <= 0 {
		return nil, errors.New("task lease duration must be positive")
	}

	now := input.Now.UTC()
	leaseExpiresAt := now.Add(input.LeaseDuration)
	leaseOwner, err := newLeaseOwner(input.LeaseOwner)
	if err != nil {
		return nil, err
	}
	var claimed *Task
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := normalizeClaimableTasks(ctx, tx, input.Lane, now); err != nil {
			return err
		}

		var taskID string
		err := tx.QueryRowContext(
			ctx,
			`SELECT candidate.id
                 FROM tasks AS candidate
                WHERE candidate.lane = ?
                  AND candidate.status = 'queued'
                  AND candidate.cancel_requested = 0
                  AND (
                        candidate.not_before IS NULL
                        OR candidate.not_before <= ?
                      )
                  AND (
                        candidate.lane <> 'runtime'
                        OR candidate.generation = (
                            SELECT target_generation FROM hub_state WHERE singleton = 1
                        )
                      )
                  AND NOT EXISTS (
                        SELECT 1 FROM tasks AS active
                         WHERE active.lane = candidate.lane
                           AND active.status = 'running'
                      )
                ORDER BY julianday(candidate.created_at), candidate.id
                LIMIT 1`,
			string(input.Lane),
			formatTaskTime(now),
		).Scan(&taskID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("select claimable task: %w", err)
		}

		result, err := tx.ExecContext(
			ctx,
			`UPDATE tasks
                    SET status = 'running', attempt = attempt + 1,
                        lease_owner = ?, lease_expires_at = ?, updated_at = ?
                  WHERE id = ? AND status = 'queued'`,
			leaseOwner,
			formatTaskTime(leaseExpiresAt),
			formatTaskTime(now),
			taskID,
		)
		if err != nil {
			return fmt.Errorf("claim task: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect task claim: %w", err)
		}
		if rows != 1 {
			return ErrTaskLeaseLost
		}

		task, err := getTask(ctx, tx, taskID)
		if err != nil {
			return err
		}
		claimed = &task
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func normalizeClaimableTasks(
	ctx context.Context,
	tx *sql.Tx,
	lane TaskLane,
	now time.Time,
) error {
	nowText := formatTaskTime(now)
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE tasks
            SET status = CASE
                    WHEN lane = 'runtime' AND generation < (
                        SELECT target_generation FROM hub_state WHERE singleton = 1
                    ) THEN 'superseded'
                    WHEN cancel_requested = 1 THEN 'canceled'
                    ELSE 'queued'
                END,
                lease_owner = NULL,
                lease_expires_at = NULL,
                updated_at = ?
          WHERE lane = ? AND status = 'running'
            AND lease_expires_at IS NOT NULL
            AND lease_expires_at <= ?`,
		nowText,
		string(lane),
		nowText,
	); err != nil {
		return fmt.Errorf("reclaim expired task leases: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE tasks
            SET status = 'canceled', updated_at = ?
          WHERE lane = ? AND status = 'queued' AND cancel_requested = 1`,
		nowText,
		string(lane),
	); err != nil {
		return fmt.Errorf("finalize queued task cancellations: %w", err)
	}
	if lane == TaskLaneRuntime {
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE tasks
                SET status = 'superseded', updated_at = ?
              WHERE lane = 'runtime' AND status = 'queued'
                AND generation < (
                    SELECT target_generation FROM hub_state WHERE singleton = 1
                )`,
			nowText,
		); err != nil {
			return fmt.Errorf("supersede stale queued runtime tasks: %w", err)
		}
	}
	return nil
}

// HeartbeatTask renews a live lease and reports cancellation/supersession.
// Renewing during cleanup is intentional: a handler owns the task until it
// reaches a safe boundary and completes it.
func (s *Store) HeartbeatTask(
	ctx context.Context,
	taskID string,
	leaseOwner string,
	now time.Time,
	leaseDuration time.Duration,
) (TaskLeaseState, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(leaseOwner) == "" {
		return TaskLeaseState{}, errors.New("task id and lease owner are required")
	}
	if now.IsZero() || leaseDuration <= 0 {
		return TaskLeaseState{}, errors.New("heartbeat time and positive lease duration are required")
	}
	now = now.UTC()
	leaseExpiresAt := now.Add(leaseDuration)

	var state TaskLeaseState
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		task, err := getTask(ctx, tx, taskID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				return ErrTaskLeaseLost
			}
			return err
		}
		if task.Status != TaskStatusRunning || task.LeaseOwner != leaseOwner ||
			task.LeaseExpiresAt == nil || !task.LeaseExpiresAt.After(now) {
			return ErrTaskLeaseLost
		}

		superseded := false
		if task.Lane == TaskLaneRuntime {
			var targetGeneration int64
			if err := tx.QueryRowContext(
				ctx,
				`SELECT target_generation FROM hub_state WHERE singleton = 1`,
			).Scan(&targetGeneration); err != nil {
				return fmt.Errorf("read runtime target generation: %w", err)
			}
			superseded = task.Generation < targetGeneration
		}
		cancelRequested := task.CancelRequested || superseded

		result, err := tx.ExecContext(
			ctx,
			`UPDATE tasks
                    SET cancel_requested = ?, lease_expires_at = ?, updated_at = ?
                  WHERE id = ? AND status = 'running' AND lease_owner = ?`,
			boolInt(cancelRequested),
			formatTaskTime(leaseExpiresAt),
			formatTaskTime(now),
			taskID,
			leaseOwner,
		)
		if err != nil {
			return fmt.Errorf("heartbeat task lease: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect task heartbeat: %w", err)
		}
		if rows != 1 {
			return ErrTaskLeaseLost
		}

		state = TaskLeaseState{
			CancelRequested: cancelRequested,
			Superseded:      superseded,
			LeaseExpiresAt:  leaseExpiresAt,
		}
		return nil
	})
	return state, err
}

// RequestTaskCancellation cancels queued work immediately and flags running
// work for cancellation at the handler's next explicit safe boundary.
