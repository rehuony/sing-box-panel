// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// A saved panel override wins over the legacy per-channel host. It only changes
// generated client endpoints, never listener addresses, ports or explicit SNI.
func (app *Service) publicationHost(ctx context.Context, _ store.SubscriptionNodeControls, fallback string) (string, error) {
	return app.publicationHostWithResolver(ctx, fallback, app.publicIP)
}

func (app *Service) publicationHostWithResolver(ctx context.Context, fallback string, resolve func(context.Context) string) (string, error) {
	value, _, err := app.settings.Current(ctx)
	if err != nil {
		return "", err
	}
	if value.Panel.PublicNodeHost != "" {
		return value.Panel.PublicNodeHost, nil
	}
	if fallback != "" {
		return fallback, nil
	}
	if resolve != nil {
		return resolve(ctx), nil
	}
	return "", nil
}

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
