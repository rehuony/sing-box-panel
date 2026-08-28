// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func (application *Application) CreateSubscriptionChannel(
	ctx context.Context,
	request CreateSubscriptionChannelRequest,
) (SubscriptionChannel, error) {
	id, err := application.newID("channel")
	if err != nil {
		return SubscriptionChannel{}, err
	}
	stored, err := application.database.CreateSubscriptionChannel(ctx, store.SubscriptionChannel{
		ID: id, Name: request.Name, Format: request.Format, PublicHost: request.PublicHost,
		Config: request.Config, Enabled: request.Enabled, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return SubscriptionChannel{}, err
	}
	return applicationSubscriptionChannel(stored), nil
}

func (application *Application) SubscriptionChannel(
	ctx context.Context,
	channelID string,
) (SubscriptionChannel, error) {
	stored, err := application.database.GetSubscriptionChannel(ctx, strings.TrimSpace(channelID))
	if err != nil {
		return SubscriptionChannel{}, err
	}
	return applicationSubscriptionChannel(stored), nil
}

func (application *Application) ListSubscriptionChannels(
	ctx context.Context,
	request SubscriptionListRequest,
) (SubscriptionChannelPage, error) {
	stored, err := application.database.ListSubscriptionChannels(ctx, store.SubscriptionChannelListFilter{
		Cursor: storeSubscriptionCursor(request.Cursor), Limit: request.Limit,
	})
	if err != nil {
		return SubscriptionChannelPage{}, err
	}
	page := SubscriptionChannelPage{Items: make([]SubscriptionChannelSummary, len(stored.Items))}
	for index, channel := range stored.Items {
		page.Items[index] = applicationSubscriptionChannelSummary(channel)
	}
	page.Next = applicationSubscriptionCursor(stored.Next)
	return page, nil
}

func (application *Application) UpdateSubscriptionChannel(
	ctx context.Context,
	channelID string,
	request UpdateSubscriptionChannelRequest,
) (SubscriptionChannel, error) {
	updatedAt := application.nextSubscriptionUpdateTime(request.ExpectedUpdatedAt)
	stored, err := application.database.UpdateSubscriptionChannel(ctx, store.UpdateSubscriptionChannelInput{
		ID: strings.TrimSpace(channelID), Name: request.Name, Format: request.Format, PublicHost: request.PublicHost,
		Config: request.Config, Enabled: request.Enabled,
		ExpectedUpdatedAt: request.ExpectedUpdatedAt, UpdatedAt: updatedAt,
	})
	if err != nil {
		return SubscriptionChannel{}, err
	}
	return applicationSubscriptionChannel(stored), nil
}

func (application *Application) DeleteSubscriptionChannel(
	ctx context.Context,
	channelID string,
	expectedUpdatedAt time.Time,
) error {
	return application.database.DeleteSubscriptionChannel(ctx, strings.TrimSpace(channelID), expectedUpdatedAt)
}
