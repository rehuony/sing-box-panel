// SPDX-License-Identifier: GPL-3.0-or-later

package releasegate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	releaseevidence "github.com/rehuony/sing-box-panel/release"
)

const (
	evidenceSchemaVersion  = 1
	maximumManifestBytes   = 256 << 10
	maximumEvidenceBytes   = 4 << 20
	maximumEvidenceRecords = 64
)

var (
	ErrGANotReady          = errors.New("GA release is not ready")
	ErrInvalidSourceCommit = errors.New("invalid release source commit")
	requiredEvidenceIDs    = []string{
		"core-version-matrix",
		"structured-capability-matrix",
		"linux-runtime-resilience",
		"browser-contract-accessibility",
		"subscription-observability-e2e",
	}
)

type EvidenceStatus struct {
	Document      string   `json:"document"`
	SchemaVersion int      `json:"schema_version"`
	SourceCommit  string   `json:"source_commit,omitempty"`
	GeneratedAt   string   `json:"generated_at,omitempty"`
	Required      []string `json:"required"`
	Verified      []string `json:"verified"`
	Missing       []string `json:"missing"`
	Invalid       []string `json:"invalid,omitempty"`
	ReasonCode    string   `json:"reason_code,omitempty"`
	Ready         bool     `json:"ready"`
}

type ReadinessStatus struct {
	Ready    bool           `json:"ready"`
	SQLite   SQLiteStatus   `json:"sqlite"`
	Evidence EvidenceStatus `json:"evidence"`
}

type evidenceManifest struct {
	SchemaVersion int              `json:"schema_version"`
	SourceCommit  string           `json:"source_commit"`
	GeneratedAt   string           `json:"generated_at"`
	Records       []evidenceRecord `json:"records"`
}

type evidenceRecord struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	VerifiedAt string `json:"verified_at"`
	VerifiedBy string `json:"verified_by"`
}

type auditEvidence struct {
	SchemaVersion int             `json:"schema_version"`
	RequirementID string          `json:"requirement_id"`
	SourceCommit  string          `json:"source_commit"`
	Result        string          `json:"result"`
	CompletedAt   string          `json:"completed_at"`
	Summary       string          `json:"summary"`
	Checks        []evidenceCheck `json:"checks"`
}

type evidenceCheck struct {
	Name     string `json:"name"`
	Result   string `json:"result"`
	Evidence string `json:"evidence"`
}

type evidenceReader func(string) ([]byte, error)

func Readiness() ReadinessStatus {
	sqliteStatus := SQLite()
	evidenceStatus := Evidence()
	return ReadinessStatus{
		Ready:  sqliteStatus.Ready && evidenceStatus.Ready,
		SQLite: sqliteStatus, Evidence: evidenceStatus,
	}
}

func Evidence() EvidenceStatus {
	manifest, err := releaseevidence.Manifest()
	if err != nil {
		return unavailableEvidenceStatus("manifest_unavailable")
	}
	return inspectEvidence(manifest, releaseevidence.Read)
}

func RequireGA() error {
	return requireGA(Readiness())
}

// RequireGAForSourceCommit applies the GA checks and additionally binds a
// formal build to the exact source commit recorded by the evidence ledger.
func RequireGAForSourceCommit(sourceCommit string) error {
	return requireGAForSourceCommit(Readiness(), sourceCommit)
}

// ValidateSourceCommit accepts only canonical full commit identifiers. The
// all-zero values are sentinels, not source identities.
func ValidateSourceCommit(value string) error {
	if !validCommit(value) {
		return fmt.Errorf("%w: expected a non-zero lowercase 40- or 64-character hexadecimal identifier", ErrInvalidSourceCommit)
	}
	return nil
}

func requireGA(status ReadinessStatus) error {
	return gaReadinessError(gaBlockers(status))
}

func requireGAForSourceCommit(status ReadinessStatus, sourceCommit string) error {
	if err := ValidateSourceCommit(sourceCommit); err != nil {
		return err
	}
	blockers := gaBlockers(status)
	if status.Evidence.SourceCommit != sourceCommit {
		blockers = append(blockers, fmt.Sprintf(
			"release evidence source commit %q does not match build source commit %q",
			status.Evidence.SourceCommit,
			sourceCommit,
		))
	}
	return gaReadinessError(blockers)
}

func gaBlockers(status ReadinessStatus) []string {
	blockers := make([]string, 0, 2)
	if !status.SQLite.Ready {
		blockers = append(blockers, fmt.Sprintf(
			"embedded SQLite %s is older than required %s",
			status.SQLite.Current,
			status.SQLite.Minimum,
		))
	}
	if !status.Evidence.Ready {
		reason := status.Evidence.ReasonCode
		if reason == "" {
			reason = "not_ready"
		}
		if len(status.Evidence.Missing) > 0 {
			reason += "; missing=" + strings.Join(status.Evidence.Missing, ",")
		}
		blockers = append(blockers, "release evidence "+reason)
	}
	return blockers
}

func gaReadinessError(blockers []string) error {
	if len(blockers) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrGANotReady, strings.Join(blockers, "; "))
}

