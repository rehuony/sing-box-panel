// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"context"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func (application *Application) PanelLogs(ctx context.Context, filter store.PanelLogFilter) (store.PanelLogPage, error) {
	return application.database.ListPanelLogs(ctx, filter)
}
