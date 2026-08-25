// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
	subscriptionrender "github.com/rehuony/sing-box-panel/internal/subscription"
)

const subscriptionTokenEntropyBytes = 32

var ErrSubscriptionPreviewArtifactState = errors.New("startup artifact cannot be previewed")

type SubscriptionChannel struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Format    store.SubscriptionFormat `json:"format"`
	Config    json.RawMessage          `json:"config"`
	Enabled   bool                     `json:"enabled"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

type CreateSubscriptionChannelRequest struct {
	Name    string                   `json:"name"`
	Format  store.SubscriptionFormat `json:"format"`
	Config  json.RawMessage          `json:"config"`
	Enabled bool                     `json:"enabled"`
}

type UpdateSubscriptionChannelRequest struct {
	Name              string                   `json:"name"`
	Format            store.SubscriptionFormat `json:"format"`
	Config            json.RawMessage          `json:"config"`
	Enabled           bool                     `json:"enabled"`
	ExpectedUpdatedAt time.Time                `json:"expected_updated_at"`
}

type SubscriptionSource struct {
	ID             string                       `json:"id"`
	Name           string                       `json:"name"`
	SourceKind     store.SubscriptionSourceKind `json:"source_kind"`
	Config         json.RawMessage              `json:"config"`
	LatestSnapshot json.RawMessage              `json:"latest_snapshot,omitempty"`
	Enabled        bool                         `json:"enabled"`
	CreatedAt      time.Time                    `json:"created_at"`
	UpdatedAt      time.Time                    `json:"updated_at"`
}

type CreateSubscriptionSourceRequest struct {
	Name           string                       `json:"name"`
	SourceKind     store.SubscriptionSourceKind `json:"source_kind"`
	Config         json.RawMessage              `json:"config"`
	LatestSnapshot json.RawMessage              `json:"latest_snapshot,omitempty"`
	Enabled        bool                         `json:"enabled"`
}

type UpdateSubscriptionSourceRequest struct {
	Name              string                       `json:"name"`
	SourceKind        store.SubscriptionSourceKind `json:"source_kind"`
	Config            json.RawMessage              `json:"config"`
	Enabled           bool                         `json:"enabled"`
	ExpectedUpdatedAt time.Time                    `json:"expected_updated_at"`
}

type UpdateSubscriptionSourceSnapshotRequest struct {
	LatestSnapshot    json.RawMessage `json:"latest_snapshot"`
	ExpectedUpdatedAt time.Time       `json:"expected_updated_at"`
}

// SubscriptionToken deliberately omits both plaintext and token_sha256. The
// public plaintext is returned only by create/rotate result types.
type SubscriptionToken struct {
	ID        string     `json:"id"`
	ChannelID string     `json:"channel_id,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	Active    bool       `json:"active"`
}

