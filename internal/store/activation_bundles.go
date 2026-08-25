// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

var (
	ErrSubscriptionSnapshotNotFound = errors.New("subscription snapshot not found")
	ErrActivationBundleNotFound     = errors.New("activation bundle not found")
	ErrActivationBundleNotReady     = errors.New("activation bundle is not ready")
)

const (
	// MaximumSubscriptionSnapshotBytes bounds each immutable publication wire.
	// It includes base64-encoded client bodies and per-item diagnostics for all
	// enabled channels, so it is deliberately larger than one startup document.
	MaximumSubscriptionSnapshotBytes int64 = 32 << 20
	maximumActivationMetadataBytes   int64 = 32 << 20
)

type MonitoringTier string

const (
	MonitoringFull        MonitoringTier = "full"
	MonitoringLimited     MonitoringTier = "limited"
	MonitoringProcessOnly MonitoringTier = "process_only"
)

type SubscriptionSnapshot struct {
	ID                  string          `json:"id"`
	CanonicalRevisionID string          `json:"canonical_revision_id"`
	StartupArtifactID   string          `json:"startup_artifact_id"`
	Content             json.RawMessage `json:"-"`
	SHA256              string          `json:"sha256"`
	CreatedAt           time.Time       `json:"created_at"`
}

type ActivationBundle struct {
	ID                     string          `json:"id"`
	StartupArtifactID      string          `json:"startup_artifact_id"`
	SubscriptionSnapshotID string          `json:"subscription_snapshot_id"`
	PublicAddresses        json.RawMessage `json:"-"`
	SourceSnapshots        json.RawMessage `json:"-"`
	MonitoringTier         MonitoringTier  `json:"monitoring_tier"`
	SHA256                 string          `json:"sha256"`
	CreatedAt              time.Time       `json:"created_at"`
}

// SaveActivationBundle atomically persists the subscription bytes and their
// immutable activation envelope after all slow preparation work is complete.
func (s *Store) SaveActivationBundle(
	ctx context.Context,
	snapshot SubscriptionSnapshot,
	bundle ActivationBundle,
) (ActivationBundle, error) {
	preparedSnapshot, err := prepareSubscriptionSnapshot(snapshot)
	if err != nil {
		return ActivationBundle{}, err
	}
	preparedBundle, err := prepareActivationBundle(bundle, preparedSnapshot)
	if err != nil {
		return ActivationBundle{}, err
	}
	var stored ActivationBundle
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		startup, err := getStartupArtifact(ctx, tx, preparedSnapshot.StartupArtifactID)
		if err != nil {
			return err
		}
		if startup.State != StartupArtifactReady || startup.CanonicalRevisionID != preparedSnapshot.CanonicalRevisionID {
			return ErrActivationBundleNotReady
		}
		if err := validateActivationCanonicalHead(ctx, tx, startup); err != nil {
			return err
		}
		core, err := getCoreArtifact(ctx, tx, startup.CoreArtifactID)
		if err != nil {
			return err
		}
		if core.VerificationState != CoreArtifactVerified {
			return ErrActivationBundleNotReady
		}
		if err := validateActivationCapabilityPin(ctx, tx, startup); err != nil {
			return err
		}
		existingSnapshot, snapshotErr := getSubscriptionSnapshot(ctx, tx, preparedSnapshot.ID)
		switch {
		case snapshotErr == nil:
			if !sameSubscriptionSnapshotIdentity(existingSnapshot, preparedSnapshot) {
				return errors.New("subscription snapshot identity collision")
			}
		case errors.Is(snapshotErr, ErrSubscriptionSnapshotNotFound):
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO subscription_snapshots(
                    id, canonical_revision_id, startup_artifact_id,
                    content_json, sha256, created_at
                 ) VALUES (?, ?, ?, ?, ?, ?)`,
				preparedSnapshot.ID,
				preparedSnapshot.CanonicalRevisionID,
				preparedSnapshot.StartupArtifactID,
				string(preparedSnapshot.Content),
				preparedSnapshot.SHA256,
				formatTaskTime(preparedSnapshot.CreatedAt),
			); err != nil {
				return fmt.Errorf("insert subscription snapshot: %w", err)
			}
		default:
			return snapshotErr
		}

		existingBundle, bundleErr := getActivationBundle(ctx, tx, preparedBundle.ID)
		switch {
		case bundleErr == nil:
			if !sameActivationBundleIdentity(existingBundle, preparedBundle) {
				return errors.New("activation bundle identity collision")
			}
			stored = existingBundle
			return nil
		case errors.Is(bundleErr, ErrActivationBundleNotFound):
			if _, err := tx.ExecContext(
				ctx,
				`INSERT INTO activation_bundles(
                    id, startup_artifact_id, subscription_snapshot_id,
                    public_addresses_json, source_snapshots_json,
                    monitoring_tier, sha256, created_at
                 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				preparedBundle.ID,
				preparedBundle.StartupArtifactID,
				preparedBundle.SubscriptionSnapshotID,
				string(preparedBundle.PublicAddresses),
				string(preparedBundle.SourceSnapshots),
				string(preparedBundle.MonitoringTier),
				preparedBundle.SHA256,
				formatTaskTime(preparedBundle.CreatedAt),
			); err != nil {
				return fmt.Errorf("insert activation bundle: %w", err)
			}
		default:
			return bundleErr
		}
		stored, err = getActivationBundle(ctx, tx, preparedBundle.ID)
		return err
	})
	return stored, err
}

