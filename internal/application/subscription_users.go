// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func (application *Application) CreateSubscriptionUser(
	ctx context.Context,
	request CreateSubscriptionUserRequest,
) (SubscriptionUser, error) {
	id, err := application.newID("user")
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

func (application *Application) SubscriptionUser(ctx context.Context, userID string) (SubscriptionUser, error) {
	stored, err := application.database.GetSubscriptionUser(ctx, strings.TrimSpace(userID))
	if err != nil {
		return SubscriptionUser{}, err
	}
	return applicationSubscriptionUser(stored), nil
}

func (application *Application) ListSubscriptionUsers(
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

func (application *Application) UpdateSubscriptionUser(
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

func (application *Application) DeleteSubscriptionUser(
	ctx context.Context,
	userID string,
	expectedUpdatedAt time.Time,
) error {
	return application.database.DeleteSubscriptionUser(ctx, strings.TrimSpace(userID), expectedUpdatedAt)
}

func (application *Application) SubscriptionUserGrants(
	ctx context.Context,
	userID string,
) (SubscriptionUserGrants, error) {
	stored, err := application.database.SubscriptionUserGrants(ctx, strings.TrimSpace(userID))
	if err != nil {
		return SubscriptionUserGrants{}, err
	}
	return SubscriptionUserGrants{User: applicationSubscriptionUser(stored.User), Grants: stored.Grants}, nil
}

func (application *Application) ReplaceSubscriptionUserGrants(
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