type CreateSubscriptionTokenRequest struct {
	ChannelID string     `json:"channel_id,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type CreatedSubscriptionToken struct {
	Metadata SubscriptionToken `json:"metadata"`
	Token    string            `json:"token"`
}

type SubscriptionTokenRotation struct {
	Revoked SubscriptionToken `json:"revoked"`
	Created SubscriptionToken `json:"created"`
	Token   string            `json:"token"`
}

type SubscriptionPreview struct {
	Channel             SubscriptionChannel        `json:"channel"`
	StartupArtifactID   string                     `json:"startup_artifact_id"`
	CanonicalRevisionID string                     `json:"canonical_revision_id"`
	ExactCoreVersion    string                     `json:"exact_core_version"`
	ArtifactState       store.StartupArtifactState `json:"artifact_state"`
	Result              subscriptionrender.Result  `json:"result"`
}

func (application *Application) CreateSubscriptionChannel(
	ctx context.Context,
	request CreateSubscriptionChannelRequest,
) (SubscriptionChannel, error) {
	id, err := application.newID("channel")
	if err != nil {
		return SubscriptionChannel{}, err
	}
	stored, err := application.database.CreateSubscriptionChannel(ctx, store.SubscriptionChannel{
		ID: id, Name: request.Name, Format: request.Format,
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
) ([]SubscriptionChannel, error) {
	stored, err := application.database.ListSubscriptionChannels(ctx)
	if err != nil {
		return nil, err
	}
	channels := make([]SubscriptionChannel, len(stored))
	for index, channel := range stored {
		channels[index] = applicationSubscriptionChannel(channel)
	}
	return channels, nil
}

func (application *Application) UpdateSubscriptionChannel(
	ctx context.Context,
	channelID string,
	request UpdateSubscriptionChannelRequest,
) (SubscriptionChannel, error) {
	updatedAt := application.nextSubscriptionUpdateTime(request.ExpectedUpdatedAt)
	stored, err := application.database.UpdateSubscriptionChannel(ctx, store.UpdateSubscriptionChannelInput{
		ID: strings.TrimSpace(channelID), Name: request.Name, Format: request.Format,
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

func (application *Application) CreateSubscriptionSource(
	ctx context.Context,
	request CreateSubscriptionSourceRequest,
) (SubscriptionSource, error) {
	id, err := application.newID("source")
	if err != nil {
		return SubscriptionSource{}, err
	}
	stored, err := application.database.CreateSubscriptionSource(ctx, store.SubscriptionSource{
		ID: id, Name: request.Name, SourceKind: request.SourceKind,
		Config: request.Config, LatestSnapshot: request.LatestSnapshot,
		Enabled: request.Enabled, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return SubscriptionSource{}, err
	}
	return applicationSubscriptionSource(stored), nil
}

func (application *Application) SubscriptionSource(
	ctx context.Context,
	sourceID string,
) (SubscriptionSource, error) {
	stored, err := application.database.GetSubscriptionSource(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return SubscriptionSource{}, err
	}
	return applicationSubscriptionSource(stored), nil
}

func (application *Application) ListSubscriptionSources(
	ctx context.Context,
) ([]SubscriptionSource, error) {
	stored, err := application.database.ListSubscriptionSources(ctx)
	if err != nil {
		return nil, err
	}
	sources := make([]SubscriptionSource, len(stored))
	for index, source := range stored {
		sources[index] = applicationSubscriptionSource(source)
	}
	return sources, nil
}

func (application *Application) UpdateSubscriptionSource(
	ctx context.Context,
	sourceID string,
	request UpdateSubscriptionSourceRequest,
) (SubscriptionSource, error) {
	stored, err := application.database.UpdateSubscriptionSource(ctx, store.UpdateSubscriptionSourceInput{
		ID: strings.TrimSpace(sourceID), Name: request.Name, SourceKind: request.SourceKind,
		Config: request.Config, Enabled: request.Enabled,
		ExpectedUpdatedAt: request.ExpectedUpdatedAt,
		UpdatedAt:         application.nextSubscriptionUpdateTime(request.ExpectedUpdatedAt),
	})
	if err != nil {
		return SubscriptionSource{}, err
	}
	return applicationSubscriptionSource(stored), nil
}

func (application *Application) UpdateSubscriptionSourceSnapshot(
	ctx context.Context,
	sourceID string,
	request UpdateSubscriptionSourceSnapshotRequest,
) (SubscriptionSource, error) {
	stored, err := application.database.UpdateSubscriptionSourceSnapshot(
		ctx,
		store.UpdateSubscriptionSourceSnapshotInput{
			ID: strings.TrimSpace(sourceID), LatestSnapshot: request.LatestSnapshot,
			ExpectedUpdatedAt: request.ExpectedUpdatedAt,
			UpdatedAt:         application.nextSubscriptionUpdateTime(request.ExpectedUpdatedAt),
		},
	)
	if err != nil {
		return SubscriptionSource{}, err
	}
	return applicationSubscriptionSource(stored), nil
}

func (application *Application) DeleteSubscriptionSource(
	ctx context.Context,
	sourceID string,
	expectedUpdatedAt time.Time,
) error {
	return application.database.DeleteSubscriptionSource(ctx, strings.TrimSpace(sourceID), expectedUpdatedAt)
}

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
		ID: id, TokenSHA256: digest, ChannelID: strings.TrimSpace(request.ChannelID),
		ExpiresAt: cloneTime(request.ExpiresAt), CreatedAt: now,
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
) ([]SubscriptionToken, error) {
	now := application.now().UTC()
	stored, err := application.database.ListSubscriptionTokens(ctx)
	if err != nil {
		return nil, err
	}
	tokens := make([]SubscriptionToken, len(stored))
	for index, token := range stored {
		tokens[index] = applicationSubscriptionToken(token, now)
	}
	return tokens, nil
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
			ID: replacementID, TokenSHA256: digest, ChannelID: current.ChannelID,
			ExpiresAt: cloneTime(expiresAt),
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

// RenderSubscriptionPreview renders from one already-checked immutable startup
// artifact and one persisted channel. It performs no mutation and never writes
// public files. Stale artifacts remain reviewable because their bytes are
// frozen; pending and failed candidates have not passed the required check.
func (application *Application) RenderSubscriptionPreview(
	ctx context.Context,
	startupArtifactID string,
	channelID string,
) (SubscriptionPreview, error) {
	artifact, err := application.database.GetStartupArtifact(ctx, strings.TrimSpace(startupArtifactID))
	if err != nil {
		return SubscriptionPreview{}, err
	}
	if artifact.State != store.StartupArtifactReady && artifact.State != store.StartupArtifactStale {
		return SubscriptionPreview{}, fmt.Errorf(
			"%w: %s is %s",
			ErrSubscriptionPreviewArtifactState,
			artifact.ID,
			artifact.State,
		)
	}
	storedChannel, err := application.database.GetSubscriptionChannel(ctx, strings.TrimSpace(channelID))
	if err != nil {
		return SubscriptionPreview{}, err
	}
	config, err := store.DecodeSubscriptionChannelConfig(storedChannel.Config)
	if err != nil {
		return SubscriptionPreview{}, err
	}
	result, err := subscriptionrender.Render(artifact.ConfigBytes, subscriptionrender.Channel{
		Format:       subscriptionrender.Format(storedChannel.Format),
		ExcludeTags:  append([]string(nil), config.ExcludeTags...),
		ExcludeTypes: append([]string(nil), config.ExcludeTypes...),
	})
	if err != nil {
		return SubscriptionPreview{}, err
	}
	return SubscriptionPreview{
		Channel:             applicationSubscriptionChannel(storedChannel),
		StartupArtifactID:   artifact.ID,
		CanonicalRevisionID: artifact.CanonicalRevisionID,
		ExactCoreVersion:    artifact.ExactCoreVersion,
		ArtifactState:       artifact.State,
		Result:              result,
	}, nil
}

func (application *Application) newSubscriptionTokenSecret() (string, string, error) {
	raw := make([]byte, subscriptionTokenEntropyBytes)
	n, err := application.random(raw)
	if err != nil {
		return "", "", fmt.Errorf("generate subscription token: %w", err)
	}
	if n != len(raw) {
		return "", "", errors.New("generate subscription token: short random read")
	}
	plaintext := base64.RawURLEncoding.EncodeToString(raw)
	digest := sha256.Sum256([]byte(plaintext))
	return plaintext, hex.EncodeToString(digest[:]), nil
}

func (application *Application) nextSubscriptionUpdateTime(expected time.Time) time.Time {
	now := application.now().UTC()
	if !now.After(expected.UTC()) {
		return expected.UTC().Add(time.Nanosecond)
	}
	return now
}

func applicationSubscriptionChannel(value store.SubscriptionChannel) SubscriptionChannel {
	return SubscriptionChannel{
		ID: value.ID, Name: value.Name, Format: value.Format,
		Config: append(json.RawMessage(nil), value.Config...), Enabled: value.Enabled,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func applicationSubscriptionSource(value store.SubscriptionSource) SubscriptionSource {
	return SubscriptionSource{
		ID: value.ID, Name: value.Name, SourceKind: value.SourceKind,
		Config:         append(json.RawMessage(nil), value.Config...),
		LatestSnapshot: append(json.RawMessage(nil), value.LatestSnapshot...),
		Enabled:        value.Enabled, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func applicationSubscriptionToken(value store.SubscriptionToken, at time.Time) SubscriptionToken {
	return SubscriptionToken{
		ID: value.ID, ChannelID: value.ChannelID,
		ExpiresAt: cloneTime(value.ExpiresAt), RevokedAt: cloneTime(value.RevokedAt),
		CreatedAt: value.CreatedAt, Active: value.Active(at),
	}
}
