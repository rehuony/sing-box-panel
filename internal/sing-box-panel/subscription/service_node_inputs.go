// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func manualPublicationNodes(values []store.ManualSubscriptionNode) ([]Node, error) {
	nodes := make([]Node, 0, len(values))
	for _, value := range values {
		parsed, _, err := ParseSource(SourceFormatSingBoxJSON, value.Outbound, "manual")
		if err != nil || len(parsed) != 1 {
			return nil, store.ErrSubscriptionNodeInvalid
		}
		node := parsed[0]
		node.Key = "manual:" + value.ID
		nodes = append(nodes, node)
	}
	return nodes, nil
}
