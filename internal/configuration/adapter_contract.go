// SPDX-License-Identifier: GPL-3.0-or-later

package configuration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
)

var (
	ErrUnsupportedCoreProfile = errors.New("unsupported sing-box core profile")
	ErrInvalidProfile         = errors.New("invalid sing-box core profile")
	ErrProjection             = errors.New("configuration projection failed")
	ErrProjectionBlocked      = errors.New("configuration projection is blocked")
	ErrIgnoredNotAccepted     = errors.New("ignored configuration fields were not accepted")
)

const maximumFeatureFingerprintBytes = 16 << 10

type ProjectionDiagnosticClass string

const (
	DiagnosticDirect   ProjectionDiagnosticClass = "direct"
	DiagnosticMapped   ProjectionDiagnosticClass = "mapped"
	DiagnosticIgnored  ProjectionDiagnosticClass = "ignored"
	DiagnosticBlocking ProjectionDiagnosticClass = "blocking"
)

type ProjectionDiagnostic struct {
	Class   ProjectionDiagnosticClass `json:"class"`
	Code    string                    `json:"code"`
	Path    string                    `json:"path"`
	Message string                    `json:"message"`
}

// CoreProfile contains every binary property that can change the executable
// configuration contract. A semantic version alone is insufficient.
type CoreProfile struct {
	ExactVersion       string          `json:"exact_version"`
	OperatingSystem    string          `json:"os"`
	Architecture       string          `json:"arch"`
	Variant            string          `json:"variant"`
	FeatureFingerprint json.RawMessage `json:"feature_fingerprint"`
}

type FeatureFingerprint struct {
	Status   string   `json:"status"`
	Features []string `json:"features,omitempty"`
}

type ProjectionRequest struct {
	CanonicalJSON []byte
}

type ProjectionResult struct {
	ConfigJSON    []byte                 `json:"config"`
	Diagnostics   []ProjectionDiagnostic `json:"diagnostics"`
	IgnoredDigest string                 `json:"ignored_digest,omitempty"`
}

type AdapterProvenance struct {
	UpstreamTag    string `json:"upstream_tag"`
	UpstreamCommit string `json:"upstream_commit"`
	Source         string `json:"source"`
}

type Adapter interface {
	ID() string
	Revision() string
	ExactVersion() string
	Provenance() AdapterProvenance
	Supports(CoreProfile) bool
	Project(ProjectionRequest) (ProjectionResult, error)
}

func ValidateCoreProfile(profile CoreProfile) (CoreProfile, error) {
	profile.ExactVersion = strings.TrimSpace(profile.ExactVersion)
	version, err := coreartifact.ParseExactVersion(profile.ExactVersion)
	if err != nil || version.IsZero() || version.String() != profile.ExactVersion {
		return CoreProfile{}, fmt.Errorf("%w: exact version %q", ErrInvalidProfile, profile.ExactVersion)
	}
	if profile.OperatingSystem != string(coreartifact.OperatingSystemLinux) {
		return CoreProfile{}, fmt.Errorf("%w: operating system %q", ErrInvalidProfile, profile.OperatingSystem)
	}
	if profile.Architecture != string(coreartifact.ArchitectureARM64) &&
		profile.Architecture != string(coreartifact.ArchitectureAMD64) {
		return CoreProfile{}, fmt.Errorf("%w: architecture %q", ErrInvalidProfile, profile.Architecture)
	}
	if strings.TrimSpace(profile.Variant) == "" {
		return CoreProfile{}, fmt.Errorf("%w: variant is empty", ErrInvalidProfile)
	}
	fingerprint, canonical, err := DecodeFeatureFingerprint(profile.FeatureFingerprint)
	if err != nil {
		return CoreProfile{}, err
	}
	profile.FeatureFingerprint = canonical
	_ = fingerprint
	return profile, nil
}

func DecodeFeatureFingerprint(raw []byte) (FeatureFingerprint, json.RawMessage, error) {
	var fingerprint FeatureFingerprint
	if err := jsonstrict.Decode(raw, maximumFeatureFingerprintBytes, &fingerprint); err != nil {
		return FeatureFingerprint{}, nil, fmt.Errorf("%w: feature fingerprint: %v", ErrInvalidProfile, err)
	}
	switch fingerprint.Status {
	case "not_reported":
		if len(fingerprint.Features) != 0 {
			return FeatureFingerprint{}, nil, fmt.Errorf("%w: unreported fingerprint contains features", ErrInvalidProfile)
		}
	case "reported":
		if len(fingerprint.Features) == 0 {
			return FeatureFingerprint{}, nil, fmt.Errorf("%w: reported fingerprint is empty", ErrInvalidProfile)
		}
	default:
		return FeatureFingerprint{}, nil, fmt.Errorf("%w: feature fingerprint status %q", ErrInvalidProfile, fingerprint.Status)
	}
	features := append([]string(nil), fingerprint.Features...)
	sort.Strings(features)
	for index, feature := range features {
		if feature == "" || strings.TrimSpace(feature) != feature {
			return FeatureFingerprint{}, nil, fmt.Errorf("%w: invalid feature name", ErrInvalidProfile)
		}
		if index > 0 && features[index-1] == feature {
			return FeatureFingerprint{}, nil, fmt.Errorf("%w: duplicate feature %q", ErrInvalidProfile, feature)
		}
	}
	fingerprint.Features = features
	canonical, err := json.Marshal(fingerprint)
	if err != nil {
		return FeatureFingerprint{}, nil, fmt.Errorf("%w: encode feature fingerprint: %v", ErrInvalidProfile, err)
	}
	return fingerprint, canonical, nil
}

