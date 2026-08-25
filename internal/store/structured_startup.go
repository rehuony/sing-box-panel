// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/capability"
)

var ErrStructuredStartupEvidenceStale = errors.New("structured startup evidence is stale")

// StartupArtifactTask is the atomic result of rendering immutable startup
// bytes and durably queueing the one checker allowed to make them ready.
type StartupArtifactTask struct {
	Artifact StartupArtifact
	Task     Task
}

// StructuredStartupEvidence binds projected bytes to the exact control-plane
// facts that were read before projection. The insert transaction rechecks all
// of them before it can create either an artifact or its checker task.
type StructuredStartupEvidence struct {
	ExpectedCanonicalHeadID string
	CapabilityRepository    string
	CapabilityCommit        string
	CapabilityDigest        string
	CapabilitySupport       capability.SupportLevel
}

// CreateStartupArtifactAndCheckTask prevents a rendered candidate from being
// left pending without durable work. Projection happens before this short
// transaction; the transaction only validates references and inserts rows.
func (s *Store) CreateStartupArtifactAndCheckTask(
	ctx context.Context,
	artifact StartupArtifact,
	task NewTask,
	evidence StructuredStartupEvidence,
) (StartupArtifactTask, error) {
	preparedArtifact, err := prepareNewStartupArtifact(artifact)
	if err != nil {
		return StartupArtifactTask{}, err
	}
	if preparedArtifact.Kind != StartupArtifactStructured {
		return StartupArtifactTask{}, errors.New("atomic render requires a structured startup artifact")
	}
	preparedTask, err := prepareEnqueuedTask(EnqueueTaskInput{
		ID:                  task.ID,
		IdempotencyKey:      task.IdempotencyKey,
		Lane:                task.Lane,
		Kind:                task.Kind,
		Generation:          task.Generation,
		CanonicalRevisionID: preparedArtifact.CanonicalRevisionID,
		StartupArtifactID:   preparedArtifact.ID,
		Payload:             task.Payload,
		CreatedAt:           task.CreatedAt,
	})
	if err != nil {
		return StartupArtifactTask{}, err
	}
	if preparedTask.Lane != TaskLaneMaintenance || preparedTask.Kind != "startup-check" {
		return StartupArtifactTask{}, errors.New("structured startup artifact requires a maintenance startup-check task")
	}
	if err := validateStructuredStartupEvidence(preparedArtifact, evidence); err != nil {
		return StartupArtifactTask{}, err
	}

	var result StartupArtifactTask
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := recheckStructuredStartupEvidence(ctx, tx, preparedArtifact, evidence); err != nil {
			return err
		}
		storedArtifact, err := insertStartupArtifactTx(ctx, tx, preparedArtifact)
		if err != nil {
			return err
		}
		if err := insertCanonicalTaskTx(ctx, tx, NewTask{
			ID:             preparedTask.ID,
			IdempotencyKey: preparedTask.IdempotencyKey,
			Lane:           preparedTask.Lane,
			Kind:           preparedTask.Kind,
			Generation:     preparedTask.Generation,
			Payload:        preparedTask.Payload,
			CreatedAt:      preparedTask.CreatedAt,
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
	if err != nil {
		return StartupArtifactTask{}, err
	}
	return result, nil
}

func validateStructuredStartupEvidence(
	artifact StartupArtifact,
	evidence StructuredStartupEvidence,
) error {
	if evidence.ExpectedCanonicalHeadID == "" ||
		evidence.ExpectedCanonicalHeadID != artifact.CanonicalRevisionID {
		return errors.New("structured startup expected canonical head is missing or inconsistent")
	}
	if evidence.CapabilityRepository != capability.ManifestRepository ||
		evidence.CapabilityCommit != artifact.CapabilityCommit ||
		evidence.CapabilityDigest != artifact.CapabilityDigest {
		return errors.New("structured startup capability evidence is missing or inconsistent")
	}
	if evidence.CapabilitySupport != capability.SupportNativeStructured &&
		evidence.CapabilitySupport != capability.SupportCompatibleStructured {
		return errors.New("structured startup requires structured capability support evidence")
	}
	return nil
}

func recheckStructuredStartupEvidence(
	ctx context.Context,
	tx *sql.Tx,
	artifact StartupArtifact,
	evidence StructuredStartupEvidence,
) error {
	var head sql.NullString
	if err := tx.QueryRowContext(
		ctx,
		`SELECT head_revision_id FROM hub_state WHERE singleton = 1`,
	).Scan(&head); err != nil {
		return fmt.Errorf("recheck structured canonical head: %w", err)
	}
	if !head.Valid || head.String != evidence.ExpectedCanonicalHeadID {
		return fmt.Errorf("%w: canonical head changed", ErrStructuredStartupEvidenceStale)
	}

	pin, err := getCapabilityPin(ctx, tx, artifact.ExactCoreVersion)
	if err != nil {
		return fmt.Errorf("%w: capability pin is unavailable: %v", ErrStructuredStartupEvidenceStale, err)
	}
	if pin.Repository != evidence.CapabilityRepository ||
		pin.CommitSHA != evidence.CapabilityCommit ||
		pin.ManifestSHA256 != evidence.CapabilityDigest ||
		pin.SupportLevel != evidence.CapabilitySupport {
		return fmt.Errorf("%w: capability pin changed", ErrStructuredStartupEvidenceStale)
	}
	manifest, err := getCapabilityGenerationManifest(
		ctx,
		tx,
		evidence.CapabilityRepository,
		evidence.CapabilityCommit,
		artifact.ExactCoreVersion,
		evidence.CapabilityDigest,
	)
	if err != nil || manifest.SupportLevel != evidence.CapabilitySupport {
		return fmt.Errorf("%w: capability generation manifest changed or is unavailable", ErrStructuredStartupEvidenceStale)
	}
	if quarantine, quarantineErr := getCapabilityQuarantine(ctx, tx, evidence.CapabilityDigest); quarantineErr == nil {
		return fmt.Errorf(
			"%w: capability manifest is quarantined (%s)",
			ErrStructuredStartupEvidenceStale,
			quarantine.ReasonCode,
		)
	} else if !errors.Is(quarantineErr, ErrCapabilityQuarantineNotFound) {
		return quarantineErr
	}
	return nil
}
