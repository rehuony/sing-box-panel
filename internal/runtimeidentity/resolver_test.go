// SPDX-License-Identifier: GPL-3.0-or-later

package runtimeidentity

import (
	"context"
	"errors"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestResolverRequiresVerifiedLiveObservation(t *testing.T) {
	observation := store.RuntimeObservation{
		PID: 42, ProcessStartToken: "start", CoreArtifactID: "core-a",
		ActivationBundleID: "bundle-a", ExactCoreVersion: "1.13.19",
		ArchiveSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BinarySHA256:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	artifact := store.CoreArtifact{
		ID: "core-a", ExactVersion: "1.13.19", ReportedVersion: "1.13.19",
		ArchiveSHA256: observation.ArchiveSHA256, BinarySHA256: observation.BinarySHA256,
	}
	inspector := &fakeInspector{}
	resolver := NewWithInspector(fakeObservationStore{observation: observation, artifact: artifact}, inspector)
	identity, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if identity.ExactCoreVersion != "1.13.19" || identity.PID != 42 || inspector.verified != 1 {
		t.Fatalf("identity=%+v verified=%d", identity, inspector.verified)
	}

	inspector.verifyErr = errors.New("gone")
	if _, err := resolver.Resolve(context.Background()); !errors.Is(err, ErrStaleObservation) {
		t.Fatalf("stale error=%v", err)
	}
}

func TestResolverNeverFallsBackWhenNoCoreIsRunning(t *testing.T) {
	resolver := NewWithInspector(fakeObservationStore{observationErr: store.ErrRuntimeObservationNotFound}, &fakeInspector{})
	if _, err := resolver.Resolve(context.Background()); !errors.Is(err, ErrNoRunningCore) {
		t.Fatalf("Resolve error=%v", err)
	}
}

type fakeObservationStore struct {
	observation    store.RuntimeObservation
	observationErr error
	artifact       store.CoreArtifact
	artifactErr    error
}

func (database fakeObservationStore) RuntimeObservation(context.Context) (store.RuntimeObservation, error) {
	return database.observation, database.observationErr
}

func (database fakeObservationStore) GetCoreArtifact(context.Context, string) (store.CoreArtifact, error) {
	return database.artifact, database.artifactErr
}

type fakeInspector struct {
	verified  int
	verifyErr error
}

func (inspector *fakeInspector) Verify(context.Context, store.RuntimeObservation, store.CoreArtifact) error {
	inspector.verified++
	return inspector.verifyErr
}

func (inspector *fakeInspector) ProcessStartToken(context.Context, int) (string, error) {
	return "start", nil
}
