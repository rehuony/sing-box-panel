// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/subscription"
)

func (application *Application) CreateSubscriptionSource(
	ctx context.Context,
	request CreateSubscriptionSourceRequest,
) (SubscriptionSource, error) {
	id, err := application.newID("source")
	if err != nil {
		return SubscriptionSource{}, err
	}
	now := application.now().UTC()
	source := store.SubscriptionSource{
		ID: id, Name: request.Name, SourceKind: request.SourceKind,
		Config:  request.Config,
		Enabled: request.Enabled, CreatedAt: now, UpdatedAt: now,
	}
	refreshTask, err := application.configuredSubscriptionSourceRefreshTask(source)
	if err != nil {
		return SubscriptionSource{}, err
	}
	stored, err := application.database.CreateSubscriptionSourceAndTask(ctx, source, refreshTask)
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
	request SubscriptionListRequest,
) (SubscriptionSourcePage, error) {
	stored, err := application.database.ListSubscriptionSources(ctx, store.SubscriptionSourceListFilter{
		Cursor: storeSubscriptionCursor(request.Cursor), Limit: request.Limit,
	})
	if err != nil {
		return SubscriptionSourcePage{}, err
	}
	page := SubscriptionSourcePage{Items: make([]SubscriptionSourceSummary, len(stored.Items))}
	for index, source := range stored.Items {
		page.Items[index] = applicationSubscriptionSourceSummary(source)
	}
	page.Next = applicationSubscriptionCursor(stored.Next)
	return page, nil
}

func (application *Application) UpdateSubscriptionSource(
	ctx context.Context,
	sourceID string,
	request UpdateSubscriptionSourceRequest,
) (SubscriptionSource, error) {
	updatedAt := application.nextSubscriptionUpdateTime(request.ExpectedUpdatedAt)
	prospective := store.SubscriptionSource{
		ID: strings.TrimSpace(sourceID), Name: request.Name, SourceKind: request.SourceKind,
		Config: request.Config, Enabled: request.Enabled, UpdatedAt: updatedAt,
	}
	refreshTask, err := application.configuredSubscriptionSourceRefreshTask(prospective)
	if err != nil {
		return SubscriptionSource{}, err
	}
	stored, err := application.database.UpdateSubscriptionSource(ctx, store.UpdateSubscriptionSourceInput{
		ID: strings.TrimSpace(sourceID), Name: request.Name, SourceKind: request.SourceKind,
		Config: request.Config, Enabled: request.Enabled,
		ExpectedUpdatedAt: request.ExpectedUpdatedAt,
		UpdatedAt:         updatedAt,
		RefreshTask:       refreshTask,
	})
	if err != nil {
		return SubscriptionSource{}, err
	}
	return applicationSubscriptionSource(stored), nil
}

func (application *Application) CreateSubscriptionSourceVersion(
	ctx context.Context,
	sourceID string,
	request CreateSubscriptionSourceVersionRequest,
) (SubscriptionSourceVersionSave, error) {
	sourceID = strings.TrimSpace(sourceID)
	nodes, detected, err := subscription.ParseSource(request.Format, request.RawBody, sourceID)
	if err != nil {
		return SubscriptionSourceVersionSave{}, err
	}
	normalizedNodes, err := json.Marshal(nodes)
	if err != nil {
		return SubscriptionSourceVersionSave{}, fmt.Errorf("encode normalized subscription nodes: %w", err)
	}
	versionID, err := application.newID("version")
	if err != nil {
		return SubscriptionSourceVersionSave{}, err
	}
	now := application.now().UTC()
	source, err := application.database.GetSubscriptionSource(ctx, sourceID)
	if err != nil {
		return SubscriptionSourceVersionSave{}, err
	}
	updatedAt := application.nextSubscriptionUpdateTime(request.ExpectedUpdatedAt)
	source.UpdatedAt = updatedAt
	refreshTask, err := application.configuredSubscriptionSourceRefreshTask(source)
	if err != nil {
		return SubscriptionSourceVersionSave{}, err
	}
	fetchedAt := request.FetchedAt.UTC()
	if request.FetchedAt.IsZero() {
		fetchedAt = now
	}
	stored, err := application.database.SaveSubscriptionSourceVersion(ctx, store.SaveSubscriptionSourceVersionInput{
		Version: store.SubscriptionSourceVersion{
			ID: versionID, SourceID: sourceID, Format: string(detected),
			RawBody: request.RawBody, NormalizedNodes: normalizedNodes,
			Diagnostics: json.RawMessage(`[]`), FetchedAt: fetchedAt, CreatedAt: now,
		},
		ExpectedSourceUpdatedAt: request.ExpectedUpdatedAt,
		UpdatedAt:               updatedAt,
		RefreshTask:             refreshTask,
	})
	if err != nil {
		return SubscriptionSourceVersionSave{}, err
	}
	return SubscriptionSourceVersionSave{
		Source:  applicationSubscriptionSource(stored.Source),
		Version: applicationSubscriptionSourceVersion(stored.Version, true),
	}, nil
}

func (application *Application) SubscriptionSourceVersion(
	ctx context.Context,
	sourceID string,
	versionID string,
) (SubscriptionSourceVersion, error) {
	stored, err := application.database.GetSubscriptionSourceVersion(
		ctx, strings.TrimSpace(sourceID), strings.TrimSpace(versionID),
	)
	if err != nil {
		return SubscriptionSourceVersion{}, err
	}
	return applicationSubscriptionSourceVersion(stored, true), nil
}

func (application *Application) ListSubscriptionSourceVersions(
	ctx context.Context,
	sourceID string,
	request SubscriptionListRequest,
) (SubscriptionSourceVersionPage, error) {
	stored, err := application.database.ListSubscriptionSourceVersions(ctx, store.SubscriptionSourceVersionListFilter{
		SourceID: strings.TrimSpace(sourceID), Cursor: storeSubscriptionCursor(request.Cursor), Limit: request.Limit,
	})
	if err != nil {
		return SubscriptionSourceVersionPage{}, err
	}
	page := SubscriptionSourceVersionPage{Items: make([]SubscriptionSourceVersion, len(stored.Items))}
	for index, version := range stored.Items {
		page.Items[index] = applicationSubscriptionSourceVersion(version, false)
	}
	page.Next = applicationSubscriptionCursor(stored.Next)
	return page, nil
}

func (application *Application) RestoreSubscriptionSourceVersion(
	ctx context.Context,
	sourceID string,
	versionID string,
	expectedUpdatedAt time.Time,
) (SubscriptionSource, error) {
	sourceID = strings.TrimSpace(sourceID)
	updatedAt := application.nextSubscriptionUpdateTime(expectedUpdatedAt)
	source, err := application.database.GetSubscriptionSource(ctx, sourceID)
	if err != nil {
		return SubscriptionSource{}, err
	}
	source.UpdatedAt = updatedAt
	refreshTask, err := application.configuredSubscriptionSourceRefreshTask(source)
	if err != nil {
		return SubscriptionSource{}, err
	}
	stored, err := application.database.ActivateSubscriptionSourceVersionAndTask(
		ctx, sourceID, strings.TrimSpace(versionID), expectedUpdatedAt,
		updatedAt, refreshTask,
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
