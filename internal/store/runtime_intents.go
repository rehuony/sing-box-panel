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

var (
	ErrNoAppliedBundle     = errors.New("no activation bundle has been applied")
	ErrNoRollbackBundle    = errors.New("no rollback activation bundle is available")
	ErrIdempotencyConflict = errors.New("idempotency key belongs to a different runtime intent")
)

type RuntimeIntentKind = TaskKind

const (
	RuntimeIntentApply    RuntimeIntentKind = TaskKindRuntimeApply
	RuntimeIntentStart    RuntimeIntentKind = TaskKindRuntimeStart
	RuntimeIntentStop     RuntimeIntentKind = TaskKindRuntimeStop
	RuntimeIntentRestart  RuntimeIntentKind = TaskKindRuntimeRestart
	RuntimeIntentRollback RuntimeIntentKind = TaskKindRuntimeRollback
)

type RuntimeIntentInput struct {
	TaskID         string
	IdempotencyKey string
	Kind           RuntimeIntentKind
	BundleID       string
	CreatedAt      time.Time
}

// RequestRuntimeIntent advances the desired generation and enqueues its task
// in the same transaction. Start/restart resolve to the last applied bundle,
// rollback resolves to the frozen prior bundle, and apply requires an explicit
// ready bundle built from the current head.
func (s *Store) RequestRuntimeIntent(ctx context.Context, input RuntimeIntentInput) (Task, error) {
	prepared, err := prepareRuntimeIntent(input)
	if err != nil {
		return Task{}, err
	}
	return s.requestRuntimeIntentNullable(ctx, prepared)
}

func (s *Store) requestRuntimeIntentNullable(ctx context.Context, input RuntimeIntentInput) (Task, error) {
	var queued Task
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		if input.IdempotencyKey != "" {
			existing, err := getTaskByLaneIdempotency(ctx, tx, TaskLaneRuntime, input.IdempotencyKey)
			if err == nil {
				if existing.Kind != input.Kind ||
					(input.BundleID != "" && existing.ActivationBundleID != input.BundleID) {
					return ErrIdempotencyConflict
				}
				queued = existing
				return nil
			}
			if !errors.Is(err, ErrTaskNotFound) {
				return err
			}
		}
		var headID, appliedBundleID, rollbackBundleID sql.NullString
		var generation int64
		if err := tx.QueryRowContext(
			ctx,
			`SELECT head_revision_id, applied_bundle_id, rollback_bundle_id, target_generation
               FROM hub_state WHERE singleton = 1`,
		).Scan(&headID, &appliedBundleID, &rollbackBundleID, &generation); err != nil {
			return fmt.Errorf("read runtime intent state: %w", err)
		}

		bundleID := input.BundleID
		desiredRunning := input.Kind != RuntimeIntentStop
		switch input.Kind {
		case RuntimeIntentApply:
			if err := validateApplicableBundle(ctx, tx, bundleID, valueOrEmpty(headID)); err != nil {
				return err
			}
		case RuntimeIntentStart, RuntimeIntentRestart:
			if bundleID == "" {
				bundleID = valueOrEmpty(appliedBundleID)
			}
			if bundleID == "" {
				return ErrNoAppliedBundle
			}
			if bundleID != valueOrEmpty(appliedBundleID) {
				return errors.New("start and restart must use the last applied bundle")
			}
			if err := validateRunnableBundle(ctx, tx, bundleID); err != nil {
				return err
			}
		case RuntimeIntentRollback:
			if bundleID == "" {
				bundleID = valueOrEmpty(rollbackBundleID)
			}
			if bundleID == "" {
				return ErrNoRollbackBundle
			}
			if bundleID != valueOrEmpty(rollbackBundleID) {
				return errors.New("rollback must use the frozen rollback bundle")
			}
			if err := validateRunnableBundle(ctx, tx, bundleID); err != nil {
				return err
			}
		case RuntimeIntentStop:
			if bundleID != "" {
				return errors.New("stop intent does not accept a bundle")
			}
			bundleID = valueOrEmpty(appliedBundleID)
		}

		generation++
		createdAt := formatTaskTime(input.CreatedAt)
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE tasks SET status = 'superseded', updated_at = ?
                  WHERE lane = 'runtime' AND status = 'queued' AND generation < ?`,
			createdAt,
			generation,
		); err != nil {
			return fmt.Errorf("supersede queued runtime intents: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE tasks SET cancel_requested = 1, updated_at = ?
                  WHERE lane = 'runtime' AND status = 'running' AND generation < ?`,
			createdAt,
			generation,
		); err != nil {
			return fmt.Errorf("cancel older running intent: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE hub_state
                    SET desired_bundle_id = CASE WHEN ? = '' THEN desired_bundle_id ELSE ? END,
                        target_generation = ?, desired_running = ?, updated_at = ?
                  WHERE singleton = 1`,
			bundleID,
			bundleID,
			generation,
			boolInt(desiredRunning),
			createdAt,
		); err != nil {
			return fmt.Errorf("advance runtime intent: %w", err)
		}
		payload, _ := json.Marshal(map[string]any{"intent": input.Kind, "bundle_id": bundleID})
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO tasks(
                    id, idempotency_key, lane, kind, status, generation,
                    activation_bundle_id, payload_json, created_at, updated_at
                 ) VALUES (?, ?, 'runtime', ?, 'queued', ?, ?, ?, ?, ?)`,
			input.TaskID,
			nullIfEmpty(input.IdempotencyKey),
			string(input.Kind),
			generation,
			nullIfEmpty(bundleID),
			string(payload),
			createdAt,
			createdAt,
		); err != nil {
			return fmt.Errorf("enqueue runtime intent: %w", err)
		}
		var getErr error
		queued, getErr = getTask(ctx, tx, input.TaskID)
		return getErr
	})
	return queued, err
}

