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
		artifact, err := getCoreArtifact(ctx, tx, prepared.CoreArtifactID)
		if err != nil {
			return err
		}
		if artifact.ExactVersion != prepared.ExactCoreVersion ||
			artifact.ReportedVersion != prepared.ExactCoreVersion ||
			artifact.ArchiveSHA256 != prepared.ArchiveSHA256 ||
			artifact.BinarySHA256 != prepared.BinarySHA256 ||
			artifact.VerificationState != CoreArtifactVerified {
			return ErrRuntimeIdentityMismatch
		}
		var bundleArtifactID string
		if err := tx.QueryRowContext(
			ctx,
			`SELECT startup.core_artifact_id
               FROM activation_bundles AS bundle
               JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
              WHERE bundle.id = ?`,
			prepared.ActivationBundleID,
		).Scan(&bundleArtifactID); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: activation bundle not found", ErrRuntimeIdentityMismatch)
		} else if err != nil {
			return fmt.Errorf("read runtime bundle identity: %w", err)
		}
		if bundleArtifactID != prepared.CoreArtifactID {
			return ErrRuntimeIdentityMismatch
		}
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO runtime_observation(
                    singleton, pid, process_start_token, core_artifact_id,
                    activation_bundle_id, exact_core_version, archive_sha256,
                    binary_sha256, started_at, observed_at
                 ) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                 ON CONFLICT(singleton) DO UPDATE SET
                    pid = excluded.pid,
                    process_start_token = excluded.process_start_token,
                    core_artifact_id = excluded.core_artifact_id,
                    activation_bundle_id = excluded.activation_bundle_id,
                    exact_core_version = excluded.exact_core_version,
                    archive_sha256 = excluded.archive_sha256,
                    binary_sha256 = excluded.binary_sha256,
                    started_at = excluded.started_at,
                    observed_at = excluded.observed_at`,
			prepared.PID,
			prepared.ProcessStartToken,
			prepared.CoreArtifactID,
			prepared.ActivationBundleID,
			prepared.ExactCoreVersion,
			prepared.ArchiveSHA256,
			prepared.BinarySHA256,
			formatTaskTime(prepared.StartedAt),
			formatTaskTime(prepared.ObservedAt),
		)
		if err != nil {
			return fmt.Errorf("record runtime observation: %w", err)
		}
		return nil
	})
	if err != nil {
		return RuntimeObservation{}, err
	}
	return s.RuntimeObservation(ctx)
}

func (s *Store) RuntimeObservation(ctx context.Context) (RuntimeObservation, error) {
	var observation RuntimeObservation
	var startedAt, observedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT pid, process_start_token, core_artifact_id, activation_bundle_id,
                exact_core_version, archive_sha256, binary_sha256, started_at, observed_at
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
	)
	if errors.Is(err, sql.ErrNoRows) {
		return RuntimeObservation{}, ErrRuntimeObservationNotFound
	}
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("read runtime observation: %w", err)
	}
	observation.StartedAt, err = parseTaskTime(startedAt)
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("parse runtime started_at: %w", err)
	}
	observation.ObservedAt, err = parseTaskTime(observedAt)
	if err != nil {
		return RuntimeObservation{}, fmt.Errorf("parse runtime observed_at: %w", err)
	}
	return observation, nil
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
		    SET observed_at = ?
		  WHERE singleton = 1 AND pid = ? AND process_start_token = ?
		    AND started_at <= ? AND observed_at <= ?`,
		formatTaskTime(observedAt),
		pid,
		processStartToken,
		formatTaskTime(observedAt),
		formatTaskTime(observedAt),
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
	return observation, nil
}

func validProcessStartToken(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}
