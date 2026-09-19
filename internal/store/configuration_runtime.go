// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// RequestConfigurationRuntimeIntent reserves the runtime lane but leaves the
// currently desired process unchanged until the selected binary accepts the
// saved configuration. This keeps a failed preflight from stopping a service.
func (s *Store) RequestConfigurationRuntimeIntent(ctx context.Context, input RuntimeIntentInput, artifact StartupArtifact) (Task, error) {
	prepared, err := prepareRuntimeIntent(input)
	if err != nil {
		return Task{}, err
	}
	if prepared.Kind != RuntimeIntentStart && prepared.Kind != RuntimeIntentRestart {
		return Task{}, errors.New("configuration runtime intent must start or restart")
	}
	startup, err := prepareNewStartupArtifact(artifact)
	if err != nil {
		return Task{}, err
	}
	var result Task
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		var head sql.NullString
		var generation int64
		if err := tx.QueryRowContext(ctx, `SELECT head_revision_id, target_generation FROM hub_state WHERE singleton=1`).Scan(&head, &generation); err != nil {
			return err
		}
		if valueOrEmpty(head) != startup.CanonicalRevisionID {
			return ErrCompiledStartupEvidenceStale
		}
		if err := requireCurrentConfigurationFileTx(ctx, tx, startup.CanonicalRevisionID); err != nil {
			return err
		}
		stored, err := insertStartupArtifactTx(ctx, tx, startup)
		if err != nil {
			return err
		}
		result, err = enqueueRuntimeIntentTx(ctx, tx, prepared, "", stored.CanonicalRevisionID, stored.ID, generation, true, false)
		return err
	})
	return result, err
}

// BindCheckedRuntimeTask attaches a ready bundle only while the same worker
// owns the current runtime generation and the configuration is still current.
func (s *Store) BindCheckedRuntimeTask(ctx context.Context, task Task, bundleID string, now time.Time) (Task, error) {
	var result Task
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		current, err := getTask(ctx, tx, task.ID)
		if err != nil {
			return err
		}
		if current.Status != TaskStatusRunning || current.LeaseOwner != task.LeaseOwner || current.LeaseExpiresAt == nil || !current.LeaseExpiresAt.After(now) {
			return ErrTaskLeaseLost
		}
		var head sql.NullString
		var generation int64
		if err := tx.QueryRowContext(ctx, `SELECT head_revision_id,target_generation FROM hub_state WHERE singleton=1`).Scan(&head, &generation); err != nil {
			return err
		}
		if current.CancelRequested || current.Generation != generation || current.Generation != task.Generation {
			return ErrTaskGenerationConflict
		}
		if current.Lane != TaskLaneRuntime || current.StartupArtifactID == "" || current.StartupArtifactID != task.StartupArtifactID {
			return ErrRuntimeIntentStale
		}
		if err := validateApplicableBundle(ctx, tx, bundleID, valueOrEmpty(head)); err != nil {
			return err
		}
		var startupID string
		if err := tx.QueryRowContext(ctx, `SELECT startup_artifact_id FROM activation_bundles WHERE id=?`, bundleID).Scan(&startupID); err != nil {
			return err
		}
		if startupID != current.StartupArtifactID {
			return ErrRuntimeIntentStale
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET activation_bundle_id=?,updated_at=? WHERE id=?`, bundleID, formatTaskTime(now), task.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hub_state SET desired_bundle_id=?,desired_running=1,updated_at=? WHERE singleton=1`, bundleID, formatTaskTime(now)); err != nil {
			return err
		}
		result, err = getTask(ctx, tx, task.ID)
		return err
	})
	return result, err
}
