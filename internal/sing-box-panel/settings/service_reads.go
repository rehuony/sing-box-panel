// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"context"
)

// WithCredentials runs action with the administrator credentials while holding
// the settings lock, so a cooperating credential writer cannot interleave a
// stale snapshot with session work.
func (app *Service) WithCredentials(ctx context.Context, action func(email, passwordHash string) error) error {
	if app.settingsPath == "" {
		return action(app.settings.Auth.Email, app.settings.Auth.PasswordHash)
	}
	lock, err := Lock(ctx, app.settingsPath)
	if err != nil {
		return err
	}
	defer lock.Close()
	value, _, err := app.CurrentLocked(ctx)
	if err != nil {
		return err
	}
	return action(value.Auth.Email, value.Auth.PasswordHash)
}

// TrafficAccounting reads only the traffic quota and period from the settings
// file, so accounting keeps working while unrelated fields are being edited.
func (app *Service) TrafficAccounting(ctx context.Context) (Traffic, error) {
	if app.settingsPath == "" {
		value := app.settings.Traffic
		if value.PeriodMonths == 0 {
			value.PeriodMonths = 1
		}
		return value, ValidateTrafficQuota(value.QuotaGiB)
	}
	lock, err := Lock(ctx, app.settingsPath)
	if err != nil {
		return Traffic{}, err
	}
	defer lock.Close()
	if err := app.Recover(ctx); err != nil {
		return Traffic{}, err
	}
	return LoadTrafficAccounting(app.settingsPath)
}
