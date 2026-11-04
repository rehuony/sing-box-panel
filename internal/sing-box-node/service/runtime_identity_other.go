// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package service

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

type runtimeIdentityUnavailableInspector struct{}

func runtimeIdentityPlatformInspector() runtimeIdentityProcessInspector {
	return runtimeIdentityUnavailableInspector{}
}

func (runtimeIdentityUnavailableInspector) Verify(context.Context, store.RuntimeObservation, store.CoreArtifact) error {
	return ErrInspectionUnavailable
}

func (runtimeIdentityUnavailableInspector) ProcessStartToken(context.Context, int) (string, error) {
	return "", ErrInspectionUnavailable
}