// MatchesOfficialLinuxPlain accepts only the reviewed normal release builds
// for the two supported Linux architectures. Binaries with missing, extra,
// or unreported build tags fail closed.
func MatchesOfficialLinuxPlain(profile CoreProfile, exactVersion string, expectedFeatures []string) bool {
	normalized, err := ValidateCoreProfile(profile)
	if err != nil || normalized.ExactVersion != exactVersion ||
		normalized.OperatingSystem != string(coreartifact.OperatingSystemLinux) ||
		(normalized.Architecture != string(coreartifact.ArchitectureARM64) &&
			normalized.Architecture != string(coreartifact.ArchitectureAMD64)) ||
		normalized.Variant != string(coreartifact.VariantPlain) {
		return false
	}
	fingerprint, _, err := DecodeFeatureFingerprint(normalized.FeatureFingerprint)
	if err != nil || fingerprint.Status != "reported" {
		return false
	}
	expected := append([]string(nil), expectedFeatures...)
	sort.Strings(expected)
	if len(expected) != len(fingerprint.Features) {
		return false
	}
	for index := range expected {
		if expected[index] != fingerprint.Features[index] {
			return false
		}
	}
	return true
}

func FinalizeProjection(config []byte, diagnostics []ProjectionDiagnostic) (ProjectionResult, error) {
	if !json.Valid(config) {
		return ProjectionResult{}, fmt.Errorf("%w: projected configuration is invalid JSON", ErrProjection)
	}
	stable := append([]ProjectionDiagnostic(nil), diagnostics...)
	sort.Slice(stable, func(left, right int) bool {
		if stable[left].Path != stable[right].Path {
			return stable[left].Path < stable[right].Path
		}
		if stable[left].Class != stable[right].Class {
			return stable[left].Class < stable[right].Class
		}
		return stable[left].Code < stable[right].Code
	})
	for _, diagnostic := range stable {
		if diagnostic.Class == DiagnosticBlocking {
			return ProjectionResult{}, fmt.Errorf("%w: %s at %s", ErrProjectionBlocked, diagnostic.Code, diagnostic.Path)
		}
		if diagnostic.Class != DiagnosticDirect && diagnostic.Class != DiagnosticMapped &&
			diagnostic.Class != DiagnosticIgnored {
			return ProjectionResult{}, fmt.Errorf("%w: invalid diagnostic class %q", ErrProjection, diagnostic.Class)
		}
		if diagnostic.Code == "" || diagnostic.Path == "" || diagnostic.Message == "" {
			return ProjectionResult{}, fmt.Errorf("%w: incomplete diagnostic", ErrProjection)
		}
	}
	ignored := make([]ProjectionDiagnostic, 0)
	for _, diagnostic := range stable {
		if diagnostic.Class == DiagnosticIgnored {
			ignored = append(ignored, diagnostic)
		}
	}
	ignoredDigest := ""
	if len(ignored) != 0 {
		encoded, err := json.Marshal(ignored)
		if err != nil {
			return ProjectionResult{}, fmt.Errorf("%w: encode ignored diagnostics: %v", ErrProjection, err)
		}
		digest := sha256.Sum256(encoded)
		ignoredDigest = hex.EncodeToString(digest[:])
	}
	compact := new(bytes.Buffer)
	if err := json.Compact(compact, config); err != nil {
		return ProjectionResult{}, fmt.Errorf("%w: compact projected configuration: %v", ErrProjection, err)
	}
	return ProjectionResult{
		ConfigJSON: append([]byte(nil), compact.Bytes()...), Diagnostics: stable, IgnoredDigest: ignoredDigest,
	}, nil
}

func RequireIgnoredAcceptance(result ProjectionResult, acceptedDigest string) error {
	acceptedDigest = strings.TrimSpace(acceptedDigest)
	if result.IgnoredDigest == "" {
		if acceptedDigest != "" {
			return fmt.Errorf("%w: no fields are ignored", ErrIgnoredNotAccepted)
		}
		return nil
	}
	if acceptedDigest != result.IgnoredDigest {
		return fmt.Errorf("%w: expected digest %s", ErrIgnoredNotAccepted, result.IgnoredDigest)
	}
	return nil
}
