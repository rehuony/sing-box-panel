// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"slices"
	"sort"

	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func nodeCollection(sourceID string, sources []store.PublicSubscriptionSourceVersion) string {
	for _, source := range sources {
		if source.SourceID == sourceID && source.SourceKind == store.SubscriptionSourceRemote {
			return sourceID
		}
	}
	return "manual"
}

func publicationNodeIDs(nodes []subscription.Node) []string {
	ids := make([]string, len(nodes))
	for i, node := range nodes {
		ids[i] = subscription.PublicationID(node)
	}
	return ids
}

// Sources arrive oldest first for stable publication identity. Presentation uses
// the source sidebar order instead: the manual collection, then newest sources.
func orderSubscriptionNodes(nodes []subscription.Node, sources []store.PublicSubscriptionSourceVersion, orders map[string]subscription.NodeOrder) []subscription.Node {
	collections := map[string]int{}
	for i := len(sources) - 1; i >= 0; i-- {
		if sources[i].SourceKind == store.SubscriptionSourceRemote {
			collections[sources[i].SourceID] = len(collections) + 1
		}
	}
	positions := make(map[string]map[string]int, len(orders))
	for collectionID, order := range orders {
		positions[collectionID] = make(map[string]int, len(order.IDs))
		for i, id := range order.IDs {
			positions[collectionID][id] = i
		}
	}
	ordered := slices.Clone(nodes)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if collections[a.SourceID] != collections[b.SourceID] {
			return collections[a.SourceID] < collections[b.SourceID]
		}
		collectionID := "manual"
		if collections[a.SourceID] != 0 {
			collectionID = a.SourceID
		}
		pa, okA := positions[collectionID][subscription.PublicationID(a)]
		pb, okB := positions[collectionID][subscription.PublicationID(b)]
		if okA != okB {
			return okA
		}
		return okA && pa < pb
	})
	return ordered
}

func (app *Application) SetSubscriptionNodeOrder(ctx context.Context, collectionID string, ids []string, revision int64) (subscription.NodeOrder, error) {
	if ids == nil || len(ids) > subscription.MaximumNodes {
		return subscription.NodeOrder{}, store.ErrSubscriptionNodeInvalid
	}
	_, entries, err := app.subscriptionCatalog(ctx)
	if err != nil {
		return subscription.NodeOrder{}, err
	}
	members := make(map[string]bool)
	for _, entry := range entries {
		if entry.CollectionID == collectionID {
			members[entry.Summary.ID] = true
		}
	}
	for _, id := range ids {
		if !members[id] {
			return subscription.NodeOrder{}, store.ErrSubscriptionNodeInvalid
		}
	}
	return app.database.SetSubscriptionNodeOrder(ctx, collectionID, ids, revision)
}
