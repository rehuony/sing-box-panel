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
func (s *Store) RequestConfigurationRuntimeIntent(ctx context.Context, input RuntimeIntentInput, artifact StartupArtifact) (RuntimeIntent, error) {
	prepared, err := prepareRuntimeIntent(input)
	if err != nil {
		return RuntimeIntent{}, err
	}
	if prepared.Kind != RuntimeIntentStart && prepared.Kind != RuntimeIntentRestart {
		return RuntimeIntent{}, errors.New("configuration runtime intent must start or restart")
	}
	if prepared.SelectOnly && prepared.Kind != RuntimeIntentRestart {
		return RuntimeIntent{}, errors.New("selection-only intent must be a checked version switch")
	}
	startup, err := prepareNewStartupArtifact(artifact)
	if err != nil {
		return RuntimeIntent{}, err
	}
	var result RuntimeIntent
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
		result, err = beginRuntimeIntentTx(ctx, tx, prepared, "", stored.CanonicalRevisionID, stored.ID, generation, true, false)
		return err
	})
	return result, err
}

// BindCheckedRuntimeIntent attaches a ready bundle only while the same worker
// owns the current runtime generation and the configuration is still current.
func (s *Store) BindCheckedRuntimeIntent(ctx context.Context, intent RuntimeIntent, bundleID string, now time.Time) (RuntimeIntent, error) {
	var result RuntimeIntent
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var head sql.NullString
		var generation int64
		if err := tx.QueryRowContext(ctx, `SELECT head_revision_id,target_generation FROM hub_state WHERE singleton=1`).Scan(&head, &generation); err != nil {
			return err
		}
		if intent.Generation != generation {
			return ErrRuntimeIntentStale
		}
		if intent.StartupArtifactID == "" {
			return ErrRuntimeIntentStale
		}
		if err := validateApplicableBundle(ctx, tx, bundleID, valueOrEmpty(head)); err != nil {
			return err
		}
		var startupID string
		if err := tx.QueryRowContext(ctx, `SELECT startup_artifact_id FROM activation_bundles WHERE id=?`, bundleID).Scan(&startupID); err != nil {
			return err
		}
		if startupID != intent.StartupArtifactID {
			return ErrRuntimeIntentStale
		}
		if _, err := tx.ExecContext(ctx, `UPDATE hub_state SET desired_bundle_id=?,desired_running=?,updated_at=? WHERE singleton=1`, bundleID, boolInt(!CoreSelectionOnly(intent)), formatTime(now)); err != nil {
			return err
		}
		result = intent
		result.ActivationBundleID = bundleID
		return nil
	})
	return result, err
}
