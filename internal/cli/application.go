// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"github.com/rehuony/sing-box-panel/internal/application"
)

type openApplicationFunc func(context.Context, string) (*application.Application, error)

func openApplication(
	ctx context.Context,
	settingsPath string,
	open openApplicationFunc,
) (*application.Application, error) {
	if open == nil {
		return nil, &Error{Kind: ErrorUnavailable, Code: "application_unavailable", Message: "application services are unavailable"}
	}
	instance, err := open(ctx, settingsPath)
	if err != nil {
		return nil, &Error{Kind: ErrorValidation, Code: "application_open_failed", Message: err.Error(), Cause: err}
	}
	return instance, nil
}
