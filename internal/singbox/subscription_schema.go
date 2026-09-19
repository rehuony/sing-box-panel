// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"encoding/json"
	"sync"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Subscription nodes target a client, not the selected server executable.
// Check the known native field types without removing or rejecting extension
// fields. Exact client compatibility is checked again when rendering a channel.
var subscriptionSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	contract, err := ConfigurationSchema("1.14.0")
	if err != nil {
		return nil, err
	}
	var schema any
	if err := decodeJSONPreservingNumbers(contract.Schema, &schema); err != nil {
		return nil, err
	}
	allowSchemaExtensions(schema)
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	return compileSchema("subscription-node-1.14.0", raw)
})

func ValidateSubscriptionNodeSchema(raw []byte) error {
	validator, err := subscriptionSchema()
	if err != nil {
		return err
	}
	var value any
	if err := decodeJSONPreservingNumbers(raw, &value); err != nil {
		return err
	}
	return validator.Validate(map[string]any{"outbounds": []any{value}})
}

func allowSchemaExtensions(value any) {
	switch value := value.(type) {
	case []any:
		for _, child := range value {
			allowSchemaExtensions(child)
		}
	case map[string]any:
		for key, child := range value {
			if (key == "additionalProperties" || key == "unevaluatedProperties") && child == false {
				value[key] = true
			} else {
				allowSchemaExtensions(child)
			}
		}
	}
}
