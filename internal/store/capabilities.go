package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

var (
	ErrCapabilityPinNotFound        = errors.New("capability pin not found")
	ErrCapabilityQuarantineNotFound = errors.New("capability quarantine not found")
)

// CapabilityPin binds one exact core version to immutable repository content.
type CapabilityPin struct {
	ExactCoreVersion string
	Repository       string
	CommitSHA        string
	ManifestSHA256   string
	SupportLevel     capability.SupportLevel
	PinnedAt         time.Time
}

// CapabilityQuarantine records a manifest that must not be used for new work.
type CapabilityQuarantine struct {
	ManifestSHA256 string
	ReasonCode     string
	Diagnostics    json.RawMessage
	QuarantinedAt  time.Time
}

// PinnedCapabilitySnapshot is one transactionally consistent view of an
// exact-version pin, the immutable generation row it names, and any permanent
// quarantine. Callers must not combine independently read pin and manifest
// records because a concurrent pin move could otherwise mix two contracts.
type PinnedCapabilitySnapshot struct {
	Pin        CapabilityPin
	Manifest   CapabilityGenerationManifest
	Quarantine *CapabilityQuarantine
}

// PinnedCapability reads all capability evidence used by projection and UI in
// one SQLite snapshot. The returned manifest and quarantine bytes are owned by
// the caller.
func (s *Store) PinnedCapability(
	ctx context.Context,
	exactCoreVersion string,
) (PinnedCapabilitySnapshot, error) {
	version, err := normalizeCapabilityVersion(exactCoreVersion)
	if err != nil {
		return PinnedCapabilitySnapshot{}, err
	}
	var result PinnedCapabilitySnapshot
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		pin, err := getCapabilityPin(ctx, tx, version)
		if err != nil {
			return err
		}
		if pin.Repository != capability.ManifestRepository {
			return fmt.Errorf(
				"capability pin repository %q is not trusted; want %q",
				pin.Repository,
				capability.ManifestRepository,
			)
		}
		manifest, err := getCapabilityGenerationManifest(
			ctx,
			tx,
			pin.Repository,
			pin.CommitSHA,
			pin.ExactCoreVersion,
			pin.ManifestSHA256,
		)
		if err != nil {
			return err
		}
		if manifest.SupportLevel != pin.SupportLevel {
			return errors.New("capability pin support level does not match stored manifest")
		}

		result.Pin = pin
		result.Manifest = manifest
		quarantine, quarantineErr := getCapabilityQuarantine(ctx, tx, pin.ManifestSHA256)
		switch {
		case quarantineErr == nil:
			result.Quarantine = &quarantine
		case errors.Is(quarantineErr, ErrCapabilityQuarantineNotFound):
		default:
			return quarantineErr
		}
		return nil
	})
	if err != nil {
		return PinnedCapabilitySnapshot{}, err
	}
	result.Manifest.ManifestJSON = append(json.RawMessage(nil), result.Manifest.ManifestJSON...)
	if result.Quarantine != nil {
		clone := *result.Quarantine
		clone.Diagnostics = append(json.RawMessage(nil), result.Quarantine.Diagnostics...)
		result.Quarantine = &clone
	}
	return result, nil
}

// UpsertCapabilityPin explicitly moves or creates an exact-version pin.
func (s *Store) UpsertCapabilityPin(
	ctx context.Context,
	pin CapabilityPin,
) (CapabilityPin, error) {
	prepared, err := prepareCapabilityPin(pin)
	if err != nil {
		return CapabilityPin{}, err
	}
	var stored CapabilityPin
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
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
			prepared.ExactCoreVersion,
			prepared.Repository,
			prepared.CommitSHA,
			prepared.ManifestSHA256,
			string(prepared.SupportLevel),
			formatTaskTime(prepared.PinnedAt),
		); err != nil {
			return fmt.Errorf("upsert capability pin: %w", err)
		}
		stored, err = getCapabilityPin(ctx, tx, prepared.ExactCoreVersion)
		return err
	})
	return stored, err
}

func (s *Store) GetCapabilityPin(
	ctx context.Context,
	exactCoreVersion string,
) (CapabilityPin, error) {
	normalizedVersion, err := normalizeCapabilityVersion(exactCoreVersion)
	if err != nil {
		return CapabilityPin{}, err
	}
	return getCapabilityPin(ctx, s.db, normalizedVersion)
}

func (s *Store) ListCapabilityPins(ctx context.Context) ([]CapabilityPin, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT exact_core_version, repository, commit_sha, manifest_sha256,
                support_level, pinned_at
           FROM capability_pins
          ORDER BY pinned_at DESC, exact_core_version DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list capability pins: %w", err)
	}
	defer rows.Close()

	pins := make([]CapabilityPin, 0)
	for rows.Next() {
		pin, err := scanCapabilityPin(rows)
		if err != nil {
			return nil, fmt.Errorf("scan capability pin: %w", err)
		}
		pins = append(pins, pin)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate capability pins: %w", err)
	}
	return pins, nil
}

