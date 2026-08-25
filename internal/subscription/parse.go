// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

type outbound struct {
	collection Collection
	index      int
	tag        string
	typeID     string
	value      map[string]any
}

type startupNodes struct {
	values           []outbound
	endpointsPresent bool
}

type filter struct {
	tags  map[string]struct{}
	types map[string]struct{}
}

var nonPublishableTypes = map[string]struct{}{
	"block":    {},
	"bridge":   {},
	"direct":   {},
	"dns":      {},
	"selector": {},
	"tor":      {},
	"url-test": {},
	"urltest":  {},
}

func parseStartup(data []byte, format Format) (startupNodes, []Diagnostic, error) {
	if len(data) == 0 {
		return startupNodes{}, nil, invalidStartup("empty_document")
	}
	if len(data) > MaximumStartupBytes {
		return startupNodes{}, nil, invalidStartup("document_too_large")
	}
	if !utf8.Valid(data) {
		return startupNodes{}, nil, invalidStartup("invalid_utf8")
	}
	if err := inspectDocument(data); err != nil {
		return startupNodes{}, nil, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return startupNodes{}, nil, invalidStartup("malformed_json")
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return startupNodes{}, nil, invalidStartup("root_not_object")
	}
	outbounds, outboundDiagnostics, err := extractCollection(root, CollectionOutbounds, format, publishableOutbound)
	if err != nil {
		return startupNodes{}, nil, err
	}
	endpoints, endpointDiagnostics, err := extractCollection(root, CollectionEndpoints, format, publishableEndpoint)
	if err != nil {
		return startupNodes{}, nil, err
	}
	if len(outbounds)+len(endpoints) > maximumOutbounds {
		return startupNodes{}, nil, invalidStartup("too_many_publishable_nodes")
	}
	candidates := append(outbounds, endpoints...)
	diagnostics := append(outboundDiagnostics, endpointDiagnostics...)
	tagCounts := make(map[string]int, len(candidates))
	for _, candidate := range candidates {
		tagCounts[candidate.tag]++
	}

	unique := candidates[:0]
	for _, candidate := range candidates {
		if tagCounts[candidate.tag] > 1 {
			diagnostics = append(diagnostics, diagnostic(format, candidate.collection, candidate.index, DiagnosticDuplicateTag))
			continue
		}
		unique = append(unique, candidate)
	}
	sort.SliceStable(unique, func(left, right int) bool {
		if unique[left].tag != unique[right].tag {
			return unique[left].tag < unique[right].tag
		}
		if unique[left].collection != unique[right].collection {
			return unique[left].collection < unique[right].collection
		}
		if unique[left].typeID != unique[right].typeID {
			return unique[left].typeID < unique[right].typeID
		}
		return unique[left].index < unique[right].index
	})
	_, endpointsPresent := root[string(CollectionEndpoints)]
	return startupNodes{values: unique, endpointsPresent: endpointsPresent}, diagnostics, nil
}

func extractCollection(
	root map[string]any,
	collection Collection,
	format Format,
	publishable func(string) bool,
) ([]outbound, []Diagnostic, error) {
	raw, exists := root[string(collection)]
	if !exists {
		return []outbound{}, []Diagnostic{}, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, nil, invalidStartup(string(collection) + "_not_array")
	}
	if len(values) > maximumOutbounds {
		return nil, nil, invalidStartup("too_many_" + string(collection))
	}
	candidates := make([]outbound, 0, len(values))
	diagnostics := make([]Diagnostic, 0)
	for index, rawValue := range values {
		value, ok := rawValue.(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, diagnostic(format, collection, index, DiagnosticInvalidOutbound))
			continue
		}
		typeID, typeOK := value["type"].(string)
		tag, tagOK := value["tag"].(string)
		if !typeOK || !tagOK || !validType(typeID) || !validTag(tag) {
			diagnostics = append(diagnostics, diagnostic(format, collection, index, DiagnosticInvalidMetadata))
			continue
		}
		if !publishable(typeID) {
			continue
		}
		candidates = append(candidates, outbound{
			collection: collection,
			index:      index,
			tag:        tag,
			typeID:     typeID,
			value:      value,
		})
	}
	return candidates, diagnostics, nil
}

