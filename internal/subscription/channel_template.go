// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	apiassets "github.com/rehuony/sing-box-panel/api"
	"github.com/rehuony/sing-box-panel/internal/configuration"

	"go.yaml.in/yaml/v3"
)

type channelTemplate struct {
	json map[string]any
	yaml *yaml.Node
	loon string
}

func parseChannelTemplate(template *NativeTemplate, format RenderFormat) (channelTemplate, error) {
	content := apiassets.SingBoxTemplate
	if format == RenderFormatLoon {
		content = apiassets.LoonTemplate
	} else if format == RenderFormatMihomo {
		content = apiassets.MihomoTemplate
	}
	if template != nil {
		if template.Format != format {
			return channelTemplate{}, policyError("policy.template.format", "incompatible_format")
		}
		content = template.Content
	}
	if len(content) > 256<<10 {
		return channelTemplate{}, policyError("policy.template.content", "too_large")
	}
	if format == RenderFormatLoon {
		return parseLoonChannelTemplate(content)
	}
	if format == RenderFormatSingBox {
		root, err := DecodeDocumentObject([]byte(content))
		if err != nil {
			var parsed any
			var syntax *json.SyntaxError
			if parseErr := json.Unmarshal([]byte(content), &parsed); errors.As(parseErr, &syntax) {
				prefix := content[:min(int(syntax.Offset)-1, len(content))]
				line := strings.Count(prefix, "\n") + 1
				column := len(prefix) - strings.LastIndex(prefix, "\n")
				return channelTemplate{}, policyError(fmt.Sprintf("policy.template.content@%d:%d", line, column), "invalid_json_object")
			}
			return channelTemplate{}, policyError("policy.template.content", "invalid_json_object")
		}
		for _, name := range []string{"outbounds", "endpoints"} {
			if _, present := root[name]; present {
				return channelTemplate{}, policyError("policy.template.content/"+name, "generated_field")
			}
		}
		if raw, exists := root["route"]; exists {
			route, ok := raw.(map[string]any)
			if !ok {
				return channelTemplate{}, policyError("policy.template.content/route", "expected_object")
			}
			for _, name := range []string{"rules", "rule_set", "final"} {
				if _, present := route[name]; present {
					return channelTemplate{}, policyError("policy.template.content/route/"+name, "generated_field")
				}
			}
		}
		return channelTemplate{json: root}, nil
	}
	decoder := yaml.NewDecoder(bytes.NewBufferString(content))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return channelTemplate{}, policyError("policy.template.content", "invalid_yaml_object")
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return channelTemplate{}, policyError("policy.template.content", "multiple_or_invalid_yaml_documents")
	}
	count := 0
	if err := inspectSourceYAML(&document, 0, &count); err != nil {
		return channelTemplate{}, policyError("policy.template.content", "invalid_yaml_structure")
	}
	root := document.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		key := root.Content[i]
		if oneOf(key.Value, "proxies", "proxy-groups", "proxy-providers", "rules", "rule-providers", "sub-rules") {
			return channelTemplate{}, policyError(fmt.Sprintf("policy.template.content/%s@%d:%d", key.Value, key.Line, key.Column), "generated_field")
		}
	}
	return channelTemplate{yaml: &document}, nil
}

func (template channelTemplate) render(format RenderFormat, generated map[string]any) ([]byte, error) {
	if format == RenderFormatSingBox {
		for key, value := range generated {
			if key == "route" {
				route, ok := template.json["route"].(map[string]any)
				if !ok {
					route = map[string]any{}
				}
				for name, entry := range value.(map[string]any) {
					route[name] = entry
				}
				template.json[key] = route
			} else {
				template.json[key] = value
			}
		}
		raw, err := json.Marshal(template.json)
		if err != nil {
			return nil, err
		}
		content, err := configuration.FormatJSON(raw)
		return append(content, '\n'), err
	}
	root := template.yaml.Content[0]
	// Stable key ordering makes preview and delivery byte-identical; existing
	// YAML nodes, comments, numeric lexemes and unrelated fields stay intact.
	for _, key := range []string{"proxies", "proxy-groups", "rule-providers", "rules"} {
		value, ok := generated[key]
		if !ok {
			continue
		}
		var node yaml.Node
		if err := node.Encode(value); err != nil {
			return nil, err
		}
		root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &node)
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	formatTemplateCollections(template.yaml)
	if err := encoder.Encode(template.yaml); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// Change collection layout without decoding scalar values or discarding comments.
// In particular, the default {} template must not force the entire output inline.
func formatTemplateCollections(node *yaml.Node) {
	if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
		node.Style &^= yaml.FlowStyle
	}
	for _, child := range node.Content {
		formatTemplateCollections(child)
	}
}
