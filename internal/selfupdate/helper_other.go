// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !darwin && !linux

package selfupdate

import (
	"context"
	"time"
)

func directoryWritable(string) bool { return false }

type HelperOptions struct {
	DataDir string
	Current string
	Now     func() time.Time
}

func (updater *Updater) ApplyStaged(context.Context, HelperOptions) (*Attempt, error) {
	return nil, ErrUnsupportedPlatform
}
