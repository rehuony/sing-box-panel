// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrNoAppliedBundle    = errors.New("no activation bundle has been applied")
	ErrNoRollbackBundle   = errors.New("no rollback activation bundle is available")
	ErrRuntimeIntentStale = errors.New("runtime intent evidence is stale")

	ErrCoreNotEnabled = errors.New("the requested core is no longer enabled")
)

type RuntimeIntentKind string

const (
	RuntimeIntentApply    RuntimeIntentKind = "runtime-apply"
	RuntimeIntentStart    RuntimeIntentKind = "runtime-start"
	RuntimeIntentStop     RuntimeIntentKind = "runtime-stop"
	RuntimeIntentRestart  RuntimeIntentKind = "runtime-restart"
	RuntimeIntentRollback RuntimeIntentKind = "runtime-rollback"
)

type RuntimeIntentInput struct {
	SelectOnly    bool   // Checked version selection without starting a stopped process.
	DisableCoreID string // Stop and clear this selection, fenced against a different enabled core.
	Kind          RuntimeIntentKind
	BundleID      string
	CreatedAt     time.Time
}

// RequestRuntimeIntent advances the desired generation and reserves its generation
// in the same transaction. Start/restart resolve to the last applied bundle,
// rollback resolves to the frozen prior bundle, and apply requires an explicit
// ready bundle built from the current head.
func (s *Store) RequestRuntimeIntent(ctx context.Context, input RuntimeIntentInput) (RuntimeIntent, error) {
	prepared, err := prepareRuntimeIntent(input)
	if err != nil {
		return RuntimeIntent{}, err
	}
	if prepared.SelectOnly {
		return RuntimeIntent{}, errors.New("version selection requires a checked configuration candidate")
	}
	return s.beginRuntimeIntent(ctx, prepared)
}

func (s *Store) beginRuntimeIntent(ctx context.Context, input RuntimeIntentInput) (RuntimeIntent, error) {
	var intent RuntimeIntent
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
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
			if err := validateApplicableBundle(ctx, tx, bundleID, valueOrEmpty(headID)); err != nil {
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
				return fmt.Errorf("%w: rollback bundle changed", ErrRuntimeIntentStale)
			}
			if err := validateRunnableBundle(ctx, tx, bundleID); err != nil {
				return err
			}
		case RuntimeIntentStop:
			if bundleID != "" {
				return errors.New("stop intent does not accept a bundle")
			}
			bundleID = valueOrEmpty(appliedBundleID)
			if input.DisableCoreID != "" {
				var enabledCoreID string
				err := tx.QueryRowContext(ctx, `SELECT startup.core_artifact_id
					FROM activation_bundles AS bundle
					JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
					WHERE bundle.id = ?`, bundleID).Scan(&enabledCoreID)
				if errors.Is(err, sql.ErrNoRows) || (err == nil && enabledCoreID != input.DisableCoreID) {
					return ErrCoreNotEnabled
				}
				if err != nil {
					return fmt.Errorf("read enabled core for disable: %w", err)
				}
			}
		}

		var beginErr error
		intent, beginErr = beginRuntimeIntentTx(ctx, tx, input, bundleID, "", "", generation, desiredRunning, true)
		return beginErr
	})
	return intent, err
}