func publishableOutbound(typeID string) bool {
	_, local := nonPublishableTypes[typeID]
	return !local
}

func publishableEndpoint(typeID string) bool {
	return typeID == "wireguard"
}

func inspectDocument(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	values := 0
	if err := inspectValue(decoder, 0, &values); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return invalidStartup("trailing_json")
	}
	return nil
}

func inspectValue(decoder *json.Decoder, depth int, values *int) error {
	if depth > maximumDepth {
		return invalidStartup("nesting_too_deep")
	}
	*values++
	if *values > maximumValues {
		return invalidStartup("too_many_values")
	}
	token, err := decoder.Token()
	if err != nil {
		return invalidStartup("malformed_json")
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return invalidStartup("malformed_json")
			}
			name, ok := nameToken.(string)
			if !ok || strings.ContainsRune(name, '\x00') {
				return invalidStartup("invalid_object_key")
			}
			if _, duplicate := seen[name]; duplicate {
				return invalidStartup("duplicate_object_key")
			}
			seen[name] = struct{}{}
			if err := inspectValue(decoder, depth+1, values); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return invalidStartup("malformed_json")
		}
	case '[':
		for decoder.More() {
			if err := inspectValue(decoder, depth+1, values); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return invalidStartup("malformed_json")
		}
	default:
		return invalidStartup("malformed_json")
	}
	return nil
}

func validateChannel(channel Channel) (filter, error) {
	if channel.Format != FormatSingBox && channel.Format != FormatMihomo && channel.Format != FormatLoon {
		return filter{}, invalidChannel("unsupported_format")
	}
	if len(channel.ExcludeTags) > maximumFilters || len(channel.ExcludeTypes) > maximumFilters {
		return filter{}, invalidChannel("too_many_exclusions")
	}
	result := filter{
		tags:  make(map[string]struct{}, len(channel.ExcludeTags)),
		types: make(map[string]struct{}, len(channel.ExcludeTypes)),
	}
	for _, tag := range channel.ExcludeTags {
		if !validTag(tag) {
			return filter{}, invalidChannel("invalid_tag_exclusion")
		}
		if _, duplicate := result.tags[tag]; duplicate {
			return filter{}, invalidChannel("duplicate_tag_exclusion")
		}
		result.tags[tag] = struct{}{}
	}
	for _, typeID := range channel.ExcludeTypes {
		if !validType(typeID) {
			return filter{}, invalidChannel("invalid_type_exclusion")
		}
		if _, duplicate := result.types[typeID]; duplicate {
			return filter{}, invalidChannel("duplicate_type_exclusion")
		}
		result.types[typeID] = struct{}{}
	}
	return result, nil
}

func applyFilter(values []outbound, exclusions filter) []outbound {
	result := make([]outbound, 0, len(values))
	for _, value := range values {
		if _, excluded := exclusions.tags[value.tag]; excluded {
			continue
		}
		if _, excluded := exclusions.types[value.typeID]; excluded {
			continue
		}
		result = append(result, value)
	}
	return result
}

func validTag(value string) bool {
	return value != "" && len(value) <= 512 && !strings.ContainsRune(value, '\x00')
}

func validType(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func diagnostic(format Format, collection Collection, index int, code DiagnosticCode) Diagnostic {
	return Diagnostic{Collection: collection, ItemIndex: index, Format: format, Code: code}
}

func sortDiagnostics(values []Diagnostic) {
	sort.SliceStable(values, func(left, right int) bool {
		if values[left].Collection != values[right].Collection {
			return values[left].Collection < values[right].Collection
		}
		if values[left].ItemIndex != values[right].ItemIndex {
			return values[left].ItemIndex < values[right].ItemIndex
		}
		return values[left].Code < values[right].Code
	})
}
