// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (application *Service) CreateSubscriptionChannel(
	ctx context.Context,
	request CreateSubscriptionChannelRequest,
) (SubscriptionChannel, error) {
	config, err := validateSubscriptionChannelConfig(request.Format, request.Config)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	stored, err := storeChannelConfig(config)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	id, err := database.NewID(application.random, "channel")
	if err != nil {
		return SubscriptionChannel{}, err
	}
	channel, err := application.database.CreateSubscriptionChannel(ctx, store.SubscriptionChannel{
		ID: id, Name: request.Name, Format: request.Format,
		Config: stored, Enabled: request.Enabled, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return SubscriptionChannel{}, err
	}
	return applicationSubscriptionChannel(channel)
}

func (application *Service) SubscriptionChannel(
	ctx context.Context,
	channelID string,
) (SubscriptionChannel, error) {
	stored, err := application.database.GetSubscriptionChannel(ctx, strings.TrimSpace(channelID))
	if err != nil {
		return SubscriptionChannel{}, err
	}
	return applicationSubscriptionChannel(stored)
}

func (application *Service) ListSubscriptionChannels(
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

// UpdateSubscriptionChannel replaces a channel with its groups, members and
// rules while its updated_at is still the expected one.
func (application *Service) UpdateSubscriptionChannel(
	ctx context.Context,
	channelID string,
	request UpdateSubscriptionChannelRequest,
) (SubscriptionChannel, error) {
	config, err := validateSubscriptionChannelConfig(request.Format, request.Config)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	stored, err := storeChannelConfig(config)
	if err != nil {
		return SubscriptionChannel{}, err
	}
	channel, err := application.database.UpdateSubscriptionChannel(ctx, store.UpdateSubscriptionChannelInput{
		Channel: store.SubscriptionChannel{
			ID: strings.TrimSpace(channelID), Name: request.Name, Format: request.Format, Config: stored, Enabled: request.Enabled,
			UpdatedAt: application.nextSubscriptionUpdateTime(request.ExpectedUpdatedAt),
		},
		ExpectedUpdatedAt: request.ExpectedUpdatedAt,
	})
	if err != nil {
		return SubscriptionChannel{}, err
	}
	return applicationSubscriptionChannel(channel)
}

func (application *Service) DeleteSubscriptionChannel(
	ctx context.Context,
	channelID string,
	expectedUpdatedAt time.Time,
) error {
	return application.database.DeleteSubscriptionChannel(ctx, strings.TrimSpace(channelID), expectedUpdatedAt)
}