func validateActivationCanonicalHead(ctx context.Context, tx *sql.Tx, startup StartupArtifact) error {
	var headRevisionID sql.NullString
	if err := tx.QueryRowContext(
		ctx,
		`SELECT head_revision_id FROM hub_state WHERE singleton = 1`,
	).Scan(&headRevisionID); err != nil {
		return fmt.Errorf("read canonical head for activation bundle: %w", err)
	}
	if valueOrEmpty(headRevisionID) != startup.CanonicalRevisionID {
		return fmt.Errorf(
			"%w: startup artifact canonical revision is not the current head",
			ErrActivationBundleNotReady,
		)
	}
	return nil
}

func validateActivationCapabilityPin(ctx context.Context, tx *sql.Tx, startup StartupArtifact) error {
	if startup.Kind != StartupArtifactStructured {
		return nil
	}
	pin, err := getCapabilityPin(ctx, tx, startup.ExactCoreVersion)
	if err != nil {
		if errors.Is(err, ErrCapabilityPinNotFound) {
			return fmt.Errorf("%w: structured startup capability pin is unavailable", ErrActivationBundleNotReady)
		}
		return err
	}
	if pin.ExactCoreVersion != startup.ExactCoreVersion ||
		pin.CommitSHA != startup.CapabilityCommit ||
		pin.ManifestSHA256 != startup.CapabilityDigest {
		return fmt.Errorf("%w: structured startup capability pin moved", ErrActivationBundleNotReady)
	}
	if _, err := getCapabilityQuarantine(ctx, tx, pin.ManifestSHA256); err == nil {
		return fmt.Errorf("%w: structured startup capability is quarantined", ErrActivationBundleNotReady)
	} else if !errors.Is(err, ErrCapabilityQuarantineNotFound) {
		return err
	}
	return nil
}

func sameSubscriptionSnapshotIdentity(left, right SubscriptionSnapshot) bool {
	return left.ID == right.ID && left.CanonicalRevisionID == right.CanonicalRevisionID &&
		left.StartupArtifactID == right.StartupArtifactID && left.SHA256 == right.SHA256 &&
		bytes.Equal(left.Content, right.Content)
}

func sameActivationBundleIdentity(left, right ActivationBundle) bool {
	return left.ID == right.ID && left.StartupArtifactID == right.StartupArtifactID &&
		left.SubscriptionSnapshotID == right.SubscriptionSnapshotID &&
		bytes.Equal(left.PublicAddresses, right.PublicAddresses) &&
		bytes.Equal(left.SourceSnapshots, right.SourceSnapshots) &&
		left.MonitoringTier == right.MonitoringTier && left.SHA256 == right.SHA256
}

func (s *Store) GetSubscriptionSnapshot(ctx context.Context, snapshotID string) (SubscriptionSnapshot, error) {
	if strings.TrimSpace(snapshotID) == "" {
		return SubscriptionSnapshot{}, errors.New("subscription snapshot id is empty")
	}
	return getSubscriptionSnapshot(ctx, s.db, snapshotID)
}

