// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// applyConfigurationSchemaPresentationOverlay adds only panel-owned x-panel
// annotations. It never adds configuration properties, branches, required
// fields, or other keywords that could change the upstream constraints.
func applyConfigurationSchemaPresentationOverlay(raw []byte) ([]byte, error) {
	var schema map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("schema contains multiple JSON values")
		}
		return nil, err
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, errors.New("upstream root schema has no properties object")
	}
	sections := []struct {
		name       string
		section    string
		order      int
		collection bool
		label      map[string]any
	}{
		{name: "log", section: "general", order: 10, label: bilingualLabel("Logging", "日志")},
		{name: "dns", section: "general", order: 20, label: bilingualLabel("DNS", "DNS")},
		{name: "ntp", section: "general", order: 30, label: bilingualLabel("NTP", "NTP")},
		{name: "certificate", section: "general", order: 35, label: bilingualLabel("Certificates", "证书")},
		{name: "route", section: "general", order: 40, label: bilingualLabel("Routing", "路由")},
		{name: "experimental", section: "general", order: 50, label: bilingualLabel("Experimental", "实验性功能")},
		{name: "endpoints", section: "managed", order: 10, collection: true, label: bilingualLabel("Endpoints", "端点")},
		{name: "inbounds", section: "managed", order: 20, collection: true, label: bilingualLabel("Inbounds", "入站")},
		{name: "outbounds", section: "managed", order: 30, collection: true, label: bilingualLabel("Outbounds", "出站")},
		{name: "services", section: "managed", order: 40, collection: true, label: bilingualLabel("Services", "服务")},
	}
	for _, section := range sections {
		node, exists := properties[section.name].(map[string]any)
		if !exists {
			continue
		}
		metadata := map[string]any{
			"section": section.section,
			"order":   section.order,
			"label":   section.label,
		}
		if section.collection {
			metadata["collection"] = section.name
			metadata["identity-field"] = "tag"
			metadata["display-field"] = "tag"
		}
		if err := addPanelAnnotation(node, metadata); err != nil {
			return nil, fmt.Errorf("section %s: %w", section.name, err)
		}
	}
	if err := addWhitelistedFieldOverlays(schema); err != nil {
		return nil, err
	}
	if err := addSensitiveFieldOverlays(schema); err != nil {
		return nil, err
	}
	if err := addPanelAnnotation(schema, map[string]any{
		"section": "configuration",
		"order":   0,
		"label":   bilingualLabel("Configuration", "配置"),
	}); err != nil {
		return nil, fmt.Errorf("root: %w", err)
	}
	return json.Marshal(schema)
}

func addPanelAnnotation(schema map[string]any, metadata map[string]any) error {
	if _, exists := schema["x-panel"]; exists {
		return errors.New("upstream schema already defines x-panel")
	}
	schema["x-panel"] = metadata
	return nil
}

type whitelistedFieldOverlay struct {
	definition string
	property   string
	order      int
	english    string
	chinese    string
}

var whitelistedFieldOverlays = []whitelistedFieldOverlay{
	{definition: "LogOptions", property: "level", order: 20, english: "Log level", chinese: "日志级别"},
}

func addWhitelistedFieldOverlays(schema map[string]any) error {
	definitions, ok := schema["$defs"].(map[string]any)
	if !ok {
		return nil
	}
	for _, overlay := range whitelistedFieldOverlays {
		definition, ok := definitions[overlay.definition].(map[string]any)
		if !ok {
			continue
		}
		properties, ok := definition["properties"].(map[string]any)
		if !ok {
			return fmt.Errorf("whitelisted definition %s has no properties object", overlay.definition)
		}
		property, ok := properties[overlay.property].(map[string]any)
		if !ok {
			continue
		}
		if err := addPanelAnnotation(property, map[string]any{
			"order": overlay.order,
			"label": bilingualLabel(overlay.english, overlay.chinese),
		}); err != nil {
			return fmt.Errorf("whitelisted property %s.%s: %w", overlay.definition, overlay.property, err)
		}
	}
	return nil
}

var sensitiveFieldNames = map[string]struct{}{
	"access_key_secret":      {},
	"access_token":           {},
	"api_key":                {},
	"api_token":              {},
	"auth_key":               {},
	"auth_str":               {},
	"auth_token":             {},
	"client_key":             {},
	"client_secret":          {},
	"key":                    {},
	"mac_key":                {},
	"password":               {},
	"pre_shared_key":         {},
	"private_key":            {},
	"private_key_passphrase": {},
	"secret":                 {},
	"security_token":         {},
	"token":                  {},
	"uuid":                   {},
	"zone_token":             {},
}

func addSensitiveFieldOverlays(value any) error {
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			if err := addSensitiveFieldOverlays(child); err != nil {
				return err
			}
		}
	case map[string]any:
		if properties, ok := typed["properties"].(map[string]any); ok {
			for name, property := range properties {
				if _, sensitive := sensitiveFieldNames[name]; !sensitive {
					continue
				}
				propertySchema, ok := property.(map[string]any)
				if !ok {
					return fmt.Errorf("sensitive property %s is not a schema object", name)
				}
				if err := addPanelAnnotation(propertySchema, map[string]any{"sensitive": true, "widget": "password"}); err != nil {
					return fmt.Errorf("sensitive property %s: %w", name, err)
				}
			}
		}
		for key, child := range typed {
			if key == "x-panel" {
				continue
			}
			if err := addSensitiveFieldOverlays(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func bilingualLabel(english string, simplifiedChinese string) map[string]any {
	return map[string]any{"en": english, "zh-CN": simplifiedChinese}
}
