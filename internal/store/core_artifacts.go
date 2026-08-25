package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
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

type CoreArtifactVerificationState string

const (
	CoreArtifactVerified    CoreArtifactVerificationState = "verified"
	CoreArtifactRevoked     CoreArtifactVerificationState = "revoked"
	CoreArtifactQuarantined CoreArtifactVerificationState = "quarantined"
)

// CoreArtifact is the persisted identity and verification state of immutable
// sing-box binary bytes.
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
	VerificationState  CoreArtifactVerificationState
	CreatedAt          time.Time
}

type CoreArtifactListFilter struct {
	ExactVersion      string
	OperatingSystem   string
	Architecture      string
	Variant           string
	SourceKind        CoreArtifactSourceKind
	VerificationState CoreArtifactVerificationState
	Cursor            *CreatedAtCursor
	Limit             int
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
    reported_version, feature_fingerprint_json, verification_state, created_at`

// UpsertCoreArtifact inserts a new immutable identity or only tightens the
// verification state of an identical identity. Installation/import retries
// must never turn quarantined or revoked bytes back into verified bytes.
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
			verificationState := stricterCoreArtifactVerification(
				existing.VerificationState,
				prepared.VerificationState,
			)
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE core_artifacts SET verification_state = ? WHERE id = ?`,
				string(verificationState),
				prepared.ID,
			); err != nil {
				return fmt.Errorf("update core artifact verification: %w", err)
			}
			stored, err = getCoreArtifact(ctx, tx, prepared.ID)
			return err
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
                feature_fingerprint_json, verification_state, created_at
             ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
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
			string(prepared.VerificationState),
			formatTaskTime(prepared.CreatedAt),
		); err != nil {
			return fmt.Errorf("insert core artifact: %w", err)
		}
		stored, err = getCoreArtifact(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func stricterCoreArtifactVerification(
	current CoreArtifactVerificationState,
	requested CoreArtifactVerificationState,
) CoreArtifactVerificationState {
	if current == CoreArtifactRevoked || requested == CoreArtifactRevoked {
		return CoreArtifactRevoked
	}
	if current == CoreArtifactQuarantined || requested == CoreArtifactQuarantined {
		return CoreArtifactQuarantined
	}
	return CoreArtifactVerified
}

// GetCoreArtifact returns one artifact by its stable ID.
func (s *Store) GetCoreArtifact(ctx context.Context, artifactID string) (CoreArtifact, error) {
	if strings.TrimSpace(artifactID) == "" {
		return CoreArtifact{}, errors.New("core artifact id is empty")
	}
	return getCoreArtifact(ctx, s.db, artifactID)
}

// RestrictCoreArtifactVerification explicitly lowers trust in immutable bytes.
// Revocation is terminal and quarantine cannot be lifted through install or
// import retries; restoring trust requires a separately reviewed workflow.
func (s *Store) RestrictCoreArtifactVerification(
	ctx context.Context,
	artifactID string,
	verificationState CoreArtifactVerificationState,
	restrictedAt time.Time,
) (CoreArtifact, error) {
	if strings.TrimSpace(artifactID) == "" {
		return CoreArtifact{}, errors.New("core artifact id is empty")
	}
	if verificationState != CoreArtifactQuarantined && verificationState != CoreArtifactRevoked {
		return CoreArtifact{}, errors.New("core artifact verification restriction must be quarantined or revoked")
	}
	if restrictedAt.IsZero() {
		return CoreArtifact{}, errors.New("core artifact verification restriction time is zero")
	}
	restrictedAt = restrictedAt.UTC()

	var stored CoreArtifact
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		existing, err := getCoreArtifact(ctx, tx, artifactID)
		if err != nil {
			return err
		}
		next := stricterCoreArtifactVerification(existing.VerificationState, verificationState)
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE core_artifacts SET verification_state = ? WHERE id = ?`,
			string(next),
			artifactID,
		); err != nil {
			return fmt.Errorf("restrict core artifact verification: %w", err)
		}
		updatedAt := formatTaskTime(restrictedAt)
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE tasks
			    SET status = 'canceled', cancel_requested = 1, updated_at = ?
			  WHERE lane = 'maintenance' AND kind = 'startup-check' AND status = 'queued'
			    AND startup_artifact_id IN (
			        SELECT id FROM startup_artifacts WHERE core_artifact_id = ?
			    )`,
			updatedAt,
			artifactID,
		); err != nil {
			return fmt.Errorf("cancel queued checks for restricted core artifact: %w", err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE tasks
			    SET cancel_requested = 1, updated_at = ?
			  WHERE lane = 'maintenance' AND kind = 'startup-check' AND status = 'running'
			    AND startup_artifact_id IN (
			        SELECT id FROM startup_artifacts WHERE core_artifact_id = ?
			    )`,
			updatedAt,
			artifactID,
		); err != nil {
			return fmt.Errorf("cancel running checks for restricted core artifact: %w", err)
		}

		var desiredUsesArtifact int
		if err := tx.QueryRowContext(
			ctx,
			`SELECT EXISTS(
			    SELECT 1
			      FROM hub_state AS hub
			      JOIN activation_bundles AS bundle ON bundle.id = hub.desired_bundle_id
			      JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
			     WHERE hub.singleton = 1 AND hub.desired_running = 1
			       AND startup.core_artifact_id = ?
			)`,
			artifactID,
		).Scan(&desiredUsesArtifact); err != nil {
			return fmt.Errorf("inspect desired restricted core artifact: %w", err)
		}
		if desiredUsesArtifact != 0 {
			var generation int64
			if err := tx.QueryRowContext(
				ctx,
				`SELECT target_generation FROM hub_state WHERE singleton = 1`,
			).Scan(&generation); err != nil {
				return fmt.Errorf("read runtime generation for core restriction: %w", err)
			}
			generation++
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE tasks SET status = 'superseded', updated_at = ?
				  WHERE lane = 'runtime' AND status = 'queued' AND generation < ?`,
				updatedAt,
				generation,
			); err != nil {
				return fmt.Errorf("supersede queued runtime work for core restriction: %w", err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE tasks SET cancel_requested = 1, updated_at = ?
				  WHERE lane = 'runtime' AND status = 'running' AND generation < ?`,
				updatedAt,
				generation,
			); err != nil {
				return fmt.Errorf("cancel running runtime work for core restriction: %w", err)
			}
			if _, err := tx.ExecContext(
				ctx,
				`UPDATE hub_state
				    SET desired_running = 0, target_generation = ?, updated_at = ?
				  WHERE singleton = 1`,
				generation,
				updatedAt,
			); err != nil {
				return fmt.Errorf("fence desired runtime for core restriction: %w", err)
			}
		}
		stored, err = getCoreArtifact(ctx, tx, artifactID)
		return err
	})
	return stored, err
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
	if filter.VerificationState != "" && !validCoreArtifactVerification(filter.VerificationState) {
		return CoreArtifactPage{}, fmt.Errorf("invalid core artifact verification %q", filter.VerificationState)
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
	if filter.VerificationState != "" {
		clauses = append(clauses, "verification_state = ?")
		args = append(args, string(filter.VerificationState))
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

// CoreArtifactRemovalEligibility reports whether deletion is currently safe.
func (s *Store) CoreArtifactRemovalEligibility(
	ctx context.Context,
	artifactID string,
) (CoreArtifactRemovalEligibility, error) {
	if strings.TrimSpace(artifactID) == "" {
		return CoreArtifactRemovalEligibility{}, errors.New("core artifact id is empty")
	}
	return coreArtifactRemovalEligibility(ctx, s.db, artifactID)
}

// RemoveCoreArtifact performs the eligibility check and deletion in one short
// transaction, closing the check/delete race.
func (s *Store) RemoveCoreArtifact(ctx context.Context, artifactID string) error {
	if strings.TrimSpace(artifactID) == "" {
		return errors.New("core artifact id is empty")
	}
	return s.WithTx(ctx, func(tx *sql.Tx) error {
		eligibility, err := coreArtifactRemovalEligibility(ctx, tx, artifactID)
		if err != nil {
			return err
		}
		if !eligibility.Eligible {
			return fmt.Errorf("%w: %s", ErrCoreArtifactInUse, artifactID)
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM core_artifacts WHERE id = ?`, artifactID)
		if err != nil {
			return fmt.Errorf("remove core artifact: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect core artifact removal: %w", err)
		}
		if rows != 1 {
			return ErrCoreArtifactNotFound
		}
		return nil
	})
}

