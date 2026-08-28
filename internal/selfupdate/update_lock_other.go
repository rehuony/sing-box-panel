// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !darwin && !linux

package selfupdate

import (
	"context"
	"errors"
)

type updateLock struct{}

func acquireUpdateLock(context.Context, string) (*updateLock, error) {
	return nil, errors.New("update locking is unsupported on this platform")
}

func (*updateLock) Close() error { return nil }
