// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (application *Service) CreateSubscriptionUser(
	ctx context.Context,
	request CreateSubscriptionUserRequest,
) (SubscriptionUser, error) {
	id, err := database.NewID(application.random, "user")
	if err != nil {
		return SubscriptionUser{}, err
	}
	stored, err := application.database.CreateSubscriptionUser(ctx, store.SubscriptionUser{
		ID: id, Name: request.Name, Description: request.Description,
		Enabled: request.Enabled, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return SubscriptionUser{}, err
	}
	return applicationSubscriptionUser(stored), nil
}

func (application *Service) SubscriptionUser(ctx context.Context, userID string) (SubscriptionUser, error) {
	stored, err := application.database.GetSubscriptionUser(ctx, strings.TrimSpace(userID))
	if err != nil {
		return SubscriptionUser{}, err
	}
	return applicationSubscriptionUser(stored), nil
}

func (application *Service) ListSubscriptionUsers(
	ctx context.Context,
	request SubscriptionListRequest,
) (SubscriptionUserPage, error) {
	stored, err := application.database.ListSubscriptionUsers(ctx, store.SubscriptionUserListFilter{
		Cursor: storeSubscriptionCursor(request.Cursor), Limit: request.Limit,
	})
	if err != nil {
		return SubscriptionUserPage{}, err
	}
	page := SubscriptionUserPage{Items: make([]SubscriptionUser, len(stored.Items))}
	for index, user := range stored.Items {
		page.Items[index] = applicationSubscriptionUser(user)
	}
	page.Next = applicationSubscriptionCursor(stored.Next)
	return page, nil
}

func (application *Service) UpdateSubscriptionUser(
	ctx context.Context,
	userID string,
	request UpdateSubscriptionUserRequest,
) (SubscriptionUser, error) {
	stored, err := application.database.UpdateSubscriptionUser(ctx, store.UpdateSubscriptionUserInput{
		ID: strings.TrimSpace(userID), Name: request.Name, Description: request.Description,
		Enabled: request.Enabled, ExpectedUpdatedAt: request.ExpectedUpdatedAt,
		UpdatedAt: application.nextSubscriptionUpdateTime(request.ExpectedUpdatedAt),
	})
	if err != nil {
		return SubscriptionUser{}, err
	}
	return applicationSubscriptionUser(stored), nil
}

func (application *Service) DeleteSubscriptionUser(
	ctx context.Context,
	userID string,
	expectedUpdatedAt time.Time,
) error {
	return application.database.DeleteSubscriptionUser(ctx, strings.TrimSpace(userID), expectedUpdatedAt)
}

func (application *Service) SubscriptionUserGrants(
	ctx context.Context,
	userID string,
) (SubscriptionUserGrants, error) {
	stored, err := application.database.SubscriptionUserGrants(ctx, strings.TrimSpace(userID))
	if err != nil {
		return SubscriptionUserGrants{}, err
	}
	return SubscriptionUserGrants{User: applicationSubscriptionUser(stored.User), Grants: stored.Grants}, nil
}

func (application *Service) ReplaceSubscriptionUserGrants(
	ctx context.Context,
	userID string,
	grants []string,
	expectedUpdatedAt time.Time,
) (SubscriptionUserGrants, error) {
	catalog, err := application.SubscriptionNodeCatalog(ctx)
	if err != nil {
		return SubscriptionUserGrants{}, err
	}
	available := make(map[string]struct{}, len(catalog.Nodes))
	for _, node := range catalog.Nodes {
		available[node.Key] = struct{}{}
	}
	for _, key := range grants {
		if _, exists := available[key]; !exists {
			return SubscriptionUserGrants{}, errors.New("subscription node grant is not in the current catalog")
		}
	}
	stored, err := application.database.ReplaceSubscriptionUserGrants(
		ctx, strings.TrimSpace(userID), grants, expectedUpdatedAt,
		application.nextSubscriptionUpdateTime(expectedUpdatedAt),
	)
	if err != nil {
		return SubscriptionUserGrants{}, err
	}
	return SubscriptionUserGrants{User: applicationSubscriptionUser(stored.User), Grants: stored.Grants}, nil
}
