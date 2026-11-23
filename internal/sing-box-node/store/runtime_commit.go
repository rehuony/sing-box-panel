// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type preparedRuntimeCommit struct {
	expected         *RuntimeObservation
	observation      *RuntimeObservation
	clearObservation bool
	transitions      []RuntimeTransitionInput
}

func prepareRuntimeCommit(input *RuntimeCommit) (*preparedRuntimeCommit, error) {
	if input == nil {
		return nil, nil
	}
	if input.ClearObservation == (input.Observation != nil) {
		return nil, errors.New("runtime commit must record or clear an observation")
	}
	prepared := &preparedRuntimeCommit{clearObservation: input.ClearObservation}
	if input.ExpectedObservation != nil {
		expected, err := prepareRuntimeObservation(*input.ExpectedObservation)
		if err != nil {
			return nil, fmt.Errorf("prepare expected runtime observation: %w", err)
		}
		prepared.expected = &expected
	}
	if input.Observation != nil {
		observation, err := prepareRuntimeObservation(*input.Observation)
		if err != nil {
			return nil, fmt.Errorf("prepare final runtime observation: %w", err)
		}
		prepared.observation = &observation
	}
	transitions, err := prepareRuntimeTransitions(input.Transitions)
	if err != nil {
		return nil, err
	}
	if len(transitions) == 0 {
		return nil, errors.New("runtime commit requires lifecycle evidence")
	}
	prepared.transitions = transitions
	return prepared, nil
}

func applyRuntimeCommit(
	ctx context.Context,
	tx *sql.Tx,
	intent RuntimeIntent,
	handlerSucceeded bool,
	commit *preparedRuntimeCommit,
) error {
	if err := validateRuntimeCommit(intent, handlerSucceeded, commit); err != nil {
		return err
	}
	matches, err := runtimeObservationMatches(ctx, tx, commit.expected)
	if err != nil {
		return err
	}
	if !matches {
		return ErrRuntimeIdentityMismatch
	}
	if commit.clearObservation {
		result, err := tx.ExecContext(ctx, `DELETE FROM runtime_observation WHERE singleton = 1`)
		if err != nil {
			return fmt.Errorf("clear runtime observation for runtime completion: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect runtime observation clear for runtime completion: %w", err)
		}
		wantRows := int64(0)
		if commit.expected != nil {
			wantRows = 1
		}
		if rows != wantRows {
			return ErrRuntimeIdentityMismatch
		}
	} else if err := recordRuntimeObservationTx(ctx, tx, *commit.observation); err != nil {
		return err
	}
	for _, transition := range commit.transitions {
		if _, err := appendRuntimeTransition(ctx, tx, transition); err != nil {
			return err
		}
	}
	return nil
}

func validateRuntimeCommit(
	intent RuntimeIntent,
	handlerSucceeded bool,
	commit *preparedRuntimeCommit,
) error {
	if commit == nil {
		return errors.New("runtime commit is nil")
	}
	for _, transition := range commit.transitions {
		if transition.Generation != intent.Generation {
			return errors.New("runtime transition does not belong to the completing intent")
		}
	}
	if !handlerSucceeded {
		return validateFailedRuntimeCommit(commit)
	}
	return validateSuccessfulRuntimeCommit(intent, commit)
}

