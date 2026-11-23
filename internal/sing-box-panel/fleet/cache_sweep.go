// SPDX-License-Identifier: GPL-3.0-or-later

package fleet

import (
	"context"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// ArtifactCacheSweepInterval is how often the panel removes cached core
// archives that no node needs.
const ArtifactCacheSweepInterval = time.Hour

// SweepArtifactCache removes the cached official archives that no node
// reports installed, no install is fetching and no install asked for within
// InstallTimeout. An install that needs one later downloads it again.
// Uploaded archives stay, because the panel cannot fetch them again.
func (service *Service) SweepArtifactCache(ctx context.Context) (int, error) {
	now := service.now().UTC()
	candidates, err := service.cores.OfficialArtifactsUnusedSince(ctx, now.Add(-InstallTimeout))
	if err != nil || len(candidates) == 0 {
		return 0, err
	}
	needed := service.grants.artifacts(now)
	records, err := service.database.Nodes(ctx)
	if err != nil {
		return 0, err
	}
	for _, record := range records {
		if state := nodeState(record); state != nil {
			for _, core := range state.Report.GetInstalled() {
				needed[core.GetArtifact().GetId()] = true
			}
		}
	}
	removed := 0
	for _, artifact := range candidates {
		if needed[artifact.ID] {
			continue
		}
		if err := service.cores.RemoveCachedArtifact(ctx, artifact.ID); err != nil && !errors.Is(err, store.ErrCachedArtifactNotFound) {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