func coreArtifactRemovalEligibility(
	ctx context.Context,
	q queryRower,
	artifactID string,
) (CoreArtifactRemovalEligibility, error) {
	if _, err := getCoreArtifact(ctx, q, artifactID); err != nil {
		return CoreArtifactRemovalEligibility{}, err
	}

	var result CoreArtifactRemovalEligibility
	err := q.QueryRowContext(
		ctx,
		`SELECT
            (SELECT count(*) FROM startup_artifacts WHERE core_artifact_id = ?),
            (
                SELECT count(*)
                  FROM activation_bundles AS bundle
                  JOIN startup_artifacts AS startup ON startup.id = bundle.startup_artifact_id
                 WHERE startup.core_artifact_id = ?
                   AND bundle.id IN (
                        SELECT desired_bundle_id FROM hub_state
                        UNION SELECT applied_bundle_id FROM hub_state
                        UNION SELECT rollback_bundle_id FROM hub_state
                   )
            ),
            (
                SELECT count(*)
                  FROM tasks AS task
                  JOIN startup_artifacts AS startup ON startup.id = task.startup_artifact_id
                 WHERE startup.core_artifact_id = ?
                   AND task.status IN ('queued', 'running')
            )`,
		artifactID,
		artifactID,
		artifactID,
	).Scan(
		&result.StartupArtifactReferences,
		&result.ActiveBundleReferences,
		&result.ActiveTaskReferences,
	)
	if err != nil {
		return CoreArtifactRemovalEligibility{}, fmt.Errorf("inspect core artifact references: %w", err)
	}
	result.Eligible = result.StartupArtifactReferences == 0 &&
		result.ActiveBundleReferences == 0 && result.ActiveTaskReferences == 0
	return result, nil
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
		&artifact.VerificationState,
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

func prepareCoreArtifact(artifact CoreArtifact) (CoreArtifact, error) {
	if strings.TrimSpace(artifact.ID) == "" {
		return CoreArtifact{}, errors.New("core artifact id is empty")
	}
	exactVersion, err := coreartifact.ParseExactVersion(artifact.ExactVersion)
	if err != nil || exactVersion.IsZero() {
		return CoreArtifact{}, fmt.Errorf("invalid core artifact exact version %q", artifact.ExactVersion)
	}
	reportedVersion, err := coreartifact.ParseExactVersion(artifact.ReportedVersion)
	if err != nil || reportedVersion.IsZero() || reportedVersion.Compare(exactVersion) != 0 {
		return CoreArtifact{}, errors.New("reported core version must equal exact version")
	}
	artifact.ExactVersion = exactVersion.String()
	artifact.ReportedVersion = reportedVersion.String()
	if artifact.OperatingSystem != "linux" {
		return CoreArtifact{}, fmt.Errorf("unsupported core artifact operating system %q", artifact.OperatingSystem)
	}
	if artifact.Architecture != "amd64" && artifact.Architecture != "arm64" {
		return CoreArtifact{}, fmt.Errorf("unsupported core artifact architecture %q", artifact.Architecture)
	}
	if !validArtifactVariant(artifact.Variant) {
		return CoreArtifact{}, fmt.Errorf("invalid core artifact variant %q", artifact.Variant)
	}
	if !validCoreArtifactSource(artifact.SourceKind) {
		return CoreArtifact{}, fmt.Errorf("invalid core artifact source %q", artifact.SourceKind)
	}
	if artifact.SourceKind == CoreArtifactSourceOfficial {
		if artifact.UserSource != "" || artifact.RepositoryID < 1 || artifact.ReleaseID < 1 || artifact.AssetID < 1 {
			return CoreArtifact{}, errors.New("official core artifact IDs must be positive")
		}
	} else {
		if artifact.RepositoryID != 0 || artifact.ReleaseID != 0 || artifact.AssetID != 0 {
			return CoreArtifact{}, errors.New("user-verified core artifact cannot contain official IDs")
		}
		if _, err := coreartifact.NewUserSource(artifact.UserSource); err != nil {
			return CoreArtifact{}, fmt.Errorf("invalid user-verified source: %w", err)
		}
	}
	digest, err := coreartifact.ParseSHA256(artifact.ArchiveSHA256)
	if err != nil || digest.IsZero() {
		return CoreArtifact{}, errors.New("core artifact SHA-256 is invalid or zero")
	}
	artifact.ArchiveSHA256 = digest.String()
	binaryDigest, err := coreartifact.ParseSHA256(artifact.BinarySHA256)
	if err != nil || binaryDigest.IsZero() {
		return CoreArtifact{}, errors.New("core artifact binary SHA-256 is invalid or zero")
	}
	artifact.BinarySHA256 = binaryDigest.String()
	if strings.TrimSpace(artifact.BinaryPath) == "" {
		return CoreArtifact{}, errors.New("core artifact binary path is empty")
	}
	featureFingerprint, err := compactJSON(artifact.FeatureFingerprint, `{}`)
	if err != nil {
		return CoreArtifact{}, fmt.Errorf("core artifact feature fingerprint: %w", err)
	}
	artifact.FeatureFingerprint = featureFingerprint
	if !validCoreArtifactVerification(artifact.VerificationState) {
		return CoreArtifact{}, fmt.Errorf("invalid core artifact verification %q", artifact.VerificationState)
	}
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = time.Now().UTC()
	} else {
		artifact.CreatedAt = artifact.CreatedAt.UTC()
	}
	return artifact, nil
}

