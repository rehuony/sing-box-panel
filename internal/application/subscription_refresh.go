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
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

const minimumSubscriptionRefreshIntervalMinutes = 15

type remoteSubscriptionSourceConfig struct {
	URL                    string                    `json:"url"`
	Format                 subscription.SourceFormat `json:"format,omitempty"`
	RefreshIntervalMinutes int                       `json:"refresh_interval_minutes,omitempty"`
}

type SubscriptionSourceRefreshResult struct {
	SourceID  string    `json:"source_id"`
	VersionID string    `json:"version_id"`
	Format    string    `json:"format"`
	SHA256    string    `json:"sha256"`
	NodeCount int       `json:"node_count"`
	FetchedAt time.Time `json:"fetched_at"`
}

func (application *Application) RefreshSubscriptionSource(ctx context.Context, sourceID string) (result SubscriptionSourceRefreshResult, operationErr error) {
	defer func() { application.RecordOperation(ctx, "subscription.refresh", "Subscription refresh", operationErr) }()
	source, err := application.database.GetSubscriptionSource(ctx, strings.TrimSpace(sourceID))
	if err != nil {
		return SubscriptionSourceRefreshResult{}, err
	}
	return application.refreshSubscriptionSource(ctx, source)
}
func (application *Application) refreshSubscriptionSource(ctx context.Context, source store.SubscriptionSource) (SubscriptionSourceRefreshResult, error) {
	if source.SourceKind != store.SubscriptionSourceRemote {
		return SubscriptionSourceRefreshResult{}, errors.New("subscription source is not remote")
	}
	config, err := decodeRemoteSubscriptionSourceConfig(source.Config)
	if err != nil {
		return SubscriptionSourceRefreshResult{}, err
	}
	body, fetchErr := subscription.FetchSource(ctx, config.URL, application.settings.Subscription.PrivateSourceCIDRs)
	if fetchErr != nil {
		_ = application.scheduleNextSubscriptionSourceRefresh(ctx, source, config)
		return SubscriptionSourceRefreshResult{}, fetchErr
	}
	saved, saveErr := application.CreateSubscriptionSourceVersion(ctx, source.ID, CreateSubscriptionSourceVersionRequest{
		Format: config.Format, RawBody: body, ExpectedUpdatedAt: source.UpdatedAt,
		FetchedAt: application.now().UTC(),
	})
	if saveErr != nil {
		_ = application.scheduleNextSubscriptionSourceRefresh(ctx, source, config)
		return SubscriptionSourceRefreshResult{}, saveErr
	}
	var nodes []subscription.Node
	if err := json.Unmarshal(saved.Version.NormalizedNodes, &nodes); err != nil {
		return SubscriptionSourceRefreshResult{}, err
	}
	return SubscriptionSourceRefreshResult{
		SourceID: saved.Source.ID, VersionID: saved.Version.ID, Format: saved.Version.Format,
		SHA256: saved.Version.SHA256, NodeCount: len(nodes), FetchedAt: saved.Version.FetchedAt,
	}, nil
}

func (application *Application) configuredSubscriptionSourceRefreshSchedule(source store.SubscriptionSource) (*store.SubscriptionRefreshSchedule, error) {
	if source.SourceKind != store.SubscriptionSourceRemote {
		return nil, nil
	}
	config, err := decodeRemoteSubscriptionSourceConfig(source.Config)
	if err != nil {
		return nil, err
	}
	if !source.Enabled || config.RefreshIntervalMinutes == 0 {
		return nil, nil
	}
	return &store.SubscriptionRefreshSchedule{SourceID: source.ID, ExpectedUpdatedAt: source.UpdatedAt, NextAt: application.now().UTC().Add(time.Duration(config.RefreshIntervalMinutes) * time.Minute)}, nil
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
		config.Format = subscription.SourceFormatAuto
	}
	switch config.Format {
	case subscription.SourceFormatAuto, subscription.SourceFormatSingBoxJSON,
		subscription.SourceFormatMihomoYAML, subscription.SourceFormatURIList:
	default:
		return remoteSubscriptionSourceConfig{}, errors.New("invalid remote subscription source format")
	}
	if config.RefreshIntervalMinutes != 0 && config.RefreshIntervalMinutes < minimumSubscriptionRefreshIntervalMinutes {
		return remoteSubscriptionSourceConfig{}, fmt.Errorf("subscription refresh interval must be zero or at least %d minutes", minimumSubscriptionRefreshIntervalMinutes)
	}
	return config, nil
}

func (application *Application) scheduleNextSubscriptionSourceRefresh(ctx context.Context, source store.SubscriptionSource, config remoteSubscriptionSourceConfig) error {
	// Retry an initial fetch, but do not enable periodic refresh for an already
	// populated manual-only source.
	if !source.Enabled || (config.RefreshIntervalMinutes == 0 && source.CurrentVersionID != "") {
		return nil
	}
	interval := config.RefreshIntervalMinutes
	if interval == 0 {
		interval = minimumSubscriptionRefreshIntervalMinutes
	}
	return application.database.ScheduleSubscriptionRefresh(ctx, store.SubscriptionRefreshSchedule{SourceID: source.ID, ExpectedUpdatedAt: source.UpdatedAt, NextAt: application.now().UTC().Add(time.Duration(interval) * time.Minute)})
}

func (application *Application) RefreshDueSubscriptionSources(ctx context.Context) error {
	due, err := application.database.DueSubscriptionRefreshes(ctx, application.now().UTC())
	if err != nil {
		return err
	}
	for _, entry := range due {
		if err := ctx.Err(); err != nil {
			return err
		}
		source, err := application.database.GetSubscriptionSource(ctx, entry.SourceID)
		if err != nil || !source.Enabled || !source.UpdatedAt.Equal(entry.ExpectedUpdatedAt) {
			continue
		}
		_, err = application.refreshSubscriptionSource(ctx, source)
		application.RecordOperation(ctx, "subscription.refresh", "Subscription source refresh", err)
	}
	return nil
}
