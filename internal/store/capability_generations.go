// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var (
	ErrCapabilityGenerationNotFound  = errors.New("capability generation not found")
	ErrCapabilityGenerationConflict  = errors.New("capability generation commit content conflicts with stored candidate")
	ErrCapabilityManifestNotFound    = errors.New("capability generation manifest not found")
	ErrCapabilityManifestQuarantined = errors.New("capability generation manifest is quarantined")
)

type CapabilityGeneration struct {
	ID            string
	Repository    string
	CommitSHA     string
	SourceSHA256  string
	ManifestCount int
	RefreshedAt   time.Time
}

type CapabilityGenerationManifest struct {
	GenerationID     string
	Repository       string
	CommitSHA        string
	ExactCoreVersion string
	Path             string
	ManifestSHA256   string
	SupportLevel     capability.SupportLevel
	ManifestJSON     json.RawMessage
}

type CapabilityGenerationSave struct {
	Generation CapabilityGeneration
	Manifests  []CapabilityGenerationManifest
	Created    bool
}

// SaveCapabilityGeneration validates the complete generation before opening a
// transaction, then publishes the parent and every manifest atomically. A
// commit can be observed with all entries or not at all.
func (s *Store) SaveCapabilityGeneration(
	ctx context.Context,
	source []byte,
	refreshedAt time.Time,
) (CapabilityGenerationSave, error) {
	generation, err := capability.DecodeGeneration(source)
	if err != nil {
		return CapabilityGenerationSave{}, err
	}
	generationDigest, err := generation.Digest()
	if err != nil {
		return CapabilityGenerationSave{}, err
	}
	sourceDigest := coreartifact.NewSHA256(sha256.Sum256(source))
	if refreshedAt.IsZero() {
		refreshedAt = time.Now().UTC()
	} else {
		refreshedAt = refreshedAt.UTC()
	}

	entries := generation.Manifests()
	prepared := CapabilityGenerationSave{
		Generation: CapabilityGeneration{
			ID:            generationDigest.String(),
			Repository:    generation.Repository(),
			CommitSHA:     generation.Commit(),
			SourceSHA256:  sourceDigest.String(),
			ManifestCount: len(entries),
			RefreshedAt:   refreshedAt,
		},
		Manifests: make([]CapabilityGenerationManifest, len(entries)),
		Created:   true,
	}
	for index, entry := range entries {
		manifest := entry.Manifest()
		if manifest == nil {
			return CapabilityGenerationSave{}, errors.New("capability generation contains an invalid manifest")
		}
		canonical, err := manifest.CanonicalJSON()
		if err != nil {
			return CapabilityGenerationSave{}, err
		}
		prepared.Manifests[index] = CapabilityGenerationManifest{
			GenerationID:     prepared.Generation.ID,
			Repository:       prepared.Generation.Repository,
			CommitSHA:        prepared.Generation.CommitSHA,
			ExactCoreVersion: manifest.CoreVersion().String(),
			Path:             entry.Path(),
			ManifestSHA256:   entry.Digest().String(),
			SupportLevel:     manifest.SupportLevel(),
			ManifestJSON:     append(json.RawMessage(nil), canonical...),
		}
	}

	var result CapabilityGenerationSave
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		for _, manifest := range prepared.Manifests {
			var reason string
			lookupErr := tx.QueryRowContext(
				ctx,
				`SELECT reason_code FROM capability_quarantine WHERE manifest_sha256 = ?`,
				manifest.ManifestSHA256,
			).Scan(&reason)
			if lookupErr == nil {
				return fmt.Errorf(
					"%w: %s (%s)",
					ErrCapabilityManifestQuarantined,
					manifest.ManifestSHA256,
					reason,
				)
			}
			if !errors.Is(lookupErr, sql.ErrNoRows) {
				return fmt.Errorf("inspect capability quarantine: %w", lookupErr)
			}
		}

		existing, lookupErr := getCapabilityGenerationByCommit(
			ctx,
			tx,
			prepared.Generation.Repository,
			prepared.Generation.CommitSHA,
		)
		if lookupErr == nil {
			if existing.ID != prepared.Generation.ID {
				return fmt.Errorf(
					"%w: %s@%s is already %s, candidate is %s",
					ErrCapabilityGenerationConflict,
					prepared.Generation.Repository,
					prepared.Generation.CommitSHA,
					existing.ID,
					prepared.Generation.ID,
				)
			}
			storedManifests, err := listCapabilityGenerationManifests(ctx, tx, existing.ID)
			if err != nil {
				return err
			}
			if len(storedManifests) != existing.ManifestCount {
				return fmt.Errorf(
					"stored capability generation %s has %d manifests, expected %d",
					existing.ID,
					len(storedManifests),
					existing.ManifestCount,
				)
			}
			result = CapabilityGenerationSave{Generation: existing, Manifests: storedManifests, Created: false}
			return nil
		}
		if !errors.Is(lookupErr, ErrCapabilityGenerationNotFound) {
			return lookupErr
		}

		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO capability_generations(
                id, repository, commit_sha, source_sha256, manifest_count, refreshed_at
             ) VALUES (?, ?, ?, ?, ?, ?)`,
			prepared.Generation.ID,
			prepared.Generation.Repository,
			prepared.Generation.CommitSHA,
			prepared.Generation.SourceSHA256,
			prepared.Generation.ManifestCount,
			formatTaskTime(prepared.Generation.RefreshedAt),
		); err != nil {
			return fmt.Errorf("insert capability generation: %w", err)
		}
		for _, manifest := range prepared.Manifests {
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO capability_generation_manifests(
                    generation_id, exact_core_version, path, manifest_sha256,
                    support_level, manifest_json
                 ) VALUES (?, ?, ?, ?, ?, ?)`,
				manifest.GenerationID,
				manifest.ExactCoreVersion,
				manifest.Path,
				manifest.ManifestSHA256,
				string(manifest.SupportLevel),
				string(manifest.ManifestJSON),
			); err != nil {
				return fmt.Errorf("insert capability manifest %s: %w", manifest.ExactCoreVersion, err)
			}
		}
		result = cloneCapabilityGenerationSave(prepared)
		return nil
	})
	return result, err
}

