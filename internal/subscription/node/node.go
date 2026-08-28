// SPDX-License-Identifier: GPL-3.0-or-later

// Package node owns normalized, grantable subscription node contracts.
package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/subscription/document"
)

const MaximumNodes = 10_000

var ErrInvalid = errors.New("invalid normalized subscription nodes")

type Collection string

const (
	CollectionOutbounds Collection = "outbounds"
	CollectionEndpoints Collection = "endpoints"
	CollectionInbounds  Collection = "inbounds"
)

type DiagnosticCode string

const (
	DiagnosticInvalidOutbound      DiagnosticCode = "invalid_outbound"
	DiagnosticInvalidMetadata      DiagnosticCode = "invalid_metadata"
	DiagnosticDuplicateTag         DiagnosticCode = "duplicate_tag"
	DiagnosticUnsupportedType      DiagnosticCode = "unsupported_type"
	DiagnosticUnsupportedOption    DiagnosticCode = "unsupported_option"
	DiagnosticUnsupportedTransport DiagnosticCode = "unsupported_transport"
	DiagnosticUnsupportedTLS       DiagnosticCode = "unsupported_tls"
	DiagnosticUnsupportedNetwork   DiagnosticCode = "unsupported_network"
	DiagnosticUnresolvedDependency DiagnosticCode = "unresolved_dependency"
	DiagnosticInvalidRequiredField DiagnosticCode = "invalid_required_field"
	DiagnosticUnpublishableInbound DiagnosticCode = "unpublishable_inbound"
	DiagnosticAmbiguousCredential  DiagnosticCode = "ambiguous_credential"
)

type ConversionDiagnostic struct {
	Collection Collection     `json:"collection"`
	ItemIndex  int            `json:"item_index"`
	Code       DiagnosticCode `json:"code"`
}

type Node struct {
	Key        string          `json:"key"`
	SourceID   string          `json:"source_id"`
	Type       string          `json:"type"`
	Tag        string          `json:"tag"`
	Credential string          `json:"credential,omitempty"`
	Outbound   json.RawMessage `json:"outbound"`
}

func PublicationDocument(nodes []Node) ([]byte, error) {
	if len(nodes) > MaximumNodes {
		return nil, invalid("too_many_publishable_nodes")
	}
	ordered := append([]Node(nil), nodes...)
	sort.SliceStable(ordered, func(left, right int) bool { return ordered[left].Key < ordered[right].Key })
	seenKeys := make(map[string]struct{}, len(ordered))
	outbounds := make([]any, 0, len(ordered))
	for _, value := range ordered {
		if !ValidKey(value.Key) {
			return nil, invalid("invalid_node_key")
		}
		if _, exists := seenKeys[value.Key]; exists {
			return nil, invalid("duplicate_node_key")
		}
		seenKeys[value.Key] = struct{}{}
		decoded, err := document.Decode(value.Outbound)
		if err != nil {
			return nil, invalid(document.Code(err))
		}
		object, ok := decoded.(map[string]any)
		if !ok {
			return nil, invalid("node_not_object")
		}
		if objectType, _ := object["type"].(string); objectType != value.Type || !ValidType(objectType) {
			return nil, invalid("node_type_mismatch")
		}
		if objectTag, _ := object["tag"].(string); objectTag != value.Tag || !ValidTag(objectTag) {
			return nil, invalid("node_tag_mismatch")
		}
		outbounds = append(outbounds, object)
	}
	encoded, err := json.Marshal(map[string]any{"outbounds": outbounds})
	if err != nil {
		return nil, invalid("encode_publication_document")
	}
	if len(encoded) > document.MaximumBytes {
		return nil, invalid("document_too_large")
	}
	return append(bytes.Clone(encoded), '\n'), nil
}

func Decode(raw []byte) ([]Node, error) {
	if len(raw) == 0 || len(raw) > document.MaximumBytes {
		return nil, invalid("invalid_normalized_nodes")
	}
	decoded, err := document.Decode(raw)
	if err != nil {
		return nil, invalid(document.Code(err))
	}
	values, ok := decoded.([]any)
	if !ok {
		return nil, invalid("invalid_normalized_nodes")
	}
	canonical, err := json.Marshal(values)
	if err != nil {
		return nil, invalid("invalid_normalized_nodes")
	}
	var nodes []Node
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	if err := decoder.Decode(&nodes); err != nil || nodes == nil {
		return nil, invalid("invalid_normalized_nodes")
	}
	if _, err := PublicationDocument(nodes); err != nil {
		return nil, err
	}
	for index := range nodes {
		nodes[index].Outbound = bytes.Clone(nodes[index].Outbound)
	}
	return nodes, nil
}

func ValidKey(value string) bool {
	if value == "" || len(value) > 512 || value != strings.TrimSpace(value) {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e || character == '\\' || character == '"' {
			return false
		}
	}
	return true
}

func ValidTag(value string) bool {
	return value != "" && len(value) <= 512 && !strings.ContainsRune(value, '\x00')
}

func ValidType(value string) bool {
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

type validationError struct{ code string }

func (err *validationError) Error() string { return fmt.Sprintf("%v: %s", ErrInvalid, err.code) }

func (err *validationError) Unwrap() error { return ErrInvalid }

func invalid(code string) error { return &validationError{code: code} }
