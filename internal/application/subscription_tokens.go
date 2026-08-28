// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func (application *Application) CreateSubscriptionToken(
	ctx context.Context,
	request CreateSubscriptionTokenRequest,
) (CreatedSubscriptionToken, error) {
	plaintext, digest, err := application.newSubscriptionTokenSecret()
	if err != nil {
		return CreatedSubscriptionToken{}, err
	}
	id, err := application.newID("token")
	if err != nil {
		return CreatedSubscriptionToken{}, err
	}
	now := application.now().UTC()
	stored, err := application.database.CreateSubscriptionToken(ctx, store.SubscriptionToken{
		ID: id, UserID: strings.TrimSpace(request.UserID), Label: strings.TrimSpace(request.Label),
		TokenSHA256: digest, Enabled: true, ExpiresAt: cloneTime(request.ExpiresAt), CreatedAt: now,
	})
	if err != nil {
		return CreatedSubscriptionToken{}, err
	}
	return CreatedSubscriptionToken{
		Metadata: applicationSubscriptionToken(stored, now),
		Token:    plaintext,
	}, nil
}

func (application *Application) SubscriptionToken(
	ctx context.Context,
	tokenID string,
) (SubscriptionToken, error) {
	now := application.now().UTC()
	stored, err := application.database.GetSubscriptionToken(ctx, strings.TrimSpace(tokenID))
	if err != nil {
		return SubscriptionToken{}, err
	}
	return applicationSubscriptionToken(stored, now), nil
}

func (application *Application) ListSubscriptionTokens(
	ctx context.Context,
	request SubscriptionListRequest,
) (SubscriptionTokenPage, error) {
	now := application.now().UTC()
	stored, err := application.database.ListSubscriptionTokens(ctx, store.SubscriptionTokenListFilter{
		Cursor: storeSubscriptionCursor(request.Cursor), Limit: request.Limit,
	})
	if err != nil {
		return SubscriptionTokenPage{}, err
	}
	page := SubscriptionTokenPage{Items: make([]SubscriptionToken, len(stored.Items))}
	for index, token := range stored.Items {
		page.Items[index] = applicationSubscriptionToken(token, now)
	}
	page.Next = applicationSubscriptionCursor(stored.Next)
	return page, nil
}

func (application *Application) AuthenticateSubscriptionToken(
	ctx context.Context,
	plaintext string,
) (SubscriptionToken, error) {
	if plaintext == "" || len(plaintext) > 512 {
		return SubscriptionToken{}, store.ErrSubscriptionTokenNotFound
	}
	digest := sha256.Sum256([]byte(plaintext))
	now := application.now().UTC()
	stored, err := application.database.FindActiveSubscriptionToken(
		ctx,
		hex.EncodeToString(digest[:]),
		now,
	)
	if err != nil {
		return SubscriptionToken{}, err
	}
	return applicationSubscriptionToken(stored, now), nil
}

func (application *Application) RotateSubscriptionToken(
	ctx context.Context,
	tokenID string,
	expiresAt *time.Time,
) (SubscriptionTokenRotation, error) {
	tokenID = strings.TrimSpace(tokenID)
	current, err := application.database.GetSubscriptionToken(ctx, tokenID)
	if err != nil {
		return SubscriptionTokenRotation{}, err
	}
	plaintext, digest, err := application.newSubscriptionTokenSecret()
	if err != nil {
		return SubscriptionTokenRotation{}, err
	}
	replacementID, err := application.newID("token")
	if err != nil {
		return SubscriptionTokenRotation{}, err
	}
	now := application.now().UTC()
	stored, err := application.database.RotateSubscriptionToken(
		ctx,
		current.ID,
		store.SubscriptionToken{
			ID: replacementID, UserID: current.UserID, Label: current.Label,
			TokenSHA256: digest, Enabled: true, ExpiresAt: cloneTime(expiresAt),
		},
		now,
	)
	if err != nil {
		return SubscriptionTokenRotation{}, err
	}
	return SubscriptionTokenRotation{
		Revoked: applicationSubscriptionToken(stored.Revoked, now),
		Created: applicationSubscriptionToken(stored.Created, now),
		Token:   plaintext,
	}, nil
}

func (application *Application) RevokeSubscriptionToken(
	ctx context.Context,
	tokenID string,
) (SubscriptionToken, error) {
	now := application.now().UTC()
	stored, err := application.database.RevokeSubscriptionToken(ctx, strings.TrimSpace(tokenID), now)
	if err != nil {
		return SubscriptionToken{}, err
	}
	return applicationSubscriptionToken(stored, now), nil
}

func (application *Application) SetSubscriptionTokenEnabled(
	ctx context.Context,
	tokenID string,
	enabled bool,
) (SubscriptionToken, error) {
	now := application.now().UTC()
	stored, err := application.database.SetSubscriptionTokenEnabled(ctx, strings.TrimSpace(tokenID), enabled)
	if err != nil {
		return SubscriptionToken{}, err
	}
	return applicationSubscriptionToken(stored, now), nil
}

func (application *Application) DeleteSubscriptionToken(ctx context.Context, tokenID string) error {
	return application.database.DeleteSubscriptionToken(ctx, strings.TrimSpace(tokenID))
}
