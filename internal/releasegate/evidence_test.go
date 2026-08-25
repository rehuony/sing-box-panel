// SPDX-License-Identifier: GPL-3.0-or-later

package releasegate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedEvidenceIsExplicitlyNotReady(t *testing.T) {
	t.Parallel()
	status := Evidence()
	if status.Ready || status.Document != "release/evidence.json" ||
		status.ReasonCode != "manifest_incomplete" || len(status.Missing) != len(requiredEvidenceIDs) {
		t.Fatalf("Evidence() = %+v", status)
	}
	readiness := Readiness()
	if readiness.Ready || readiness.Evidence.Ready {
		t.Fatalf("Readiness() = %+v", readiness)
	}
}

func TestInspectEvidenceRequiresCompleteDigestPinnedLedger(t *testing.T) {
	t.Parallel()
	manifest, documents := completeEvidenceFixture(t)
	read := func(path string) ([]byte, error) {
		document, ok := documents[path]
		if !ok {
			return nil, errors.New("not found")
		}
		return append([]byte(nil), document...), nil
	}
	status := inspectEvidence(manifest, read)
	if !status.Ready || len(status.Missing) != 0 || len(status.Invalid) != 0 ||
		len(status.Verified) != len(requiredEvidenceIDs) {
		t.Fatalf("complete evidence status = %+v", status)
	}

	var decoded evidenceManifest
	if err := json.Unmarshal(manifest, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.Records[0].SHA256 = strings.Repeat("0", 64)
	tamperedManifest, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	tampered := inspectEvidence(tamperedManifest, read)
	if tampered.Ready || tampered.ReasonCode != "evidence_invalid" ||
		len(tampered.Invalid) == 0 || len(tampered.Missing) != 1 {
		t.Fatalf("tampered evidence status = %+v", tampered)
	}

	missingManifest := decoded
	missingManifest.Records = append([]evidenceRecord(nil), decoded.Records[1:]...)
	missingManifestJSON, err := json.Marshal(missingManifest)
	if err != nil {
		t.Fatal(err)
	}
	missing := inspectEvidence(missingManifestJSON, read)
	if missing.Ready || missing.ReasonCode != "required_evidence_missing" || len(missing.Missing) != 1 {
		t.Fatalf("missing evidence status = %+v", missing)
	}
}

func TestInspectEvidenceRejectsAmbiguousOrIncompleteManifest(t *testing.T) {
	t.Parallel()
	duplicate := []byte(`{"schema_version":1,"schema_version":1,"source_commit":"","generated_at":"","records":[]}`)
	status := inspectEvidence(duplicate, func(string) ([]byte, error) { return nil, errors.New("unused") })
	if status.Ready || status.ReasonCode != "manifest_invalid" {
		t.Fatalf("duplicate-key manifest status = %+v", status)
	}

	incomplete := []byte(`{"schema_version":1,"source_commit":"","generated_at":"","records":[]}`)
	status = inspectEvidence(incomplete, func(string) ([]byte, error) { return nil, errors.New("unused") })
	if status.Ready || status.ReasonCode != "manifest_incomplete" || len(status.Missing) != len(requiredEvidenceIDs) {
		t.Fatalf("incomplete manifest status = %+v", status)
	}
}

func TestRequireGARequiresSQLiteAndEvidence(t *testing.T) {
	t.Parallel()
	readySQLite := SQLiteStatus{Current: MinimumSQLiteVersion, Minimum: MinimumSQLiteVersion, Ready: true}
	blockedSQLite := SQLiteStatus{Current: "3.53.3", Minimum: MinimumSQLiteVersion, Ready: false}
	readyEvidence := EvidenceStatus{Ready: true}
	blockedEvidence := EvidenceStatus{ReasonCode: "required_evidence_missing", Missing: []string{"core-version-matrix"}}

	tests := []struct {
		name     string
		status   ReadinessStatus
		wantPass bool
	}{
		{name: "both ready", status: ReadinessStatus{SQLite: readySQLite, Evidence: readyEvidence}, wantPass: true},
		{name: "SQLite blocked", status: ReadinessStatus{SQLite: blockedSQLite, Evidence: readyEvidence}},
		{name: "evidence blocked", status: ReadinessStatus{SQLite: readySQLite, Evidence: blockedEvidence}},
		{name: "both blocked", status: ReadinessStatus{SQLite: blockedSQLite, Evidence: blockedEvidence}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := requireGA(test.status)
			if test.wantPass && err != nil {
				t.Fatalf("requireGA(): %v", err)
			}
			if !test.wantPass && !errors.Is(err, ErrGANotReady) {
				t.Fatalf("requireGA() error = %v, want ErrGANotReady", err)
			}
		})
	}
}

