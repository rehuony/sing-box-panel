// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var ErrManualReattachEvidenceStale = errors.New("manual reattach preview evidence is stale")

// ManualReattachEvidence is the immutable database identity covered by a
// reattach preview. The application returns the corresponding wire value to an
// operator, who must return it unchanged when applying conflict decisions.
type ManualReattachEvidence struct {
	SourceArtifactID     string
	SourceConfigSHA256   string
	BaseRevisionID       string
	BaseRevisionSHA256   string
	ExpectedHead         string
	ExpectedHeadSHA256   string
	ExactCoreVersion     string
	CoreArtifactID       string
	CapabilityRepository string
	CapabilityCommit     string
	CapabilitySHA256     string
	CapabilitySupport    capability.SupportLevel
}

// SaveReattachedManualArtifactInput creates a fresh semantic revision and a
// fresh exact-byte candidate. SourceArtifact is never updated in place.
type SaveReattachedManualArtifactInput struct {
	Evidence  ManualReattachEvidence
	Revision  NewCanonicalRevision
	Artifact  NewManualStartupArtifact
	CheckTask NewTask
}

type SaveReattachedManualArtifactResult struct {
	Revision        CanonicalRevision
	StartupArtifact StartupArtifact
	CheckTask       Task
}

// SaveReattachedManualArtifactAndTask atomically revalidates all preview
// evidence, advances the canonical head, stales the source candidate, inserts
// a new manual startup artifact, and queues its exact-binary checker. It does
// no filesystem, network, or child-process work while the transaction is open.
func (s *Store) SaveReattachedManualArtifactAndTask(
	ctx context.Context,
	input SaveReattachedManualArtifactInput,
) (SaveReattachedManualArtifactResult, error) {
	if err := validateManualReattachEvidence(input.Evidence); err != nil {
		return SaveReattachedManualArtifactResult{}, err
	}
	preparedRevision, preparedTask, err := prepareCanonicalSave(input.Revision, input.CheckTask)
	if err != nil {
		return SaveReattachedManualArtifactResult{}, err
	}
	if preparedTask.Lane != TaskLaneMaintenance {
		return SaveReattachedManualArtifactResult{}, errors.New(
			"manual reattach check task must use the maintenance lane",
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
		return SaveReattachedManualArtifactResult{}, err
	}
	if preparedArtifact.ExactCoreVersion != input.Evidence.ExactCoreVersion ||
		preparedArtifact.CoreArtifactID != input.Evidence.CoreArtifactID ||
		preparedArtifact.ConfigSHA256 != input.Evidence.SourceConfigSHA256 {
		return SaveReattachedManualArtifactResult{}, errors.New(
			"reattached artifact does not preserve the previewed exact bytes and binary binding",
		)
	}

	var result SaveReattachedManualArtifactResult
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if err := validateManualReattachEvidenceTx(ctx, tx, input.Evidence); err != nil {
			return err
		}

		storedRevision, _, err := saveCanonicalRevisionTx(
			ctx,
			tx,
			input.Evidence.ExpectedHead,
			preparedRevision,
			false,
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
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE startup_artifacts SET state = 'stale' WHERE id = ? AND state <> 'stale'`,
			input.Evidence.SourceArtifactID,
		); err != nil {
			return fmt.Errorf("mark reattached source artifact stale: %w", err)
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
		result = SaveReattachedManualArtifactResult{
			Revision: storedRevision, StartupArtifact: storedArtifact, CheckTask: storedTask,
		}
		return nil
	})
	if err != nil {
		return SaveReattachedManualArtifactResult{}, err
	}
	return result, nil
}

func validateManualReattachEvidence(evidence ManualReattachEvidence) error {
	for name, value := range map[string]string{
		"source artifact id": evidence.SourceArtifactID,
		"base revision id":   evidence.BaseRevisionID,
		"expected head":      evidence.ExpectedHead,
		"core artifact id":   evidence.CoreArtifactID,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("manual reattach %s is empty", name)
		}
	}
	for name, value := range map[string]string{
		"source config digest": evidence.SourceConfigSHA256,
		"base revision digest": evidence.BaseRevisionSHA256,
		"expected head digest": evidence.ExpectedHeadSHA256,
		"capability digest":    evidence.CapabilitySHA256,
	} {
		digest, err := coreartifact.ParseSHA256(value)
		if err != nil || digest.IsZero() {
			return fmt.Errorf("manual reattach %s is invalid", name)
		}
	}
	version, err := coreartifact.ParseExactVersion(evidence.ExactCoreVersion)
	if err != nil || version.IsZero() || version.String() != evidence.ExactCoreVersion {
		return errors.New("manual reattach exact core version is invalid")
	}
	digest, _ := coreartifact.ParseSHA256(evidence.CapabilitySHA256)
	if _, err := capability.NewReference(
		evidence.CapabilityRepository,
		evidence.CapabilityCommit,
		digest,
	); err != nil {
		return fmt.Errorf("manual reattach capability reference: %w", err)
	}
	if evidence.CapabilityRepository != capability.ManifestRepository {
		return fmt.Errorf(
			"manual reattach capability repository must be %q",
			capability.ManifestRepository,
		)
	}
	if !structuredCapabilitySupport(evidence.CapabilitySupport) {
		return errors.New("manual reattach requires a structured capability manifest")
	}
	return nil
}

func validateManualReattachEvidenceTx(
	ctx context.Context,
	tx *sql.Tx,
	evidence ManualReattachEvidence,
) error {
	source, err := getStartupArtifact(ctx, tx, evidence.SourceArtifactID)
	if err != nil {
		return err
	}
	if source.Kind != StartupArtifactManual ||
		source.ConfigSHA256 != evidence.SourceConfigSHA256 ||
		source.CanonicalRevisionID != evidence.BaseRevisionID ||
		source.ExactCoreVersion != evidence.ExactCoreVersion ||
		source.CoreArtifactID != evidence.CoreArtifactID {
		return fmt.Errorf("%w: source manual artifact changed", ErrManualReattachEvidenceStale)
	}
	base, err := getCanonicalRevision(tx.QueryRowContext(
		ctx,
		`SELECT `+canonicalRevisionColumns+` FROM canonical_revisions WHERE id = ?`,
		evidence.BaseRevisionID,
	))
	if err != nil {
		return err
	}
	if base.SHA256 != evidence.BaseRevisionSHA256 {
		return fmt.Errorf("%w: base revision changed", ErrManualReattachEvidenceStale)
	}
	head, err := getCanonicalRevision(tx.QueryRowContext(
		ctx,
		`SELECT `+canonicalRevisionColumns+` FROM canonical_revisions WHERE id = ?`,
		evidence.ExpectedHead,
	))
	if err != nil {
		return err
	}
	if head.SHA256 != evidence.ExpectedHeadSHA256 {
		return fmt.Errorf("%w: current head changed", ErrManualReattachEvidenceStale)
	}

	pin, err := getCapabilityPin(ctx, tx, evidence.ExactCoreVersion)
	if err != nil {
		return fmt.Errorf("%w: capability pin is no longer available: %v", ErrManualReattachEvidenceStale, err)
	}
	if pin.Repository != evidence.CapabilityRepository ||
		pin.CommitSHA != evidence.CapabilityCommit ||
		pin.ManifestSHA256 != evidence.CapabilitySHA256 ||
		pin.SupportLevel != evidence.CapabilitySupport ||
		!structuredCapabilitySupport(pin.SupportLevel) {
		return fmt.Errorf("%w: capability pin changed", ErrManualReattachEvidenceStale)
	}
	manifest, err := getCapabilityGenerationManifest(
		ctx,
		tx,
		evidence.CapabilityRepository,
		evidence.CapabilityCommit,
		evidence.ExactCoreVersion,
		evidence.CapabilitySHA256,
	)
	if err != nil {
		return fmt.Errorf("%w: pinned manifest is no longer available: %v", ErrManualReattachEvidenceStale, err)
	}
	if manifest.SupportLevel != evidence.CapabilitySupport {
		return fmt.Errorf("%w: pinned manifest support changed", ErrManualReattachEvidenceStale)
	}
	if quarantine, err := getCapabilityQuarantine(ctx, tx, evidence.CapabilitySHA256); err == nil {
		return fmt.Errorf(
			"%w: %s (%s)",
			ErrCapabilityManifestQuarantined,
			evidence.CapabilitySHA256,
			quarantine.ReasonCode,
		)
	} else if !errors.Is(err, ErrCapabilityQuarantineNotFound) {
		return err
	}
	return nil
}

func structuredCapabilitySupport(level capability.SupportLevel) bool {
	return level == capability.SupportNativeStructured ||
		level == capability.SupportCompatibleStructured
}
