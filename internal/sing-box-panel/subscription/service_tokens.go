// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

func (application *Service) CreateSubscriptionToken(
	ctx context.Context,
	request CreateSubscriptionTokenRequest,
) (CreatedSubscriptionToken, error) {
	plaintext, digest, err := application.newSubscriptionTokenSecret()
	if err != nil {
		return CreatedSubscriptionToken{}, err
	}
	id, err := database.NewID(application.random, "token")
	if err != nil {
		return CreatedSubscriptionToken{}, err
	}
	now := application.now().UTC()
	stored, err := application.database.CreateSubscriptionToken(ctx, store.SubscriptionToken{
		ID: id, UserID: strings.TrimSpace(request.UserID), Label: strings.TrimSpace(request.Label),
		TokenSHA256: digest, Secret: plaintext, Enabled: true, ExpiresAt: store.CloneTime(request.ExpiresAt), CreatedAt: now, DownloadLimit: request.DownloadLimit,
	})
	if err != nil {
		return CreatedSubscriptionToken{}, err
	}
	return CreatedSubscriptionToken{
		Metadata: applicationSubscriptionToken(stored, now),
		Token:    plaintext,
	}, nil
}

// SubscriptionTokenSecret is exposed only through authenticated administration.
func (application *Service) SubscriptionTokenSecret(ctx context.Context, tokenID string) (SubscriptionTokenSecret, error) {
	secret, err := application.database.SubscriptionTokenSecret(ctx, strings.TrimSpace(tokenID))
	if err != nil {
		return SubscriptionTokenSecret{}, err
	}
	return SubscriptionTokenSecret{Token: secret}, nil
}

func (application *Service) SubscriptionToken(
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

// ListSubscriptionTokens returns every key in its saved order.
func (application *Service) ListSubscriptionTokens(ctx context.Context) (SubscriptionTokenList, error) {
	now := application.now().UTC()
	stored, err := application.database.ListSubscriptionTokens(ctx)
	if err != nil {
		return SubscriptionTokenList{}, err
	}
	list := SubscriptionTokenList{Items: make([]SubscriptionToken, len(stored))}
	for index, token := range stored {
		list.Items[index] = applicationSubscriptionToken(token, now)
	}
	return list, nil
}

// SetSubscriptionTokenOrder saves the keys' order. ids must name every key
// exactly once; otherwise they changed since they were read and it fails
// with store.ErrSubscriptionTokensChanged.
func (application *Service) SetSubscriptionTokenOrder(ctx context.Context, ids []string) error {
	return application.database.SetSubscriptionTokenOrder(ctx, ids)
}

func (application *Service) AuthenticateSubscriptionToken(
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

func (application *Service) RotateSubscriptionToken(
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
	replacementID, err := database.NewID(application.random, "token")
	if err != nil {
		return SubscriptionTokenRotation{}, err
	}
	now := application.now().UTC()
	stored, err := application.database.RotateSubscriptionToken(
		ctx,
		current.ID,
		store.SubscriptionToken{
			ID: replacementID, UserID: current.UserID, Label: current.Label,
			TokenSHA256: digest, Secret: plaintext, ExpiresAt: store.CloneTime(expiresAt),
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

func (application *Service) RevokeSubscriptionToken(
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

func (application *Service) SetSubscriptionTokenEnabled(
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

func (application *Service) DeleteSubscriptionToken(ctx context.Context, tokenID string) error {
	return application.database.DeleteSubscriptionToken(ctx, strings.TrimSpace(tokenID))
}
