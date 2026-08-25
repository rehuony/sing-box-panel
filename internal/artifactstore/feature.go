// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

const (
	maximumReportedFeatures = 256
	maximumFeatureLength    = 128
)

type FeatureFingerprintStatus string

const (
	FeatureFingerprintNotReported FeatureFingerprintStatus = "not_reported"
	FeatureFingerprintReported    FeatureFingerprintStatus = "reported"
)

// FeatureFingerprint records only feature evidence reported by the inspected
// binary. NotReported is intentionally distinct from a reported empty set:
// sing-box omits the Tags line when build-tag evidence is unavailable.
type FeatureFingerprint struct {
	Status   FeatureFingerprintStatus `json:"status"`
	Features []string                 `json:"features,omitempty"`
}

func UnknownFeatureFingerprint() FeatureFingerprint {
	return FeatureFingerprint{Status: FeatureFingerprintNotReported}
}

func newReportedFeatureFingerprint(features []string) (FeatureFingerprint, error) {
	normalized, err := normalizeFeatures(features)
	if err != nil {
		return FeatureFingerprint{}, err
	}
	if len(normalized) == 0 {
		return FeatureFingerprint{}, errors.New("reported feature fingerprint is empty")
	}
	return FeatureFingerprint{Status: FeatureFingerprintReported, Features: normalized}, nil
}

func (fingerprint FeatureFingerprint) normalized() (FeatureFingerprint, error) {
	switch fingerprint.Status {
	case "", FeatureFingerprintNotReported:
		if len(fingerprint.Features) != 0 {
			return FeatureFingerprint{}, errors.New("unreported feature fingerprint contains features")
		}
		return UnknownFeatureFingerprint(), nil
	case FeatureFingerprintReported:
		return newReportedFeatureFingerprint(fingerprint.Features)
	default:
		return FeatureFingerprint{}, errors.New("unknown feature fingerprint status")
	}
}

func (fingerprint FeatureFingerprint) CanonicalJSON() (json.RawMessage, error) {
	normalized, err := fingerprint.normalized()
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func normalizeFeatures(features []string) ([]string, error) {
	if len(features) > maximumReportedFeatures {
		return nil, errors.New("too many reported features")
	}
	unique := make(map[string]struct{}, len(features))
	for _, feature := range features {
		if !validFeature(feature) {
			return nil, errors.New("invalid reported feature")
		}
		unique[feature] = struct{}{}
	}
	if len(unique) > maximumReportedFeatures {
		return nil, errors.New("too many reported features")
	}
	normalized := make([]string, 0, len(unique))
	for feature := range unique {
		normalized = append(normalized, feature)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func validFeature(feature string) bool {
	if feature == "" || len(feature) > maximumFeatureLength || strings.TrimSpace(feature) != feature {
		return false
	}
	for _, character := range feature {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '.' {
			continue
		}
		return false
	}
	return true
}
