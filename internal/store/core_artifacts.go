package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrCoreArtifactNotFound         = errors.New("core artifact not found")
	ErrCoreArtifactIdentityConflict = errors.New("core artifact immutable identity conflict")
	ErrCoreArtifactInUse            = errors.New("core artifact is still referenced")
)

type CoreArtifactSourceKind string

const (
	CoreArtifactSourceOfficial     CoreArtifactSourceKind = "official"
	CoreArtifactSourceUserVerified CoreArtifactSourceKind = "user_verified"
)

// CoreArtifact is the persisted identity of immutable sing-box binary bytes.
type CoreArtifact struct {
	ID                 string
	ExactVersion       string
	OperatingSystem    string
	Architecture       string
	Variant            string
	SourceKind         CoreArtifactSourceKind
	UserSource         string
	RepositoryID       int64
	ReleaseID          int64
	AssetID            int64
	ArchiveSHA256      string
	BinarySHA256       string
	BinaryPath         string
	ReportedVersion    string
	FeatureFingerprint json.RawMessage
	CreatedAt          time.Time
}

type CoreArtifactListFilter struct {
	ExactVersion    string
	OperatingSystem string
	Architecture    string
	Variant         string
	SourceKind      CoreArtifactSourceKind
	Cursor          *CreatedAtCursor
	Limit           int
}

type CoreArtifactPage struct {
	Items []CoreArtifact
	Next  *CreatedAtCursor
}

// CoreArtifactRemovalEligibility explains every current hard reference that
// prevents safe deletion.
type CoreArtifactRemovalEligibility struct {
	Eligible                  bool
	StartupArtifactReferences int64
	ActiveBundleReferences    int64
	ActiveTaskReferences      int64
}

const coreArtifactColumns = `
    id, exact_version, operating_system, architecture, variant, source_kind,
    user_source, repository_id, release_id, asset_id, archive_sha256, binary_sha256, binary_path,
    reported_version, feature_fingerprint_json, created_at`

