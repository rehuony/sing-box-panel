// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import "sort"

// NodeOrder stores publication IDs independently of source contents and policies.
// A missing record has revision zero; absent nodes can retain their saved slots.
type NodeOrder struct {
	IDs      []string `json:"ids"`
	Revision int64    `json:"revision"`
}

func orderChannelOutbounds(values []outbound, ids []string) {
	if len(ids) == 0 {
		return
	}
	positions := make(map[string]int, len(ids))
	for i, id := range ids {
		positions[id] = i
	}
	sort.SliceStable(values, func(i, j int) bool {
		a, okA := positions[values[i].nodeID]
		b, okB := positions[values[j].nodeID]
		if okA != okB {
			return okA
		}
		return okA && a < b
	})
}
