// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

type SubscriptionDraftPreview struct {
	Format store.SubscriptionFormat `json:"format"`
	Config json.RawMessage          `json:"config"`
}

func (application *Service) RenderSubscriptionDraft(ctx context.Context, userID, channelID string, draft *SubscriptionDraftPreview) (SubscriptionPreview, error) {
	state, err := application.database.LoadSubscriptionPreviewState(
		ctx, strings.TrimSpace(userID), strings.TrimSpace(channelID),
	)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	channel, err := applicationSubscriptionChannel(state.Channel)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	ruleSets, err := channelRuleSets(state.RuleSets)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	if draft != nil {
		config, err := validateSubscriptionChannelConfig(draft.Format, draft.Config)
		if err != nil {
			return SubscriptionPreview{}, err
		}
		pool, err := application.database.SubscriptionPool(ctx)
		if err != nil {
			return SubscriptionPreview{}, err
		}
		available := make(map[string]bool, len(pool))
		for _, node := range pool {
			available[node.ID] = true
		}
		if err := validateChannelPoolReferences(config, available); err != nil {
			return SubscriptionPreview{}, err
		}
		if ruleSets, err = application.readChannelRuleSets(ctx, config); err != nil {
			return SubscriptionPreview{}, err
		}
		channel.Config, channel.Format = config, draft.Format
	}
	rendered, _, err := application.renderSubscriptionState(state, channel.Format, channel.Config, ruleSets)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	publications, err := application.database.LoadNodePublications(ctx)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	locals := application.localNodes(publications)
	return SubscriptionPreview{
		UserID: userID, Channel: channel, NodePublications: locals.Summaries,
		Result: SubscriptionPreviewResult{
			Format: rendered.Format, MediaType: rendered.MediaType, Content: rendered.Content, NodeCount: rendered.NodeCount,
			Diagnostics: append(inboundPreviewDiagnostics(locals, rendered.Format), rendered.PreviewDiagnostics...),
		},
	}, nil
}

// inboundPreviewDiagnostics describes the diagnostics of every node's
// applied configuration, naming the node and, when there is one, the inbound.
func inboundPreviewDiagnostics(locals nodeLocals, format RenderFormat) []PreviewDiagnostic {
	result := make([]PreviewDiagnostic, 0, len(locals.Diagnostics))
	names := map[string]string{}
	for _, summary := range locals.Summaries {
		names[summary.NodeID] = summary.NodeName
	}
	for _, diagnostic := range locals.Diagnostics {
		issue := PreviewDiagnostic{
			RenderDiagnostic: RenderDiagnostic{
				Collection: diagnostic.Collection, ItemIndex: diagnostic.ItemIndex, Code: diagnostic.Code, Format: format,
			},
			FieldPath:    fmt.Sprintf("%s[%d]", diagnostic.Collection, diagnostic.ItemIndex),
			OriginNodeID: diagnostic.NodeID, OriginNodeName: names[diagnostic.NodeID],
		}
		root, _ := DecodeDocumentObject(locals.Documents[diagnostic.NodeID])
		values, _ := root[string(diagnostic.Collection)].([]any)
		if diagnostic.ItemIndex >= 0 && diagnostic.ItemIndex < len(values) {
			value, _ := values[diagnostic.ItemIndex].(map[string]any)
			if name, ok := value["tag"].(string); ok && ValidTag(name) {
				issue.NodeName = name
			}
			if protocol, ok := value["type"].(string); ok && ValidType(protocol) {
				issue.NodeType = protocol
			}
		}
		result = append(result, issue)
	}
	return result
}
