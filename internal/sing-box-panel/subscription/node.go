// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"encoding/json"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// MaximumNodes bounds one node collection.
const MaximumNodes = store.MaximumSubscriptionNodes

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
	DiagnosticUnsupportedVersion   DiagnosticCode = "unsupported_core_version"
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
	// NodeID names the node whose applied configuration the diagnostic is
	// about.
	NodeID     string         `json:"node_id,omitempty"`
	Collection Collection     `json:"collection"`
	ItemIndex  int            `json:"item_index"`
	Code       DiagnosticCode `json:"code"`
}

// Node is one client node an inbound converts to.
type Node struct {
	// OriginTag is the inbound's tag; Tag names the node, with a suffix for
	// each credential of a multi-user inbound.
	OriginTag string
	// OriginName is the name the pool shows the node by: the inbound's tag,
	// with its user's name for a multi-user inbound.
	OriginName string
	// Key identifies the inbound and credential: "local:" and a digest of
	// the inbound's type and tag and the credential's identity.
	Key        string
	SourceID   string
	Type       string
	Tag        string
	Credential string
	Outbound   json.RawMessage
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