func (s *Store) DeleteCapabilityPin(ctx context.Context, exactCoreVersion string) error {
	normalizedVersion, err := normalizeCapabilityVersion(exactCoreVersion)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM capability_pins WHERE exact_core_version = ?`,
		normalizedVersion,
	)
	if err != nil {
		return fmt.Errorf("delete capability pin: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect capability pin deletion: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("%w: %s", ErrCapabilityPinNotFound, normalizedVersion)
	}
	return nil
}

// UpsertCapabilityQuarantine records a permanent quarantine for one manifest
// digest. The first record wins so later retries cannot rewrite its audit
// reason or timestamp.
func (s *Store) UpsertCapabilityQuarantine(
	ctx context.Context,
	quarantine CapabilityQuarantine,
) (CapabilityQuarantine, error) {
	prepared, err := prepareCapabilityQuarantine(quarantine)
	if err != nil {
		return CapabilityQuarantine{}, err
	}
	var stored CapabilityQuarantine
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO capability_quarantine(
                manifest_sha256, reason_code, diagnostics_json, quarantined_at
             ) VALUES (?, ?, ?, ?)
             ON CONFLICT(manifest_sha256) DO NOTHING`,
			prepared.ManifestSHA256,
			prepared.ReasonCode,
			string(prepared.Diagnostics),
			formatTaskTime(prepared.QuarantinedAt),
		); err != nil {
			return fmt.Errorf("upsert capability quarantine: %w", err)
		}
		stored, err = getCapabilityQuarantine(ctx, tx, prepared.ManifestSHA256)
		return err
	})
	return stored, err
}

func (s *Store) GetCapabilityQuarantine(
	ctx context.Context,
	manifestSHA256 string,
) (CapabilityQuarantine, error) {
	normalizedDigest, err := normalizeManifestDigest(manifestSHA256)
	if err != nil {
		return CapabilityQuarantine{}, err
	}
	return getCapabilityQuarantine(ctx, s.db, normalizedDigest)
}

func (s *Store) ListCapabilityQuarantines(
	ctx context.Context,
) ([]CapabilityQuarantine, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT manifest_sha256, reason_code, diagnostics_json, quarantined_at
           FROM capability_quarantine
          ORDER BY quarantined_at DESC, manifest_sha256 DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list capability quarantines: %w", err)
	}
	defer rows.Close()

	quarantines := make([]CapabilityQuarantine, 0)
	for rows.Next() {
		quarantine, err := scanCapabilityQuarantine(rows)
		if err != nil {
			return nil, fmt.Errorf("scan capability quarantine: %w", err)
		}
		quarantines = append(quarantines, quarantine)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate capability quarantines: %w", err)
	}
	return quarantines, nil
}

