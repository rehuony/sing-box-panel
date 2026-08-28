// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"sort"

	"github.com/rehuony/sing-box-panel/internal/subscription/node"
)

func (application *Application) SubscriptionNodeCatalog(ctx context.Context) (SubscriptionNodeCatalog, error) {
	state, err := application.database.LoadSubscriptionNodeCatalogState(ctx)
	if err != nil {
		return SubscriptionNodeCatalog{}, err
	}
	startupJSON, err := application.subscriptionStartupJSONWithCore(state.Startup, state.Core)
	if err != nil {
		return SubscriptionNodeCatalog{}, err
	}
	conversion, err := application.convertInboundNodes(
		state.Startup.ExactCoreVersion, startupJSON, "validation.invalid",
	)
	if err != nil {
		return SubscriptionNodeCatalog{}, err
	}
	nodes, diagnostics := conversion.Nodes, conversion.Diagnostics
	for _, source := range state.Sources {
		sourceNodes, decodeErr := node.Decode(source.NormalizedNodes)
		if decodeErr != nil {
			return SubscriptionNodeCatalog{}, decodeErr
		}
		nodes = append(nodes, sourceNodes...)
	}
	sort.SliceStable(nodes, func(left, right int) bool { return nodes[left].Key < nodes[right].Key })
	result := SubscriptionNodeCatalog{
		AppliedBundleID: state.AppliedBundleID, Diagnostics: diagnostics,
		Nodes: make([]SubscriptionNodeSummary, len(nodes)),
	}
	for index, node := range nodes {
		result.Nodes[index] = SubscriptionNodeSummary{
			Key: node.Key, SourceID: node.SourceID, Type: node.Type,
			Tag: node.Tag, Credential: node.Credential,
		}
	}
	return result, nil
}

// RenderSubscriptionPreview renders the same consistent live publication view
// that an enabled user's token would receive, without requiring a token.
