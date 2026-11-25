// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// retentionBatch bounds each delete of a retention pass. Every batch is its
// own transaction, so other writers, such as arriving node samples, get the
// database between batches instead of waiting out one large delete.
const retentionBatch = 2000

// deleteInBatches runs query, a DELETE whose last placeholder is the batch
// size, until a batch deletes fewer rows than the batch size.
func (s *Store) deleteInBatches(ctx context.Context, name, query string, args ...any) (int64, error) {
	var total int64
	for {
		result, err := s.db.ExecContext(ctx, query, append(args, retentionBatch)...)
		if err != nil {
			return total, fmt.Errorf("delete %s: %w", name, err)
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("read deleted %s count: %w", name, err)
		}
		total += deleted
		if deleted < retentionBatch {
			return total, nil
		}
	}
}

// DeleteTrafficSamplesBefore removes raw traffic samples taken before
// cutoff. Daily traffic and periods stay.
func (s *Store) DeleteTrafficSamplesBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, errors.New("traffic sample retention cutoff is required")
	}
	return s.deleteInBatches(ctx, "expired traffic samples",
		`DELETE FROM traffic_samples WHERE rowid IN (SELECT rowid FROM traffic_samples WHERE sampled_at < ? LIMIT ?)`,
		formatTime(cutoff.UTC()))
}

// DeleteRuntimeTransitionsBefore removes runtime transitions that occurred
// before cutoff, except the newest of each node instance: it gives the
// state a timeline starts in, and it is where the node's next session
// resumes replicating its history, which would otherwise be sent again.
func (s *Store) DeleteRuntimeTransitionsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, errors.New("runtime history retention cutoff is required")
	}
	return s.deleteInBatches(ctx, "expired runtime transitions",
		`DELETE FROM runtime_transitions WHERE id IN (
			SELECT transition.id FROM runtime_transitions AS transition
			 WHERE transition.occurred_at < ?
			   AND transition.source_id < (
				SELECT max(newest.source_id) FROM runtime_transitions AS newest
				 WHERE newest.node_id = transition.node_id AND newest.instance_id = transition.instance_id)
			 LIMIT ?)`,
		formatTime(cutoff.UTC()))
}

// PruneCanonicalRevisions keeps each node's newest keep revisions and every
// revision still in use: the current text's, the latest that parsed and the
// one the node reports applied, which its subscription inbounds come from.
func (s *Store) PruneCanonicalRevisions(ctx context.Context, keep int) (int64, error) {
	if keep < 1 {
		return 0, errors.New("revision retention must keep at least one revision")
	}
	return s.deleteInBatches(ctx, "old configuration revisions",
		`DELETE FROM canonical_revisions WHERE id IN (
			SELECT ranked.id FROM (
				SELECT id, node_id, ROW_NUMBER() OVER (PARTITION BY node_id ORDER BY sequence DESC) AS position
				  FROM canonical_revisions) AS ranked
			 WHERE ranked.position > ?
			   AND NOT EXISTS (SELECT 1 FROM node_configurations AS file
				WHERE file.canonical_revision_id = ranked.id OR file.head_revision_id = ranked.id)
			   AND NOT EXISTS (SELECT 1 FROM nodes WHERE nodes.id = ranked.node_id AND nodes.applied_revision_id = ranked.id)
			 LIMIT ?)`,
		keep)
}
