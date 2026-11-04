// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrCachedArtifactNotFound = errors.New("cached core artifact not found")

// Cached artifact sources.
const (
	CachedArtifactOfficial     = "official"
	CachedArtifactUserVerified = "user_verified"
)

// CachedArtifact is a core archive the panel keeps and distributes to nodes.
// Its ID derives from its identity, so nodes record their installed copy
// under the same ID.
type CachedArtifact struct {
	ID           string
	SourceKind   string
	RepositoryID int64
	ReleaseID    int64
	AssetID      int64
	UserSource   string
	SHA256       string
	Size         int64
	OS           string
	Arch         string
	Variant      string
	ExactVersion string
	CreatedAt    time.Time
}

const cachedArtifactColumns = `id, source_kind, repository_id, release_id, asset_id, user_source, sha256, size,
	os, arch, variant, exact_version, created_at`

// RecordCachedArtifact records artifact once; a repeated identity keeps the
// first record.
func (s *Store) RecordCachedArtifact(ctx context.Context, artifact CachedArtifact) (CachedArtifact, error) {
	var repository, release, asset any
	var userSource any
	if artifact.SourceKind == CachedArtifactOfficial {
		repository, release, asset = artifact.RepositoryID, artifact.ReleaseID, artifact.AssetID
	} else {
		userSource = artifact.UserSource
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO cached_artifacts(`+cachedArtifactColumns+`)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`,
		artifact.ID, artifact.SourceKind, repository, release, asset, userSource, artifact.SHA256, artifact.Size,
		artifact.OS, artifact.Arch, artifact.Variant, artifact.ExactVersion, formatTime(artifact.CreatedAt)); err != nil {
		return CachedArtifact{}, err
	}
	return s.CachedArtifact(ctx, artifact.ID)
}

func (s *Store) CachedArtifact(ctx context.Context, id string) (CachedArtifact, error) {
	var artifact CachedArtifact
	var repository, release, asset sql.NullInt64
	var userSource sql.NullString
	var createdAt string
	err := s.db.QueryRowContext(ctx, `SELECT `+cachedArtifactColumns+` FROM cached_artifacts WHERE id = ?`, id).Scan(
		&artifact.ID, &artifact.SourceKind, &repository, &release, &asset, &userSource, &artifact.SHA256, &artifact.Size,
		&artifact.OS, &artifact.Arch, &artifact.Variant, &artifact.ExactVersion, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CachedArtifact{}, ErrCachedArtifactNotFound
	}
	if err != nil {
		return CachedArtifact{}, err
	}
	artifact.RepositoryID, artifact.ReleaseID, artifact.AssetID = repository.Int64, release.Int64, asset.Int64
	artifact.UserSource = userSource.String
	artifact.CreatedAt, err = parseTime(createdAt)
	return artifact, err
}
