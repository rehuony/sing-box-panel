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
	ErrActivationBundleNotFound = errors.New("activation bundle not found")
	ErrActivationBundleNotReady = errors.New("activation bundle is not ready")
)

type MonitoringTier string

const (
	MonitoringFull        MonitoringTier = "full"
	MonitoringLimited     MonitoringTier = "limited"
	MonitoringProcessOnly MonitoringTier = "process_only"
)

type ActivationBundle struct {
	ID                string         `json:"id"`
	StartupArtifactID string         `json:"startup_artifact_id"`
	MonitoringTier    MonitoringTier `json:"monitoring_tier"`
	SHA256            string         `json:"sha256"`
	CreatedAt         time.Time      `json:"created_at"`
}

// SaveActivationBundle persists only runtime identity. Subscriptions are
// assembled from their current authorized node versions at request time.
func (s *Store) SaveActivationBundle(ctx context.Context, bundle ActivationBundle) (ActivationBundle, error) {
	prepared, err := prepareActivationBundle(bundle)
	if err != nil {
		return ActivationBundle{}, err
	}
	var stored ActivationBundle
	err = s.WithTx(ctx, func(tx *sql.Tx) error {
		startup, err := getStartupArtifact(ctx, tx, prepared.StartupArtifactID)
		if err != nil {
			return err
		}
		if startup.State != StartupArtifactReady {
			return ErrActivationBundleNotReady
		}
		if err := validateActivationCanonicalHead(ctx, tx, startup); err != nil {
			return err
		}
		core, err := getCoreArtifact(ctx, tx, startup.CoreArtifactID)
		if err != nil || core.VerificationState != CoreArtifactVerified {
			return ErrActivationBundleNotReady
		}
		existing, getErr := getActivationBundle(ctx, tx, prepared.ID)
		switch {
		case getErr == nil:
			if existing != prepared {
				return errors.New("activation bundle identity collision")
			}
			stored = existing
			return nil
		case !errors.Is(getErr, ErrActivationBundleNotFound):
			return getErr
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO activation_bundles(
			id, startup_artifact_id, monitoring_tier, sha256, created_at
		) VALUES (?, ?, ?, ?, ?)`, prepared.ID, prepared.StartupArtifactID,
			string(prepared.MonitoringTier), prepared.SHA256, formatTaskTime(prepared.CreatedAt)); err != nil {
			return fmt.Errorf("insert activation bundle: %w", err)
		}
		stored, err = getActivationBundle(ctx, tx, prepared.ID)
		return err
	})
	return stored, err
}

func validateActivationCanonicalHead(ctx context.Context, tx *sql.Tx, startup StartupArtifact) error {
	var headRevisionID sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT head_revision_id FROM hub_state WHERE singleton = 1`).Scan(&headRevisionID); err != nil {
		return fmt.Errorf("read canonical head for activation bundle: %w", err)
	}
	if valueOrEmpty(headRevisionID) != startup.CanonicalRevisionID {
		return fmt.Errorf("%w: startup artifact canonical revision is not the current head", ErrActivationBundleNotReady)
	}
	return nil
}

func (s *Store) GetActivationBundle(ctx context.Context, bundleID string) (ActivationBundle, error) {
	if strings.TrimSpace(bundleID) == "" {
		return ActivationBundle{}, errors.New("activation bundle id is empty")
	}
	return getActivationBundle(ctx, s.db, bundleID)
}

func prepareActivationBundle(bundle ActivationBundle) (ActivationBundle, error) {
	if strings.TrimSpace(bundle.ID) == "" || strings.TrimSpace(bundle.StartupArtifactID) == "" {
		return ActivationBundle{}, errors.New("activation bundle identity fields are required")
	}
	if !validMonitoringTier(bundle.MonitoringTier) {
		return ActivationBundle{}, fmt.Errorf("invalid monitoring tier %q", bundle.MonitoringTier)
	}
	digestInput, err := json.Marshal(struct {
		StartupArtifactID string         `json:"startup_artifact_id"`
		MonitoringTier    MonitoringTier `json:"monitoring_tier"`
	}{bundle.StartupArtifactID, bundle.MonitoringTier})
	if err != nil {
		return ActivationBundle{}, err
	}
	sum := sha256.Sum256(digestInput)
	digest := hex.EncodeToString(sum[:])
	if bundle.SHA256 != "" && bundle.SHA256 != digest {
		return ActivationBundle{}, errors.New("activation bundle digest does not match contents")
	}
	bundle.SHA256 = digest
	if bundle.CreatedAt.IsZero() {
		bundle.CreatedAt = time.Now().UTC()
	} else {
		bundle.CreatedAt = bundle.CreatedAt.UTC()
	}
	return bundle, nil
}

func getActivationBundle(ctx context.Context, q queryRower, bundleID string) (ActivationBundle, error) {
	var bundle ActivationBundle
	var createdAt string
	err := q.QueryRowContext(ctx, `SELECT id, startup_artifact_id, monitoring_tier, sha256, created_at
		FROM activation_bundles WHERE id = ?`, bundleID).Scan(
		&bundle.ID, &bundle.StartupArtifactID, &bundle.MonitoringTier, &bundle.SHA256, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ActivationBundle{}, fmt.Errorf("%w: %s", ErrActivationBundleNotFound, bundleID)
	}
	if err != nil {
		return ActivationBundle{}, fmt.Errorf("get activation bundle: %w", err)
	}
	bundle.CreatedAt, err = parseTaskTime(createdAt)
	if err != nil {
		return ActivationBundle{}, fmt.Errorf("parse activation bundle created_at: %w", err)
	}
	return bundle, nil
}

func canonicalJSONObject(value json.RawMessage, fallback string) (json.RawMessage, error) {
	return canonicalJSONObjectWithLimit(value, fallback, 4<<20)
}

func canonicalJSONObjectWithLimit(value json.RawMessage, fallback string, maximum int64) (json.RawMessage, error) {
	if len(value) == 0 {
		value = json.RawMessage(fallback)
	}
	var object map[string]any
	if err := jsonstrict.Decode(value, maximum, &object); err != nil || object == nil {
		return nil, errors.New("value must be one strict JSON object")
	}
	encoded, err := json.Marshal(object)
	return bytes.Clone(encoded), err
}

func canonicalJSONArray(value json.RawMessage, fallback string) (json.RawMessage, error) {
	return canonicalJSONArrayWithLimit(value, fallback, 4<<20)
}

func canonicalJSONArrayWithLimit(value json.RawMessage, fallback string, maximum int64) (json.RawMessage, error) {
	if len(value) == 0 {
		value = json.RawMessage(fallback)
	}
	var array []any
	if err := jsonstrict.Decode(value, maximum, &array); err != nil || array == nil {
		return nil, errors.New("value must be one strict JSON array")
	}
	encoded, err := json.Marshal(array)
	return bytes.Clone(encoded), err
}

func validMonitoringTier(value MonitoringTier) bool {
	return value == MonitoringFull || value == MonitoringLimited || value == MonitoringProcessOnly
}
