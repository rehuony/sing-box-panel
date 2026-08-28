// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) RequestTaskCancellation(
	ctx context.Context,
	taskID string,
	now time.Time,
) (Task, error) {
	if strings.TrimSpace(taskID) == "" {
		return Task{}, errors.New("task id is empty")
	}
	if now.IsZero() {
		return Task{}, errors.New("task cancellation time is zero")
	}
	now = now.UTC()

	var task Task
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getTask(ctx, tx, taskID)
		if err != nil {
			return err
		}
		switch current.Status {
		case TaskStatusQueued:
			_, err = tx.ExecContext(
				ctx,
				`UPDATE tasks
                        SET status = 'canceled', cancel_requested = 1, updated_at = ?
                      WHERE id = ? AND status = 'queued'`,
				formatTaskTime(now),
				taskID,
			)
		case TaskStatusRunning:
			_, err = tx.ExecContext(
				ctx,
				`UPDATE tasks
                        SET cancel_requested = 1, updated_at = ?
                      WHERE id = ? AND status = 'running'`,
				formatTaskTime(now),
				taskID,
			)
		default:
			// Terminal cancellation is idempotent.
		}
		if err != nil {
			return fmt.Errorf("request task cancellation: %w", err)
		}
		task, err = getTask(ctx, tx, taskID)
		return err
	})
	return task, err
}

// CompleteTask commits the terminal state only for the current, unexpired
// lease owner. A newer runtime generation or cancellation request wins over a
// late successful handler result.
func (s *Store) CompleteTask(
	ctx context.Context,
	taskID string,
	leaseOwner string,
	now time.Time,
	completion TaskCompletion,
) (Task, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(leaseOwner) == "" {
		return Task{}, errors.New("task id and lease owner are required")
	}
	if now.IsZero() {
		return Task{}, errors.New("task completion time is zero")
	}
	if len(completion.Result) != 0 && !json.Valid(completion.Result) {
		return Task{}, errors.New("task result is not valid JSON")
	}
	if len(completion.Failure) != 0 && !json.Valid(completion.Failure) {
		return Task{}, errors.New("task failure is not valid JSON")
	}
	now = now.UTC()

	var completed Task
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getTask(ctx, tx, taskID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				return ErrTaskLeaseLost
			}
			return err
		}
		if current.Status != TaskStatusRunning || current.LeaseOwner != leaseOwner ||
			current.LeaseExpiresAt == nil || !current.LeaseExpiresAt.After(now) {
			return ErrTaskLeaseLost
		}

		status := TaskStatusFailed
		resultJSON := nullableJSON(completion.Result)
		failureJSON := nullableJSON(completion.Failure)
		if completion.Succeeded {
			status = TaskStatusSucceeded
			failureJSON = nil
		}
		if current.CancelRequested {
			status = TaskStatusCanceled
			resultJSON = nil
			failureJSON = nil
		}
		if current.Lane == TaskLaneRuntime {
			var targetGeneration int64
			if err := tx.QueryRowContext(
				ctx,
				`SELECT target_generation FROM hub_state WHERE singleton = 1`,
			).Scan(&targetGeneration); err != nil {
				return fmt.Errorf("read runtime target generation: %w", err)
			}
			if current.Generation < targetGeneration {
				status = TaskStatusSuperseded
				resultJSON = nil
				failureJSON = nil
			}
		}
		if status == TaskStatusSucceeded && current.Lane == TaskLaneRuntime {
			if err := commitSuccessfulRuntimeIntent(ctx, tx, current, now); err != nil {
				return err
			}
		}

		result, err := tx.ExecContext(
			ctx,
			`UPDATE tasks
                    SET status = ?, result_json = ?, error_json = ?,
                        lease_owner = NULL, lease_expires_at = NULL, updated_at = ?
                  WHERE id = ? AND status = 'running' AND lease_owner = ?`,
			string(status),
			resultJSON,
			failureJSON,
			formatTaskTime(now),
			taskID,
			leaseOwner,
		)
		if err != nil {
			return fmt.Errorf("complete task: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect task completion: %w", err)
		}
		if rows != 1 {
			return ErrTaskLeaseLost
		}
		completed, err = getTask(ctx, tx, taskID)
		return err
	})
	return completed, err
}

// GetTask returns one durable task by identifier.
