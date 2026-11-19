// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// fleetInbound is one client node a fleet node's applied inbounds yield.
type fleetInbound struct {
	NodeID   string
	NodeName string
	Node     Node
}

// nodeLocals is every node's publishable inbounds and the diagnostics of the
// nodes and inbounds that cannot be published.
type nodeLocals struct {
	Inbounds    []fleetInbound
	Diagnostics []ConversionDiagnostic
	Summaries   []NodePublicationSummary
	// Documents holds each node's applied configuration, for preview
	// diagnostics.
	Documents map[string][]byte
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

// localNodes converts the inbounds of every node that publishes, in the
// order of publications and, within a node, configuration order. A node that
// cannot be published adds a diagnostic and contributes no inbounds, without
// failing the others.
func (application *Service) localNodes(publications []store.NodePublication) nodeLocals {
	result := nodeLocals{Documents: map[string][]byte{}, Summaries: []NodePublicationSummary{}, Diagnostics: []ConversionDiagnostic{}}
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
			fail(DiagnosticPublicHostUnavailable)
			continue
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
		for _, node := range conversion.Nodes {
			result.Inbounds = append(result.Inbounds, fleetInbound{NodeID: publication.NodeID, NodeName: publication.NodeName, Node: node})
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
