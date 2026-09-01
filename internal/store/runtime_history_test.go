// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"testing"
	"time"
)

func TestRuntimeHistoryStartsWithUnknownInitializationMarker(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)

	page, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("initial runtime transitions = %d, want 1: %+v", len(page.Items), page.Items)
	}
	marker := page.Items[0]
	if marker.State != RuntimeTransitionUnknown || marker.Reason != "history_initialized" ||
		marker.UncertainSince == nil || !marker.UncertainSince.Equal(marker.OccurredAt) {
		t.Fatalf("runtime history initialization marker = %+v", marker)
	}
	if !page.HistoryStartedAt.Equal(marker.OccurredAt) || page.Preceding != nil || page.Next != nil {
		t.Fatalf("initial runtime history page = %+v", page)
	}
	if _, err := database.db.ExecContext(ctx, `UPDATE runtime_transitions SET reason = 'changed' WHERE id = ?`, marker.ID); err == nil {
		t.Fatal("runtime transition update unexpectedly succeeded")
	}
	if _, err := database.db.ExecContext(ctx, `DELETE FROM runtime_transitions WHERE id = ?`, marker.ID); err == nil {
		t.Fatal("runtime transition delete unexpectedly succeeded")
	}
}

func TestRuntimeHistoryPaginationFiltersPrecedingAndDedupe(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	initialized, err := database.LatestRuntimeTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := initialized.OccurredAt.Add(time.Minute)
	bundle, observation := seedAppliedRuntime(t, ctx, database, now)

	states := []RuntimeTransitionState{
		RuntimeTransitionRunning,
		RuntimeTransitionFailed,
		RuntimeTransitionStopped,
	}
	reasons := []string{"start_succeeded", "unexpected_exit", "stop_succeeded"}
	stored := make([]RuntimeTransition, 0, len(states))
	for index, state := range states {
		occurredAt := now.Add(time.Duration(index+10) * time.Second)
		input := runtimeTransitionTestInput(
			"history-page-"+reasons[index],
			state,
			reasons[index],
			bundle.ID,
			observation,
			occurredAt,
		)
		transition, err := database.AppendRuntimeTransition(ctx, input)
		if err != nil {
			t.Fatalf("AppendRuntimeTransition(%s) error = %v", state, err)
		}
		stored = append(stored, transition)
	}

	duplicate, err := database.AppendRuntimeTransition(ctx, runtimeTransitionTestInput(
		"history-page-start_succeeded",
		RuntimeTransitionRunning,
		"start_succeeded",
		bundle.ID,
		observation,
		now.Add(10*time.Second),
	))
	if err != nil || duplicate.ID != stored[0].ID {
		t.Fatalf("idempotent runtime transition = %+v, %v; want ID %d", duplicate, err, stored[0].ID)
	}
	conflict := runtimeTransitionTestInput(
		"history-page-start_succeeded",
		RuntimeTransitionRunning,
		"recovery_succeeded",
		bundle.ID,
		observation,
		now.Add(10*time.Second),
	)
	if _, err := database.AppendRuntimeTransition(ctx, conflict); !errors.Is(err, ErrRuntimeTransitionConflict) {
		t.Fatalf("conflicting runtime transition error = %v", err)
	}

	page, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{
		ActivationBundleID: bundle.ID,
		Limit:              2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != stored[2].ID || page.Items[1].ID != stored[1].ID || page.Next == nil {
		t.Fatalf("first runtime history page = %+v", page)
	}
	next, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{
		ActivationBundleID: bundle.ID,
		Cursor:             page.Next,
		Limit:              2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].ID != stored[0].ID || next.Next != nil {
		t.Fatalf("second runtime history page = %+v", next)
	}

	from := stored[1].OccurredAt
	filtered, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{
		From:               &from,
		State:              RuntimeTransitionFailed,
		ActivationBundleID: bundle.ID,
		Limit:              10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].ID != stored[1].ID || filtered.Preceding != nil {
		t.Fatalf("filtered runtime history = %+v", filtered)
	}

	rangePage, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{
		From:               &from,
		ActivationBundleID: bundle.ID,
		Limit:              10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rangePage.Preceding == nil || rangePage.Preceding.ID != stored[0].ID {
		t.Fatalf("runtime history preceding transition = %+v, want ID %d", rangePage.Preceding, stored[0].ID)
	}
}

func TestRuntimeObservationHeartbeatDoesNotProveStableRun(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	initialized, err := database.LatestRuntimeTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, observation := seedAppliedRuntime(t, ctx, database, initialized.OccurredAt.Add(time.Minute))
	heartbeatAt := observation.ObservedAt.Add(30 * time.Second)
	heartbeat, err := database.HeartbeatRuntimeObservation(
		ctx, observation.PID, observation.ProcessStartToken, heartbeatAt,
	)
	if err != nil || !heartbeat {
		t.Fatalf("HeartbeatRuntimeObservation() = %t, %v", heartbeat, err)
	}
	afterHeartbeat, err := database.RuntimeObservation(ctx)
	if err != nil || !afterHeartbeat.ObservedAt.Equal(heartbeatAt) || afterHeartbeat.StableObservedAt != nil {
		t.Fatalf("runtime observation after heartbeat = %+v, %v", afterHeartbeat, err)
	}

	stableAt := observation.StartedAt.Add(RuntimeRecoveryStableWindow)
	confirmed, err := database.ConfirmRuntimeObservation(
		ctx, observation.PID, observation.ProcessStartToken, stableAt,
	)
	if err != nil || !confirmed {
		t.Fatalf("ConfirmRuntimeObservation() = %t, %v", confirmed, err)
	}
	stable, err := database.RuntimeObservation(ctx)
	if err != nil || stable.StableObservedAt == nil || !stable.StableObservedAt.Equal(stableAt) {
		t.Fatalf("stable runtime observation = %+v, %v", stable, err)
	}
}

func TestRuntimeObservationTransitionFencePreservesNewIncarnation(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	initialized, err := database.LatestRuntimeTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, oldObservation := seedAppliedRuntime(t, ctx, database, initialized.OccurredAt.Add(time.Minute))
	newObservation := oldObservation
	newObservation.PID++
	newObservation.ProcessStartToken = "runtime-history-new-incarnation"
	newObservation.StartedAt = oldObservation.StartedAt.Add(time.Minute)
	newObservation.ObservedAt = newObservation.StartedAt.Add(time.Second)
	if _, err := database.RecordRuntimeObservation(ctx, newObservation); err != nil {
		t.Fatal(err)
	}
	transition := runtimeTransitionTestInput(
		"stale-incarnation-stop",
		RuntimeTransitionStopped,
		"stop_succeeded",
		oldObservation.ActivationBundleID,
		oldObservation,
		newObservation.ObservedAt.Add(time.Second),
	)
	cleared, err := database.ClearRuntimeObservationAndTransitions(
		ctx,
		oldObservation.PID,
		oldObservation.ProcessStartToken,
		[]RuntimeTransitionInput{transition},
	)
	if err != nil || cleared {
		t.Fatalf("stale runtime clear = %t, %v", cleared, err)
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != newObservation.PID || stored.ProcessStartToken != newObservation.ProcessStartToken {
		t.Fatalf("new runtime observation after stale clear = %+v, %v", stored, err)
	}
	page, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{Reason: "stop_succeeded", Limit: 10})
	if err != nil || len(page.Items) != 0 {
		t.Fatalf("history after stale clear = %+v, %v", page, err)
	}
}

func TestRuntimeRecoveryAtomicallyRecordsFailedIncarnation(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	initialized, err := database.LatestRuntimeTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := initialized.OccurredAt.Add(time.Minute)
	bundle, observation := seedAppliedRuntime(t, ctx, database, now)
	transition := runtimeTransitionTestInput(
		"recovery-failed-incarnation",
		RuntimeTransitionFailed,
		"unexpected_exit",
		bundle.ID,
		observation,
		observation.ObservedAt.Add(time.Second),
	)
	decision, err := database.RequestRuntimeRecovery(ctx, RuntimeRecoveryInput{
		TaskID:              "runtime-history-recovery",
		NewEpisodeID:        "runtime-history-episode",
		ExpectedBundleID:    bundle.ID,
		ExpectedGeneration:  1,
		ExpectedObservation: &observation,
		CreatedAt:           transition.OccurredAt,
		Transition:          &transition,
	})
	if err != nil || decision.Task == nil {
		t.Fatalf("RequestRuntimeRecovery() = %+v, %v", decision, err)
	}
	if _, err := database.RuntimeObservation(ctx); !errors.Is(err, ErrRuntimeObservationNotFound) {
		t.Fatalf("runtime observation after recovery = %v", err)
	}
	page, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{
		Reason: "unexpected_exit",
		Limit:  10,
	})
	if err != nil || len(page.Items) != 1 || page.Items[0].PID != observation.PID {
		t.Fatalf("failed runtime history after recovery = %+v, %v", page, err)
	}
}

func runtimeTransitionTestInput(
	dedupeKey string,
	state RuntimeTransitionState,
	reason string,
	bundleID string,
	observation RuntimeObservation,
	occurredAt time.Time,
) RuntimeTransitionInput {
	startedAt := observation.StartedAt
	return RuntimeTransitionInput{
		DedupeKey:          dedupeKey,
		State:              state,
		Reason:             reason,
		ActivationBundleID: bundleID,
		Generation:         1,
		PID:                observation.PID,
		ProcessStartToken:  observation.ProcessStartToken,
		ProcessStartedAt:   &startedAt,
		OccurredAt:         occurredAt,
	}
}
