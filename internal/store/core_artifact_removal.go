// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// CoreArtifactRemovalEligibility reports whether deletion is currently safe.
func (s *Store) CoreArtifactRemovalEligibility(
	ctx context.Context,
	artifactID string,
) (CoreArtifactRemovalEligibility, error) {
	if strings.TrimSpace(artifactID) == "" {
		return CoreArtifactRemovalEligibility{}, errors.New("core artifact id is empty")
	}
	return coreArtifactRemovalEligibility(ctx, s.db, artifactID)
}

// RemoveCoreArtifact performs the eligibility check and deletion in one short
// transaction, closing the check/delete race.
func (s *Store) RemoveCoreArtifact(ctx context.Context, artifactID string) error {
	if strings.TrimSpace(artifactID) == "" {
		return errors.New("core artifact id is empty")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		eligibility, err := coreArtifactRemovalEligibility(ctx, tx, artifactID)
		if err != nil {
			return err
		}
		if !eligibility.Eligible {
			return fmt.Errorf("%w: %s", ErrCoreArtifactInUse, artifactID)
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM core_artifacts WHERE id = ?`, artifactID)
		if err != nil {
			return fmt.Errorf("remove core artifact: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect core artifact removal: %w", err)
		}
		if rows != 1 {
			return ErrCoreArtifactNotFound
		}
		return nil
	})
}

func coreArtifactRemovalEligibility(
	ctx context.Context,
	q queryRower,
	artifactID string,
) (CoreArtifactRemovalEligibility, error) {
	if _, err := getCoreArtifact(ctx, q, artifactID); err != nil {
		return CoreArtifactRemovalEligibility{}, err
	}

	var result CoreArtifactRemovalEligibility
	err := q.QueryRowContext(
		ctx,
		`SELECT
            (SELECT count(*) FROM startup_artifacts WHERE core_artifact_id = ?),
            (
                SELECT count(*)
                  FROM activation_bundles AS bundle
                  JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
                 WHERE startup.core_artifact_id = ?
                   AND bundle.id IN (
                        SELECT desired_bundle_id FROM hub_state
                        UNION SELECT applied_bundle_id FROM hub_state
                        UNION SELECT rollback_bundle_id FROM hub_state
                   )
            ),
            (
                SELECT count(*)
                  FROM tasks AS task
                  JOIN startup_artifacts AS startup ON startup.id = task.startup_artifact_id
                 WHERE startup.core_artifact_id = ?
                   AND task.status IN ('queued', 'running')
            )`,
		artifactID,
		artifactID,
		artifactID,
	).Scan(
		&result.StartupArtifactReferences,
		&result.ActiveBundleReferences,
		&result.ActiveTaskReferences,
	)
	if err != nil {
		return CoreArtifactRemovalEligibility{}, fmt.Errorf("inspect core artifact references: %w", err)
	}
	result.Eligible = result.StartupArtifactReferences == 0 &&
		result.ActiveBundleReferences == 0 && result.ActiveTaskReferences == 0
	return result, nil
}
