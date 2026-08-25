// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var ErrManualProjectionEvidenceStale = errors.New("manual reverse projection capability evidence is stale")

// NewManualStartupArtifact is caller-owned manual configuration. The store
// derives its kind, canonical revision binding, digest, state, and check time.
// ConfigBytes are persisted verbatim; they do not need to be strict JSON.
type NewManualStartupArtifact struct {
	ID               string
	ExactCoreVersion string
	RendererVersion  string
	CoreArtifactID   string
	ConfigBytes      []byte
	ConfigSHA256     string
	Diagnostics      json.RawMessage
	CreatedAt        time.Time
}

// SaveCanonicalManualArtifactInput describes one manual JSON save boundary.
// Revision is the proposed canonical projection. CheckTask must be maintenance
// work; its durable references are derived from the committed revision and
// startup artifact rather than trusted from caller-provided JSON.
type SaveCanonicalManualArtifactInput struct {
	ExpectedHead       string
	ProjectionEvidence *ManualProjectionEvidence
	Revision           NewCanonicalRevision
	Artifact           NewManualStartupArtifact
	CheckTask          NewTask
}

// ManualProjectionEvidence identifies the exact immutable capability used to
// reverse-project manual bytes into a proposed canonical revision. A nil value
// carries no reverse-projection capability claim.
type ManualProjectionEvidence struct {
	ExactCoreVersion string
	Repository       string
	CommitSHA        string
	ManifestSHA256   string
	SupportLevel     capability.SupportLevel
}

// SaveCanonicalManualArtifactResult is committed only when all three records
// are durable. NoChange means the existing canonical head was reused while a
// new immutable manual candidate and its checker task were still created.
type SaveCanonicalManualArtifactResult struct {
	Revision        CanonicalRevision
	NoChange        bool
	StartupArtifact StartupArtifact
	CheckTask       Task
}

// SaveCanonicalManualArtifactAndTask atomically compares the canonical head,
// creates or reuses the canonical revision, invalidates superseded manual
// candidates, inserts exact manual bytes as a pending startup artifact, and
// queues its maintenance checker. Any failure rolls back the whole save.
func (s *Store) SaveCanonicalManualArtifactAndTask(
	ctx context.Context,
	input SaveCanonicalManualArtifactInput,
) (SaveCanonicalManualArtifactResult, error) {
	var projectionEvidence *ManualProjectionEvidence
	if input.ProjectionEvidence != nil {
		copied := *input.ProjectionEvidence
		if err := validateManualProjectionEvidence(copied); err != nil {
			return SaveCanonicalManualArtifactResult{}, err
		}
		projectionEvidence = &copied
	}
	preparedRevision, preparedTask, err := prepareCanonicalSave(input.Revision, input.CheckTask)
	if err != nil {
		return SaveCanonicalManualArtifactResult{}, err
	}
	if preparedTask.Lane != TaskLaneMaintenance {
		return SaveCanonicalManualArtifactResult{}, errors.New(
			"manual startup artifact check task must use the maintenance lane",
		)
	}

	artifactCreatedAt := input.Artifact.CreatedAt
	if artifactCreatedAt.IsZero() {
		artifactCreatedAt = preparedRevision.CreatedAt
	}
	preparedArtifact, err := prepareNewStartupArtifact(StartupArtifact{
		ID:                  input.Artifact.ID,
		Kind:                StartupArtifactManual,
		CanonicalRevisionID: preparedRevision.ID,
		ExactCoreVersion:    input.Artifact.ExactCoreVersion,
		RendererVersion:     input.Artifact.RendererVersion,
		CoreArtifactID:      input.Artifact.CoreArtifactID,
		ConfigBytes:         input.Artifact.ConfigBytes,
		ConfigSHA256:        input.Artifact.ConfigSHA256,
		Diagnostics:         input.Artifact.Diagnostics,
		CreatedAt:           artifactCreatedAt,
	})
	if err != nil {
		return SaveCanonicalManualArtifactResult{}, err
	}
	if projectionEvidence != nil && projectionEvidence.ExactCoreVersion != preparedArtifact.ExactCoreVersion {
		return SaveCanonicalManualArtifactResult{}, errors.New(
			"manual reverse projection evidence and startup artifact exact versions disagree",
		)
	}

	var result SaveCanonicalManualArtifactResult
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if projectionEvidence != nil {
			if err := validateManualProjectionEvidenceTx(ctx, tx, *projectionEvidence); err != nil {
				return err
			}
		}
		storedRevision, created, err := saveCanonicalRevisionTx(
			ctx,
			tx,
			input.ExpectedHead,
			preparedRevision,
			true,
		)
		if err != nil {
			return err
		}
		preparedArtifact.CanonicalRevisionID = storedRevision.ID

		if err := markSupersededManualStartupArtifactsStale(
			ctx,
			tx,
			storedRevision.ID,
			preparedArtifact.ExactCoreVersion,
			preparedArtifact.CoreArtifactID,
		); err != nil {
			return err
		}
		storedArtifact, err := insertStartupArtifactTx(ctx, tx, preparedArtifact)
		if err != nil {
			return err
		}
		if err := insertCanonicalTaskTx(
			ctx,
			tx,
			preparedTask,
			storedRevision.ID,
			storedArtifact.ID,
		); err != nil {
			return err
		}
		storedTask, err := getTask(ctx, tx, preparedTask.ID)
		if err != nil {
			return err
		}

		result = SaveCanonicalManualArtifactResult{
			Revision:        storedRevision,
			NoChange:        !created,
			StartupArtifact: storedArtifact,
			CheckTask:       storedTask,
		}
		return nil
	})
	if err != nil {
		return SaveCanonicalManualArtifactResult{}, err
	}
	return result, nil
}

