// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

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
	// Hostless holds the nodes that publish while neither a public host nor a
	// public IP is known for them.
	Hostless map[string]bool
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
// failing the others. A node whose conversion inputs are those of the last
// call reuses its conversion.
func (application *Service) localNodes(publications []store.NodePublication) nodeLocals {
	result := nodeLocals{
		Documents: map[string][]byte{}, Summaries: []NodePublicationSummary{}, Diagnostics: []ConversionDiagnostic{}, Hostless: map[string]bool{},
	}
	conversions.Lock()
	defer conversions.Unlock()
	kept := make(map[conversionKey]nodeConversion, len(publications))
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
		key := newConversionKey(publication, host)
		converted, ok := kept[key]
		if !ok {
			if converted, ok = conversions.entries[key]; !ok {
				converted = application.convertPublication(publication, host)
			}
			kept[key] = converted
		}
		if !converted.applied {
			fail(DiagnosticAppliedRevisionUnavailable)
			continue
		}
		if host == "" {
			result.Hostless[publication.NodeID] = true
			fail(DiagnosticPublicHostUnavailable)
			continue
		}
		switch {
		case errors.Is(converted.err, ErrUnsupportedCoreVersion):
			fail(DiagnosticUnsupportedVersion)
			continue
		case converted.err != nil:
			fail(DiagnosticInvalidStartup)
			continue
		}
		result.Documents[publication.NodeID] = converted.document
		for _, diagnostic := range converted.inbounds.Diagnostics {
			diagnostic.NodeID = publication.NodeID
			result.Diagnostics = append(result.Diagnostics, diagnostic)
		}
		for _, node := range converted.inbounds.Nodes {
			result.Inbounds = append(result.Inbounds, fleetInbound{NodeID: publication.NodeID, NodeName: publication.NodeName, Node: node})
		}
	}
	conversions.entries = kept
	return result
}

// conversionKey digests what one node's conversion derives from: the
// applied configuration as stored, the digest the node reported for it, its
// core version and the host it publishes.
type conversionKey [sha256.Size]byte

func newConversionKey(publication store.NodePublication, host string) conversionKey {
	digest := sha256.New()
	for _, field := range []string{publication.ExactCoreVersion, host, publication.ConfigSHA256} {
		digest.Write([]byte(field))
		digest.Write([]byte{0})
	}
	if publication.Document != nil {
		digest.Write([]byte{1})
		digest.Write(publication.Document)
	}
	var key conversionKey
	digest.Sum(key[:0])
	return key
}

// nodeConversion is what one node's applied configuration yields: the
// configuration as the node runs it, whether it matches the digest the node
// reported and, with a host, its inbounds.
type nodeConversion struct {
	document []byte
	applied  bool
	inbounds InboundResult
	err      error
}

// conversions keeps the conversions of the nodes the latest localNodes call
// read, by what each derives from. A conversion depends on nothing else, so
// every Service shares them; readers must not change them.
var conversions = struct {
	sync.Mutex
	entries map[conversionKey]nodeConversion
}{entries: map[conversionKey]nodeConversion{}}

func (application *Service) convertPublication(publication store.NodePublication, host string) nodeConversion {
	document, applied := appliedDocument(publication)
	converted := nodeConversion{document: document, applied: applied}
	if applied && host != "" {
		converted.inbounds, converted.err = application.convertInboundNodes(publication.ExactCoreVersion, document, host)
	}
	return converted
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
