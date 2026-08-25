package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrTaskNotFound           = errors.New("task not found")
	ErrTaskLeaseLost          = errors.New("task lease lost")
	ErrTaskGenerationConflict = errors.New("runtime task generation conflict")
)

type TaskStatus string

const (
	TaskStatusQueued     TaskStatus = "queued"
	TaskStatusRunning    TaskStatus = "running"
	TaskStatusSucceeded  TaskStatus = "succeeded"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusCanceled   TaskStatus = "canceled"
	TaskStatusSuperseded TaskStatus = "superseded"
)

// Task is one durable unit of runtime or maintenance work.
type Task struct {
	ID                  string
	IdempotencyKey      string
	Lane                TaskLane
	Kind                string
	Status              TaskStatus
	Generation          int64
	CanonicalRevisionID string
	StartupArtifactID   string
	ActivationBundleID  string
	Payload             json.RawMessage
	Result              json.RawMessage
	Failure             json.RawMessage
	CancelRequested     bool
	Attempt             int
	LeaseOwner          string
	LeaseExpiresAt      *time.Time
	NotBefore           *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// EnqueueTaskInput contains task data prepared outside the transaction.
// Runtime tasks are intents: their generation must be newer than hub_state's
// target_generation and atomically supersedes older runtime work.
type EnqueueTaskInput struct {
	ID                  string
	IdempotencyKey      string
	Lane                TaskLane
	Kind                string
	Generation          int64
	CanonicalRevisionID string
	StartupArtifactID   string
	ActivationBundleID  string
	Payload             json.RawMessage
	NotBefore           *time.Time
	CreatedAt           time.Time
}

// ClaimTaskInput identifies one lane worker and the lease it is requesting.
// LeaseOwner is a human-readable prefix; the returned Task.LeaseOwner is a
// unique fencing token and must be used for heartbeat and completion.
type ClaimTaskInput struct {
	Lane          TaskLane
	LeaseOwner    string
	Now           time.Time
	LeaseDuration time.Duration
}

// TaskLeaseState is returned at each heartbeat/safe cancellation boundary.
type TaskLeaseState struct {
	CancelRequested bool
	Superseded      bool
	LeaseExpiresAt  time.Time
}

// TaskCompletion is the handler result. Cancellation and generation
// supersession take precedence over this requested result.
type TaskCompletion struct {
	Succeeded bool
	Result    json.RawMessage
	Failure   json.RawMessage
}

type taskScanner interface {
	Scan(...any) error
}

const taskColumns = `
    id, idempotency_key, lane, kind, status, generation,
    canonical_revision_id, startup_artifact_id, activation_bundle_id,
    payload_json, result_json, error_json, cancel_requested, attempt,
    lease_owner, lease_expires_at, not_before, created_at, updated_at`

// EnqueueTask inserts durable work. A runtime task also advances the desired
// generation, terminally supersedes older queued intents, and asks an older
// running intent to stop at its next safe cancellation boundary.
func (s *Store) EnqueueTask(ctx context.Context, input EnqueueTaskInput) (Task, error) {
	prepared, err := prepareEnqueuedTask(input)
	if err != nil {
		return Task{}, err
	}

	var queued Task
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if prepared.IdempotencyKey != "" {
			existing, existingErr := getTaskByLaneIdempotency(ctx, tx, prepared.Lane, prepared.IdempotencyKey)
			if existingErr == nil {
				if !sameIdempotentTask(existing, prepared) {
					return ErrIdempotencyConflict
				}
				queued = existing
				return nil
			}
			if !errors.Is(existingErr, ErrTaskNotFound) {
				return existingErr
			}
		}
		if prepared.Lane == TaskLaneRuntime {
			var currentGeneration int64
			if err := tx.QueryRowContext(
				ctx,
				`SELECT target_generation FROM hub_state WHERE singleton = 1`,
			).Scan(&currentGeneration); err != nil {
				return fmt.Errorf("read runtime target generation: %w", err)
			}
			if prepared.Generation <= currentGeneration {
				return fmt.Errorf(
					"%w: requested=%d current=%d",
					ErrTaskGenerationConflict,
					prepared.Generation,
					currentGeneration,
				)
			}

			updatedAt := formatTaskTime(prepared.CreatedAt)
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE tasks
                    SET status = 'superseded', updated_at = ?
                  WHERE lane = 'runtime' AND status = 'queued' AND generation < ?`,
				updatedAt,
				prepared.Generation,
			); err != nil {
				return fmt.Errorf("supersede queued runtime tasks: %w", err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE tasks
                    SET cancel_requested = 1, updated_at = ?
                  WHERE lane = 'runtime' AND status = 'running' AND generation < ?`,
				updatedAt,
				prepared.Generation,
			); err != nil {
				return fmt.Errorf("request cancellation of older runtime task: %w", err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE hub_state
                    SET target_generation = ?, updated_at = ?
                  WHERE singleton = 1`,
				prepared.Generation,
				updatedAt,
			); err != nil {
				return fmt.Errorf("advance runtime target generation: %w", err)
			}
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO tasks(
                id, idempotency_key, lane, kind, status, generation,
                canonical_revision_id, startup_artifact_id, activation_bundle_id,
                payload_json, not_before, created_at, updated_at
             ) VALUES (?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?, ?, ?, ?)`,
			prepared.ID,
			nullIfEmpty(prepared.IdempotencyKey),
			string(prepared.Lane),
			prepared.Kind,
			prepared.Generation,
			nullIfEmpty(prepared.CanonicalRevisionID),
			nullIfEmpty(prepared.StartupArtifactID),
			nullIfEmpty(prepared.ActivationBundleID),
			string(prepared.Payload),
			nullableTaskTime(prepared.NotBefore),
			formatTaskTime(prepared.CreatedAt),
			formatTaskTime(prepared.CreatedAt),
		); err != nil {
			return fmt.Errorf("enqueue task: %w", err)
		}

		queued, err = getTask(ctx, tx, prepared.ID)
		return err
	})
	if err != nil {
		return Task{}, err
	}
	return queued, nil
}

