// SPDX-License-Identifier: GPL-3.0-or-later

package canonical

import (
	"errors"
	"reflect"
	"testing"
)

func TestEntityEditingPreservesStableOrderAndIDs(t *testing.T) {
	document := Empty()
	var err error
	document, err = document.CreateEntity(CollectionNodes, map[string]any{"id": "node-a", "kind": "outbound", "enabled": true})
	if err != nil {
		t.Fatal(err)
	}
	document, err = document.CreateEntity(CollectionNodes, map[string]any{"id": "node-b", "kind": "inbound", "enabled": true})
	if err != nil {
		t.Fatal(err)
	}
	document, err = document.MoveEntity(CollectionNodes, "node-b", "node-a")
	if err != nil {
		t.Fatal(err)
	}
	document, err = document.SetEntityEnabled(CollectionNodes, "node-a", false)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := document.Entities(CollectionNodes)
	if err != nil {
		t.Fatal(err)
	}
	if got := []any{nodes[0]["id"], nodes[1]["id"], nodes[1]["enabled"]}; !reflect.DeepEqual(got, []any{"node-b", "node-a", false}) {
		t.Fatalf("nodes = %#v", nodes)
	}
	if _, err := document.ReplaceEntity(CollectionNodes, "node-a", map[string]any{"id": "renamed", "kind": "outbound", "enabled": true}); err == nil {
		t.Fatal("ReplaceEntity() allowed an ID change")
	}
}

func TestDeleteEntityRejectsRemainingReferences(t *testing.T) {
	document, err := Parse([]byte(`{
      "schema_version":1,
      "global":{},
      "nodes":[{"id":"node-a","kind":"outbound","enabled":true}],
      "rules":[{"id":"rule-a","enabled":true,"action":{"outbound_id":"node-a"}}],
      "subscription":{}
    }`))
	if err != nil {
		t.Fatal(err)
	}
	_, err = document.DeleteEntity(CollectionNodes, "node-a")
	if !errors.Is(err, ErrEntityReferenced) {
		t.Fatalf("DeleteEntity() error = %v, want ErrEntityReferenced", err)
	}

	document, err = document.DeleteEntity(CollectionRules, "rule-a")
	if err != nil {
		t.Fatal(err)
	}
	document, err = document.DeleteEntity(CollectionNodes, "node-a")
	if err != nil {
		t.Fatal(err)
	}
	nodes, _ := document.Entities(CollectionNodes)
	if len(nodes) != 0 {
		t.Fatalf("nodes after delete = %#v", nodes)
	}
}
