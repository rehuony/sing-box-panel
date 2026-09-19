// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux && !darwin

package settings

import (
	"context"
	"errors"
	"os"
)

func Lock(context.Context, string) (*os.File, error) {
	return nil, errors.New("settings writes require Linux or macOS")
}

func preserveOwner(*os.File, string) error { return nil }

func TryLock(ctx context.Context, path string) (*os.File, error) { return Lock(ctx, path) }
