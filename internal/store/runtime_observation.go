// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var (
	ErrRuntimeObservationNotFound = errors.New("runtime observation not found")
	ErrRuntimeIdentityMismatch    = errors.New("runtime identity does not match the immutable bundle")
)

// RuntimeObservation is the last process identity confirmed by the owning
// server. Consumers must still verify PID/start-token liveness before treating
// this record as currently running; it is evidence, not a desired-state flag.
type RuntimeObservation struct {
	PID                int
	ProcessStartToken  string
	CoreArtifactID     string
	ActivationBundleID string
	ExactCoreVersion   string
	ArchiveSHA256      string
	BinarySHA256       string
	StartedAt          time.Time
	ObservedAt         time.Time
	StableObservedAt   *time.Time
}

func (s *Store) RecordRuntimeObservation(
	ctx context.Context,
	observation RuntimeObservation,
) (RuntimeObservation, error) {
	prepared, err := prepareRuntimeObservation(observation)
	if err != nil {
		return RuntimeObservation{}, err
	}
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		return recordRuntimeObservationTx(ctx, tx, prepared)
	})
	if err != nil {
		return RuntimeObservation{}, err
	}
	return s.RuntimeObservation(ctx)
}

// RecordRuntimeObservationAndTransitions replaces only the process incarnation
// captured by expected and appends the supplied lifecycle evidence in the same
// transaction. A late operation cannot overwrite a newer observation or add history
// for a process it no longer owns.
func (s *Store) RecordRuntimeObservationAndTransitions(
	ctx context.Context,
	expected *RuntimeObservation,
	observation RuntimeObservation,
	transitions []RuntimeTransitionInput,
) (RuntimeObservation, error) {
	prepared, err := prepareRuntimeObservation(observation)
	if err != nil {
		return RuntimeObservation{}, err
	}
	preparedTransitions, err := prepareRuntimeTransitions(transitions)
	if err != nil {
		return RuntimeObservation{}, err
	}
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		matches, err := runtimeObservationMatches(ctx, tx, expected)
		if err != nil {
			return err
		}
		if !matches {
			return ErrRuntimeIdentityMismatch
		}
		if err := recordRuntimeObservationTx(ctx, tx, prepared); err != nil {
			return err
		}
		for _, transition := range preparedTransitions {
			if _, err := appendRuntimeTransition(ctx, tx, transition); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return RuntimeObservation{}, err
	}
	return s.RuntimeObservation(ctx)
}

// AppendRuntimeObservationTransition appends evidence only while the exact
// persisted PID incarnation still matches. It is used to mark an observation
// uncertain without erasing the fence needed for a later recovery decision.
func (s *Store) AppendRuntimeObservationTransition(
	ctx context.Context,
	expected RuntimeObservation,
	input RuntimeTransitionInput,
) (bool, error) {
	prepared, err := prepareRuntimeTransition(input)
	if err != nil {
		return false, err
	}
	matched := false
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		matched, err = runtimeObservationMatches(ctx, tx, &expected)
		if err != nil || !matched {
			return err
		}
		_, err = appendRuntimeTransition(ctx, tx, prepared)
		return err
	})
	return matched && err == nil, err
}

// ClearRuntimeObservationAndTransitions clears and records lifecycle evidence
// under one exact PID/start-token fence. If the observation changed, neither
// the row nor history is touched.
func (s *Store) ClearRuntimeObservationAndTransitions(
	ctx context.Context,
	pid int,
	processStartToken string,
	transitions []RuntimeTransitionInput,
) (bool, error) {
	if pid <= 0 || !validProcessStartToken(processStartToken) {
		return false, errors.New("runtime PID and process start token are required")
	}
	preparedTransitions, err := prepareRuntimeTransitions(transitions)
	if err != nil {
		return false, err
	}
	cleared := false
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(
			ctx,
			`DELETE FROM runtime_observation
              WHERE singleton = 1 AND pid = ? AND process_start_token = ?`,
			pid,
			processStartToken,
		)
		if err != nil {
			return fmt.Errorf("clear fenced runtime observation: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect fenced runtime observation clear: %w", err)
		}
		if rows == 0 {
			return nil
		}
		if rows != 1 {
			return errors.New("fenced runtime observation clear affected multiple rows")
		}
		cleared = true
		for _, transition := range preparedTransitions {
			if _, err := appendRuntimeTransition(ctx, tx, transition); err != nil {
				return err
			}
		}
		return nil
	})
	return cleared && err == nil, err
}

