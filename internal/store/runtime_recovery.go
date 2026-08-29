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

const (
	RuntimeRecoveryMaximumAttempts = 3
	RuntimeRecoveryStableWindow    = 5 * time.Minute
)

var runtimeRecoveryDelays = [...]time.Duration{
	time.Second,
	5 * time.Second,
	30 * time.Second,
}

// RuntimeRecoveryMetadata is persisted in a recovery task's payload. Keeping
// the episode and attempt in task history makes the retry budget durable across
// panel restarts without adding mutable recovery state beside the task log.
type RuntimeRecoveryMetadata struct {
	EpisodeID              string     `json:"episode_id"`
	EpisodeGeneration      int64      `json:"episode_generation"`
	Attempt                int        `json:"attempt"`
	MaximumAttempts        int        `json:"maximum_attempts"`
	StableWindowSeconds    int64      `json:"stable_window_seconds"`
	RequestedAt            time.Time  `json:"requested_at"`
	PreviousGeneration     int64      `json:"previous_generation"`
	FailedProcessStartedAt *time.Time `json:"failed_process_started_at,omitempty"`
}

type runtimeRecoveryPayload struct {
	Intent                    RuntimeIntentKind        `json:"intent"`
	BundleID                  string                   `json:"bundle_id"`
	Origin                    string                   `json:"origin,omitempty"`
	RecoveryEpisodeGeneration int64                    `json:"recovery_episode_generation,omitempty"`
	RecoveryAttempt           int                      `json:"recovery_attempt,omitempty"`
	Recovery                  *RuntimeRecoveryMetadata `json:"recovery,omitempty"`
}

// RuntimeRecoveryInput contains the state observed by one reconciliation pass.
// ExpectedObservation is nil only when the caller observed that no persisted
// process identity exists. A non-nil value is an exact PID-incarnation fence.
type RuntimeRecoveryInput struct {
	TaskID              string
	NewEpisodeID        string
	ExpectedBundleID    string
	ExpectedGeneration  int64
	ExpectedObservation *RuntimeObservation
	StableRunProven     bool
	CleanBoundaryProven bool
	CreatedAt           time.Time
}

// RuntimeRecoveryDecision reports whether recovery was scheduled or its
// durable episode exhausted. A zero decision means that concurrent state made
// the observation stale or another runtime task already owns reconciliation.
type RuntimeRecoveryDecision struct {
	Task       *Task
	BundleID   string
	Generation int64
	EpisodeID  string
	Attempt    int
	Exhausted  bool
}

// RequestRuntimeRecovery atomically fences stale process evidence, checks that
// the observed desired/applied generation is still current, and schedules at
// most one bounded recovery attempt. Explicit user runtime intents race through
// the same hub generation, so whichever transaction commits later supersedes or
// prevents recovery without an unfenced check-then-enqueue window.
func (s *Store) RequestRuntimeRecovery(
	ctx context.Context,
	input RuntimeRecoveryInput,
) (RuntimeRecoveryDecision, error) {
	prepared, err := prepareRuntimeRecoveryInput(input)
	if err != nil {
		return RuntimeRecoveryDecision{}, err
	}

	var decision RuntimeRecoveryDecision
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		eligible, err := runtimeRecoveryStateMatches(ctx, tx, prepared)
		if err != nil || !eligible {
			return err
		}
		active, err := hasActiveRuntimeTask(ctx, tx)
		if err != nil || active {
			return err
		}
		observationMatches, err := runtimeRecoveryObservationMatches(ctx, tx, prepared.ExpectedObservation)
		if err != nil || !observationMatches {
			return err
		}
		if err := validateRunnableBundle(ctx, tx, prepared.ExpectedBundleID); err != nil {
			return err
		}

		current, err := runtimeTaskAtGeneration(ctx, tx, prepared.ExpectedGeneration)
		if err != nil && !errors.Is(err, ErrTaskNotFound) {
			return err
		}
		metadata, err := nextRuntimeRecoveryMetadata(prepared, current)
		if err != nil {
			return err
		}
		decision.EpisodeID = metadata.EpisodeID
		decision.Attempt = metadata.Attempt
		decision.BundleID = prepared.ExpectedBundleID
		decision.Generation = prepared.ExpectedGeneration

		if err := clearRuntimeRecoveryObservation(ctx, tx, prepared.ExpectedObservation); err != nil {
			return err
		}
		if metadata.Attempt > RuntimeRecoveryMaximumAttempts {
			decision.Attempt = RuntimeRecoveryMaximumAttempts
			decision.Exhausted = true
			return nil
		}

		payload, err := json.Marshal(runtimeRecoveryPayload{
			Intent: RuntimeIntentStart, BundleID: prepared.ExpectedBundleID,
			Origin: "auto_recovery", RecoveryEpisodeGeneration: metadata.EpisodeGeneration,
			RecoveryAttempt: metadata.Attempt, Recovery: &metadata,
		})
		if err != nil {
			return fmt.Errorf("encode runtime recovery task: %w", err)
		}
		notBefore := prepared.CreatedAt.Add(runtimeRecoveryDelays[metadata.Attempt-1])
		taskInput, err := prepareEnqueuedTask(EnqueueTaskInput{
			ID:                 prepared.TaskID,
			IdempotencyKey:     fmt.Sprintf("runtime-recovery:%s:%d", metadata.EpisodeID, metadata.Attempt),
			Lane:               TaskLaneRuntime,
			Kind:               TaskKindRuntimeStart,
			Generation:         prepared.ExpectedGeneration + 1,
			ActivationBundleID: prepared.ExpectedBundleID,
			Payload:            payload,
			NotBefore:          &notBefore,
			CreatedAt:          prepared.CreatedAt,
		})
		if err != nil {
			return err
		}
		queued, err := enqueuePreparedTaskTx(ctx, tx, taskInput)
		if err != nil {
			return err
		}
		decision.Task = &queued
		decision.Generation = queued.Generation
		return nil
	})
	return decision, err
}

