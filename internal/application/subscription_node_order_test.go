// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"reflect"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func TestSourceNodeOrderAcrossCollectionsAndRefresh(t *testing.T) {
	sources := []store.PublicSubscriptionSourceVersion{
		{SourceID: "older", SourceKind: store.SubscriptionSourceRemote},
		{SourceID: "imported", SourceKind: store.SubscriptionSourceLocal},
		{SourceID: "newer", SourceKind: store.SubscriptionSourceRemote},
	}
	nodes := []subscription.Node{
		{SourceID: "local", Key: "system", Tag: "System"},
		{SourceID: "manual", Key: "manual", Tag: "Manual"},
		{SourceID: "older", Tag: "Old A"}, {SourceID: "older", Tag: "Old B"},
		{SourceID: "imported", Tag: "Imported"},
		{SourceID: "newer", Tag: "New A"}, {SourceID: "newer", Tag: "New B"},
	}
	orders := map[string]subscription.NodeOrder{
		"manual": {IDs: []string{subscription.PublicationID(nodes[4]), subscription.PublicationID(nodes[0]), subscription.PublicationID(nodes[1])}},
		"newer":  {IDs: []string{subscription.PublicationID(nodes[6]), subscription.PublicationID(nodes[5])}},
	}
	for _, test := range []struct {
		name   string
		nodes  []subscription.Node
		orders map[string]subscription.NodeOrder
		want   []string
	}{
		{"default", nodes, nil, []string{"System", "Manual", "Imported", "New A", "New B", "Old A", "Old B"}},
		{"formerly remote", nodes, map[string]subscription.NodeOrder{"imported": {IDs: []string{subscription.PublicationID(nodes[4])}}}, []string{"System", "Manual", "Imported", "New A", "New B", "Old A", "Old B"}},
		{"saved", nodes, orders, []string{"Imported", "System", "Manual", "New B", "New A", "Old A", "Old B"}},
		{"refresh", append(append([]subscription.Node{}, nodes[:5]...), nodes[6], subscription.Node{SourceID: "newer", Tag: "New C"}), orders, []string{"Imported", "System", "Manual", "New B", "New C", "Old A", "Old B"}},
		{"returns", append(append([]subscription.Node{}, nodes...), subscription.Node{SourceID: "newer", Tag: "New C"}), orders, []string{"Imported", "System", "Manual", "New B", "New A", "New C", "Old A", "Old B"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := append([]subscription.Node(nil), test.nodes...)
			got := []string{}
			for _, node := range orderSubscriptionNodes(test.nodes, sources, test.orders) {
				got = append(got, node.Tag)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("order %v, want %v", got, test.want)
			}
			if !reflect.DeepEqual(test.nodes, original) {
				t.Fatal("source nodes mutated")
			}
		})
	}
}