func validateSuccessfulRuntimeCommit(intent RuntimeIntent, commit *preparedRuntimeCommit) error {
	wantState := RuntimeTransitionRunning
	kind := RuntimeIntentKind(intent.Kind)
	if CoreSelectionOnly(intent) {
		kind = RuntimeIntentStop
	}
	switch kind {
	case RuntimeIntentApply, RuntimeIntentStart, RuntimeIntentRestart, RuntimeIntentRollback:
		if commit.clearObservation || commit.observation == nil {
			return errors.New("successful running intent must record a runtime observation")
		}
		if commit.observation.ActivationBundleID != intent.ActivationBundleID {
			return ErrRuntimeIdentityMismatch
		}
	case RuntimeIntentStop:
		wantState = RuntimeTransitionStopped
		if !commit.clearObservation || commit.observation != nil {
			return errors.New("successful stop intent must clear the runtime observation")
		}
		if len(commit.transitions) != 1 {
			return errors.New("successful stop intent requires exactly one stopped transition")
		}
	default:
		return fmt.Errorf("invalid runtime kind %q", intent.Kind)
	}
	if wantState == RuntimeTransitionRunning && len(commit.transitions) > 2 {
		return errors.New("successful running intent has too many lifecycle transitions")
	}

	terminalEvidence := 0
	for _, transition := range commit.transitions {
		if transition.State != wantState {
			if wantState != RuntimeTransitionRunning || transition.State != RuntimeTransitionStopped {
				return errors.New("runtime commit contains an unexpected transition state")
			}
			continue
		}
		terminalEvidence++
		if wantState == RuntimeTransitionRunning {
			if transition.ActivationBundleID != intent.ActivationBundleID ||
				transition.PID != commit.observation.PID ||
				transition.ProcessStartToken != commit.observation.ProcessStartToken ||
				transition.ProcessStartedAt == nil ||
				!transition.ProcessStartedAt.Equal(commit.observation.StartedAt) {
				return ErrRuntimeIdentityMismatch
			}
		} else {
			if commit.expected == nil {
				if transition.PID != 0 || transition.ProcessStartToken != "" || transition.ProcessStartedAt != nil {
					return ErrRuntimeIdentityMismatch
				}
			} else if transition.PID != commit.expected.PID ||
				transition.ProcessStartToken != commit.expected.ProcessStartToken ||
				transition.ProcessStartedAt == nil ||
				!transition.ProcessStartedAt.Equal(commit.expected.StartedAt) ||
				transition.ActivationBundleID != commit.expected.ActivationBundleID {
				return ErrRuntimeIdentityMismatch
			}
		}
	}
	if terminalEvidence != 1 {
		return fmt.Errorf("runtime commit has %d terminal transitions, want 1", terminalEvidence)
	}
	return nil
}

func validateFailedRuntimeCommit(commit *preparedRuntimeCommit) error {
	if !commit.clearObservation || commit.observation != nil {
		return errors.New("failed runtime handler may only clear its fenced observation")
	}
	if len(commit.transitions) != 1 {
		return errors.New("failed runtime handler requires exactly one final lifecycle transition")
	}
	transition := commit.transitions[0]
	if transition.State == RuntimeTransitionRunning {
		return errors.New("failed runtime handler cannot commit a running transition")
	}
	if commit.expected == nil {
		if transition.PID != 0 || transition.ProcessStartToken != "" || transition.ProcessStartedAt != nil {
			return ErrRuntimeIdentityMismatch
		}
		return nil
	}
	if transition.PID == 0 {
		return nil
	}
	if transition.PID != commit.expected.PID ||
		transition.ProcessStartToken != commit.expected.ProcessStartToken ||
		transition.ProcessStartedAt == nil ||
		!transition.ProcessStartedAt.Equal(commit.expected.StartedAt) {
		return ErrRuntimeIdentityMismatch
	}
	return nil
}

// CompleteRuntimeIntent commits only the current generation and verified process
// incarnation. The server holds its runtime mutex and data-directory lease.
func (s *Store) CompleteRuntimeIntent(ctx context.Context, intent RuntimeIntent, succeeded bool, commit *RuntimeCommit, now time.Time) error {
	prepared, err := prepareRuntimeCommit(commit)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		var generation int64
		if err := tx.QueryRowContext(ctx, `SELECT target_generation FROM hub_state WHERE singleton=1`).Scan(&generation); err != nil {
			return err
		}
		if generation != intent.Generation {
			return ErrRuntimeIntentStale
		}
		if succeeded && prepared == nil {
			return errors.New("runtime completion requires process evidence")
		}
		if prepared != nil {
			if err := applyRuntimeCommit(ctx, tx, intent, succeeded, prepared); err != nil {
				return err
			}
		}
		if intent.Recovery != nil {
			if _, err := tx.ExecContext(ctx, `UPDATE runtime_recovery SET succeeded=? WHERE singleton=1 AND generation=?`, succeeded, intent.Generation); err != nil {
				return err
			}
		}
		if succeeded {
			return commitSuccessfulRuntimeIntent(ctx, tx, intent, now)
		}
		return nil
	})
}

func (s *Store) CheckRuntimeGeneration(ctx context.Context, generation int64) error {
	var current int64
	if err := s.db.QueryRowContext(ctx, `SELECT target_generation FROM hub_state WHERE singleton=1`).Scan(&current); err != nil {
		return err
	}
	if current != generation {
		return ErrRuntimeIntentStale
	}
	return nil
}