func (s *Store) GetActivationBundle(ctx context.Context, bundleID string) (ActivationBundle, error) {
	if strings.TrimSpace(bundleID) == "" {
		return ActivationBundle{}, errors.New("activation bundle id is empty")
	}
	return getActivationBundle(ctx, s.db, bundleID)
}

func prepareSubscriptionSnapshot(snapshot SubscriptionSnapshot) (SubscriptionSnapshot, error) {
	if strings.TrimSpace(snapshot.ID) == "" || strings.TrimSpace(snapshot.CanonicalRevisionID) == "" ||
		strings.TrimSpace(snapshot.StartupArtifactID) == "" {
		return SubscriptionSnapshot{}, errors.New("subscription snapshot identity fields are required")
	}
	content, err := canonicalJSONObjectWithLimit(
		snapshot.Content,
		`{}`,
		MaximumSubscriptionSnapshotBytes,
	)
	if err != nil {
		return SubscriptionSnapshot{}, fmt.Errorf("subscription snapshot content: %w", err)
	}
	sum := sha256.Sum256(content)
	digest := hex.EncodeToString(sum[:])
	if snapshot.SHA256 != "" && snapshot.SHA256 != digest {
		return SubscriptionSnapshot{}, errors.New("subscription snapshot digest does not match content")
	}
	snapshot.Content = content
	snapshot.SHA256 = digest
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	} else {
		snapshot.CreatedAt = snapshot.CreatedAt.UTC()
	}
	return snapshot, nil
}

func prepareActivationBundle(
	bundle ActivationBundle,
	snapshot SubscriptionSnapshot,
) (ActivationBundle, error) {
	if strings.TrimSpace(bundle.ID) == "" || strings.TrimSpace(bundle.StartupArtifactID) == "" ||
		strings.TrimSpace(bundle.SubscriptionSnapshotID) == "" {
		return ActivationBundle{}, errors.New("activation bundle identity fields are required")
	}
	if bundle.StartupArtifactID != snapshot.StartupArtifactID || bundle.SubscriptionSnapshotID != snapshot.ID {
		return ActivationBundle{}, errors.New("activation bundle and subscription snapshot identities disagree")
	}
	publicAddresses, err := canonicalJSONObject(bundle.PublicAddresses, `{}`)
	if err != nil {
		return ActivationBundle{}, fmt.Errorf("activation bundle public addresses: %w", err)
	}
	sourceSnapshots, err := canonicalJSONArrayWithLimit(
		bundle.SourceSnapshots,
		`[]`,
		maximumActivationMetadataBytes,
	)
	if err != nil {
		return ActivationBundle{}, fmt.Errorf("activation bundle source snapshots: %w", err)
	}
	if !validMonitoringTier(bundle.MonitoringTier) {
		return ActivationBundle{}, fmt.Errorf("invalid monitoring tier %q", bundle.MonitoringTier)
	}
	digestInput, err := json.Marshal(struct {
		StartupArtifactID  string          `json:"startup_artifact_id"`
		SubscriptionSHA256 string          `json:"subscription_sha256"`
		PublicAddresses    json.RawMessage `json:"public_addresses"`
		SourceSnapshots    json.RawMessage `json:"source_snapshots"`
		MonitoringTier     MonitoringTier  `json:"monitoring_tier"`
	}{
		StartupArtifactID:  bundle.StartupArtifactID,
		SubscriptionSHA256: snapshot.SHA256,
		PublicAddresses:    publicAddresses,
		SourceSnapshots:    sourceSnapshots,
		MonitoringTier:     bundle.MonitoringTier,
	})
	if err != nil {
		return ActivationBundle{}, fmt.Errorf("encode activation bundle identity: %w", err)
	}
	sum := sha256.Sum256(digestInput)
	digest := hex.EncodeToString(sum[:])
	if bundle.SHA256 != "" && bundle.SHA256 != digest {
		return ActivationBundle{}, errors.New("activation bundle digest does not match contents")
	}
	bundle.PublicAddresses = publicAddresses
	bundle.SourceSnapshots = sourceSnapshots
	bundle.SHA256 = digest
	if bundle.CreatedAt.IsZero() {
		bundle.CreatedAt = time.Now().UTC()
	} else {
		bundle.CreatedAt = bundle.CreatedAt.UTC()
	}
	return bundle, nil
}

