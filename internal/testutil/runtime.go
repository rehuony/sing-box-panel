// SPDX-License-Identifier: GPL-3.0-or-later

// Package testutil provides persisted runtime fixtures for transport tests.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
)

// ApplyBundle commits a verified process fixture then models its clean exit.
// It never launches a process; runtime execution is covered by server tests.
func ApplyBundle(t *testing.T, database *store.Store, bundleID string) {
	t.Helper()
	ctx := context.Background()
	bundle, err := database.GetActivationBundle(ctx, bundleID)
	if err != nil {
		t.Fatal(err)
	}
	startup, err := database.GetStartupArtifact(ctx, bundle.StartupArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	core, err := database.GetCoreArtifact(ctx, startup.CoreArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := database.RequestRuntimeIntent(ctx, store.RuntimeIntentInput{Kind: store.RuntimeIntentApply, BundleID: bundleID, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	var expected *store.RuntimeObservation
	previous, err := database.RuntimeObservation(ctx)
	if err == nil {
		expected = &previous
	} else if !errors.Is(err, store.ErrRuntimeObservationNotFound) {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	observation := store.RuntimeObservation{PID: 12345, ProcessStartToken: fmt.Sprintf("fixture-%d", intent.Generation), CoreArtifactID: core.ID, ActivationBundleID: bundleID, ExactCoreVersion: core.ExactVersion, ArchiveSHA256: core.ArchiveSHA256, BinarySHA256: core.BinarySHA256, StartedAt: at, ObservedAt: at}
	transition := store.RuntimeTransitionInput{DedupeKey: fmt.Sprintf("fixture-%d", intent.Generation), State: store.RuntimeTransitionRunning, Reason: "fixture_running", Generation: intent.Generation, ActivationBundleID: bundleID, PID: observation.PID, ProcessStartToken: observation.ProcessStartToken, ProcessStartedAt: &at, OccurredAt: at}
	if err := database.CompleteRuntimeIntent(ctx, intent, true, &store.RuntimeCommit{ExpectedObservation: expected, Observation: &observation, Transitions: []store.RuntimeTransitionInput{transition}}, at); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ClearRuntimeObservation(ctx, observation.PID, observation.ProcessStartToken); err != nil {
		t.Fatal(err)
	}
}
