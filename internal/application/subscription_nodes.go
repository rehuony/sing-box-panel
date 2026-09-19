// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"

	"github.com/rehuony/sing-box-panel/internal/singbox"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

type SubscriptionNodeWrite struct {
	Outbound json.RawMessage `json:"outbound"`
	Revision int64           `json:"revision"`
}

func (app *Application) CreateSubscriptionNode(ctx context.Context, raw json.RawMessage) (SubscriptionNodeDetail, error) {
	id, err := app.newID("manual")
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	return app.saveSubscriptionNode(ctx, id, raw, 0, SubscriptionNodeSummary{})
}

func (app *Application) UpdateSubscriptionNode(ctx context.Context, id string, input SubscriptionNodeWrite) (SubscriptionNodeDetail, error) {
	entry, err := app.subscriptionNodeEntry(ctx, id)
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	if entry.ManualID == "" {
		return SubscriptionNodeDetail{}, store.ErrSubscriptionNodeInvalid
	}
	return app.saveSubscriptionNode(ctx, entry.ManualID, input.Outbound, input.Revision, entry.Summary)
}

func (app *Application) saveSubscriptionNode(ctx context.Context, id string, raw json.RawMessage, expected int64, previous SubscriptionNodeSummary) (SubscriptionNodeDetail, error) {
	if err := subscription.ValidateManualNode(raw); err != nil {
		return SubscriptionNodeDetail{}, err
	}
	if err := singbox.ValidateSubscriptionNodeSchema(raw); err != nil {
		return SubscriptionNodeDetail{}, &subscription.NodeFieldError{Code: "native_schema_violation"}
	}
	stored, err := app.database.SaveManualSubscriptionNode(ctx, store.ManualSubscriptionNode{ID: id, Outbound: raw, UpdatedAt: app.now().UTC()}, expected)
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	nodes, err := manualPublicationNodes([]store.ManualSubscriptionNode{stored})
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	summary, err := summarizeSubscriptionNode(nodes[0])
	if err != nil {
		return SubscriptionNodeDetail{}, err
	}
	summary.SourceName, summary.Origin, summary.Available = "手动节点", "manual", true
	summary.Revision, summary.Hidden, summary.VisibilityRevision = stored.Revision, previous.Hidden, previous.VisibilityRevision
	return SubscriptionNodeDetail{SubscriptionNodeSummary: summary, OutboundJSON: string(stored.Outbound)}, nil
}

func (app *Application) DeleteSubscriptionNode(ctx context.Context, id string, expected int64) error {
	entry, err := app.subscriptionNodeEntry(ctx, id)
	if err != nil {
		return err
	}
	if entry.ManualID == "" {
		return store.ErrSubscriptionNodeInvalid
	}
	return app.database.DeleteManualSubscriptionNode(ctx, entry.ManualID, expected, id)
}

func (app *Application) SetSubscriptionNodeVisibility(ctx context.Context, id string, hidden bool, expected int64) (SubscriptionNodeSummary, error) {
	entry, err := app.subscriptionNodeEntry(ctx, id)
	if err != nil {
		return SubscriptionNodeSummary{}, err
	}
	value, err := app.database.SetSubscriptionNodeVisibility(ctx, id, hidden, expected)
	if err != nil {
		return SubscriptionNodeSummary{}, err
	}
	entry.Summary.Hidden, entry.Summary.VisibilityRevision = value.Hidden, value.Revision
	return entry.Summary, nil
}

// ParseSubscriptionNode performs no I/O and never saves on paste. The caller
// confirms the normalized single node through the regular create endpoint.
func (app *Application) ParseSubscriptionNode(raw string) (json.RawMessage, error) {
	if len(raw) > store.MaximumManualNodeBytes {
		return nil, store.ErrSubscriptionNodeInvalid
	}
	nodes, _, err := subscription.ParseSource(subscription.SourceFormatAuto, []byte(raw), "manual")
	if err != nil || len(nodes) != 1 {
		return nil, store.ErrSubscriptionNodeInvalid
	}
	return nodes[0].Outbound, nil
}
