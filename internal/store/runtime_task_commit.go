// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type preparedRuntimeTaskCommit struct {
	expected         *RuntimeObservation
	observation      *RuntimeObservation
	clearObservation bool
	transitions      []RuntimeTransitionInput
}

func prepareRuntimeTaskCommit(input *RuntimeTaskCommit) (*preparedRuntimeTaskCommit, error) {
	if input == nil {
		return nil, nil
	}
	if input.ClearObservation == (input.Observation != nil) {
		return nil, errors.New("runtime task commit must record or clear an observation")
	}
	prepared := &preparedRuntimeTaskCommit{clearObservation: input.ClearObservation}
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
		return nil, errors.New("runtime task commit requires lifecycle evidence")
	}
	prepared.transitions = transitions
	return prepared, nil
}

func applyRuntimeTaskCommit(
	ctx context.Context,
	tx *sql.Tx,
	task Task,
	handlerSucceeded bool,
	terminalStatus TaskStatus,
	commit *preparedRuntimeTaskCommit,
) error {
	if task.Lane != TaskLaneRuntime {
		return errors.New("runtime task commit belongs to a non-runtime task")
	}
	if err := validateRuntimeTaskCommit(task, handlerSucceeded, terminalStatus, commit); err != nil {
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
			return fmt.Errorf("clear runtime observation for task completion: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect runtime observation clear for task completion: %w", err)
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

func validateRuntimeTaskCommit(
	task Task,
	handlerSucceeded bool,
	terminalStatus TaskStatus,
	commit *preparedRuntimeTaskCommit,
) error {
	if commit == nil {
		return errors.New("runtime task commit is nil")
	}
	switch terminalStatus {
	case TaskStatusSucceeded, TaskStatusFailed, TaskStatusCanceled, TaskStatusSuperseded:
	default:
		return fmt.Errorf("runtime task commit has non-terminal task status %q", terminalStatus)
	}
	for _, transition := range commit.transitions {
		if transition.TaskID != task.ID || transition.Generation != task.Generation {
			return errors.New("runtime transition does not belong to the completing task")
		}
	}
	if !handlerSucceeded {
		return validateFailedRuntimeTaskCommit(commit)
	}
	return validateSuccessfulRuntimeTaskCommit(task, commit)
}

func validateSuccessfulRuntimeTaskCommit(task Task, commit *preparedRuntimeTaskCommit) error {
	wantState := RuntimeTransitionRunning
	kind := RuntimeIntentKind(task.Kind)
	if CoreSelectionOnly(task) {
		kind = RuntimeIntentStop
	}
	switch kind {
	case RuntimeIntentApply, RuntimeIntentStart, RuntimeIntentRestart, RuntimeIntentRollback:
		if commit.clearObservation || commit.observation == nil {
			return errors.New("successful running intent must record a runtime observation")
		}
		if commit.observation.ActivationBundleID != task.ActivationBundleID {
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
		return fmt.Errorf("invalid runtime task kind %q", task.Kind)
	}
	if wantState == RuntimeTransitionRunning && len(commit.transitions) > 2 {
		return errors.New("successful running intent has too many lifecycle transitions")
	}

	terminalEvidence := 0
	for _, transition := range commit.transitions {
		if transition.State != wantState {
			if wantState != RuntimeTransitionRunning || transition.State != RuntimeTransitionStopped {
				return errors.New("runtime task commit contains an unexpected transition state")
			}
			continue
		}
		terminalEvidence++
		if wantState == RuntimeTransitionRunning {
			if transition.ActivationBundleID != task.ActivationBundleID ||
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
		return fmt.Errorf("runtime task commit has %d terminal transitions, want 1", terminalEvidence)
	}
	return nil
}

func validateFailedRuntimeTaskCommit(commit *preparedRuntimeTaskCommit) error {
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