func (s *Store) CapabilityGenerationByCommit(
	ctx context.Context,
	commitSHA string,
) (CapabilityGeneration, error) {
	if err := validateCapabilityGenerationCommit(commitSHA); err != nil {
		return CapabilityGeneration{}, err
	}
	return getCapabilityGenerationByCommit(ctx, s.db, capability.ManifestRepository, commitSHA)
}

func (s *Store) ListCapabilityGenerations(
	ctx context.Context,
	limit int,
) ([]CapabilityGeneration, error) {
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 200 {
		return nil, errors.New("capability generation limit must be between 1 and 200")
	}
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, repository, commit_sha, source_sha256, manifest_count, refreshed_at
           FROM capability_generations
          WHERE repository = ?
          ORDER BY refreshed_at DESC, id DESC
          LIMIT ?`,
		capability.ManifestRepository,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list capability generations: %w", err)
	}
	defer rows.Close()
	result := make([]CapabilityGeneration, 0)
	for rows.Next() {
		generation, err := scanCapabilityGeneration(rows)
		if err != nil {
			return nil, fmt.Errorf("scan capability generation: %w", err)
		}
		result = append(result, generation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate capability generations: %w", err)
	}
	return result, nil
}

func (s *Store) CapabilityGenerationManifest(
	ctx context.Context,
	commitSHA string,
	exactCoreVersion string,
	manifestSHA256 string,
) (CapabilityGenerationManifest, error) {
	if err := validateCapabilityGenerationCommit(commitSHA); err != nil {
		return CapabilityGenerationManifest{}, err
	}
	version, err := normalizeCapabilityVersion(exactCoreVersion)
	if err != nil {
		return CapabilityGenerationManifest{}, err
	}
	digest := ""
	if strings.TrimSpace(manifestSHA256) != "" {
		digest, err = normalizeManifestDigest(manifestSHA256)
		if err != nil {
			return CapabilityGenerationManifest{}, err
		}
	}
	return getCapabilityGenerationManifest(
		ctx,
		s.db,
		capability.ManifestRepository,
		commitSHA,
		version,
		digest,
	)
}

// PinCapabilityGenerationManifest moves an exact-version pin only after the
// requested immutable candidate and its digest are found and confirmed not to
// be quarantined, all within one transaction.
func (s *Store) PinCapabilityGenerationManifest(
	ctx context.Context,
	commitSHA string,
	exactCoreVersion string,
	manifestSHA256 string,
	pinnedAt time.Time,
) (CapabilityPin, error) {
	if err := validateCapabilityGenerationCommit(commitSHA); err != nil {
		return CapabilityPin{}, err
	}
	version, err := normalizeCapabilityVersion(exactCoreVersion)
	if err != nil {
		return CapabilityPin{}, err
	}
	digest, err := normalizeManifestDigest(manifestSHA256)
	if err != nil {
		return CapabilityPin{}, err
	}
	if pinnedAt.IsZero() {
		pinnedAt = time.Now().UTC()
	} else {
		pinnedAt = pinnedAt.UTC()
	}

	var result CapabilityPin
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		manifest, err := getCapabilityGenerationManifest(
			ctx,
			tx,
			capability.ManifestRepository,
			commitSHA,
			version,
			digest,
		)
		if err != nil {
			return err
		}
		var reason string
		err = tx.QueryRowContext(
			ctx,
			`SELECT reason_code FROM capability_quarantine WHERE manifest_sha256 = ?`,
			manifest.ManifestSHA256,
		).Scan(&reason)
		if err == nil {
			return fmt.Errorf(
				"%w: %s (%s)",
				ErrCapabilityManifestQuarantined,
				manifest.ManifestSHA256,
				reason,
			)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("inspect capability quarantine: %w", err)
		}
		pin := CapabilityPin{
			ExactCoreVersion: manifest.ExactCoreVersion,
			Repository:       manifest.Repository,
			CommitSHA:        manifest.CommitSHA,
			ManifestSHA256:   manifest.ManifestSHA256,
			SupportLevel:     manifest.SupportLevel,
			PinnedAt:         pinnedAt,
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO capability_pins(
                    exact_core_version, repository, commit_sha, manifest_sha256,
                    support_level, pinned_at
                 ) VALUES (?, ?, ?, ?, ?, ?)
                 ON CONFLICT(exact_core_version) DO UPDATE SET
                    repository = excluded.repository,
                    commit_sha = excluded.commit_sha,
                    manifest_sha256 = excluded.manifest_sha256,
                    support_level = excluded.support_level,
                    pinned_at = excluded.pinned_at`,
			pin.ExactCoreVersion,
			pin.Repository,
			pin.CommitSHA,
			pin.ManifestSHA256,
			string(pin.SupportLevel),
			formatTaskTime(pin.PinnedAt),
		); err != nil {
			return fmt.Errorf("pin capability generation manifest: %w", err)
		}
		result, err = getCapabilityPin(ctx, tx, pin.ExactCoreVersion)
		return err
	})
	return result, err
}