func (s *Store) RuntimeObservation(ctx context.Context) (RuntimeObservation, error) {
	var observation RuntimeObservation
	var startedAt, observedAt string
	var stableObservedAt sql.NullString
	err := s.db.QueryRowContext(
		ctx,
		`SELECT pid, process_start_token, core_artifact_id, activation_bundle_id,
                exact_core_version, archive_sha256, binary_sha256, started_at,
                observed_at, stable_observed_at
           FROM runtime_observation WHERE singleton = 1`,
	).Scan(
		&observation.PID,
		&observation.ProcessStartToken,
		&observation.CoreArtifactID,
		&observation.ActivationBundleID,
		&observation.ExactCoreVersion,
		&observation.ArchiveSHA256,
		&observation.BinarySHA256,
		&startedAt,
		&observedAt,
		&stableObservedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeObservation{}, ErrRuntimeObservationNotFound
	}
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("read runtime observation: %w", err)
	}
	observation.StartedAt, err = parseTime(startedAt)
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("parse runtime started_at: %w", err)
	}
	observation.ObservedAt, err = parseTime(observedAt)
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("parse runtime observed_at: %w", err)
	}
	if stableObservedAt.Valid {
		parsed, err := parseTime(stableObservedAt.String)
		if err != nil {
			return RuntimeObservation{}, fmt.Errorf("parse runtime stable_observed_at: %w", err)
		}
		observation.StableObservedAt = &parsed
	}
	return observation, nil
}

// HeartbeatRuntimeObservation advances the durable observation watermark for
// one exact PID incarnation without claiming that the five-minute stability
// window has been proven.
func (s *Store) HeartbeatRuntimeObservation(
	ctx context.Context,
	pid int,
	processStartToken string,
	observedAt time.Time,
) (bool, error) {
	if pid <= 0 || !validProcessStartToken(processStartToken) || observedAt.IsZero() {
		return false, errors.New("runtime PID, process start token, and observation time are required")
	}
	observedAt = observedAt.UTC()
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE runtime_observation
            SET observed_at = ?
          WHERE singleton = 1 AND pid = ? AND process_start_token = ?
            AND started_at <= ? AND observed_at <= ?`,
		formatTime(observedAt),
		pid,
		processStartToken,
		formatTime(observedAt),
		formatTime(observedAt),
	)
	if err != nil {
		return false, fmt.Errorf("heartbeat fenced runtime observation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect fenced runtime observation heartbeat: %w", err)
	}
	return rows == 1, nil
}

// ConfirmRuntimeObservation advances liveness evidence only for the exact PID
// incarnation captured by the caller. A delayed confirmation from an older
// supervisor cannot overwrite or extend a newer child's observation.
func (s *Store) ConfirmRuntimeObservation(
	ctx context.Context,
	pid int,
	processStartToken string,
	observedAt time.Time,
) (bool, error) {
	if pid <= 0 || !validProcessStartToken(processStartToken) || observedAt.IsZero() {
		return false, errors.New("runtime PID, process start token, and observation time are required")
	}
	observedAt = observedAt.UTC()
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE runtime_observation
		    SET observed_at = ?,
		        stable_observed_at = COALESCE(stable_observed_at, ?)
		  WHERE singleton = 1 AND pid = ? AND process_start_token = ?
		    AND started_at <= ? AND observed_at <= ?`,
		formatTime(observedAt),
		formatTime(observedAt),
		pid,
		processStartToken,
		formatTime(observedAt),
		formatTime(observedAt),
	)
	if err != nil {
		return false, fmt.Errorf("confirm fenced runtime observation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect fenced runtime observation confirmation: %w", err)
	}
	return rows == 1, nil
}