func (s *Store) DeleteCapabilityQuarantine(
	ctx context.Context,
	manifestSHA256 string,
) error {
	normalizedDigest, err := normalizeManifestDigest(manifestSHA256)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM capability_quarantine WHERE manifest_sha256 = ?`,
		normalizedDigest,
	)
	if err != nil {
		return fmt.Errorf("delete capability quarantine: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect capability quarantine deletion: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("%w: %s", ErrCapabilityQuarantineNotFound, normalizedDigest)
	}
	return nil
}

func prepareCapabilityPin(pin CapabilityPin) (CapabilityPin, error) {
	normalizedVersion, err := normalizeCapabilityVersion(pin.ExactCoreVersion)
	if err != nil {
		return CapabilityPin{}, err
	}
	digest, err := coreartifact.ParseSHA256(pin.ManifestSHA256)
	if err != nil || digest.IsZero() {
		return CapabilityPin{}, errors.New("capability pin manifest digest is invalid or zero")
	}
	reference, err := capability.NewReference(pin.Repository, pin.CommitSHA, digest)
	if err != nil {
		return CapabilityPin{}, err
	}
	switch pin.SupportLevel {
	case capability.SupportNativeStructured,
		capability.SupportCompatibleStructured,
		capability.SupportManualJSON,
		capability.SupportUnavailable:
	default:
		return CapabilityPin{}, fmt.Errorf("invalid capability support level %q", pin.SupportLevel)
	}
	pin.ExactCoreVersion = normalizedVersion
	pin.Repository = reference.Repository()
	pin.CommitSHA = reference.Commit()
	pin.ManifestSHA256 = reference.Digest().String()
	if pin.PinnedAt.IsZero() {
		pin.PinnedAt = time.Now().UTC()
	} else {
		pin.PinnedAt = pin.PinnedAt.UTC()
	}
	return pin, nil
}

func getCapabilityPin(
	ctx context.Context,
	q queryRower,
	exactCoreVersion string,
) (CapabilityPin, error) {
	pin, err := scanCapabilityPin(q.QueryRowContext(
		ctx,
		`SELECT exact_core_version, repository, commit_sha, manifest_sha256,
                support_level, pinned_at
           FROM capability_pins
          WHERE exact_core_version = ?`,
		exactCoreVersion,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return CapabilityPin{}, fmt.Errorf("%w: %s", ErrCapabilityPinNotFound, exactCoreVersion)
	}
	if err != nil {
		return CapabilityPin{}, fmt.Errorf("get capability pin: %w", err)
	}
	return pin, nil
}

func scanCapabilityPin(row taskScanner) (CapabilityPin, error) {
	var (
		pin      CapabilityPin
		pinnedAt string
	)
	if err := row.Scan(
		&pin.ExactCoreVersion,
		&pin.Repository,
		&pin.CommitSHA,
		&pin.ManifestSHA256,
		&pin.SupportLevel,
		&pinnedAt,
	); err != nil {
		return CapabilityPin{}, err
	}
	parsed, err := parseTaskTime(pinnedAt)
	if err != nil {
		return CapabilityPin{}, fmt.Errorf("parse pinned_at: %w", err)
	}
	pin.PinnedAt = parsed
	return pin, nil
}

func prepareCapabilityQuarantine(
	quarantine CapabilityQuarantine,
) (CapabilityQuarantine, error) {
	digest, err := coreartifact.ParseSHA256(quarantine.ManifestSHA256)
	if err != nil || digest.IsZero() {
		return CapabilityQuarantine{}, errors.New("capability quarantine digest is invalid or zero")
	}
	if strings.TrimSpace(quarantine.ReasonCode) == "" {
		return CapabilityQuarantine{}, errors.New("capability quarantine reason is empty")
	}
	diagnostics, err := compactJSON(quarantine.Diagnostics, `{}`)
	if err != nil {
		return CapabilityQuarantine{}, fmt.Errorf("capability quarantine diagnostics: %w", err)
	}
	quarantine.ManifestSHA256 = digest.String()
	quarantine.Diagnostics = diagnostics
	if quarantine.QuarantinedAt.IsZero() {
		quarantine.QuarantinedAt = time.Now().UTC()
	} else {
		quarantine.QuarantinedAt = quarantine.QuarantinedAt.UTC()
	}
	return quarantine, nil
}

func getCapabilityQuarantine(
	ctx context.Context,
	q queryRower,
	manifestSHA256 string,
) (CapabilityQuarantine, error) {
	quarantine, err := scanCapabilityQuarantine(q.QueryRowContext(
		ctx,
		`SELECT manifest_sha256, reason_code, diagnostics_json, quarantined_at
           FROM capability_quarantine
          WHERE manifest_sha256 = ?`,
		manifestSHA256,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return CapabilityQuarantine{}, fmt.Errorf(
			"%w: %s",
			ErrCapabilityQuarantineNotFound,
			manifestSHA256,
		)
	}
	if err != nil {
		return CapabilityQuarantine{}, fmt.Errorf("get capability quarantine: %w", err)
	}
	return quarantine, nil
}

func scanCapabilityQuarantine(row taskScanner) (CapabilityQuarantine, error) {
	var (
		quarantine    CapabilityQuarantine
		diagnostics   string
		quarantinedAt string
	)
	if err := row.Scan(
		&quarantine.ManifestSHA256,
		&quarantine.ReasonCode,
		&diagnostics,
		&quarantinedAt,
	); err != nil {
		return CapabilityQuarantine{}, err
	}
	quarantine.Diagnostics = append(json.RawMessage(nil), diagnostics...)
	parsed, err := parseTaskTime(quarantinedAt)
	if err != nil {
		return CapabilityQuarantine{}, fmt.Errorf("parse quarantined_at: %w", err)
	}
	quarantine.QuarantinedAt = parsed
	return quarantine, nil
}

func normalizeCapabilityVersion(value string) (string, error) {
	version, err := coreartifact.ParseExactVersion(value)
	if err != nil || version.IsZero() {
		return "", errors.New("capability pin exact core version is invalid")
	}
	return version.String(), nil
}

func normalizeManifestDigest(value string) (string, error) {
	digest, err := coreartifact.ParseSHA256(value)
	if err != nil || digest.IsZero() {
		return "", errors.New("capability manifest digest is invalid or zero")
	}
	return digest.String(), nil
}