func getSubscriptionSnapshot(ctx context.Context, q queryRower, snapshotID string) (SubscriptionSnapshot, error) {
	var snapshot SubscriptionSnapshot
	var content, createdAt string
	err := q.QueryRowContext(
		ctx,
		`SELECT id, canonical_revision_id, startup_artifact_id,
                content_json, sha256, created_at
           FROM subscription_snapshots WHERE id = ?`,
		snapshotID,
	).Scan(
		&snapshot.ID,
		&snapshot.CanonicalRevisionID,
		&snapshot.StartupArtifactID,
		&content,
		&snapshot.SHA256,
		&createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionSnapshot{}, fmt.Errorf("%w: %s", ErrSubscriptionSnapshotNotFound, snapshotID)
	}
	if err != nil {
		return SubscriptionSnapshot{}, fmt.Errorf("get subscription snapshot: %w", err)
	}
	snapshot.Content = append(json.RawMessage(nil), content...)
	snapshot.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return SubscriptionSnapshot{}, fmt.Errorf("parse subscription snapshot created_at: %w", err)
	}
	return snapshot, nil
}

func getActivationBundle(ctx context.Context, q queryRower, bundleID string) (ActivationBundle, error) {
	var bundle ActivationBundle
	var publicAddresses, sourceSnapshots, createdAt string
	err := q.QueryRowContext(
		ctx,
		`SELECT id, startup_artifact_id, subscription_snapshot_id,
                public_addresses_json, source_snapshots_json, monitoring_tier,
                sha256, created_at
           FROM activation_bundles WHERE id = ?`,
		bundleID,
	).Scan(
		&bundle.ID,
		&bundle.StartupArtifactID,
		&bundle.SubscriptionSnapshotID,
		&publicAddresses,
		&sourceSnapshots,
		&bundle.MonitoringTier,
		&bundle.SHA256,
		&createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ActivationBundle{}, fmt.Errorf("%w: %s", ErrActivationBundleNotFound, bundleID)
	}
	if err != nil {
		return ActivationBundle{}, fmt.Errorf("get activation bundle: %w", err)
	}
	bundle.PublicAddresses = append(json.RawMessage(nil), publicAddresses...)
	bundle.SourceSnapshots = append(json.RawMessage(nil), sourceSnapshots...)
	bundle.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return ActivationBundle{}, fmt.Errorf("parse activation bundle created_at: %w", err)
	}
	return bundle, nil
}

func canonicalJSONObject(value json.RawMessage, fallback string) (json.RawMessage, error) {
	return canonicalJSONObjectWithLimit(value, fallback, 4<<20)
}

func canonicalJSONObjectWithLimit(
	value json.RawMessage,
	fallback string,
	maximum int64,
) (json.RawMessage, error) {
	if len(value) == 0 {
		value = json.RawMessage(fallback)
	}
	var object map[string]any
	if err := jsonstrict.Decode(value, maximum, &object); err != nil || object == nil {
		return nil, errors.New("value must be one strict JSON object")
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil, err
	}
	return bytes.Clone(encoded), nil
}

func canonicalJSONArray(value json.RawMessage, fallback string) (json.RawMessage, error) {
	return canonicalJSONArrayWithLimit(value, fallback, 4<<20)
}

func canonicalJSONArrayWithLimit(
	value json.RawMessage,
	fallback string,
	maximum int64,
) (json.RawMessage, error) {
	if len(value) == 0 {
		value = json.RawMessage(fallback)
	}
	var array []any
	if err := jsonstrict.Decode(value, maximum, &array); err != nil || array == nil {
		return nil, errors.New("value must be one strict JSON array")
	}
	encoded, err := json.Marshal(array)
	if err != nil {
		return nil, err
	}
	return bytes.Clone(encoded), nil
}

func validMonitoringTier(value MonitoringTier) bool {
	return value == MonitoringFull || value == MonitoringLimited || value == MonitoringProcessOnly
}
