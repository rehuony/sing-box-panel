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

// RuntimeRecoveryMetadata records the bounded recovery episode across restarts.
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

// RuntimeRecoveryInput contains the state observed by one reconciliation pass.
// ExpectedObservation is nil only when the caller observed that no persisted
// process identity exists. A non-nil value is an exact PID-incarnation fence.
type RuntimeRecoveryInput struct {
	NewEpisodeID        string
	ExpectedBundleID    string
	ExpectedGeneration  int64
	ExpectedObservation *RuntimeObservation
	StableRunProven     bool
	CleanBoundaryProven bool
	CreatedAt           time.Time
	Transition          *RuntimeTransitionInput
}

// RuntimeRecoveryDecision reports whether recovery was scheduled or its
// durable episode exhausted. A zero decision means that concurrent state made
// the observation stale or a serialized control request owns reconciliation.
type RuntimeRecoveryDecision struct {
	Intent     *RuntimeIntent
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
// prevents recovery without an unfenced check-then-start window.
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
		observationMatches, err := runtimeRecoveryObservationMatches(ctx, tx, prepared.ExpectedObservation)
		if err != nil || !observationMatches {
			return err
		}
		if err := validateRunnableBundle(ctx, tx, prepared.ExpectedBundleID); err != nil {
			return err
		}

		previous, nextAt, succeeded, err := readRuntimeRecovery(ctx, tx, prepared.ExpectedGeneration)
		if err != nil {
			return err
		}
		// A pending deadline survives panel restarts. Only a due attempt reserves
		// a generation; polling does not consume the retry budget.
		if nextAt != nil {
			if prepared.CreatedAt.Before(*nextAt) {
				return nil
			}
			generation := prepared.ExpectedGeneration + 1
			if _, err := tx.ExecContext(ctx, `UPDATE hub_state SET target_generation=?,updated_at=? WHERE singleton=1`, generation, formatTime(prepared.CreatedAt)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE runtime_recovery SET generation=?,next_attempt_at=NULL,succeeded=0 WHERE singleton=1`, generation); err != nil {
				return err
			}
			decision.Intent = &RuntimeIntent{Kind: RuntimeIntentStart, Generation: generation, ActivationBundleID: prepared.ExpectedBundleID, Recovery: previous}
			decision.Generation = generation
			decision.BundleID = prepared.ExpectedBundleID
			decision.EpisodeID = previous.EpisodeID
			decision.Attempt = previous.Attempt
			return nil
		}
		metadata := nextRuntimeRecoveryMetadata(prepared, previous, succeeded)
		decision.EpisodeID = metadata.EpisodeID
		decision.Attempt = metadata.Attempt
		decision.BundleID = prepared.ExpectedBundleID
		decision.Generation = prepared.ExpectedGeneration

		if err := clearRuntimeRecoveryObservation(ctx, tx, prepared.ExpectedObservation); err != nil {
			return err
		}
		if prepared.Transition != nil {
			if _, err := appendRuntimeTransition(ctx, tx, *prepared.Transition); err != nil {
				return err
			}
		}
		if metadata.Attempt > RuntimeRecoveryMaximumAttempts {
			decision.Attempt = RuntimeRecoveryMaximumAttempts
			decision.Exhausted = true
			return nil
		}

		raw, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		nextAtValue := prepared.CreatedAt.Add(runtimeRecoveryDelays[metadata.Attempt-1])
		_, err = tx.ExecContext(ctx, `INSERT INTO runtime_recovery(singleton,generation,metadata_json,next_attempt_at,succeeded) VALUES(1,?,?,?,0)
          ON CONFLICT(singleton) DO UPDATE SET generation=excluded.generation,metadata_json=excluded.metadata_json,next_attempt_at=excluded.next_attempt_at,succeeded=0`,
			prepared.ExpectedGeneration, string(raw), formatTime(nextAtValue))
		if err != nil {
			return err
		}

		return nil
	})
	return decision, err
}

func prepareRuntimeRecoveryInput(input RuntimeRecoveryInput) (RuntimeRecoveryInput, error) {
	if strings.TrimSpace(input.NewEpisodeID) == "" {
		return RuntimeRecoveryInput{}, errors.New("runtime recovery episode ID are required")
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
	if input.Transition != nil {
		transition, err := prepareRuntimeTransition(*input.Transition)
		if err != nil {
			return RuntimeRecoveryInput{}, fmt.Errorf("prepare runtime recovery transition: %w", err)
		}
		if transition.ActivationBundleID != "" && transition.ActivationBundleID != input.ExpectedBundleID {
			return RuntimeRecoveryInput{}, errors.New("runtime recovery transition bundle does not match recovery bundle")
		}
		if transition.Generation != 0 && transition.Generation != input.ExpectedGeneration {
			return RuntimeRecoveryInput{}, errors.New("runtime recovery transition generation does not match recovery generation")
		}
		if input.ExpectedObservation == nil {
			if transition.PID != 0 {
				return RuntimeRecoveryInput{}, errors.New("runtime recovery transition has an unobserved process identity")
			}
		} else if transition.PID != 0 &&
			(transition.PID != input.ExpectedObservation.PID ||
				transition.ProcessStartToken != input.ExpectedObservation.ProcessStartToken ||
				transition.ProcessStartedAt == nil ||
				!transition.ProcessStartedAt.Equal(input.ExpectedObservation.StartedAt)) {
			return RuntimeRecoveryInput{}, errors.New("runtime recovery transition process fence does not match")
		}
		input.Transition = &transition
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

func runtimeRecoveryObservationMatches(
	ctx context.Context,
	tx *sql.Tx,
	expected *RuntimeObservation,
) (bool, error) {
	return runtimeObservationMatches(ctx, tx, expected)
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

func readRuntimeRecovery(ctx context.Context, tx *sql.Tx, generation int64) (*RuntimeRecoveryMetadata, *time.Time, bool, error) {
	var raw string
	var next sql.NullString
	var succeeded bool
	err := tx.QueryRowContext(ctx, `SELECT metadata_json,next_attempt_at,succeeded FROM runtime_recovery WHERE singleton=1 AND generation=?`, generation).Scan(&raw, &next, &succeeded)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, err
	}
	var metadata RuntimeRecoveryMetadata
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, nil, false, err
	}
	if metadata.EpisodeID == "" || metadata.Attempt < 1 || metadata.Attempt > RuntimeRecoveryMaximumAttempts {
		return nil, nil, false, ErrSchemaInconsistent
	}
	var nextAt *time.Time
	if next.Valid {
		parsed, err := parseTime(next.String)
		if err != nil {
			return nil, nil, false, err
		}
		nextAt = &parsed
	}
	return &metadata, nextAt, succeeded, nil
}

func nextRuntimeRecoveryMetadata(input RuntimeRecoveryInput, previous *RuntimeRecoveryMetadata, succeeded bool) RuntimeRecoveryMetadata {
	attempt := 1
	episodeID := input.NewEpisodeID
	episodeGeneration := input.ExpectedGeneration + 1
	// Reset only after an observed stable PID incarnation or a proven clean stop.
	if previous != nil && !(succeeded && (input.StableRunProven || input.CleanBoundaryProven)) {
		episodeID = previous.EpisodeID
		episodeGeneration = previous.EpisodeGeneration
		attempt = previous.Attempt + 1
	}
	metadata := RuntimeRecoveryMetadata{EpisodeID: episodeID, EpisodeGeneration: episodeGeneration, Attempt: attempt,
		MaximumAttempts: RuntimeRecoveryMaximumAttempts, StableWindowSeconds: int64(RuntimeRecoveryStableWindow / time.Second),
		RequestedAt: input.CreatedAt, PreviousGeneration: input.ExpectedGeneration}
	if input.ExpectedObservation != nil {
		started := input.ExpectedObservation.StartedAt
		metadata.FailedProcessStartedAt = &started
	}
	return metadata
}
