// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrCompiledStartupEvidenceStale = errors.New("compiled startup evidence is stale")

// CompiledStartupEvidence binds startup bytes to the immutable global head
// observed before the short insert transaction.
type CompiledStartupEvidence struct {
	ExpectedCanonicalHeadID string
}

func (s *Store) CreateCompiledStartupArtifact(
	ctx context.Context,
	artifact StartupArtifact,
	evidence CompiledStartupEvidence,
) (StartupArtifact, error) {
	preparedArtifact, err := prepareNewStartupArtifact(artifact)
	if err != nil {
		return StartupArtifact{}, err
	}
	if evidence.ExpectedCanonicalHeadID == "" || evidence.ExpectedCanonicalHeadID != preparedArtifact.CanonicalRevisionID {
		return StartupArtifact{}, errors.New("compiled startup evidence is missing or inconsistent")
	}

	var result StartupArtifact
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		var head sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT head_revision_id FROM hub_state WHERE singleton = 1`).Scan(&head); err != nil {
			return fmt.Errorf("recheck compiled canonical head: %w", err)
		}
		if !head.Valid || head.String != evidence.ExpectedCanonicalHeadID {
			return fmt.Errorf("%w: canonical head changed", ErrCompiledStartupEvidenceStale)
		}
		if err := requireCurrentConfigurationFileTx(ctx, tx, evidence.ExpectedCanonicalHeadID); err != nil {
			return err
		}
		storedArtifact, err := insertStartupArtifactTx(ctx, tx, preparedArtifact)
		if err != nil {
			return err
		}
		result = storedArtifact
		return nil
	})
	return result, err
}
