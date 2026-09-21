// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func runtimeTransitionDedupeKey(parts ...string) string {
	digest := sha256.New()
	for _, part := range parts {
		_, _ = digest.Write([]byte(part))
		_, _ = digest.Write([]byte{0})
	}
	return "runtime_" + hex.EncodeToString(digest.Sum(nil))
}

func runtimeIntentTransitionKey(intent store.RuntimeIntent, phase string) string {
	return runtimeTransitionDedupeKey("generation", fmt.Sprint(intent.Generation), phase)
}

func runtimeIncarnationTransitionKey(
	observation store.RuntimeObservation,
	state store.RuntimeTransitionState,
	reason string,
) string {
	return runtimeTransitionDedupeKey(
		"incarnation",
		fmt.Sprint(observation.PID),
		observation.ProcessStartToken,
		observation.StartedAt.UTC().Format(time.RFC3339Nano),
		string(state),
		reason,
	)
}

func runtimeTransitionFromObservation(
	dedupeKey string,
	state store.RuntimeTransitionState,
	reason string,
	observation store.RuntimeObservation,
	occurredAt time.Time,
	uncertainSince *time.Time,
	intent store.RuntimeIntent,
) store.RuntimeTransitionInput {
	startedAt := observation.StartedAt
	return store.RuntimeTransitionInput{
		DedupeKey:          dedupeKey,
		State:              state,
		Reason:             reason,
		ActivationBundleID: observation.ActivationBundleID,
		Generation:         intent.Generation,
		PID:                observation.PID,
		ProcessStartToken:  observation.ProcessStartToken,
		ProcessStartedAt:   &startedAt,
		OccurredAt:         occurredAt,
		UncertainSince:     uncertainSince,
	}
}

func runtimeTransitionWithoutObservation(
	dedupeKey string,
	state store.RuntimeTransitionState,
	reason string,
	bundleID string,
	occurredAt time.Time,
	uncertainSince *time.Time,
	intent store.RuntimeIntent,
) store.RuntimeTransitionInput {
	return store.RuntimeTransitionInput{
		DedupeKey:          dedupeKey,
		State:              state,
		Reason:             reason,
		ActivationBundleID: bundleID,
		Generation:         intent.Generation,
		OccurredAt:         occurredAt,
		UncertainSince:     uncertainSince,
	}
}

func runtimeIntentReason(intent store.RuntimeIntent, suffix string) string {
	if runtimeIntentIsRecovery(intent) {
		return "recovery_" + suffix
	}
	kind := strings.TrimPrefix(string(intent.Kind), "runtime-")
	kind = strings.ReplaceAll(kind, "-", "_")
	if kind == "" {
		kind = "runtime"
	}
	return kind + "_" + suffix
}

func runtimeIntentIsRecovery(intent store.RuntimeIntent) bool { return intent.Recovery != nil }

func runtimeFailureReason(live coreruntime.LiveIdentity, fallback string) string {
	if live.Failure == nil {
		return fallback
	}
	switch live.Failure.Operation {
	case "health_check":
		return "health_check_failed"
	case "process":
		return "unexpected_exit"
	default:
		return fallback
	}
}

func runtimeTransitionTime(live coreruntime.LiveIdentity, fallback time.Time) time.Time {
	if !live.TransitionedAt.IsZero() {
		return live.TransitionedAt.UTC()
	}
	return fallback.UTC()
}

func runtimeFailureTime(live coreruntime.LiveIdentity, fallback time.Time) time.Time {
	if live.Failure != nil && !live.Failure.FailedAt.IsZero() {
		return live.Failure.FailedAt.UTC()
	}
	return runtimeTransitionTime(live, fallback)
}

func runtimeEventTime(value, floor time.Time) time.Time {
	value = value.UTC()
	if value.Before(floor) {
		return floor.UTC()
	}
	return value
}

func (services *runtimeServices) appendRuntimeTransition(
	ctx context.Context,
	transition store.RuntimeTransitionInput,
) error {
	_, err := services.database.AppendRuntimeTransition(ctx, transition)
	return err
}

func (services *runtimeServices) ensureRuntimeStopped(
	ctx context.Context,
	occurredAt time.Time,
	reason string,
) error {
	latest, err := services.database.LatestRuntimeTransition(ctx)
	if err != nil {
		return err
	}
	if latest.State == store.RuntimeTransitionStopped {
		return nil
	}
	transition := runtimeTransitionWithoutObservation(
		runtimeTransitionDedupeKey("known-stopped", fmt.Sprint(latest.ID), reason),
		store.RuntimeTransitionStopped,
		reason,
		latest.ActivationBundleID,
		runtimeEventTime(occurredAt, latest.OccurredAt),
		nil,
		store.RuntimeIntent{},
	)
	return services.appendRuntimeTransition(ctx, transition)
}

func (services *runtimeServices) runtimeIntentFailureCommit(
	intent store.RuntimeIntent,
	material application.RuntimeMaterial,
	captured *store.RuntimeObservation,
) (*store.RuntimeCommit, error) {
	live := services.manager.ObserveLiveIdentity()
	if live.Running {
		// A failed restart whose old process remains verified did not change the
		// observed runtime state, so intent failure must not masquerade as runtime
		// failure history.
		return nil, nil
	}
	reason := runtimeFailureReason(live, runtimeIntentReason(intent, "failed"))
	occurredAt := runtimeFailureTime(live, time.Now())
	if captured != nil {
		exited, err := services.capturedObservationExited(captured)
		if err != nil {
			return nil, errors.Join(errRuntimeEvidenceUnavailable, err)
		}
		if !exited {
			return nil, nil
		}
		occurredAt = runtimeEventTime(occurredAt, captured.ObservedAt)
	}
	transition := runtimeTransitionWithoutObservation(
		runtimeIntentTransitionKey(intent, "failed"),
		store.RuntimeTransitionFailed,
		reason,
		material.Activation.ID,
		occurredAt,
		nil,
		intent,
	)
	commit := &store.RuntimeCommit{
		ExpectedObservation: captured,
		ClearObservation:    true,
		Transitions:         []store.RuntimeTransitionInput{transition},
	}
	return commit, nil
}
