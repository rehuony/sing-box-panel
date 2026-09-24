// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

// A saved panel override wins over the legacy per-channel host. It only changes
// generated client endpoints, never listener addresses, ports or explicit SNI.
func (app *Application) publicationHost(ctx context.Context, _ store.SubscriptionNodeControls, fallback string) (string, error) {
	value, _, err := app.currentSettings(ctx)
	if err != nil {
		return "", err
	}
	if value.Panel.PublicNodeHost != "" {
		return value.Panel.PublicNodeHost, nil
	}
	if fallback != "" {
		return fallback, nil
	}
	if app.publicIP != nil {
		return app.publicIP(ctx), nil
	}
	return "", nil
}

func manualPublicationNodes(values []store.ManualSubscriptionNode) ([]subscription.Node, error) {
	nodes := make([]subscription.Node, 0, len(values))
	for _, value := range values {
		parsed, _, err := subscription.ParseSource(subscription.SourceFormatSingBoxJSON, value.Outbound, "manual")
		if err != nil || len(parsed) != 1 {
			return nil, store.ErrSubscriptionNodeInvalid
		}
		node := parsed[0]
		node.Key = "manual:" + value.ID
		nodes = append(nodes, node)
	}
	return nodes, nil
}
