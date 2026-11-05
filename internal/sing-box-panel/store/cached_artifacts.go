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
	// UsedAt is when an install last asked for the archive.
	UsedAt time.Time
}

const cachedArtifactColumns = `id, source_kind, repository_id, release_id, asset_id, user_source, sha256, size,
	os, arch, variant, exact_version, created_at, used_at`

// RecordCachedArtifact records artifact once; a repeated identity keeps the
// first record and marks it used again.
func (s *Store) RecordCachedArtifact(ctx context.Context, artifact CachedArtifact) (CachedArtifact, error) {
	var repository, release, asset any
	var userSource any
	if artifact.SourceKind == CachedArtifactOfficial {
		repository, release, asset = artifact.RepositoryID, artifact.ReleaseID, artifact.AssetID
	} else {
		userSource = artifact.UserSource
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO cached_artifacts(`+cachedArtifactColumns+`)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET used_at = max(used_at, excluded.used_at)`,
		artifact.ID, artifact.SourceKind, repository, release, asset, userSource, artifact.SHA256, artifact.Size,
		artifact.OS, artifact.Arch, artifact.Variant, artifact.ExactVersion, formatTime(artifact.CreatedAt), formatTime(artifact.CreatedAt)); err != nil {
		return CachedArtifact{}, err
	}
	return s.CachedArtifact(ctx, artifact.ID)
}

func (s *Store) CachedArtifact(ctx context.Context, id string) (CachedArtifact, error) {
	artifact, err := scanCachedArtifact(s.db.QueryRowContext(ctx, `SELECT `+cachedArtifactColumns+` FROM cached_artifacts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return CachedArtifact{}, ErrCachedArtifactNotFound
	}
	return artifact, err
}

// OfficialArtifactsUnusedSince lists the official archives no install asked
// for since before; they can be downloaded again when needed.
func (s *Store) OfficialArtifactsUnusedSince(ctx context.Context, before time.Time) ([]CachedArtifact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+cachedArtifactColumns+` FROM cached_artifacts
		WHERE source_kind = 'official' AND used_at < ? ORDER BY used_at, id`, formatTime(before))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var artifacts []CachedArtifact
	for rows.Next() {
		artifact, err := scanCachedArtifact(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, rows.Err()
}

// DeleteCachedArtifact forgets a cached archive and reports whether another
// record still uses the same bytes.
func (s *Store) DeleteCachedArtifact(ctx context.Context, id string) (shared bool, err error) {
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		var digest string
		if err := tx.QueryRowContext(ctx, `DELETE FROM cached_artifacts WHERE id = ? RETURNING sha256`, id).Scan(&digest); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrCachedArtifactNotFound
			}
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM cached_artifacts WHERE sha256 = ?)`, digest).Scan(&shared)
	})
	return shared, err
}

func scanCachedArtifact(row rowScanner) (CachedArtifact, error) {
	var artifact CachedArtifact
	var repository, release, asset sql.NullInt64
	var userSource sql.NullString
	var createdAt, usedAt string
	if err := row.Scan(&artifact.ID, &artifact.SourceKind, &repository, &release, &asset, &userSource, &artifact.SHA256, &artifact.Size,
		&artifact.OS, &artifact.Arch, &artifact.Variant, &artifact.ExactVersion, &createdAt, &usedAt); err != nil {
		return CachedArtifact{}, err
	}
	artifact.RepositoryID, artifact.ReleaseID, artifact.AssetID = repository.Int64, release.Int64, asset.Int64
	artifact.UserSource = userSource.String
	var err error
	if artifact.CreatedAt, err = parseTime(createdAt); err != nil {
		return CachedArtifact{}, err
	}
	artifact.UsedAt, err = parseTime(usedAt)
	return artifact, err
}