func prepareRuntimeIntent(input RuntimeIntentInput) (RuntimeIntentInput, error) {
	if strings.TrimSpace(input.TaskID) == "" {
		return RuntimeIntentInput{}, errors.New("runtime task id is empty")
	}
	switch input.Kind {
	case RuntimeIntentApply, RuntimeIntentStart, RuntimeIntentStop, RuntimeIntentRestart, RuntimeIntentRollback:
	default:
		return RuntimeIntentInput{}, fmt.Errorf("invalid runtime intent %q", input.Kind)
	}
	if input.Kind == RuntimeIntentApply && strings.TrimSpace(input.BundleID) == "" {
		return RuntimeIntentInput{}, errors.New("apply intent requires an activation bundle")
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	} else {
		input.CreatedAt = input.CreatedAt.UTC()
	}
	return input, nil
}

func validateApplicableBundle(ctx context.Context, tx *sql.Tx, bundleID, headID string) error {
	if err := validateRunnableBundle(ctx, tx, bundleID); err != nil {
		return err
	}
	var startupArtifactID string
	if err := tx.QueryRowContext(
		ctx,
		`SELECT startup_artifact_id FROM activation_bundles WHERE id = ?`,
		bundleID,
	).Scan(&startupArtifactID); err != nil {
		return fmt.Errorf("read activation bundle startup artifact: %w", err)
	}
	startup, err := getStartupArtifact(ctx, tx, startupArtifactID)
	if err != nil {
		return err
	}
	if startup.CanonicalRevisionID != headID || startup.State != StartupArtifactReady {
		return ErrActivationBundleNotReady
	}
	return nil
}

func validateRunnableBundle(ctx context.Context, tx *sql.Tx, bundleID string) error {
	var state StartupArtifactState
	var canonicalRevisionID string
	var verification CoreArtifactVerificationState
	err := tx.QueryRowContext(
		ctx,
		`SELECT startup.state, startup.canonical_revision_id, core.verification_state
           FROM activation_bundles AS bundle
           JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
           JOIN core_artifacts AS core ON core.id = startup.core_artifact_id
          WHERE bundle.id = ?`,
		bundleID,
	).Scan(&state, &canonicalRevisionID, &verification)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrActivationBundleNotFound
	}
	if err != nil {
		return fmt.Errorf("read activation bundle eligibility: %w", err)
	}
	if state != StartupArtifactReady || verification != CoreArtifactVerified {
		return ErrActivationBundleNotReady
	}
	return nil
}

