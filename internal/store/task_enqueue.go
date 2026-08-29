// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) EnqueueTask(ctx context.Context, input EnqueueTaskInput) (Task, error) {
	prepared, err := prepareEnqueuedTask(input)
	if err != nil {
		return Task{}, err
	}

	var queued Task
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		var enqueueErr error
		queued, enqueueErr = enqueuePreparedTaskTx(ctx, tx, prepared)
		return enqueueErr
	})
	if err != nil {
		return Task{}, err
	}
	return queued, nil
}

func enqueuePreparedTaskTx(ctx context.Context, tx *sql.Tx, prepared EnqueueTaskInput) (Task, error) {
	if prepared.IdempotencyKey != "" {
		existing, err := getTaskByLaneIdempotency(ctx, tx, prepared.Lane, prepared.IdempotencyKey)
		if err == nil {
			if !sameIdempotentTask(existing, prepared) {
				return Task{}, ErrIdempotencyConflict
			}
			return existing, nil
		}
		if !errors.Is(err, ErrTaskNotFound) {
			return Task{}, err
		}
	}
	if prepared.Lane == TaskLaneRuntime {
		var currentGeneration int64
		if err := tx.QueryRowContext(ctx, `SELECT target_generation FROM hub_state WHERE singleton = 1`).Scan(&currentGeneration); err != nil {
			return Task{}, fmt.Errorf("read runtime target generation: %w", err)
		}
		if prepared.Generation <= currentGeneration {
			return Task{}, fmt.Errorf("%w: requested=%d current=%d", ErrTaskGenerationConflict, prepared.Generation, currentGeneration)
		}
		updatedAt := formatTaskTime(prepared.CreatedAt)
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status = 'superseded', updated_at = ?
            WHERE lane = 'runtime' AND status = 'queued' AND generation < ?`, updatedAt, prepared.Generation); err != nil {
			return Task{}, fmt.Errorf("supersede queued runtime tasks: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET cancel_requested = 1, updated_at = ?
            WHERE lane = 'runtime' AND status = 'running' AND generation < ?`, updatedAt, prepared.Generation); err != nil {
			return Task{}, fmt.Errorf("request cancellation of older runtime task: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hub_state SET target_generation = ?, updated_at = ?
            WHERE singleton = 1`, prepared.Generation, updatedAt); err != nil {
			return Task{}, fmt.Errorf("advance runtime target generation: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tasks(
        id, idempotency_key, lane, kind, status, generation,
        canonical_revision_id, startup_artifact_id, activation_bundle_id,
        payload_json, not_before, created_at, updated_at
    ) VALUES (?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?, ?, ?, ?)`,
		prepared.ID, nullIfEmpty(prepared.IdempotencyKey), string(prepared.Lane), prepared.Kind,
		prepared.Generation, nullIfEmpty(prepared.CanonicalRevisionID), nullIfEmpty(prepared.StartupArtifactID),
		nullIfEmpty(prepared.ActivationBundleID), string(prepared.Payload), nullableTaskTime(prepared.NotBefore),
		formatTaskTime(prepared.CreatedAt), formatTaskTime(prepared.CreatedAt)); err != nil {
		return Task{}, fmt.Errorf("enqueue task: %w", err)
	}
	return getTask(ctx, tx, prepared.ID)
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
	if !validTaskLaneKind(input.Lane, input.Kind) {
		return EnqueueTaskInput{}, fmt.Errorf("invalid %s task kind %q", input.Lane, input.Kind)
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
