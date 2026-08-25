// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/json"
)

// MergeSourceSnapshots builds the publication-only startup document consumed
// by Render. The runtime document is never changed. Each source snapshot may
// be a sing-box object containing outbounds/endpoints, one outbound object, or
// an array of outbound objects. Sources are appended before the renderer does
// its normal duplicate-tag, dependency, filtering, and stable-sort checks.
func MergeSourceSnapshots(finalStartupJSON []byte, sourceSnapshots [][]byte) ([]byte, error) {
	root, err := decodePublicationObject(finalStartupJSON)
	if err != nil {
		return nil, err
	}
	outbounds, _, err := publicationCollection(root, CollectionOutbounds)
	if err != nil {
		return nil, err
	}
	endpoints, endpointsPresent, err := publicationCollection(root, CollectionEndpoints)
	if err != nil {
		return nil, err
	}

	for _, snapshot := range sourceSnapshots {
		if len(snapshot) == 0 {
			continue
		}
		value, decodeErr := decodePublicationValue(snapshot)
		if decodeErr != nil {
			return nil, decodeErr
		}
		switch source := value.(type) {
		case []any:
			outbounds = append(outbounds, source...)
		case map[string]any:
			sourceOutbounds, hasOutbounds, collectionErr := publicationCollection(source, CollectionOutbounds)
			if collectionErr != nil {
				return nil, collectionErr
			}
			sourceEndpoints, hasEndpoints, collectionErr := publicationCollection(source, CollectionEndpoints)
			if collectionErr != nil {
				return nil, collectionErr
			}
			if hasOutbounds || hasEndpoints {
				outbounds = append(outbounds, sourceOutbounds...)
				endpoints = append(endpoints, sourceEndpoints...)
				endpointsPresent = endpointsPresent || hasEndpoints
			} else {
				outbounds = append(outbounds, source)
			}
		default:
			return nil, invalidStartup("source_root_not_object_or_array")
		}
		if len(outbounds)+len(endpoints) > maximumOutbounds {
			return nil, invalidStartup("too_many_publishable_nodes")
		}
	}

	publication := map[string]any{string(CollectionOutbounds): outbounds}
	if endpointsPresent {
		publication[string(CollectionEndpoints)] = endpoints
	}
	encoded, err := json.Marshal(publication)
	if err != nil {
		return nil, invalidStartup("encode_publication_document")
	}
	if len(encoded) > MaximumStartupBytes {
		return nil, invalidStartup("document_too_large")
	}
	return append(bytes.Clone(encoded), '\n'), nil
}

func decodePublicationObject(data []byte) (map[string]any, error) {
	value, err := decodePublicationValue(data)
	if err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, invalidStartup("root_not_object")
	}
	return root, nil
}

func decodePublicationValue(data []byte) (any, error) {
	if len(data) == 0 {
		return nil, invalidStartup("empty_document")
	}
	if len(data) > MaximumStartupBytes {
		return nil, invalidStartup("document_too_large")
	}
	if err := inspectDocument(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, invalidStartup("malformed_json")
	}
	return value, nil
}

func publicationCollection(root map[string]any, collection Collection) ([]any, bool, error) {
	raw, exists := root[string(collection)]
	if !exists {
		return []any{}, false, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, true, invalidStartup(string(collection) + "_not_array")
	}
	if len(values) > maximumOutbounds {
		return nil, true, invalidStartup("too_many_" + string(collection))
	}
	return append([]any(nil), values...), true, nil
}