func TestValidateSourceCommit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "sha1", value: strings.Repeat("a", 40), valid: true},
		{name: "sha256", value: strings.Repeat("b", 64), valid: true},
		{name: "empty"},
		{name: "short", value: strings.Repeat("a", 39)},
		{name: "long", value: strings.Repeat("a", 65)},
		{name: "uppercase", value: strings.Repeat("A", 40)},
		{name: "non hexadecimal", value: strings.Repeat("g", 40)},
		{name: "all-zero sha1", value: strings.Repeat("0", 40)},
		{name: "all-zero sha256", value: strings.Repeat("0", 64)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSourceCommit(test.value)
			if test.valid && err != nil {
				t.Fatalf("ValidateSourceCommit(%q): %v", test.value, err)
			}
			if !test.valid && !errors.Is(err, ErrInvalidSourceCommit) {
				t.Fatalf("ValidateSourceCommit(%q) error = %v, want ErrInvalidSourceCommit", test.value, err)
			}
		})
	}
}

func TestRequireGAForSourceCommitBindsEvidenceToBuild(t *testing.T) {
	t.Parallel()
	ledgerCommit := strings.Repeat("a", 40)
	ready := ReadinessStatus{
		Ready:  true,
		SQLite: SQLiteStatus{Current: MinimumSQLiteVersion, Minimum: MinimumSQLiteVersion, Ready: true},
		Evidence: EvidenceStatus{
			Ready:        true,
			SourceCommit: ledgerCommit,
		},
	}
	if err := requireGAForSourceCommit(ready, ledgerCommit); err != nil {
		t.Fatalf("requireGAForSourceCommit() with matching commit: %v", err)
	}

	mismatched := strings.Repeat("b", 40)
	err := requireGAForSourceCommit(ready, mismatched)
	if !errors.Is(err, ErrGANotReady) || !strings.Contains(err.Error(), "does not match build source commit") {
		t.Fatalf("mismatched source commit error = %v, want ErrGANotReady mismatch", err)
	}

	err = requireGAForSourceCommit(ready, strings.Repeat("0", 40))
	if !errors.Is(err, ErrInvalidSourceCommit) {
		t.Fatalf("invalid source commit error = %v, want ErrInvalidSourceCommit", err)
	}
}

func completeEvidenceFixture(t *testing.T) ([]byte, map[string][]byte) {
	t.Helper()
	commit := strings.Repeat("a", 40)
	completedAt := time.Date(2026, time.August, 25, 10, 0, 0, 0, time.UTC)
	verifiedAt := completedAt.Add(time.Hour)
	generatedAt := verifiedAt.Add(time.Hour)
	documents := make(map[string][]byte, len(requiredEvidenceIDs))
	records := make([]evidenceRecord, 0, len(requiredEvidenceIDs))
	for _, id := range requiredEvidenceIDs {
		document, err := json.Marshal(auditEvidence{
			SchemaVersion: evidenceSchemaVersion,
			RequirementID: id,
			SourceCommit:  commit,
			Result:        "pass",
			CompletedAt:   completedAt.Format(time.RFC3339),
			Summary:       "reviewed acceptance evidence",
			Checks: []evidenceCheck{{
				Name: "acceptance", Result: "pass", Evidence: "artifact digest and command transcript reviewed",
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		path := fmt.Sprintf("evidence/%s.json", id)
		documents[path] = document
		digest := sha256.Sum256(document)
		records = append(records, evidenceRecord{
			ID: id, Path: path, SHA256: hex.EncodeToString(digest[:]),
			VerifiedAt: verifiedAt.Format(time.RFC3339), VerifiedBy: "release-reviewers",
		})
	}
	manifest, err := json.Marshal(evidenceManifest{
		SchemaVersion: evidenceSchemaVersion,
		SourceCommit:  commit,
		GeneratedAt:   generatedAt.Format(time.RFC3339),
		Records:       records,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manifest, documents
}
