// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"

	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

type subscriptionCatalogEntry struct {
	Summary  SubscriptionNodeSummary
	Node     subscription.Node
	ManualID string
}

func (app *Application) SubscriptionNodeCatalog(ctx context.Context) (SubscriptionNodeCatalog, error) {
	catalog, _, err := app.subscriptionCatalog(ctx)
	return catalog, err
}

func (app *Application) subscriptionCatalog(ctx context.Context) (SubscriptionNodeCatalog, []subscriptionCatalogEntry, error) {
	state, err := app.database.LoadSubscriptionNodeCatalogState(ctx)
	if err != nil {
		return SubscriptionNodeCatalog{}, nil, err
	}
	host, err := app.publicationHost(ctx, state.SubscriptionNodeControls, "")
	if err != nil {
		return SubscriptionNodeCatalog{}, nil, err
	}
	conversionHost := host
	if conversionHost == "" {
		conversionHost = "validation.invalid"
	}
	var conversion subscription.InboundResult
	listeners := make(map[string]string)
	if state.AppliedBundleID != "" {
		startupJSON, err := app.subscriptionStartupJSONWithCore(state.Startup, state.Core)
		if err != nil {
			return SubscriptionNodeCatalog{}, nil, err
		}
		conversion, err = app.convertInboundNodes(state.Startup.ExactCoreVersion, startupJSON, conversionHost)
		if errors.Is(err, subscription.ErrUnsupportedCoreVersion) {
			conversion.Diagnostics = []subscription.ConversionDiagnostic{{Collection: subscription.CollectionInbounds, ItemIndex: -1, Code: subscription.DiagnosticUnsupportedVersion}}
		} else if err != nil {
			return SubscriptionNodeCatalog{}, nil, err
		}
		var startup struct {
			Inbounds []struct {
				Tag    string `json:"tag"`
				Listen string `json:"listen"`
				Port   int    `json:"listen_port"`
			} `json:"inbounds"`
		}
		if err := json.Unmarshal(startupJSON, &startup); err != nil {
			return SubscriptionNodeCatalog{}, nil, err
		}
		for _, inbound := range startup.Inbounds {
			listeners[inbound.Tag] = net.JoinHostPort(inbound.Listen, strconv.Itoa(inbound.Port))
		}
	}
	manual, err := manualPublicationNodes(state.ManualNodes)
	if err != nil {
		return SubscriptionNodeCatalog{}, nil, err
	}
	nodes := append(append([]subscription.Node(nil), conversion.Nodes...), manual...)
	sourceNames := map[string]string{"local": "手动节点", "manual": "手动节点"}
	enabledSources := map[string]bool{"local": true, "manual": true}
	manualByKey := make(map[string]store.ManualSubscriptionNode)
	for index, node := range manual {
		manualByKey[node.Key] = state.ManualNodes[index]
	}
	for _, source := range state.Sources {
		values, err := subscription.DecodeNodes(source.NormalizedNodes)
		if err != nil {
			return SubscriptionNodeCatalog{}, nil, err
		}
		nodes = append(nodes, values...)
		sourceNames[source.SourceID], enabledSources[source.SourceID] = source.Name, source.Enabled
		if source.SourceKind == store.SubscriptionSourceLocal {
			sourceNames[source.SourceID] = "手动节点"
		}
	}
	if len(nodes) > subscription.MaximumNodes {
		return SubscriptionNodeCatalog{}, nil, store.ErrSubscriptionLimitExceeded
	}
	result := SubscriptionNodeCatalog{AppliedBundleID: state.AppliedBundleID, Diagnostics: conversion.Diagnostics, Nodes: make([]SubscriptionNodeSummary, 0, len(nodes))}
	if result.Diagnostics == nil {
		result.Diagnostics = []subscription.ConversionDiagnostic{}
	}
	entries := make([]subscriptionCatalogEntry, 0, len(nodes))
	for _, node := range nodes {
		id := subscription.PublicationID(node)
		visibility := state.Visibility[id]
		summary, err := summarizeSubscriptionNode(node)
		if err != nil {
			return SubscriptionNodeCatalog{}, nil, err
		}
		summary.SourceName = sourceNames[node.SourceID]
		summary.Hidden, summary.VisibilityRevision = visibility.Hidden, visibility.Revision
		summary.Available = enabledSources[node.SourceID]
		if node.SourceID == "local" {
			summary.Origin = "local"
			summary.Name = node.OriginTag
			if summary.Name == "" {
				summary.Name = node.Tag
			}
			summary.Listener = listeners[summary.Name]
			if host == "" {
				summary.Server = ""
				summary.Available = false
				decoded, decodeErr := subscription.DecodeDocument(node.Outbound)
				if decodeErr != nil {
					return SubscriptionNodeCatalog{}, nil, decodeErr
				}
				outbound := decoded.(map[string]any)
				delete(outbound, "server")
				if tls, ok := outbound["tls"].(map[string]any); ok && tls["server_name"] == conversionHost {
					delete(tls, "server_name")
				}
				node.Outbound, err = json.Marshal(outbound)
				if err != nil {
					return SubscriptionNodeCatalog{}, nil, err
				}
				if summary.SNI == conversionHost {
					summary.SNI = ""
				}
			}
		} else if node.SourceID == "manual" {
			summary.Origin = "manual"
			summary.Revision = manualByKey[node.Key].Revision
		}
		entries = append(entries, subscriptionCatalogEntry{Summary: summary, Node: node, ManualID: manualByKey[node.Key].ID})
		result.Nodes = append(result.Nodes, summary)
	}
	return result, entries, nil
}