func getCapabilityGenerationByCommit(
	ctx context.Context,
	q queryRower,
	repository string,
	commitSHA string,
) (CapabilityGeneration, error) {
	generation, err := scanCapabilityGeneration(q.QueryRowContext(
		ctx,
		`SELECT id, repository, commit_sha, source_sha256, manifest_count, refreshed_at
           FROM capability_generations
          WHERE repository = ? AND commit_sha = ?`,
		repository,
		commitSHA,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return CapabilityGeneration{}, fmt.Errorf(
			"%w: %s@%s",
			ErrCapabilityGenerationNotFound,
			repository,
			commitSHA,
		)
	}
	if err != nil {
		return CapabilityGeneration{}, fmt.Errorf("get capability generation: %w", err)
	}
	return generation, nil
}

func getCapabilityGenerationManifest(
	ctx context.Context,
	q queryRower,
	repository string,
	commitSHA string,
	exactCoreVersion string,
	manifestSHA256 string,
) (CapabilityGenerationManifest, error) {
	query := `SELECT m.generation_id, g.repository, g.commit_sha,
                       m.exact_core_version, m.path, m.manifest_sha256,
                       m.support_level, m.manifest_json
                  FROM capability_generation_manifests AS m
                  JOIN capability_generations AS g ON g.id = m.generation_id
                 WHERE g.repository = ? AND g.commit_sha = ?
                   AND m.exact_core_version = ?`
	arguments := []any{repository, commitSHA, exactCoreVersion}
	if manifestSHA256 != "" {
		query += ` AND m.manifest_sha256 = ?`
		arguments = append(arguments, manifestSHA256)
	}
	manifest, err := scanCapabilityGenerationManifest(q.QueryRowContext(ctx, query, arguments...))
	if errors.Is(err, sql.ErrNoRows) {
		return CapabilityGenerationManifest{}, fmt.Errorf(
			"%w: %s@%s version %s digest %s",
			ErrCapabilityManifestNotFound,
			repository,
			commitSHA,
			exactCoreVersion,
			manifestSHA256,
		)
	}
	if err != nil {
		return CapabilityGenerationManifest{}, fmt.Errorf("get capability generation manifest: %w", err)
	}
	return manifest, nil
}

func listCapabilityGenerationManifests(
	ctx context.Context,
	q interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	},
	generationID string,
) ([]CapabilityGenerationManifest, error) {
	rows, err := q.QueryContext(
		ctx,
		`SELECT m.generation_id, g.repository, g.commit_sha,
                m.exact_core_version, m.path, m.manifest_sha256,
                m.support_level, m.manifest_json
           FROM capability_generation_manifests AS m
           JOIN capability_generations AS g ON g.id = m.generation_id
          WHERE m.generation_id = ?
          ORDER BY m.exact_core_version ASC`,
		generationID,
	)
	if err != nil {
		return nil, fmt.Errorf("list capability generation manifests: %w", err)
	}
	defer rows.Close()
	result := make([]CapabilityGenerationManifest, 0)
	for rows.Next() {
		manifest, err := scanCapabilityGenerationManifest(rows)
		if err != nil {
			return nil, fmt.Errorf("scan capability generation manifest: %w", err)
		}
		result = append(result, manifest)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate capability generation manifests: %w", err)
	}
	sort.SliceStable(result, func(left, right int) bool {
		leftVersion, leftErr := coreartifact.ParseExactVersion(result[left].ExactCoreVersion)
		rightVersion, rightErr := coreartifact.ParseExactVersion(result[right].ExactCoreVersion)
		if leftErr != nil || rightErr != nil {
			return result[left].ExactCoreVersion < result[right].ExactCoreVersion
		}
		return leftVersion.Compare(rightVersion) < 0
	})
	return result, nil
}

