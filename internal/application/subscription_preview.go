// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"github.com/rehuony/sing-box-panel/internal/store"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func (application *Application) RenderSubscriptionPreview(
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

func (application *Application) RenderSubscriptionDraft(ctx context.Context, userID, channelID string, draft *SubscriptionDraftPreview) (SubscriptionPreview, error) {
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
		Result: subscription.RenderResult{
			Format: subscription.RenderFormat(rendered.Format), MediaType: rendered.MediaType,
			Content: rendered.Body, NodeCount: rendered.NodeCount, Diagnostics: rendered.Diagnostics,
		},
	}, nil
}
