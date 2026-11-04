// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/json"
	"testing"
)

func TestChannelConfigRejectsRemovedExportKeyBindings(t *testing.T) {
	for _, config := range []string{`{"export_token_ids":["token-first"]}`, `{"export_token_ids":[]}`, `{"export_token_ids":null}`} {
		if _, err := DecodeChannelConfig(json.RawMessage(config)); err == nil {
			t.Fatalf("removed field accepted: %s", config)
		}
	}
}

func TestChannelConfigRejectsInvalidExclusions(t *testing.T) {
	for name, config := range map[string]string{
		"unknown field":   `{"unknown":true}`,
		"duplicate key":   `{"exclude_tags":[],"exclude_tags":[]}`,
		"duplicate value": `{"exclude_tags":["same","same"]}`,
		"invalid type":    `{"exclude_types":["VMess"]}`,
		"null root":       `null`,
		"null list":       `{"exclude_tags":null}`,
	} {
		if _, err := CanonicalChannelConfig(json.RawMessage(config), RenderFormatSingBox); err == nil {
			t.Errorf("%s: invalid config %s was accepted", name, config)
		}
	}
	canonical, err := CanonicalChannelConfig(json.RawMessage(` { "exclude_tags": ["private"] } `), RenderFormatLoon)
	if err != nil || string(canonical) != `{"exclude_tags":["private"]}` {
		t.Fatalf("canonical config = %s, %v", canonical, err)
	}
}
