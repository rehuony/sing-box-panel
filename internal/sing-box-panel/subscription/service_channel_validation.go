// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/schema"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Modern sing-box channels use the reviewed native 1.14 schema, independently
// of the server's selected core. The subscriber still owns runtime validation.
const channelSingBoxSchemaVersion = "1.14.0"

func validateSubscriptionChannelConfig(format store.SubscriptionFormat, raw json.RawMessage) (json.RawMessage, error) {
	config, err := DecodeChannelConfig(raw)
	if err != nil {
		return nil, err
	}
	if err = ValidateChannelPolicy(config.Policy, RenderFormat(format)); err != nil {
		return nil, err
	}
	if config.Policy != nil && config.Policy.Template != nil && format == store.SubscriptionFormatSingBox {
		if err = validateSubscriptionNativeJSON([]byte(config.Policy.Template.Content), "policy.template.content"); err != nil {
			return nil, err
		}
	}
	return CanonicalChannelConfig(raw, RenderFormat(format))
}

func validateSubscriptionNativeJSON(raw []byte, path string) error {
	err := schema.ValidateConfiguration(channelSingBoxSchemaVersion, raw)
	if err == nil {
		return nil
	}
	var issue *jsonschema.ValidationError
	if errors.As(err, &issue) {
		for len(issue.Causes) > 0 {
			issue = issue.Causes[0]
		}
		for _, part := range issue.InstanceLocation {
			// Return field locations, never invalid values or the validator's message,
			// which may include passwords, certificate material or complete documents.
			if len(part) > 64 || strings.ContainsAny(part, "\r\n\x00") {
				part = "*"
			}
			path += "/" + strings.ReplaceAll(strings.ReplaceAll(part, "~", "~0"), "/", "~1")
		}
	}
	return &PolicyError{Path: path, Code: "native_schema_violation"}
}
