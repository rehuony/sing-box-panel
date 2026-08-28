// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription/node"
	"github.com/rehuony/sing-box-panel/internal/subscription/source"
	"github.com/rehuony/sing-box-panel/internal/subscriptionfetch"
)

const minimumSubscriptionRefreshIntervalMinutes = 15

type remoteSubscriptionSourceConfig struct {
	URL                    string        `json:"url"`
	Format                 source.Format `json:"format,omitempty"`
	RefreshIntervalMinutes int           `json:"refresh_interval_minutes,omitempty"`
}

type subscriptionSourceRefreshPayload struct {
	SourceID          string    `json:"source_id"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
}

type SubscriptionSourceRefreshResult struct {
	SourceID  string    `json:"source_id"`
	VersionID string    `json:"version_id"`
	Format    string    `json:"format"`
	SHA256    string    `json:"sha256"`
	NodeCount int       `json:"node_count"`
	FetchedAt time.Time `json:"fetched_at"`
}

func (application *Application) QueueSubscriptionSourceRefresh(
	ctx context.Context,
	sourceID string,
) (Task, error) {
	source, err := application.database.GetSubscriptionSource(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return Task{}, err
	}
	if source.SourceKind != store.SubscriptionSourceRemote {
		return Task{}, errors.New("subscription source is not remote")
	}
	if _, err := decodeRemoteSubscriptionSourceConfig(source.Config); err != nil {
		return Task{}, err
	}
	return application.enqueueSubscriptionSourceRefresh(ctx, source, nil)
}

func (application *Application) ExecuteSubscriptionSourceRefresh(
	ctx context.Context,
	payload json.RawMessage,
	safePoint func(context.Context) error,
) (SubscriptionSourceRefreshResult, error) {
	var input subscriptionSourceRefreshPayload
	if err := jsonstrict.Decode(payload, 64<<10, &input); err != nil {
		return SubscriptionSourceRefreshResult{}, errors.New("invalid subscription source refresh task")
	}
	source, err := application.database.GetSubscriptionSource(ctx, strings.TrimSpace(input.SourceID))
	if err != nil {
		return SubscriptionSourceRefreshResult{}, err
	}
	if !source.UpdatedAt.Equal(input.ExpectedUpdatedAt.UTC()) {
		return SubscriptionSourceRefreshResult{}, store.ErrSubscriptionConflict
	}
	if source.SourceKind != store.SubscriptionSourceRemote {
		return SubscriptionSourceRefreshResult{}, errors.New("subscription source is not remote")
	}
	config, err := decodeRemoteSubscriptionSourceConfig(source.Config)
	if err != nil {
		return SubscriptionSourceRefreshResult{}, err
	}
	if safePoint != nil {
		if err := safePoint(ctx); err != nil {
			return SubscriptionSourceRefreshResult{}, err
		}
	}
	body, fetchErr := subscriptionfetch.Fetch(ctx, config.URL, application.settings.Subscription.PrivateSourceCIDRs)
	if fetchErr != nil {
		_ = application.scheduleNextSubscriptionSourceRefresh(ctx, source, config)
		return SubscriptionSourceRefreshResult{}, fetchErr
	}
	if safePoint != nil {
		if err := safePoint(ctx); err != nil {
			return SubscriptionSourceRefreshResult{}, err
		}
	}
	saved, saveErr := application.CreateSubscriptionSourceVersion(ctx, source.ID, CreateSubscriptionSourceVersionRequest{
		Format: config.Format, RawBody: body, ExpectedUpdatedAt: source.UpdatedAt,
		FetchedAt: application.now().UTC(),
	})
	if saveErr != nil {
		_ = application.scheduleNextSubscriptionSourceRefresh(ctx, source, config)
		return SubscriptionSourceRefreshResult{}, saveErr
	}
	var nodes []node.Node
	if err := json.Unmarshal(saved.Version.NormalizedNodes, &nodes); err != nil {
		return SubscriptionSourceRefreshResult{}, err
	}
	return SubscriptionSourceRefreshResult{
		SourceID: saved.Source.ID, VersionID: saved.Version.ID, Format: saved.Version.Format,
		SHA256: saved.Version.SHA256, NodeCount: len(nodes), FetchedAt: saved.Version.FetchedAt,
	}, nil
}

func (application *Application) scheduleConfiguredSubscriptionSourceRefresh(
	ctx context.Context,
	source store.SubscriptionSource,
) error {
	if source.SourceKind != store.SubscriptionSourceRemote || !source.Enabled {
		return nil
	}
	config, err := decodeRemoteSubscriptionSourceConfig(source.Config)
	if err != nil {
		return err
	}
	return application.scheduleNextSubscriptionSourceRefresh(ctx, source, config)
}

func decodeRemoteSubscriptionSourceConfig(raw json.RawMessage) (remoteSubscriptionSourceConfig, error) {
	var config remoteSubscriptionSourceConfig
	if err := jsonstrict.Decode(raw, 64<<10, &config); err != nil {
		return remoteSubscriptionSourceConfig{}, errors.New("invalid remote subscription source config")
	}
	if config.URL == "" || config.URL != strings.TrimSpace(config.URL) {
		return remoteSubscriptionSourceConfig{}, errors.New("remote subscription source URL is required")
	}
	if config.Format == "" {
		config.Format = source.FormatAuto
	}
	switch config.Format {
	case source.FormatAuto, source.FormatSingBoxJSON,
		source.FormatMihomoYAML, source.FormatURIList:
	default:
		return remoteSubscriptionSourceConfig{}, errors.New("invalid remote subscription source format")
	}
	if config.RefreshIntervalMinutes != 0 && config.RefreshIntervalMinutes < minimumSubscriptionRefreshIntervalMinutes {
		return remoteSubscriptionSourceConfig{}, fmt.Errorf("subscription refresh interval must be zero or at least %d minutes", minimumSubscriptionRefreshIntervalMinutes)
	}
	return config, nil
}

func (application *Application) scheduleNextSubscriptionSourceRefresh(
	ctx context.Context,
	source store.SubscriptionSource,
	config remoteSubscriptionSourceConfig,
) error {
	if config.RefreshIntervalMinutes == 0 {
		return nil
	}
	notBefore := application.now().UTC().Add(time.Duration(config.RefreshIntervalMinutes) * time.Minute)
	_, err := application.enqueueSubscriptionSourceRefresh(ctx, source, &notBefore)
	return err
}

func (application *Application) enqueueSubscriptionSourceRefresh(
	ctx context.Context,
	source store.SubscriptionSource,
	notBefore *time.Time,
) (Task, error) {
	payload, err := json.Marshal(subscriptionSourceRefreshPayload{
		SourceID: source.ID, ExpectedUpdatedAt: source.UpdatedAt,
	})
	if err != nil {
		return Task{}, err
	}
	taskID, err := application.newID("task")
	if err != nil {
		return Task{}, err
	}
	keySuffix := "manual"
	if notBefore != nil {
		keySuffix = notBefore.UTC().Format(time.RFC3339Nano)
	}
	key := "subscription-source-refresh:" + source.ID + ":" +
		source.UpdatedAt.UTC().Format(time.RFC3339Nano) + ":" + keySuffix
	queued, err := application.database.EnqueueTask(ctx, store.EnqueueTaskInput{
		ID: taskID, IdempotencyKey: key, Lane: store.TaskLaneMaintenance,
		Kind: "subscription-source-refresh", Payload: payload,
		NotBefore: notBefore, CreatedAt: application.now().UTC(),
	})
	if err != nil {
		return Task{}, err
	}
	return applicationTask(queued), nil
}