func getTaskByLaneIdempotency(
	ctx context.Context,
	q queryRower,
	lane TaskLane,
	idempotencyKey string,
) (Task, error) {
	task, err := scanTask(q.QueryRowContext(
		ctx,
		`SELECT `+taskColumns+`
		   FROM tasks
		  WHERE lane = ? AND idempotency_key = ?
		    AND status IN ('queued', 'running')`,
		string(lane),
		idempotencyKey,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrTaskNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("read idempotent task: %w", err)
	}
	return task, nil
}

func commitSuccessfulRuntimeIntent(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	completedAt time.Time,
) error {
	switch RuntimeIntentKind(task.Kind) {
	case RuntimeIntentApply, RuntimeIntentStart, RuntimeIntentRestart:
		if task.ActivationBundleID == "" {
			return errors.New("successful runtime intent has no activation bundle")
		}
		if err := validateRunnableBundle(ctx, tx, task.ActivationBundleID); err != nil {
			return err
		}
		var appliedBundleID, rollbackBundleID sql.NullString
		if err := tx.QueryRowContext(
			ctx,
			`SELECT applied_bundle_id, rollback_bundle_id FROM hub_state WHERE singleton = 1`,
		).Scan(&appliedBundleID, &rollbackBundleID); err != nil {
			return fmt.Errorf("read applied runtime state: %w", err)
		}
		rollback := valueOrEmpty(rollbackBundleID)
		if appliedBundleID.Valid && appliedBundleID.String != task.ActivationBundleID {
			rollback = appliedBundleID.String
		}
		result, err := tx.ExecContext(
			ctx,
			`UPDATE hub_state
                    SET desired_bundle_id = ?, applied_bundle_id = ?,
                        rollback_bundle_id = ?, desired_running = 1,
                        applied_at = CASE
                            WHEN applied_bundle_id IS NULL OR applied_bundle_id <> ? THEN ?
                            ELSE applied_at
                        END,
                        updated_at = ?
                  WHERE singleton = 1 AND target_generation = ?`,
			task.ActivationBundleID,
			task.ActivationBundleID,
			nullIfEmpty(rollback),
			task.ActivationBundleID,
			formatTaskTime(completedAt),
			formatTaskTime(completedAt),
			task.Generation,
		)
		if err != nil {
			return fmt.Errorf("commit applied activation bundle: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return errors.Join(ErrTaskGenerationConflict, err)
		}
	case RuntimeIntentRollback:
		if task.ActivationBundleID == "" {
			return errors.New("successful rollback intent has no activation bundle")
		}
		if err := validateRunnableBundle(ctx, tx, task.ActivationBundleID); err != nil {
			return err
		}
		var appliedBundleID, rollbackBundleID sql.NullString
		if err := tx.QueryRowContext(
			ctx,
			`SELECT applied_bundle_id, rollback_bundle_id FROM hub_state WHERE singleton = 1`,
		).Scan(&appliedBundleID, &rollbackBundleID); err != nil {
			return fmt.Errorf("read rollback runtime state: %w", err)
		}
		if valueOrEmpty(rollbackBundleID) != task.ActivationBundleID || !appliedBundleID.Valid {
			return errors.New("frozen rollback bundle changed before completion")
		}
		result, err := tx.ExecContext(
			ctx,
			`UPDATE hub_state
			        SET desired_bundle_id = ?, applied_bundle_id = ?, rollback_bundle_id = ?,
			            desired_running = 1, applied_at = ?, updated_at = ?
			      WHERE singleton = 1 AND target_generation = ?`,
			task.ActivationBundleID,
			task.ActivationBundleID,
			appliedBundleID.String,
			formatTaskTime(completedAt),
			formatTaskTime(completedAt),
			task.Generation,
		)
		if err != nil {
			return fmt.Errorf("commit rollback activation bundle: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return errors.Join(ErrTaskGenerationConflict, err)
		}
	case RuntimeIntentStop:
		result, err := tx.ExecContext(
			ctx,
			`UPDATE hub_state SET desired_running = 0, updated_at = ?
                  WHERE singleton = 1 AND target_generation = ?`,
			formatTaskTime(completedAt),
			task.Generation,
		)
		if err != nil {
			return fmt.Errorf("commit stopped runtime state: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return errors.Join(ErrTaskGenerationConflict, err)
		}
	}
	return nil
}
