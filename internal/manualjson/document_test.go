// SPDX-License-Identifier: GPL-3.0-or-later

package manualjson

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func testBinding(t *testing.T) Binding {
	t.Helper()
	version, err := coreartifact.ParseExactVersion("1.13.19")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := coreartifact.ParseSHA256(strings.Repeat("1", 64))
	if err != nil {
		t.Fatal(err)
	}
	return Binding{CoreVersion: version, ArtifactDigest: digest, BaseRevisionID: "rev_base"}
}

func TestParsePreservesExactJSONCBytes(t *testing.T) {
	raw := []byte("// exact comment\n{\n  \"log\": {\"level\": \"warn\",},\n}\n")
	document, err := Parse(raw, testBinding(t))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !bytes.Equal(document.RawBytes(), raw) {
		t.Fatalf("RawBytes() changed exact JSONC:\n%s", document.RawBytes())
	}
	if bytes.Contains(document.StandardJSON(), []byte("comment")) || !bytes.Contains(document.StandardJSON(), []byte(`"log"`)) {
		t.Fatalf("StandardJSON() = %s", document.StandardJSON())
	}
	clone := document.RawBytes()
	clone[0] = 'x'
	if bytes.Equal(clone, document.RawBytes()) {
		t.Fatal("RawBytes() exposed document-owned storage")
	}
}

func TestParseRejectsDuplicateKeysAndInvalidBinding(t *testing.T) {
	_, err := Parse([]byte(`{"log":{},"log":{}}`), testBinding(t))
	if !errors.Is(err, ErrInvalidManualJSON) || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate Parse() error = %v", err)
	}
	_, err = Parse([]byte(`{}`), Binding{})
	if !errors.Is(err, ErrInvalidManualJSON) {
		t.Fatalf("empty binding Parse() error = %v", err)
	}
}

func TestParseRejectsExcessiveNestingBeforeASTParse(t *testing.T) {
	raw := []byte(strings.Repeat("[", maximumDepth+1) + strings.Repeat("]", maximumDepth+1))
	_, err := Parse(raw, testBinding(t))
	if !errors.Is(err, ErrInvalidManualJSON) || !strings.Contains(err.Error(), "nesting") {
		t.Fatalf("Parse() error = %v", err)
	}
}