func beginRuntimeIntentTx(ctx context.Context, tx *sql.Tx, input RuntimeIntentInput, bundleID, canonicalID, startupID string, generation int64, desiredRunning, advanceDesired bool) (RuntimeIntent, error) {
	generation++
	createdAt := formatTime(input.CreatedAt)
	if advanceDesired {
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
			return RuntimeIntent{}, fmt.Errorf("advance runtime intent: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE hub_state SET target_generation=?, updated_at=? WHERE singleton=1`, generation, createdAt); err != nil {
		return RuntimeIntent{}, fmt.Errorf("reserve runtime generation: %w", err)
	}

	// Explicit commands reset the bounded automatic recovery episode.
	if _, err := tx.ExecContext(ctx, `DELETE FROM runtime_recovery WHERE singleton=1`); err != nil {
		return RuntimeIntent{}, err
	}
	return RuntimeIntent{Kind: input.Kind, Generation: generation,
		ActivationBundleID: bundleID, CanonicalRevisionID: canonicalID,
		StartupArtifactID: startupID, SelectOnly: input.SelectOnly, DisableCoreID: input.DisableCoreID}, nil
}

func prepareRuntimeIntent(input RuntimeIntentInput) (RuntimeIntentInput, error) {
	switch input.Kind {
	case RuntimeIntentApply, RuntimeIntentStart, RuntimeIntentStop, RuntimeIntentRestart, RuntimeIntentRollback:
	default:
		return RuntimeIntentInput{}, fmt.Errorf("invalid runtime intent %q", input.Kind)
	}
	if input.Kind == RuntimeIntentApply && strings.TrimSpace(input.BundleID) == "" {
		return RuntimeIntentInput{}, errors.New("apply intent requires an activation bundle")
	}
	if input.DisableCoreID != "" && (input.Kind != RuntimeIntentStop || input.SelectOnly) {
		return RuntimeIntentInput{}, errors.New("disabling a core requires a stop intent")
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	} else {
		input.CreatedAt = input.CreatedAt.UTC()
	}
	return input, nil
}

func validateApplicableBundle(ctx context.Context, tx *sql.Tx, bundleID, headID string) error {
	if err := requireCurrentConfigurationFileTx(ctx, tx, headID); err != nil {
		return err
	}
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
	err := tx.QueryRowContext(
		ctx,
		`SELECT startup.state, startup.canonical_revision_id
           FROM activation_bundles AS bundle
           JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
           JOIN core_artifacts AS core ON core.id = startup.core_artifact_id
          WHERE bundle.id = ?`,
		bundleID,
	).Scan(&state, &canonicalRevisionID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrActivationBundleNotFound
	}
	if err != nil {
		return fmt.Errorf("read activation bundle eligibility: %w", err)
	}
	if state != StartupArtifactReady {
		return ErrActivationBundleNotReady
	}
	return nil
}

func commitSuccessfulRuntimeIntent(
	ctx context.Context,
	tx *sql.Tx,
	intent RuntimeIntent,
	completedAt time.Time,
) error {
	switch RuntimeIntentKind(intent.Kind) {
	case RuntimeIntentApply, RuntimeIntentStart, RuntimeIntentRestart:
		if intent.ActivationBundleID == "" {
			return errors.New("successful runtime intent has no activation bundle")
		}
		if err := validateRunnableBundle(ctx, tx, intent.ActivationBundleID); err != nil {
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
		if appliedBundleID.Valid && appliedBundleID.String != intent.ActivationBundleID {
			rollback = appliedBundleID.String
		}
		result, err := tx.ExecContext(
			ctx,
			`UPDATE hub_state
                    SET desired_bundle_id = ?, applied_bundle_id = ?,
                        rollback_bundle_id = ?, desired_running = ?,
                        applied_at = CASE
                            WHEN applied_bundle_id IS NULL OR applied_bundle_id <> ? THEN ?
                            ELSE applied_at
                        END,
                        updated_at = ?
                  WHERE singleton = 1 AND target_generation = ?`,
			intent.ActivationBundleID,
			intent.ActivationBundleID,
			nullIfEmpty(rollback),
			boolInt(!CoreSelectionOnly(intent)),
			intent.ActivationBundleID,
			formatTime(completedAt),
			formatTime(completedAt),
			intent.Generation,
		)
		if err != nil {
			return fmt.Errorf("commit applied activation bundle: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return errors.Join(ErrRuntimeIntentStale, err)
		}
	case RuntimeIntentRollback:
		if intent.ActivationBundleID == "" {
			return errors.New("successful rollback intent has no activation bundle")
		}
		if err := validateRunnableBundle(ctx, tx, intent.ActivationBundleID); err != nil {
			return err
		}
		var appliedBundleID, rollbackBundleID sql.NullString
		if err := tx.QueryRowContext(
			ctx,
			`SELECT applied_bundle_id, rollback_bundle_id FROM hub_state WHERE singleton = 1`,
		).Scan(&appliedBundleID, &rollbackBundleID); err != nil {
			return fmt.Errorf("read rollback runtime state: %w", err)
		}
		if valueOrEmpty(rollbackBundleID) != intent.ActivationBundleID || !appliedBundleID.Valid {
			return errors.New("frozen rollback bundle changed before completion")
		}
		result, err := tx.ExecContext(
			ctx,
			`UPDATE hub_state
			        SET desired_bundle_id = ?, applied_bundle_id = ?, rollback_bundle_id = ?,
			            desired_running = 1, applied_at = ?, updated_at = ?
			      WHERE singleton = 1 AND target_generation = ?`,
			intent.ActivationBundleID,
			intent.ActivationBundleID,
			appliedBundleID.String,
			formatTime(completedAt),
			formatTime(completedAt),
			intent.Generation,
		)
		if err != nil {
			return fmt.Errorf("commit rollback activation bundle: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return errors.Join(ErrRuntimeIntentStale, err)
		}
	case RuntimeIntentStop:
		clearSelection := disabledCoreID(intent) != ""
		result, err := tx.ExecContext(
			ctx,
			`UPDATE hub_state SET desired_running = 0,
			      desired_bundle_id = CASE WHEN ? THEN NULL ELSE desired_bundle_id END,
			      applied_bundle_id = CASE WHEN ? THEN NULL ELSE applied_bundle_id END,
			      rollback_bundle_id = CASE WHEN ? THEN NULL ELSE rollback_bundle_id END,
			      applied_at = CASE WHEN ? THEN NULL ELSE applied_at END,
			      updated_at = ?
                  WHERE singleton = 1 AND target_generation = ?`,
			clearSelection, clearSelection, clearSelection, clearSelection,
			formatTime(completedAt),
			intent.Generation,
		)
		if err != nil {
			return fmt.Errorf("commit stopped runtime state: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return errors.Join(ErrRuntimeIntentStale, err)
		}
	}
	return nil
}

func disabledCoreID(intent RuntimeIntent) string  { return intent.DisableCoreID }
func CoreSelectionOnly(intent RuntimeIntent) bool { return intent.SelectOnly }
