// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"fmt"
	"maps"
)

// singBoxSourceParser reads a sing-box configuration's outbounds and
// endpoints, an outbound array, or one outbound. Outbounds that route rather
// than connect, such as direct, block, dns, selector and urltest, are not
// nodes.
type singBoxSourceParser struct{}

func (singBoxSourceParser) Format() SourceFormat { return SourceFormatSingBox }

func (singBoxSourceParser) Sniff(body []byte) int {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return 0
	}
	switch trimmed[0] {
	case '{':
		if bytes.Contains(trimmed, []byte(`"outbounds"`)) || bytes.Contains(trimmed, []byte(`"endpoints"`)) {
			return 100
		}
		if bytes.Contains(trimmed, []byte(`"server"`)) && bytes.Contains(trimmed, []byte(`"type"`)) {
			return 60
		}
	case '[':
		if bytes.Contains(trimmed, []byte(`"type"`)) {
			return 70
		}
	}
	return 0
}

func (singBoxSourceParser) Items(body []byte) ([]SourceItem, error) {
	value, err := DecodeDocument(body)
	if err != nil {
		return nil, contentError("", "the JSON document is invalid: "+DocumentErrorCode(err))
	}
	switch root := value.(type) {
	case []any:
		return singBoxItems(root, "", CollectionOutbounds)
	case map[string]any:
		_, hasOutbounds := root["outbounds"]
		_, hasEndpoints := root["endpoints"]
		if !hasOutbounds && !hasEndpoints {
			if _, single := root["type"]; !single {
				return nil, contentError("", "the configuration has no outbounds or endpoints")
			}
			return singBoxItems([]any{root}, "outbound", CollectionOutbounds)
		}
		items := []SourceItem{}
		for _, collection := range []Collection{CollectionOutbounds, CollectionEndpoints} {
			raw, exists := root[string(collection)]
			if !exists {
				continue
			}
			values, ok := raw.([]any)
			if !ok {
				return nil, contentError(string(collection), "not an array")
			}
			collected, err := singBoxItems(values, string(collection), collection)
			if err != nil {
				return nil, err
			}
			items = append(items, collected...)
		}
		return items, nil
	}
	return nil, contentError("", "the document is not an object or array")
}

// singBoxItems reads values; prefix names their array in locations, or is
// the whole location of a single outbound.
func singBoxItems(values []any, prefix string, collection Collection) ([]SourceItem, error) {
	items := make([]SourceItem, 0, len(values))
	for index, raw := range values {
		location := fmt.Sprintf("%s[%d]", prefix, index)
		if prefix == "outbound" {
			location = prefix
		}
		object, ok := raw.(map[string]any)
		if !ok {
			return nil, contentError(location, "not an object")
		}
		typeID, ok := object["type"].(string)
		if !ok || !ValidType(typeID) {
			return nil, contentError(location, "the type is missing or invalid")
		}
		if collection == CollectionEndpoints && !publishableEndpoint(typeID) || collection == CollectionOutbounds && !publishableOutbound(typeID) {
			continue
		}
		name, _ := object["tag"].(string)
		if name == "" {
			server, _ := object["server"].(string)
			port, _ := DocumentInteger(object["server_port"], 1, 65535)
			name = unnamedNode(typeID, server, port)
		}
		items = append(items, SourceItem{Location: location, Name: name, Protocol: typeID, convert: func() (map[string]any, error) {
			return maps.Clone(object), nil
		}})
	}
	return items, nil
}
