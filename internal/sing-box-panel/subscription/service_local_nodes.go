// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// Diagnostics of a node whose inbounds cannot be published.
const (
	// DiagnosticAppliedRevisionUnavailable: the panel no longer holds the
	// revision the node reports running, or its bytes differ from it.
	DiagnosticAppliedRevisionUnavailable DiagnosticCode = "applied_revision_unavailable"
	// DiagnosticPublicHostUnavailable: the node has no public host override
	// and reports no public IP.
	DiagnosticPublicHostUnavailable DiagnosticCode = "public_host_unavailable"
	// DiagnosticInvalidStartup: the applied configuration cannot be read.
	DiagnosticInvalidStartup DiagnosticCode = "invalid_startup"
)

// catalogHost replaces a missing public host while the catalog validates a
// node's inbounds; such inbounds are shown as unavailable.
const catalogHost = "validation.invalid"

// NodePublicationSummary is what one node publishes from: the bundle it
// reports running and the host its inbounds use.
type NodePublicationSummary struct {
	NodeID              string `json:"node_id"`
	NodeName            string `json:"node_name"`
	AppliedBundleID     string `json:"applied_bundle_id"`
	CanonicalRevisionID string `json:"canonical_revision_id"`
	ExactCoreVersion    string `json:"exact_core_version"`
	// PublicHost is the host subscriptions publish; empty while the node has
	// neither an override nor a reported public IP.
	PublicHost string `json:"public_host"`
}

// nodeLocals is every node's local inbounds and the diagnostics of the
// nodes and inbounds that cannot be published.
type nodeLocals struct {
	Nodes       []Node
	Diagnostics []ConversionDiagnostic
	Summaries   []NodePublicationSummary
	// Documents holds each node's applied configuration, for preview
	// diagnostics.
	Documents map[string][]byte
	// Listeners maps a local node's key to the listen address of its inbound.
	Listeners map[string]string
	// Hostless holds the nodes converted with catalogHost.
	Hostless map[string]bool
}

// publishedHost is the host a node's inbounds publish: its override, or the
// public IP it reports.
func publishedHost(publication store.NodePublication) string {
	if publication.PublicHost != "" {
		return publication.PublicHost
	}
	if host, err := store.NormalizePublishedHost(publication.ReportedPublicIP); err == nil {
		return host
	}
	return ""
}

// localNodes converts the inbounds of every node that publishes. A node that
// cannot be published adds a diagnostic instead of failing the others. For
// the catalog, a node without a public host is converted with catalogHost.
func (application *Service) localNodes(publications []store.NodePublication, catalog bool) nodeLocals {
	result := nodeLocals{Documents: map[string][]byte{}, Listeners: map[string]string{}, Hostless: map[string]bool{}}
	for _, publication := range publications {
		host := publishedHost(publication)
		result.Summaries = append(result.Summaries, NodePublicationSummary{
			NodeID: publication.NodeID, NodeName: publication.NodeName, AppliedBundleID: publication.BundleID,
			CanonicalRevisionID: publication.RevisionID, ExactCoreVersion: publication.ExactCoreVersion, PublicHost: host,
		})
		fail := func(code DiagnosticCode) {
			result.Diagnostics = append(result.Diagnostics, ConversionDiagnostic{
				NodeID: publication.NodeID, Collection: CollectionInbounds, ItemIndex: -1, Code: code,
			})
		}
		document, ok := appliedDocument(publication)
		if !ok {
			fail(DiagnosticAppliedRevisionUnavailable)
			continue
		}
		if host == "" {
			if !catalog {
				fail(DiagnosticPublicHostUnavailable)
				continue
			}
			host = catalogHost
			result.Hostless[publication.NodeID] = true
		}
		conversion, err := application.convertInboundNodes(publication.ExactCoreVersion, document, host)
		switch {
		case errors.Is(err, ErrUnsupportedCoreVersion):
			fail(DiagnosticUnsupportedVersion)
			continue
		case err != nil:
			fail(DiagnosticInvalidStartup)
			continue
		}
		result.Documents[publication.NodeID] = document
		for _, diagnostic := range conversion.Diagnostics {
			diagnostic.NodeID = publication.NodeID
			result.Diagnostics = append(result.Diagnostics, diagnostic)
		}
		listeners := inboundListeners(document)
		for _, node := range conversion.Nodes {
			// Several nodes may run the same tags, so a local node's tag and key
			// carry its node.
			node.Key = "local:" + publication.NodeID + ":" + strings.TrimPrefix(node.Key, "local:")
			node.Tag = publication.NodeName + " · " + node.Tag
			node.NodeID, node.NodeName = publication.NodeID, publication.NodeName
			outbound, err := DecodeDocumentObject(node.Outbound)
			if err != nil {
				fail(DiagnosticInvalidStartup)
				continue
			}
			outbound["tag"] = node.Tag
			if node.Outbound, err = json.Marshal(outbound); err != nil {
				fail(DiagnosticInvalidStartup)
				continue
			}
			if listener, ok := listeners[node.OriginTag]; ok {
				result.Listeners[node.Key] = listener
			}
			result.Nodes = append(result.Nodes, node)
		}
	}
	return result
}

// appliedDocument returns the node's applied configuration as the node runs
// it, after checking it against the digest the node reported.
func appliedDocument(publication store.NodePublication) ([]byte, bool) {
	if publication.Document == nil {
		return nil, false
	}
	document, err := configuration.Parse(publication.Document)
	if err != nil {
		return nil, false
	}
	canonical := document.CanonicalJSON()
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]) == strings.ToLower(publication.ConfigSHA256)
}

func inboundListeners(document []byte) map[string]string {
	var startup struct {
		Inbounds []struct {
			Tag    string `json:"tag"`
			Listen string `json:"listen"`
			Port   int    `json:"listen_port"`
		} `json:"inbounds"`
	}
	listeners := map[string]string{}
	if json.Unmarshal(document, &startup) != nil {
		return listeners
	}
	for _, inbound := range startup.Inbounds {
		listeners[inbound.Tag] = net.JoinHostPort(inbound.Listen, strconv.Itoa(inbound.Port))
	}
	return listeners
}

// nodeIDsOf returns the nodes whose local inbounds are among nodes, an empty
// list when there are none.
func nodeIDsOf(nodes []Node) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, node := range nodes {
		if node.NodeID != "" && !seen[node.NodeID] {
			seen[node.NodeID] = true
			ids = append(ids, node.NodeID)
		}
	}
	return ids
}
