// SPDX-License-Identifier: GPL-3.0-or-later

package schema

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/nodeapi"
)

// TestReviewed113ContractMatchesTheWebFixture keeps the Web application's
// 1.13 test fixture the schema the panel serves.
func TestReviewed113ContractMatchesTheWebFixture(t *testing.T) {
	contract, err := Reviewed113()
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "apps", "panel", "src", "tests", "fixtures", "configuration-schema-1.13.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contract.Schema(), fixture) {
		t.Fatal("the reviewed 1.13 contract differs from the Web fixture")
	}
}

func TestPrepareCanonicalizesOverlaysAndBoundsTheSchema(t *testing.T) {
	raw := []byte(`{ "type": "object", "properties": { "log": {"$ref": "#/$defs/LogOptions"} },
		"$defs": { "LogOptions": { "type": "object", "properties": { "level": {"enum": ["warn", "error"]}, "password": {"type": "string"} } } },
		"maximum": 1.50 }`)
	contract, err := Prepare(raw)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(contract.Schema())
	if contract.SHA256() != hex.EncodeToString(digest[:]) {
		t.Fatal("the contract's digest is not the digest of its schema")
	}
	var prepared struct {
		Panel   map[string]any `json:"x-panel"`
		Maximum json.Number    `json:"maximum"`
		Defs    struct {
			LogOptions struct {
				Properties map[string]struct {
					Panel map[string]any `json:"x-panel"`
				} `json:"properties"`
			} `json:"LogOptions"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(contract.Schema(), &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.Panel["section"] != "configuration" || prepared.Maximum != "1.50" ||
		prepared.Defs.LogOptions.Properties["level"].Panel == nil || prepared.Defs.LogOptions.Properties["password"].Panel["sensitive"] != true {
		t.Fatalf("prepared schema = %s", contract.Schema())
	}
	again, err := Prepare(bytes.ReplaceAll(raw, []byte(" "), nil))
	if err != nil || again.SHA256() != contract.SHA256() {
		t.Fatalf("a reformatted schema prepared to %v, %v; want the same digest", again, err)
	}
	if err := contract.Validate([]byte(`{"log":{"level":"warn"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := contract.Validate([]byte(`{"log":{"level":"verbose"}}`)); err == nil {
		t.Fatal("an invalid configuration passed the schema")
	}

	for name, raw := range map[string][]byte{
		"too large":          append(append([]byte(`{"properties":{},"$comment":"`), bytes.Repeat([]byte("a"), nodeapi.MaximumCoreSchemaBytes)...), `"}`...),
		"external reference": []byte(`{"properties":{"log":{"$ref":"https://example.com/log.json"}}}`),
		"not an object":      []byte(`[]`),
		"several values":     []byte(`{"properties":{}} {}`),
		"existing overlay":   []byte(`{"properties":{},"x-panel":{}}`),
	} {
		if _, err := Prepare(raw); err == nil {
			t.Errorf("Prepare accepted a schema that is %s", name)
		} else if name == "external reference" && !strings.Contains(err.Error(), "external schema reference") {
			t.Errorf("Prepare(%s) error = %v", name, err)
		}
	}
}