// UpsertCoreArtifact inserts an immutable identity or returns the existing one.
func (s *Store) UpsertCoreArtifact(
	ctx context.Context,
	artifact CoreArtifact,
) (CoreArtifact, error) {
	prepared, err := prepareCoreArtifact(artifact)
	if err != nil {
		return CoreArtifact{}, err
	}

	var stored CoreArtifact
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		existing, err := getCoreArtifact(ctx, tx, prepared.ID)
		if err == nil {
			if !sameCoreArtifactIdentity(existing, prepared) {
				return fmt.Errorf("%w: %s", ErrCoreArtifactIdentityConflict, prepared.ID)
			}
			stored = existing
			return nil
		}
		if !errors.Is(err, ErrCoreArtifactNotFound) {
			return err
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO core_artifacts(
                id, exact_version, operating_system, architecture, variant,
                source_kind, user_source, repository_id, release_id, asset_id,
                archive_sha256, binary_sha256, binary_path, reported_version,
                feature_fingerprint_json, created_at
             ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			prepared.ID,
			prepared.ExactVersion,
			prepared.OperatingSystem,
			prepared.Architecture,
			prepared.Variant,
			string(prepared.SourceKind),
			nullIfEmpty(prepared.UserSource),
			nullablePositiveID(prepared.RepositoryID),
			nullablePositiveID(prepared.ReleaseID),
			nullablePositiveID(prepared.AssetID),
			prepared.ArchiveSHA256,
			prepared.BinarySHA256,
			prepared.BinaryPath,
			prepared.ReportedVersion,
			string(prepared.FeatureFingerprint),
			formatTaskTime(prepared.CreatedAt),
		); err != nil {
			return fmt.Errorf("insert core artifact: %w", err)
		}
		stored, err = getCoreArtifact(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

// GetCoreArtifact returns one artifact by its stable ID.
func (s *Store) GetCoreArtifact(ctx context.Context, artifactID string) (CoreArtifact, error) {
	if strings.TrimSpace(artifactID) == "" {
		return CoreArtifact{}, errors.New("core artifact id is empty")
	}
	return getCoreArtifact(ctx, s.db, artifactID)
}

// ListCoreArtifacts returns a newest-first keyset page.
func (s *Store) ListCoreArtifacts(
	ctx context.Context,
	filter CoreArtifactListFilter,
) (CoreArtifactPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return CoreArtifactPage{}, err
	}
	if filter.SourceKind != "" && !validCoreArtifactSource(filter.SourceKind) {
		return CoreArtifactPage{}, fmt.Errorf("invalid core artifact source %q", filter.SourceKind)
	}
	if err := validateCreatedAtCursor(filter.Cursor); err != nil {
		return CoreArtifactPage{}, err
	}

	clauses := []string{"1 = 1"}
	args := make([]any, 0, 12)
	if filter.ExactVersion != "" {
		clauses = append(clauses, "exact_version = ?")
		args = append(args, filter.ExactVersion)
	}
	if filter.OperatingSystem != "" {
		clauses = append(clauses, "operating_system = ?")
		args = append(args, filter.OperatingSystem)
	}
	if filter.Architecture != "" {
		clauses = append(clauses, "architecture = ?")
		args = append(args, filter.Architecture)
	}
	if filter.Variant != "" {
		clauses = append(clauses, "variant = ?")
		args = append(args, filter.Variant)
	}
	if filter.SourceKind != "" {
		clauses = append(clauses, "source_kind = ?")
		args = append(args, string(filter.SourceKind))
	}
	if filter.Cursor != nil {
		cursorTime := formatTaskTime(filter.Cursor.CreatedAt)
		clauses = append(clauses, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	args = append(args, limit+1)

	query := `SELECT ` + coreArtifactColumns + `
        FROM core_artifacts
        WHERE ` + strings.Join(clauses, " AND ") + `
        ORDER BY created_at DESC, id DESC
        LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return CoreArtifactPage{}, fmt.Errorf("list core artifacts: %w", err)
	}
	defer rows.Close()

	items := make([]CoreArtifact, 0, limit+1)
	for rows.Next() {
		artifact, err := scanCoreArtifact(rows)
		if err != nil {
			return CoreArtifactPage{}, fmt.Errorf("scan listed core artifact: %w", err)
		}
		items = append(items, artifact)
	}
	if err := rows.Err(); err != nil {
		return CoreArtifactPage{}, fmt.Errorf("iterate listed core artifacts: %w", err)
	}

	page := CoreArtifactPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &CreatedAtCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	return page, nil
}

func getCoreArtifact(ctx context.Context, q queryRower, artifactID string) (CoreArtifact, error) {
	artifact, err := scanCoreArtifact(q.QueryRowContext(
		ctx,
		`SELECT `+coreArtifactColumns+` FROM core_artifacts WHERE id = ?`,
		artifactID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return CoreArtifact{}, fmt.Errorf("%w: %s", ErrCoreArtifactNotFound, artifactID)
	}
	if err != nil {
		return CoreArtifact{}, fmt.Errorf("get core artifact %q: %w", artifactID, err)
	}
	return artifact, nil
}

func scanCoreArtifact(row taskScanner) (CoreArtifact, error) {
	var (
		artifact           CoreArtifact
		repositoryID       sql.NullString
		releaseID          sql.NullString
		assetID            sql.NullString
		userSource         sql.NullString
		featureFingerprint string
		createdAt          string
	)
	if err := row.Scan(
		&artifact.ID,
		&artifact.ExactVersion,
		&artifact.OperatingSystem,
		&artifact.Architecture,
		&artifact.Variant,
		&artifact.SourceKind,
		&userSource,
		&repositoryID,
		&releaseID,
		&assetID,
		&artifact.ArchiveSHA256,
		&artifact.BinarySHA256,
		&artifact.BinaryPath,
		&artifact.ReportedVersion,
		&featureFingerprint,
		&createdAt,
	); err != nil {
		return CoreArtifact{}, err
	}

	var err error
	artifact.UserSource = valueOrEmpty(userSource)
	artifact.RepositoryID, err = parseNullablePositiveID(repositoryID)
	if err != nil {
		return CoreArtifact{}, fmt.Errorf("parse repository_id: %w", err)
	}
	artifact.ReleaseID, err = parseNullablePositiveID(releaseID)
	if err != nil {
		return CoreArtifact{}, fmt.Errorf("parse release_id: %w", err)
	}
	artifact.AssetID, err = parseNullablePositiveID(assetID)
	if err != nil {
		return CoreArtifact{}, fmt.Errorf("parse asset_id: %w", err)
	}
	artifact.FeatureFingerprint = append(json.RawMessage(nil), featureFingerprint...)
	artifact.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return CoreArtifact{}, fmt.Errorf("parse created_at: %w", err)
	}
	return artifact, nil
}
