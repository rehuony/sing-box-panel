// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var ErrTaskRetryUnsupported = errors.New("this task cannot be retried from panel logs")

// RetryTask queues a new, idempotent maintenance attempt. Runtime operations and
// imported temporary files require a new explicit action rather than replaying
// stale generations, configuration snapshots or upload paths.
func (a *Application) RetryTask(ctx context.Context, id string) (Task, error) {
	previous, err := a.database.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if previous.Status != store.TaskStatusFailed && previous.Status != store.TaskStatusCanceled {
		return Task{}, ErrTaskRetryUnsupported
	}
	var payload any
	switch previous.Kind {
	case store.TaskKindCatalogRefresh:
		payload = CatalogRefreshOptions{}
	case store.TaskKindCoreInstall:
		var old coreInstallPayload
		if json.Unmarshal(previous.Payload, &old) != nil {
			return Task{}, ErrTaskRetryUnsupported
		}
		asset, err := a.catalogAsset(ctx, old.Asset.AssetID)
		if err != nil {
			return Task{}, err
		}
		if _, err = asset.TrustedDigest(); err != nil {
			return Task{}, err
		}
		payload = coreInstallPayload{Asset: asset}
	case store.TaskKindSubscriptionSourceRefresh:
		var old subscriptionSourceRefreshPayload
		if json.Unmarshal(previous.Payload, &old) != nil {
			return Task{}, ErrTaskRetryUnsupported
		}
		source, err := a.database.GetSubscriptionSource(ctx, old.SourceID)
		if err != nil {
			return Task{}, err
		}
		if source.SourceKind != store.SubscriptionSourceRemote || !source.Enabled {
			return Task{}, ErrTaskRetryUnsupported
		}
		if _, err = decodeRemoteSubscriptionSourceConfig(source.Config); err != nil {
			return Task{}, err
		}
		payload = subscriptionSourceRefreshPayload{SourceID: source.ID, ExpectedUpdatedAt: source.UpdatedAt}
	default:
		return Task{}, ErrTaskRetryUnsupported
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Task{}, err
	}
	return a.queueMaintenanceTask(ctx, previous.Kind, raw, "panel-retry:"+id)
}
