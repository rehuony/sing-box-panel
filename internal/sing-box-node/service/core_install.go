// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

// InstallCore receives a core the panel offers, verifies it against the
// digest, size, platform and exact version the panel declared, publishes it
// and records it as installed.
func (application *Node) InstallCore(ctx context.Context, candidate artifacts.Candidate) (result store.CoreArtifact, operationErr error) {
	defer func() {
		application.recordOperation("core.install", "Core installation", operationErr, OperationContext{CoreID: result.ID})
	}()
	installer, err := application.coreInstaller()
	if err != nil {
		return store.CoreArtifact{}, err
	}
	installed, err := installer.Install(ctx, candidate)
	if err != nil {
		return store.CoreArtifact{}, err
	}
	return application.PersistInstalledCore(ctx, installed)
}

func (application *Node) SetArtifactInstaller(installer ArtifactInstaller) {
	application.artifacts = installer
}
func (application *Node) coreInstaller() (ArtifactInstaller, error) {
	if application.artifacts != nil {
		return application.artifacts, nil
	}
	return artifacts.New(artifacts.Options{Root: filepath.Join(application.dataDir, "artifacts")})
}
func (application *Node) RemoveCoreArtifact(ctx context.Context, artifactID string) error {
	return application.database.RemoveCoreArtifact(ctx, artifactID)
}
func (application *Node) PersistInstalledCore(ctx context.Context, result artifacts.Result) (store.CoreArtifact, error) {
	featureFingerprint, err := result.FeatureFingerprint.CanonicalJSON()
	if err != nil {
		return store.CoreArtifact{}, fmt.Errorf("normalize installed core feature fingerprint: %w", err)
	}
	id, err := result.Identity.ID()
	if err != nil {
		return store.CoreArtifact{}, err
	}
	source := result.Identity.Source()
	persisted := store.CoreArtifact{
		ID: id, ExactVersion: result.Identity.ReportedVersion().String(),
		OperatingSystem: string(result.Identity.OperatingSystem()), Architecture: string(result.Identity.Architecture()),
		Variant: string(result.Identity.Variant()), ArchiveSHA256: result.Identity.Digest().String(),
		BinarySHA256: result.BinarySHA256.String(), BinaryPath: result.BinaryPath,
		ReportedVersion: result.Identity.ReportedVersion().String(), FeatureFingerprint: featureFingerprint,
		CreatedAt: application.now().UTC(),
	}
	switch source.Kind() {
	case coreartifact.SourceOfficial:
		persisted.SourceKind = store.CoreArtifactSourceOfficial
		persisted.RepositoryID = source.RepositoryID()
		persisted.ReleaseID = source.ReleaseID()
		persisted.AssetID = source.AssetID()
	case coreartifact.SourceUser:
		persisted.SourceKind = store.CoreArtifactSourceUserVerified
		persisted.UserSource = source.UserSource()
	default:
		return store.CoreArtifact{}, errors.New("installed core has an unknown source kind")
	}
	return application.database.UpsertCoreArtifact(ctx, persisted)
}

type ArtifactInstaller interface {
	Install(context.Context, artifacts.Candidate) (artifacts.Result, error)
}