func validateManualProjectionEvidence(evidence ManualProjectionEvidence) error {
	version, err := coreartifact.ParseExactVersion(evidence.ExactCoreVersion)
	if err != nil || version.IsZero() || version.String() != evidence.ExactCoreVersion {
		return errors.New("manual reverse projection exact core version is invalid")
	}
	digest, err := coreartifact.ParseSHA256(evidence.ManifestSHA256)
	if err != nil || digest.IsZero() {
		return errors.New("manual reverse projection manifest digest is invalid")
	}
	if _, err := capability.NewReference(evidence.Repository, evidence.CommitSHA, digest); err != nil {
		return fmt.Errorf("manual reverse projection capability reference: %w", err)
	}
	if evidence.Repository != capability.ManifestRepository {
		return fmt.Errorf(
			"manual reverse projection capability repository must be %q",
			capability.ManifestRepository,
		)
	}
	if !structuredCapabilitySupport(evidence.SupportLevel) {
		return errors.New("manual reverse projection requires a structured capability manifest")
	}
	return nil
}

func validateManualProjectionEvidenceTx(
	ctx context.Context,
	tx *sql.Tx,
	evidence ManualProjectionEvidence,
) error {
	pin, err := getCapabilityPin(ctx, tx, evidence.ExactCoreVersion)
	if err != nil {
		return fmt.Errorf("%w: capability pin is no longer available: %v", ErrManualProjectionEvidenceStale, err)
	}
	if pin.ExactCoreVersion != evidence.ExactCoreVersion ||
		pin.Repository != evidence.Repository ||
		pin.CommitSHA != evidence.CommitSHA ||
		pin.ManifestSHA256 != evidence.ManifestSHA256 ||
		pin.SupportLevel != evidence.SupportLevel ||
		!structuredCapabilitySupport(pin.SupportLevel) {
		return fmt.Errorf("%w: capability pin changed", ErrManualProjectionEvidenceStale)
	}
	manifest, err := getCapabilityGenerationManifest(
		ctx,
		tx,
		evidence.Repository,
		evidence.CommitSHA,
		evidence.ExactCoreVersion,
		evidence.ManifestSHA256,
	)
	if err != nil {
		return fmt.Errorf("%w: pinned manifest is no longer available: %v", ErrManualProjectionEvidenceStale, err)
	}
	if manifest.SupportLevel != evidence.SupportLevel {
		return fmt.Errorf("%w: pinned manifest support changed", ErrManualProjectionEvidenceStale)
	}
	if quarantine, err := getCapabilityQuarantine(ctx, tx, evidence.ManifestSHA256); err == nil {
		return fmt.Errorf(
			"%w: %s (%s)",
			ErrCapabilityManifestQuarantined,
			evidence.ManifestSHA256,
			quarantine.ReasonCode,
		)
	} else if !errors.Is(err, ErrCapabilityQuarantineNotFound) {
		return err
	}
	return nil
}

// markSupersededManualStartupArtifactsStale invalidates every manual
// candidate based on an older semantic revision plus earlier candidates for
// the exact target binary. Candidates for another binary on the current
// revision remain usable for independent multi-version operation.
func markSupersededManualStartupArtifactsStale(
	ctx context.Context,
	tx *sql.Tx,
	canonicalRevisionID string,
	exactCoreVersion string,
	coreArtifactID string,
) error {
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE startup_artifacts
                SET state = 'stale'
              WHERE kind = 'manual'
                AND state IN ('pending', 'ready')
                AND (
                    canonical_revision_id <> ?
                    OR (
                        canonical_revision_id = ?
                        AND exact_core_version = ?
                        AND core_artifact_id = ?
                    )
                )`,
		canonicalRevisionID,
		canonicalRevisionID,
		exactCoreVersion,
		coreArtifactID,
	); err != nil {
		return fmt.Errorf("mark superseded manual startup artifacts stale: %w", err)
	}
	return nil
}
