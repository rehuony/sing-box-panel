// SPDX-License-Identifier: GPL-3.0-or-later

package singbox11225

import (
	"github.com/rehuony/sing-box-panel/internal/subscription/document"
	"github.com/rehuony/sing-box-panel/internal/subscription/inbound"
	"github.com/rehuony/sing-box-panel/internal/subscription/node"
)

const Version = "1.12.25"

type converter struct{}

func New() inbound.Converter { return converter{} }

func (converter) ExactVersion() string { return Version }

var convertibleInboundTypes = map[string]struct{}{
	"mixed": {}, "socks": {}, "http": {}, "shadowsocks": {}, "vmess": {},
	"trojan": {}, "hysteria": {}, "shadowtls": {}, "vless": {}, "tuic": {},
	"hysteria2": {}, "anytls": {},
}

var unpublishableInboundTypes = map[string]struct{}{
	"direct": {}, "tun": {}, "redirect": {}, "tproxy": {}, "cloudflared": {},
}

func (converter) Convert(request inbound.Request) (inbound.Result, error) {
	if _, err := normalizedPublicHost(request.PublicHost); err != nil {
		return inbound.Result{}, err
	}
	value, err := document.Decode(request.FinalStartupJSON)
	if err != nil {
		return inbound.Result{}, err
	}
	root, ok := value.(map[string]any)
	if !ok {
		return inbound.Result{}, document.Invalid("root_not_object")
	}
	rawInbounds, exists := root[string(node.CollectionInbounds)]
	if !exists {
		return inbound.Result{Nodes: []node.Node{}, Diagnostics: []node.ConversionDiagnostic{}}, nil
	}
	inbounds, ok := rawInbounds.([]any)
	if !ok {
		return inbound.Result{}, document.Invalid("inbounds_not_array")
	}
	if len(inbounds) > node.MaximumNodes {
		return inbound.Result{}, document.Invalid("too_many_inbounds")
	}

	nodes := make([]node.Node, 0, len(inbounds))
	diagnostics := make([]node.ConversionDiagnostic, 0)
	seenKeys := make(map[string]struct{})
	seenTags := make(map[string]struct{})
	for index, rawInbound := range inbounds {
		inboundValue, ok := rawInbound.(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, inboundDiagnostic(index, node.DiagnosticInvalidOutbound))
			continue
		}
		typeID, typeOK := inboundValue["type"].(string)
		tag, tagOK := inboundValue["tag"].(string)
		if !typeOK || !tagOK || !node.ValidType(typeID) || !node.ValidTag(tag) {
			diagnostics = append(diagnostics, inboundDiagnostic(index, node.DiagnosticInvalidMetadata))
			continue
		}
		if _, known := unpublishableInboundTypes[typeID]; known {
			diagnostics = append(diagnostics, inboundDiagnostic(index, node.DiagnosticUnpublishableInbound))
			continue
		}
		if _, known := convertibleInboundTypes[typeID]; !known {
			diagnostics = append(diagnostics, inboundDiagnostic(index, node.DiagnosticUnsupportedType))
			continue
		}
		port, ok := document.Integer(inboundValue["listen_port"], 1, 65535)
		if !ok {
			diagnostics = append(diagnostics, inboundDiagnostic(index, node.DiagnosticInvalidRequiredField))
			continue
		}
		credentials, credentialErr := inboundCredentials(typeID, inboundValue)
		if credentialErr != nil {
			return inbound.Result{Diagnostics: append(diagnostics, inboundDiagnostic(index, node.DiagnosticAmbiguousCredential))}, credentialErr
		}
		for credentialIndex, credential := range credentials {
			convertedNode, convertErr := convertInboundCredential(
				typeID, tag, request.PublicHost, port, inboundValue, credential, credentialIndex, len(credentials),
			)
			if convertErr != nil {
				diagnostics = append(diagnostics, inboundDiagnostic(index, node.DiagnosticInvalidRequiredField))
				continue
			}
			if _, duplicate := seenKeys[convertedNode.Key]; duplicate {
				return inbound.Result{Diagnostics: append(diagnostics, inboundDiagnostic(index, node.DiagnosticAmbiguousCredential))}, inbound.ErrAmbiguousInboundCredential
			}
			if _, duplicate := seenTags[convertedNode.Tag]; duplicate {
				return inbound.Result{Diagnostics: append(diagnostics, inboundDiagnostic(index, node.DiagnosticAmbiguousCredential))}, inbound.ErrAmbiguousInboundCredential
			}
			seenKeys[convertedNode.Key] = struct{}{}
			seenTags[convertedNode.Tag] = struct{}{}
			nodes = append(nodes, convertedNode)
		}
	}
	return inbound.Result{Nodes: nodes, Diagnostics: diagnostics}, nil
}

func inboundDiagnostic(index int, code node.DiagnosticCode) node.ConversionDiagnostic {
	return node.ConversionDiagnostic{Collection: node.CollectionInbounds, ItemIndex: index, Code: code}
}
