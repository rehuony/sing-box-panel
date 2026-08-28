// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/rehuony/sing-box-panel/internal/taskrunner"
)

func acknowledgeCanonicalSave(
	ctx context.Context,
	task store.Task,
	control taskrunner.Control,
) (json.RawMessage, error) {
	if err := control.SafePoint(ctx); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"revision_id": task.CanonicalRevisionID})
}

func catalogRefreshHandler(commands *application.Application) taskrunner.HandlerFunc {
	return func(ctx context.Context, task store.Task, control taskrunner.Control) (json.RawMessage, error) {
		if err := control.SafePoint(ctx); err != nil {
			return nil, err
		}
		var options application.CatalogRefreshOptions
		if err := json.Unmarshal(task.Payload, &options); err != nil {
			return nil, fmt.Errorf("decode catalog refresh task: %w", err)
		}
		result, err := commands.RefreshCatalog(ctx, options)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	}
}

func subscriptionSourceRefreshHandler(commands *application.Application) taskrunner.HandlerFunc {
	return func(ctx context.Context, task store.Task, control taskrunner.Control) (json.RawMessage, error) {
		result, err := commands.ExecuteSubscriptionSourceRefresh(ctx, task.Payload, control.SafePoint)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	}
}

func coreArtifactHandler(
	commands *application.Application,
	artifacts application.ArtifactInstaller,
) taskrunner.HandlerFunc {
	return func(ctx context.Context, task store.Task, control taskrunner.Control) (json.RawMessage, error) {
		if err := control.SafePoint(ctx); err != nil {
			return nil, err
		}
		result, err := commands.ExecuteCoreArtifactTask(
			ctx,
			task.Kind,
			task.Payload,
			artifacts,
			control.SafePoint,
		)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	}
}

func prepareDataDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create panel data directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect panel data directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("panel data path is not a physical directory: %s", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("secure panel data directory: %w", err)
	}
	return nil
}
