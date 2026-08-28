// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"sort"
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

func parseStartup(data []byte, format RenderFormat) (startupNodes, []RenderDiagnostic, error) {
	decoded, err := DecodeDocument(data)
	if err != nil {
		return startupNodes{}, nil, invalidStartup(DocumentErrorCode(err))
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
	format RenderFormat,
	publishable func(string) bool,
) ([]outbound, []RenderDiagnostic, error) {
	raw, exists := root[string(collection)]
	if !exists {
		return []outbound{}, []RenderDiagnostic{}, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, nil, invalidStartup(string(collection) + "_not_array")
	}
	if len(values) > maximumOutbounds {
		return nil, nil, invalidStartup("too_many_" + string(collection))
	}
	candidates := make([]outbound, 0, len(values))
	diagnostics := make([]RenderDiagnostic, 0)
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

func validateChannel(channel RenderChannel) (filter, error) {
	if channel.Format != RenderFormatSingBox && channel.Format != RenderFormatMihomo && channel.Format != RenderFormatLoon {
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
	return ValidTag(value)
}

func validType(value string) bool {
	return ValidType(value)
}

func diagnostic(format RenderFormat, collection Collection, index int, code DiagnosticCode) RenderDiagnostic {
	return RenderDiagnostic{Collection: collection, ItemIndex: index, Format: format, Code: code}
}

func sortDiagnostics(values []RenderDiagnostic) {
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