func prepareRuntimeRecoveryInput(input RuntimeRecoveryInput) (RuntimeRecoveryInput, error) {
	if strings.TrimSpace(input.TaskID) == "" || strings.TrimSpace(input.NewEpisodeID) == "" {
		return RuntimeRecoveryInput{}, errors.New("runtime recovery task and episode IDs are required")
	}
	if strings.TrimSpace(input.ExpectedBundleID) == "" {
		return RuntimeRecoveryInput{}, errors.New("runtime recovery bundle is required")
	}
	if input.ExpectedGeneration < 1 {
		return RuntimeRecoveryInput{}, errors.New("runtime recovery generation must be positive")
	}
	if input.CreatedAt.IsZero() {
		return RuntimeRecoveryInput{}, errors.New("runtime recovery time is required")
	}
	if input.StableRunProven && input.ExpectedObservation == nil {
		return RuntimeRecoveryInput{}, errors.New("stable runtime recovery proof requires a process observation")
	}
	if input.CleanBoundaryProven && input.ExpectedObservation != nil {
		return RuntimeRecoveryInput{}, errors.New("clean runtime recovery boundary cannot retain a process observation")
	}
	input.CreatedAt = input.CreatedAt.UTC()
	if input.ExpectedObservation != nil {
		observation := *input.ExpectedObservation
		if observation.PID <= 0 || !validProcessStartToken(observation.ProcessStartToken) || observation.StartedAt.IsZero() {
			return RuntimeRecoveryInput{}, errors.New("runtime recovery observation fence is invalid")
		}
		observation.StartedAt = observation.StartedAt.UTC()
		input.ExpectedObservation = &observation
	}
	return input, nil
}

func runtimeRecoveryStateMatches(
	ctx context.Context,
	tx *sql.Tx,
	input RuntimeRecoveryInput,
) (bool, error) {
	var desiredBundle, appliedBundle sql.NullString
	var generation int64
	var desiredRunning int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT desired_bundle_id, applied_bundle_id, target_generation, desired_running
	       FROM hub_state WHERE singleton = 1`,
	).Scan(&desiredBundle, &appliedBundle, &generation, &desiredRunning); err != nil {
		return false, fmt.Errorf("read runtime recovery state: %w", err)
	}
	return desiredRunning != 0 &&
		generation == input.ExpectedGeneration &&
		valueOrEmpty(desiredBundle) == input.ExpectedBundleID &&
		valueOrEmpty(appliedBundle) == input.ExpectedBundleID, nil
}

func hasActiveRuntimeTask(ctx context.Context, tx *sql.Tx) (bool, error) {
	var active int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT EXISTS(
	        SELECT 1 FROM tasks
	         WHERE lane = 'runtime' AND status IN ('queued', 'running')
	    )`,
	).Scan(&active); err != nil {
		return false, fmt.Errorf("inspect active runtime recovery work: %w", err)
	}
	return active != 0, nil
}

