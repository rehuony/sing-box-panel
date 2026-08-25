// SPDX-License-Identifier: GPL-3.0-or-later

// Package runtimeidentity verifies that a persisted runtime observation still
// names the exact live Linux process and immutable executable artifact.
package runtimeidentity

import (
	"context"
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/store"
)

var (
	ErrNoRunningCore         = errors.New("no sing-box core is currently running")
	ErrStaleObservation      = errors.New("the recorded sing-box runtime identity is stale")
	ErrInspectionUnavailable = errors.New("live sing-box identity inspection is unavailable")
)

type Identity struct {
	PID                int    `json:"pid"`
	ProcessStartToken  string `json:"process_start_token"`
	ExactCoreVersion   string `json:"exact_core_version"`
	CoreArtifactID     string `json:"core_artifact_id"`
	ArchiveSHA256      string `json:"archive_sha256"`
	BinarySHA256       string `json:"binary_sha256"`
	ActivationBundleID string `json:"activation_bundle_id"`
}

type observationStore interface {
	RuntimeObservation(context.Context) (store.RuntimeObservation, error)
	GetCoreArtifact(context.Context, string) (store.CoreArtifact, error)
}

type ProcessInspector interface {
	Verify(context.Context, store.RuntimeObservation, store.CoreArtifact) error
	ProcessStartToken(context.Context, int) (string, error)
}

type Resolver struct {
	store     observationStore
	inspector ProcessInspector
}

func New(database observationStore) *Resolver {
	return &Resolver{store: database, inspector: platformInspector()}
}

func NewWithInspector(database observationStore, inspector ProcessInspector) *Resolver {
	return &Resolver{store: database, inspector: inspector}
}

func (resolver *Resolver) Resolve(ctx context.Context) (Identity, error) {
	if resolver == nil || resolver.store == nil || resolver.inspector == nil {
		return Identity{}, ErrInspectionUnavailable
	}
	observation, err := resolver.store.RuntimeObservation(ctx)
	if errors.Is(err, store.ErrRuntimeObservationNotFound) {
		return Identity{}, ErrNoRunningCore
	}
	if err != nil {
		return Identity{}, fmt.Errorf("read runtime identity: %w", err)
	}
	artifact, err := resolver.store.GetCoreArtifact(ctx, observation.CoreArtifactID)
	if err != nil {
		return Identity{}, fmt.Errorf("read runtime artifact: %w", err)
	}
	if err := resolver.inspector.Verify(ctx, observation, artifact); err != nil {
		if errors.Is(err, ErrInspectionUnavailable) {
			return Identity{}, err
		}
		return Identity{}, fmt.Errorf("%w: %v", ErrStaleObservation, err)
	}
	return Identity{
		PID:                observation.PID,
		ProcessStartToken:  observation.ProcessStartToken,
		ExactCoreVersion:   observation.ExactCoreVersion,
		CoreArtifactID:     observation.CoreArtifactID,
		ArchiveSHA256:      observation.ArchiveSHA256,
		BinarySHA256:       observation.BinarySHA256,
		ActivationBundleID: observation.ActivationBundleID,
	}, nil
}

func (resolver *Resolver) ProcessStartToken(ctx context.Context, pid int) (string, error) {
	if resolver == nil || resolver.inspector == nil {
		return "", ErrInspectionUnavailable
	}
	return resolver.inspector.ProcessStartToken(ctx, pid)
}
