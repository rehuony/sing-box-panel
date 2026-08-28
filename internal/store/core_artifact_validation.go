// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

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