func runtimeRecoveryObservationMatches(
	ctx context.Context,
	tx *sql.Tx,
	expected *RuntimeObservation,
) (bool, error) {
	var pid int
	var processStartToken string
	err := tx.QueryRowContext(
		ctx,
		`SELECT pid, process_start_token FROM runtime_observation WHERE singleton = 1`,
	).Scan(&pid, &processStartToken)
	if errors.Is(err, sql.ErrNoRows) {
		return expected == nil, nil
	}
	if err != nil {
		return false, fmt.Errorf("read runtime recovery observation fence: %w", err)
	}
	return expected != nil && pid == expected.PID && processStartToken == expected.ProcessStartToken, nil
}

func clearRuntimeRecoveryObservation(
	ctx context.Context,
	tx *sql.Tx,
	expected *RuntimeObservation,
) error {
	if expected == nil {
		return nil
	}
	result, err := tx.ExecContext(
		ctx,
		`DELETE FROM runtime_observation
	      WHERE singleton = 1 AND pid = ? AND process_start_token = ?`,
		expected.PID,
		expected.ProcessStartToken,
	)
	if err != nil {
		return fmt.Errorf("clear fenced runtime recovery observation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect fenced runtime recovery observation clear: %w", err)
	}
	if rows != 1 {
		return ErrRuntimeIdentityMismatch
	}
	return nil
}

func runtimeTaskAtGeneration(
	ctx context.Context,
	tx *sql.Tx,
	generation int64,
) (Task, error) {
	task, err := scanTask(tx.QueryRowContext(
		ctx,
		`SELECT `+taskColumns+`
	       FROM tasks
	      WHERE lane = 'runtime' AND generation = ?
	      ORDER BY created_at DESC, id DESC
	      LIMIT 1`,
		generation,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Task{}, ErrTaskNotFound
	}
	if err != nil {
		return Task{}, fmt.Errorf("read runtime recovery history: %w", err)
	}
	return task, nil
}

func nextRuntimeRecoveryMetadata(
	input RuntimeRecoveryInput,
	current Task,
) (RuntimeRecoveryMetadata, error) {
	attempt := 1
	episodeID := input.NewEpisodeID
	episodeGeneration := input.ExpectedGeneration + 1
	if current.ID != "" {
		previous, err := runtimeRecoveryMetadataFromTask(current)
		if err != nil {
			return RuntimeRecoveryMetadata{}, err
		}
		// Wall-clock age is not evidence of continuous uptime: both the child
		// and panel may have been down for most of the interval. Only the live
		// reconciler can prove that one PID incarnation remained observable for
		// the full stability window in this panel process.
		stable := previous != nil && current.Status == TaskStatusSucceeded &&
			(input.StableRunProven || input.CleanBoundaryProven)
		if previous != nil && !stable {
			episodeID = previous.EpisodeID
			episodeGeneration = previous.EpisodeGeneration
			attempt = previous.Attempt + 1
		}
	}
	metadata := RuntimeRecoveryMetadata{
		EpisodeID:           episodeID,
		EpisodeGeneration:   episodeGeneration,
		Attempt:             attempt,
		MaximumAttempts:     RuntimeRecoveryMaximumAttempts,
		StableWindowSeconds: int64(RuntimeRecoveryStableWindow / time.Second),
		RequestedAt:         input.CreatedAt,
		PreviousGeneration:  input.ExpectedGeneration,
	}
	if input.ExpectedObservation != nil {
		startedAt := input.ExpectedObservation.StartedAt
		metadata.FailedProcessStartedAt = &startedAt
	}
	return metadata, nil
}

func runtimeRecoveryMetadataFromTask(task Task) (*RuntimeRecoveryMetadata, error) {
	var payload runtimeRecoveryPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return nil, fmt.Errorf("decode runtime recovery history: %w", err)
	}
	if payload.Recovery == nil {
		return nil, nil
	}
	metadata := *payload.Recovery
	if task.Kind != TaskKindRuntimeStart || payload.Intent != RuntimeIntentStart || payload.Origin != "auto_recovery" ||
		payload.BundleID != task.ActivationBundleID || strings.TrimSpace(metadata.EpisodeID) == "" ||
		payload.RecoveryEpisodeGeneration != metadata.EpisodeGeneration || payload.RecoveryAttempt != metadata.Attempt ||
		metadata.EpisodeGeneration < 1 || metadata.EpisodeGeneration > task.Generation ||
		metadata.Attempt < 1 || metadata.Attempt > RuntimeRecoveryMaximumAttempts ||
		metadata.MaximumAttempts != RuntimeRecoveryMaximumAttempts ||
		metadata.StableWindowSeconds != int64(RuntimeRecoveryStableWindow/time.Second) ||
		metadata.RequestedAt.IsZero() || metadata.PreviousGeneration < 1 {
		return nil, fmt.Errorf("%w: invalid runtime recovery task payload", ErrSchemaInconsistent)
	}
	return &metadata, nil
}
