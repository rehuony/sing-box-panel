// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !linux

package runtimeidentity

import (
	"context"

	"github.com/rehuony/sing-box-panel/internal/store"
)

type unavailableInspector struct{}

func platformInspector() ProcessInspector { return unavailableInspector{} }

func (unavailableInspector) Verify(context.Context, store.RuntimeObservation, store.CoreArtifact) error {
	return ErrInspectionUnavailable
}

func (unavailableInspector) ProcessStartToken(context.Context, int) (string, error) {
	return "", ErrInspectionUnavailable
}
