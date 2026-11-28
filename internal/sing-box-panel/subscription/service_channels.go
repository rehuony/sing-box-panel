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

// ListSubscriptionChannels returns every channel's summary in the channels'
// saved order.
func (application *Service) ListSubscriptionChannels(ctx context.Context) (SubscriptionChannelList, error) {
	stored, err := application.database.ListSubscriptionChannels(ctx)
	if err != nil {
		return SubscriptionChannelList{}, err
	}
	pool, err := application.database.SubscriptionPool(ctx)
	if err != nil {
		return SubscriptionChannelList{}, err
	}
	nodes := channelNodes(pool)
	list := SubscriptionChannelList{Items: make([]SubscriptionChannelSummary, len(stored))}
	for index, channel := range stored {
		if list.Items[index], err = applicationSubscriptionChannelSummary(channel, nodes); err != nil {
			return SubscriptionChannelList{}, err
		}
	}
	return list, nil
}

// SetSubscriptionChannelOrder saves the channels' order. ids must name every
// channel exactly once; otherwise they changed since they were read and it
// fails with store.ErrSubscriptionChannelsChanged.
func (application *Service) SetSubscriptionChannelOrder(ctx context.Context, ids []string) error {
	return application.database.SetSubscriptionChannelOrder(ctx, ids)
}

// UpdateSubscriptionChannel replaces a channel with its groups, members and
// exit rules while its updated_at is still the expected one.
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
