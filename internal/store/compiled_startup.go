// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrCompiledStartupEvidenceStale = errors.New("compiled startup evidence is stale")

type StartupArtifactTask struct {
	Artifact StartupArtifact
	Task     Task
}

// CompiledStartupEvidence binds projected bytes to the immutable global head
// and compile-time adapter identity observed before the short insert
// transaction. The Store cannot load adapters; it only prevents stale writes.
type CompiledStartupEvidence struct {
	ExpectedCanonicalHeadID string
	AdapterID               string
	AdapterRevision         string
}

func (s *Store) CreateStartupArtifactAndCheckTask(
	ctx context.Context,
	artifact StartupArtifact,
	task NewTask,
	evidence CompiledStartupEvidence,
) (StartupArtifactTask, error) {
	preparedArtifact, err := prepareNewStartupArtifact(artifact)
	if err != nil {
		return StartupArtifactTask{}, err
	}
	preparedTask, err := prepareEnqueuedTask(EnqueueTaskInput{
		ID: task.ID, IdempotencyKey: task.IdempotencyKey, Lane: task.Lane, Kind: task.Kind,
		Generation: task.Generation, CanonicalRevisionID: preparedArtifact.CanonicalRevisionID,
		StartupArtifactID: preparedArtifact.ID, Payload: task.Payload, CreatedAt: task.CreatedAt,
	})
	if err != nil {
		return StartupArtifactTask{}, err
	}
	if preparedTask.Lane != TaskLaneMaintenance || preparedTask.Kind != TaskKindStartupCheck {
		return StartupArtifactTask{}, errors.New("compiled startup artifact requires a maintenance startup-check task")
	}
	if evidence.ExpectedCanonicalHeadID == "" || evidence.ExpectedCanonicalHeadID != preparedArtifact.CanonicalRevisionID ||
		evidence.AdapterID != preparedArtifact.AdapterID || evidence.AdapterRevision != preparedArtifact.AdapterRevision {
		return StartupArtifactTask{}, errors.New("compiled startup evidence is missing or inconsistent")
	}

	var result StartupArtifactTask
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		var head sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT head_revision_id FROM hub_state WHERE singleton = 1`).Scan(&head); err != nil {
			return fmt.Errorf("recheck compiled canonical head: %w", err)
		}
		if !head.Valid || head.String != evidence.ExpectedCanonicalHeadID {
			return fmt.Errorf("%w: canonical head changed", ErrCompiledStartupEvidenceStale)
		}
		storedArtifact, err := insertStartupArtifactTx(ctx, tx, preparedArtifact)
		if err != nil {
			return err
		}
		if err := insertCanonicalTaskTx(ctx, tx, NewTask{
			ID: preparedTask.ID, IdempotencyKey: preparedTask.IdempotencyKey,
			Lane: preparedTask.Lane, Kind: preparedTask.Kind, Generation: preparedTask.Generation,
			Payload: preparedTask.Payload, CreatedAt: preparedTask.CreatedAt,
		}, storedArtifact.CanonicalRevisionID, storedArtifact.ID); err != nil {
			return err
		}
		storedTask, err := getTask(ctx, tx, preparedTask.ID)
		if err != nil {
			return err
		}
		result = StartupArtifactTask{Artifact: storedArtifact, Task: storedTask}
		return nil
	})
	return result, err
}