// ClearRuntimeObservation only clears the exact PID incarnation supplied by
// its owner. A late cleanup from an old supervisor cannot erase a newer child.
func (s *Store) ClearRuntimeObservation(
	ctx context.Context,
	pid int,
	processStartToken string,
) (bool, error) {
	if pid <= 0 || !validProcessStartToken(processStartToken) {
		return false, errors.New("runtime PID and process start token are required")
	}
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM runtime_observation WHERE singleton = 1 AND pid = ? AND process_start_token = ?`,
		pid,
		processStartToken,
	)
	if err != nil {
		return false, fmt.Errorf("clear runtime observation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inspect runtime observation clear: %w", err)
	}
	return rows == 1, nil
}

func prepareRuntimeObservation(observation RuntimeObservation) (RuntimeObservation, error) {
	if observation.PID <= 0 || !validProcessStartToken(observation.ProcessStartToken) {
		return RuntimeObservation{}, errors.New("runtime PID and process start token are required")
	}
	if strings.TrimSpace(observation.CoreArtifactID) == "" || strings.TrimSpace(observation.ActivationBundleID) == "" {
		return RuntimeObservation{}, errors.New("runtime artifact and bundle IDs are required")
	}
	version, err := coreartifact.ParseExactVersion(observation.ExactCoreVersion)
	if err != nil || version.IsZero() {
		return RuntimeObservation{}, errors.New("runtime exact core version is invalid")
	}
	digest, err := coreartifact.ParseSHA256(observation.ArchiveSHA256)
	if err != nil || digest.IsZero() {
		return RuntimeObservation{}, errors.New("runtime artifact digest is invalid")
	}
	if observation.StartedAt.IsZero() || observation.ObservedAt.IsZero() || observation.ObservedAt.Before(observation.StartedAt) {
		return RuntimeObservation{}, errors.New("runtime observation timestamps are invalid")
	}
	observation.ExactCoreVersion = version.String()
	observation.ArchiveSHA256 = digest.String()
	binaryDigest, err := coreartifact.ParseSHA256(observation.BinarySHA256)
	if err != nil || binaryDigest.IsZero() {
		return RuntimeObservation{}, errors.New("runtime binary digest is invalid")
	}
	observation.BinarySHA256 = binaryDigest.String()
	observation.StartedAt = observation.StartedAt.UTC()
	observation.ObservedAt = observation.ObservedAt.UTC()
	observation.StableObservedAt = normalizedRuntimeTime(observation.StableObservedAt)
	if observation.StableObservedAt != nil &&
		(observation.StableObservedAt.Before(observation.StartedAt) || observation.StableObservedAt.After(observation.ObservedAt)) {
		return RuntimeObservation{}, errors.New("runtime stable observation timestamp is invalid")
	}
	return observation, nil
}

func recordRuntimeObservationTx(
	ctx context.Context,
	tx *sql.Tx,
	observation RuntimeObservation,
) error {
	artifact, err := getCoreArtifact(ctx, tx, observation.CoreArtifactID)
	if err != nil {
		return err
	}
	if artifact.ExactVersion != observation.ExactCoreVersion ||
		artifact.ReportedVersion != observation.ExactCoreVersion ||
		artifact.ArchiveSHA256 != observation.ArchiveSHA256 ||
		artifact.BinarySHA256 != observation.BinarySHA256 {
		return ErrRuntimeIdentityMismatch
	}
	var bundleArtifactID string
	if err := tx.QueryRowContext(
		ctx,
		`SELECT startup.core_artifact_id
           FROM activation_bundles AS bundle
           JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
          WHERE bundle.id = ?`,
		observation.ActivationBundleID,
	).Scan(&bundleArtifactID); errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: activation bundle not found", ErrRuntimeIdentityMismatch)
	} else if err != nil {
		return fmt.Errorf("read runtime bundle identity: %w", err)
	}
	if bundleArtifactID != observation.CoreArtifactID {
		return ErrRuntimeIdentityMismatch
	}
	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO runtime_observation(
                singleton, pid, process_start_token, core_artifact_id,
                activation_bundle_id, exact_core_version, archive_sha256,
                binary_sha256, started_at, observed_at, stable_observed_at
             ) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
             ON CONFLICT(singleton) DO UPDATE SET
                stable_observed_at = CASE
                    WHEN runtime_observation.pid = excluded.pid
                     AND runtime_observation.process_start_token = excluded.process_start_token
                     AND runtime_observation.started_at = excluded.started_at
                    THEN COALESCE(excluded.stable_observed_at, runtime_observation.stable_observed_at)
                    ELSE excluded.stable_observed_at
                END,
                pid = excluded.pid,
                process_start_token = excluded.process_start_token,
                core_artifact_id = excluded.core_artifact_id,
                activation_bundle_id = excluded.activation_bundle_id,
                exact_core_version = excluded.exact_core_version,
                archive_sha256 = excluded.archive_sha256,
                binary_sha256 = excluded.binary_sha256,
                started_at = excluded.started_at,
                observed_at = excluded.observed_at`,
		observation.PID,
		observation.ProcessStartToken,
		observation.CoreArtifactID,
		observation.ActivationBundleID,
		observation.ExactCoreVersion,
		observation.ArchiveSHA256,
		observation.BinarySHA256,
		formatTime(observation.StartedAt),
		formatTime(observation.ObservedAt),
		nullRuntimeTime(observation.StableObservedAt),
	)
	if err != nil {
		return fmt.Errorf("record runtime observation: %w", err)
	}
	return nil
}

func runtimeObservationMatches(
	ctx context.Context,
	q queryRower,
	expected *RuntimeObservation,
) (bool, error) {
	var pid int
	var processStartToken, startedAt string
	err := q.QueryRowContext(
		ctx,
		`SELECT pid, process_start_token, started_at
           FROM runtime_observation
          WHERE singleton = 1`,
	).Scan(&pid, &processStartToken, &startedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return expected == nil, nil
	}
	if err != nil {
		return false, fmt.Errorf("read runtime observation fence: %w", err)
	}
	if expected == nil {
		return false, nil
	}
	parsedStartedAt, err := parseTime(startedAt)
	if err != nil {
		return false, fmt.Errorf("parse runtime observation fence: %w", err)
	}
	return pid == expected.PID && processStartToken == expected.ProcessStartToken &&
		parsedStartedAt.Equal(expected.StartedAt), nil
}

func prepareRuntimeTransitions(inputs []RuntimeTransitionInput) ([]RuntimeTransitionInput, error) {
	prepared := make([]RuntimeTransitionInput, len(inputs))
	for index, input := range inputs {
		transition, err := prepareRuntimeTransition(input)
		if err != nil {
			return nil, fmt.Errorf("prepare runtime transition %d: %w", index, err)
		}
		prepared[index] = transition
	}
	return prepared, nil
}

func validProcessStartToken(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}