func sameCoreArtifactIdentity(left, right CoreArtifact) bool {
	return left.ID == right.ID &&
		left.ExactVersion == right.ExactVersion &&
		left.OperatingSystem == right.OperatingSystem &&
		left.Architecture == right.Architecture &&
		left.Variant == right.Variant &&
		left.SourceKind == right.SourceKind &&
		left.UserSource == right.UserSource &&
		left.RepositoryID == right.RepositoryID &&
		left.ReleaseID == right.ReleaseID &&
		left.AssetID == right.AssetID &&
		left.ArchiveSHA256 == right.ArchiveSHA256 &&
		left.BinarySHA256 == right.BinarySHA256 &&
		left.BinaryPath == right.BinaryPath &&
		left.ReportedVersion == right.ReportedVersion &&
		bytes.Equal(left.FeatureFingerprint, right.FeatureFingerprint)
}

func compactJSON(value json.RawMessage, defaultValue string) (json.RawMessage, error) {
	if len(value) == 0 {
		value = json.RawMessage(defaultValue)
	}
	if !json.Valid(value) {
		return nil, errors.New("invalid JSON")
	}
	var compacted bytes.Buffer
	if err := json.Compact(&compacted, value); err != nil {
		return nil, fmt.Errorf("compact JSON: %w", err)
	}
	return append(json.RawMessage(nil), compacted.Bytes()...), nil
}

func validArtifactVariant(value string) bool {
	if len(value) == 0 || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func validCoreArtifactSource(value CoreArtifactSourceKind) bool {
	return value == CoreArtifactSourceOfficial || value == CoreArtifactSourceUserVerified
}

func validCoreArtifactVerification(value CoreArtifactVerificationState) bool {
	switch value {
	case CoreArtifactVerified, CoreArtifactRevoked, CoreArtifactQuarantined:
		return true
	default:
		return false
	}
}

func nullablePositiveID(value int64) any {
	if value == 0 {
		return nil
	}
	return strconv.FormatInt(value, 10)
}

func parseNullablePositiveID(value sql.NullString) (int64, error) {
	if !value.Valid {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value.String, 10, 64)
	if err != nil || parsed < 1 {
		return 0, errors.New("expected a positive decimal integer")
	}
	return parsed, nil
}