func (app *Application) subscriptionNodeEntry(ctx context.Context, id string) (subscriptionCatalogEntry, error) {
	if !validPublicID(id) {
		return subscriptionCatalogEntry{}, store.ErrSubscriptionNodeNotFound
	}
	_, entries, err := app.subscriptionCatalog(ctx)
	if err != nil {
		return subscriptionCatalogEntry{}, err
	}
	for _, entry := range entries {
		if entry.Summary.ID == id {
			return entry, nil
		}
	}
	return subscriptionCatalogEntry{}, store.ErrSubscriptionNodeNotFound
}

func (app *Application) SubscriptionNode(ctx context.Context, id string) (SubscriptionNodeDetail, error) {
	entry, err := app.subscriptionNodeEntry(ctx, id)
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	return SubscriptionNodeDetail{SubscriptionNodeSummary: entry.Summary, OutboundJSON: string(entry.Node.Outbound)}, nil
}

func summarizeSubscriptionNode(node subscription.Node) (SubscriptionNodeSummary, error) {
	var endpoint struct {
		Server      string          `json:"server"`
		Port        int             `json:"server_port"`
		ServerPorts json.RawMessage `json:"server_ports"`
		Realm       struct {
			ServerURL string `json:"server_url"`
		} `json:"realm"`
		TLS struct {
			Enabled    bool   `json:"enabled"`
			ServerName string `json:"server_name"`
			Reality    struct {
				Enabled bool `json:"enabled"`
			} `json:"reality"`
		} `json:"tls"`
	}
	if err := json.Unmarshal(node.Outbound, &endpoint); err != nil {
		return SubscriptionNodeSummary{}, err
	}
	var ports []string
	if len(endpoint.ServerPorts) > 0 {
		if err := json.Unmarshal(endpoint.ServerPorts, &ports); err != nil {
			var single string
			if err := json.Unmarshal(endpoint.ServerPorts, &single); err != nil {
				return SubscriptionNodeSummary{}, err
			}
			ports = []string{single}
		}
	}
	if node.Type == "ssh" && endpoint.Port == 0 {
		endpoint.Port = 22
	}
	return SubscriptionNodeSummary{
		ServerPorts: ports, RealmURL: endpoint.Realm.ServerURL,
		ID: subscription.PublicationID(node), Key: node.Key, SourceID: node.SourceID,
		Type: node.Type, Tag: node.Tag, Name: node.Tag, Credential: node.Credential, Origin: "source",
		Server: endpoint.Server, Port: endpoint.Port, TLS: endpoint.TLS.Enabled, Reality: endpoint.TLS.Reality.Enabled, SNI: endpoint.TLS.ServerName,
	}, nil
}
