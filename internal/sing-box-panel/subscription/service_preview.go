// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (application *Service) RenderSubscriptionPreview(
	ctx context.Context,
	userID string,
	channelID string,
) (SubscriptionPreview, error) {
	return application.RenderSubscriptionDraft(ctx, userID, channelID, nil)
}

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
	if draft != nil {
		if _, err := validateSubscriptionChannelConfig(draft.Format, draft.Config); err != nil {
			return SubscriptionPreview{}, err
		}
		state.Channel.Config = draft.Config
		state.Channel.Format = draft.Format
	}
	rendered, err := application.renderSubscriptionState(ctx, state)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	return SubscriptionPreview{
		UserID: userID, AppliedBundleID: state.AppliedBundleID,
		Channel:           applicationSubscriptionChannel(state.Channel),
		StartupArtifactID: state.Startup.ID, CanonicalRevisionID: state.Startup.CanonicalRevisionID,
		ExactCoreVersion: state.Startup.ExactCoreVersion, ArtifactState: state.Startup.State,
		Result: SubscriptionPreviewResult{
			Format: rendered.Format, MediaType: rendered.MediaType,
			Content: rendered.Content, NodeCount: rendered.NodeCount, Diagnostics: rendered.PreviewDiagnostics,
		},
	}, nil
}

func inboundPreviewDiagnostics(startupJSON []byte, diagnostics []ConversionDiagnostic, format RenderFormat) []PreviewDiagnostic {
	result := make([]PreviewDiagnostic, 0, len(diagnostics))
	if len(diagnostics) == 0 {
		return result
	}
	root, _ := DecodeDocumentObject(startupJSON)
	for _, diagnostic := range diagnostics {
		issue := PreviewDiagnostic{
			RenderDiagnostic: RenderDiagnostic{
				Collection: diagnostic.Collection, ItemIndex: diagnostic.ItemIndex, Code: diagnostic.Code, Format: format,
			},
			FieldPath: fmt.Sprintf("%s[%d]", diagnostic.Collection, diagnostic.ItemIndex),
		}
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