func sameIdempotentTask(existing Task, requested EnqueueTaskInput) bool {
	return existing.Lane == requested.Lane &&
		existing.Kind == requested.Kind &&
		existing.Generation == requested.Generation &&
		existing.CanonicalRevisionID == requested.CanonicalRevisionID &&
		existing.StartupArtifactID == requested.StartupArtifactID &&
		existing.ActivationBundleID == requested.ActivationBundleID &&
		bytes.Equal(existing.Payload, requested.Payload)
}

func prepareEnqueuedTask(input EnqueueTaskInput) (EnqueueTaskInput, error) {
	if strings.TrimSpace(input.ID) == "" {
		return EnqueueTaskInput{}, errors.New("task id is empty")
	}
	if input.Lane != TaskLaneRuntime && input.Lane != TaskLaneMaintenance {
		return EnqueueTaskInput{}, fmt.Errorf("invalid task lane %q", input.Lane)
	}
	if strings.TrimSpace(input.Kind) == "" {
		return EnqueueTaskInput{}, errors.New("task kind is empty")
	}
	if input.Generation < 0 {
		return EnqueueTaskInput{}, errors.New("task generation must not be negative")
	}
	if input.Lane == TaskLaneRuntime && input.Generation == 0 {
		return EnqueueTaskInput{}, errors.New("runtime task generation must be positive")
	}
	if len(input.Payload) == 0 {
		input.Payload = json.RawMessage(`{}`)
	}
	if !json.Valid(input.Payload) {
		return EnqueueTaskInput{}, errors.New("task payload is not valid JSON")
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	} else {
		input.CreatedAt = input.CreatedAt.UTC()
	}
	if input.NotBefore != nil {
		notBefore := input.NotBefore.UTC()
		input.NotBefore = &notBefore
	}
	return input, nil
}

// ClaimTask atomically reclaims expired work and leases the oldest runnable
// task in a lane. At most one task may have running status in a lane, even when
// multiple panel processes race to claim work.
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
func (s *Store) GetTask(ctx context.Context, taskID string) (Task, error) {
	if strings.TrimSpace(taskID) == "" {
		return Task{}, errors.New("task id is empty")
	}
	return getTask(ctx, s.db, taskID)
}

func getTask(ctx context.Context, q queryRower, taskID string) (Task, error) {
	row := q.QueryRowContext(
		ctx,
		`SELECT `+taskColumns+` FROM tasks WHERE id = ?`,
		taskID,
	)
	task, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	if err != nil {
		return Task{}, fmt.Errorf("scan task %q: %w", taskID, err)
	}
	return task, nil
}

func scanTask(row taskScanner) (Task, error) {
	var (
		task                Task
		idempotencyKey      sql.NullString
		canonicalRevisionID sql.NullString
		startupArtifactID   sql.NullString
		activationBundleID  sql.NullString
		payload             string
		result              sql.NullString
		failure             sql.NullString
		cancelRequested     int
		leaseOwner          sql.NullString
		leaseExpiresAt      sql.NullString
		notBefore           sql.NullString
		createdAt           string
		updatedAt           string
	)
	if err := row.Scan(
		&task.ID,
		&idempotencyKey,
		&task.Lane,
		&task.Kind,
		&task.Status,
		&task.Generation,
		&canonicalRevisionID,
		&startupArtifactID,
		&activationBundleID,
		&payload,
		&result,
		&failure,
		&cancelRequested,
		&task.Attempt,
		&leaseOwner,
		&leaseExpiresAt,
		&notBefore,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Task{}, err
	}

	var err error
	task.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse created_at: %w", err)
	}
	task.UpdatedAt, err = parseTaskTime(updatedAt)
	if err != nil {
		return Task{}, fmt.Errorf("parse updated_at: %w", err)
	}
	if leaseExpiresAt.Valid {
		parsed, err := parseTaskTime(leaseExpiresAt.String)
		if err != nil {
			return Task{}, fmt.Errorf("parse lease_expires_at: %w", err)
		}
		task.LeaseExpiresAt = &parsed
	}
	if notBefore.Valid {
		parsed, err := parseTaskTime(notBefore.String)
		if err != nil {
			return Task{}, fmt.Errorf("parse not_before: %w", err)
		}
		task.NotBefore = &parsed
	}

	task.IdempotencyKey = valueOrEmpty(idempotencyKey)
	task.CanonicalRevisionID = valueOrEmpty(canonicalRevisionID)
	task.StartupArtifactID = valueOrEmpty(startupArtifactID)
	task.ActivationBundleID = valueOrEmpty(activationBundleID)
	task.Payload = json.RawMessage(payload)
	if result.Valid {
		task.Result = json.RawMessage(result.String)
	}
	if failure.Valid {
		task.Failure = json.RawMessage(failure.String)
	}
	task.CancelRequested = cancelRequested != 0
	task.LeaseOwner = valueOrEmpty(leaseOwner)
	return task, nil
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func nullableTaskTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTaskTime(*value)
}

func formatTaskTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func parseTaskTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func newLeaseOwner(prefix string) (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate task lease token: %w", err)
	}
	return prefix + "/" + hex.EncodeToString(token[:]), nil
}
