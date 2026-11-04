// SPDX-License-Identifier: GPL-3.0-or-later

package store

import "context"

// RuntimeHubState reads control pointers without materializing the saved config.
func (s *Store) RuntimeHubState(ctx context.Context) (HubState, error) {
	var hub HubState
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(head_revision_id,''), COALESCE(desired_bundle_id,''),
 COALESCE(applied_bundle_id,''), COALESCE(rollback_bundle_id,''), target_generation, desired_running
 FROM hub_state WHERE singleton=1`).Scan(&hub.HeadRevisionID, &hub.DesiredBundleID, &hub.AppliedBundleID, &hub.RollbackBundleID, &hub.TargetGeneration, &hub.DesiredRunning)
	return hub, err
}

type RuntimeCoreProjection struct{ CoreArtifactID, ExactCoreVersion, CanonicalRevisionID string }

func (s *Store) RuntimeCore(ctx context.Context, bundleID string) (RuntimeCoreProjection, error) {
	var result RuntimeCoreProjection
	err := s.db.QueryRowContext(ctx, `SELECT s.core_artifact_id, s.exact_core_version, s.canonical_revision_id
 FROM activation_bundles a JOIN startup_artifacts s ON s.id=a.startup_artifact_id WHERE a.id=?`, bundleID).Scan(
		&result.CoreArtifactID, &result.ExactCoreVersion, &result.CanonicalRevisionID)
	return result, err
}