func scanCapabilityGeneration(row taskScanner) (CapabilityGeneration, error) {
	var generation CapabilityGeneration
	var refreshedAt string
	if err := row.Scan(
		&generation.ID,
		&generation.Repository,
		&generation.CommitSHA,
		&generation.SourceSHA256,
		&generation.ManifestCount,
		&refreshedAt,
	); err != nil {
		return CapabilityGeneration{}, err
	}
	parsed, err := parseTaskTime(refreshedAt)
	if err != nil {
		return CapabilityGeneration{}, fmt.Errorf("parse refreshed_at: %w", err)
	}
	generation.RefreshedAt = parsed
	return generation, nil
}

func scanCapabilityGenerationManifest(row taskScanner) (CapabilityGenerationManifest, error) {
	var manifest CapabilityGenerationManifest
	var document string
	if err := row.Scan(
		&manifest.GenerationID,
		&manifest.Repository,
		&manifest.CommitSHA,
		&manifest.ExactCoreVersion,
		&manifest.Path,
		&manifest.ManifestSHA256,
		&manifest.SupportLevel,
		&document,
	); err != nil {
		return CapabilityGenerationManifest{}, err
	}
	manifest.ManifestJSON = append(json.RawMessage(nil), document...)
	return manifest, nil
}

func validateCapabilityGenerationCommit(commitSHA string) error {
	digest := coreartifact.NewSHA256([32]byte{1})
	_, err := capability.NewReference(capability.ManifestRepository, commitSHA, digest)
	return err
}

func cloneCapabilityGenerationSave(source CapabilityGenerationSave) CapabilityGenerationSave {
	clone := source
	clone.Manifests = make([]CapabilityGenerationManifest, len(source.Manifests))
	for index, manifest := range source.Manifests {
		clone.Manifests[index] = manifest
		clone.Manifests[index].ManifestJSON = append(json.RawMessage(nil), manifest.ManifestJSON...)
	}
	return clone
}
