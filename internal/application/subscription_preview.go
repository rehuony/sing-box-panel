// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func (application *Application) RenderSubscriptionPreview(
	ctx context.Context,
	userID string,
	channelID string,
) (SubscriptionPreview, error) {
	state, err := application.database.LoadSubscriptionPreviewState(
		ctx, strings.TrimSpace(userID), strings.TrimSpace(channelID),
	)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	rendered, err := application.renderSubscriptionState(state)
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
