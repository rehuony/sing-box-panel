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

// CoreArtifactRemoval is what removing a core leaves to do outside the
// database.
type CoreArtifactRemoval struct {
	// Artifact is the core as it was installed.
	Artifact CoreArtifact
	// FilesShared reports another installed core with the same archive,
	// whose files must stay.
	FilesShared bool
}

// RemoveCoreArtifact removes an installed core unless the node selected or
// runs it: the desired or applied activation bundle starts it, or the
// observed process is it. Its row stays, marked removed, because the startup
// records, activation bundles and append-only runtime history that name it
// keep their references; every operation then treats it as not installed,
// and installing it again restores it. A rollback pointer to it is cleared,
// as the rollback could no longer run it. The caller deletes its files unless
// another installed core shares them.
func (s *Store) RemoveCoreArtifact(ctx context.Context, artifactID string, removedAt time.Time) (CoreArtifactRemoval, error) {
	if strings.TrimSpace(artifactID) == "" {
		return CoreArtifactRemoval{}, errors.New("core artifact id is empty")
	}
	var removal CoreArtifactRemoval
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		artifact, err := getCoreArtifact(ctx, tx, artifactID)
		if err != nil {
			return err
		}
		inUse, err := coreArtifactInUse(ctx, tx, artifactID)
		if err != nil {
			return err
		}
		if inUse {
			return fmt.Errorf("%w: %s", ErrCoreArtifactInUse, artifactID)
		}
		at := formatTime(removedAt.UTC())
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE hub_state SET rollback_bundle_id = NULL, updated_at = ?
              WHERE singleton = 1 AND rollback_bundle_id IN (
                    SELECT bundle.id
                      FROM activation_bundles AS bundle
                      JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
                     WHERE startup.core_artifact_id = ?
              )`,
			at, artifactID,
		); err != nil {
			return fmt.Errorf("clear rollback to removed core artifact: %w", err)
		}
		result, err := tx.ExecContext(ctx, `UPDATE core_artifacts SET removed_at = ? WHERE id = ? AND removed_at IS NULL`, at, artifactID)
		if err != nil {
			return fmt.Errorf("remove core artifact: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return errors.Join(ErrCoreArtifactNotFound, err)
		}
		var shared bool
		if err := tx.QueryRowContext(
			ctx,
			`SELECT EXISTS(SELECT 1 FROM core_artifacts WHERE archive_sha256 = ? AND id <> ? AND removed_at IS NULL)`,
			artifact.ArchiveSHA256, artifactID,
		).Scan(&shared); err != nil {
			return fmt.Errorf("inspect shared core artifact files: %w", err)
		}
		removal = CoreArtifactRemoval{Artifact: artifact, FilesShared: shared}
		return nil
	})
	return removal, err
}

// coreArtifactInUse reports whether the node selected or runs the core: the
// desired or applied activation bundle starts it, or the observed process is
// it. Its earlier startup records and bundles are history and do not count.
func coreArtifactInUse(ctx context.Context, q queryRower, artifactID string) (bool, error) {
	var inUse bool
	err := q.QueryRowContext(
		ctx,
		`SELECT EXISTS(
                SELECT 1
                  FROM hub_state AS hub
                  JOIN activation_bundles AS bundle
                    ON bundle.id IN (hub.desired_bundle_id, hub.applied_bundle_id)
                  JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
                 WHERE startup.core_artifact_id = ?
            ) OR EXISTS(SELECT 1 FROM runtime_observation WHERE core_artifact_id = ?)`,
		artifactID, artifactID,
	).Scan(&inUse)
	if err != nil {
		return false, fmt.Errorf("inspect core artifact use: %w", err)
	}
	return inUse, nil
}