func inspectEvidence(manifestJSON []byte, read evidenceReader) EvidenceStatus {
	status := unavailableEvidenceStatus("")
	var manifest evidenceManifest
	if err := jsonstrict.Decode(manifestJSON, maximumManifestBytes, &manifest); err != nil {
		status.ReasonCode = "manifest_invalid"
		status.Invalid = []string{"manifest"}
		return status
	}
	status.SchemaVersion = manifest.SchemaVersion
	status.SourceCommit = manifest.SourceCommit
	status.GeneratedAt = manifest.GeneratedAt
	status.Missing = status.Missing[:0]

	generatedAt, generatedAtOK := parseAuditTime(manifest.GeneratedAt)
	metadataValid := manifest.SchemaVersion == evidenceSchemaVersion &&
		validCommit(manifest.SourceCommit) && generatedAtOK &&
		len(manifest.Records) <= maximumEvidenceRecords
	if !metadataValid {
		status.Invalid = append(status.Invalid, "manifest_metadata")
	}

	required := make(map[string]struct{}, len(requiredEvidenceIDs))
	for _, id := range requiredEvidenceIDs {
		required[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(manifest.Records))
	verified := make(map[string]struct{}, len(manifest.Records))
	for index, record := range manifest.Records {
		identifier := record.ID
		if _, known := required[identifier]; !known {
			status.Invalid = append(status.Invalid, fmt.Sprintf("record_%d", index+1))
			continue
		}
		if _, duplicate := seen[identifier]; duplicate {
			status.Invalid = append(status.Invalid, identifier)
			continue
		}
		seen[identifier] = struct{}{}
		if !metadataValid || validateEvidenceRecord(record, manifest, generatedAt, read) != nil {
			status.Invalid = append(status.Invalid, identifier)
			continue
		}
		verified[identifier] = struct{}{}
	}

	for _, id := range requiredEvidenceIDs {
		if _, ok := verified[id]; ok {
			status.Verified = append(status.Verified, id)
		} else {
			status.Missing = append(status.Missing, id)
		}
	}
	sort.Strings(status.Invalid)
	status.Ready = metadataValid && len(status.Invalid) == 0 && len(status.Missing) == 0
	switch {
	case status.Ready:
		status.ReasonCode = ""
	case !metadataValid:
		status.ReasonCode = "manifest_incomplete"
	case len(status.Invalid) > 0:
		status.ReasonCode = "evidence_invalid"
	default:
		status.ReasonCode = "required_evidence_missing"
	}
	return status
}

func validateEvidenceRecord(
	record evidenceRecord,
	manifest evidenceManifest,
	generatedAt time.Time,
	read evidenceReader,
) error {
	if record.Path != "evidence/"+record.ID+".json" || !validSHA256(record.SHA256) ||
		!validAuditText(record.VerifiedBy, 256) {
		return errors.New("invalid evidence record identity")
	}
	verifiedAt, ok := parseAuditTime(record.VerifiedAt)
	if !ok || verifiedAt.After(generatedAt) {
		return errors.New("invalid evidence review time")
	}
	documentJSON, err := read(record.Path)
	if err != nil || len(documentJSON) == 0 || len(documentJSON) > maximumEvidenceBytes {
		return errors.New("evidence document unavailable")
	}
	digest := sha256.Sum256(documentJSON)
	if hex.EncodeToString(digest[:]) != record.SHA256 {
		return errors.New("evidence digest mismatch")
	}
	var document auditEvidence
	if err := jsonstrict.Decode(documentJSON, maximumEvidenceBytes, &document); err != nil {
		return errors.New("invalid evidence document")
	}
	completedAt, ok := parseAuditTime(document.CompletedAt)
	if document.SchemaVersion != evidenceSchemaVersion || document.RequirementID != record.ID ||
		document.SourceCommit != manifest.SourceCommit || document.Result != "pass" || !ok ||
		completedAt.After(verifiedAt) || !validAuditText(document.Summary, 2<<10) ||
		len(document.Checks) == 0 || len(document.Checks) > 256 {
		return errors.New("incomplete evidence document")
	}
	seenChecks := make(map[string]struct{}, len(document.Checks))
	for _, check := range document.Checks {
		if !validAuditText(check.Name, 256) || check.Result != "pass" ||
			!validAuditText(check.Evidence, 4<<10) {
			return errors.New("invalid evidence check")
		}
		if _, duplicate := seenChecks[check.Name]; duplicate {
			return errors.New("duplicate evidence check")
		}
		seenChecks[check.Name] = struct{}{}
	}
	return nil
}

func unavailableEvidenceStatus(reason string) EvidenceStatus {
	return EvidenceStatus{
		Document:   "release/evidence.json",
		Required:   append([]string(nil), requiredEvidenceIDs...),
		Verified:   make([]string, 0),
		Missing:    append([]string(nil), requiredEvidenceIDs...),
		ReasonCode: reason,
	}
}

func validCommit(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	allZero := true
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
		if character != '0' {
			allZero = false
		}
	}
	return !allZero
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(decoded) == value
}

func parseAuditTime(value string) (time.Time, bool) {
	parsed, err := time.Parse(time.RFC3339, value)
	return parsed, err == nil && !parsed.IsZero()
}

func validAuditText(value string, maximum int) bool {
	if value == "" || len(value) > maximum || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}
