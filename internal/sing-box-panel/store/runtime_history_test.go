// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"errors"
	"testing"
	"time"
)

func TestRuntimeHistoryReplicatesEachNodeInstanceAppendOnly(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	empty, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{NodeID: testNodeID, Limit: 10})
	if err != nil || len(empty.Items) != 0 || empty.HistoryStartedAt.IsZero() {
		t.Fatalf("history before any report = %+v, %v; want no entries and the node's creation as its start", empty, err)
	}
	initialized := time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC)
	marker := RuntimeTransitionInput{
		NodeID: testNodeID, InstanceID: "instance-a", SourceID: 1, State: RuntimeTransitionUnknown,
		Reason: "history_initialized", OccurredAt: initialized, UncertainSince: &initialized,
	}
	// Reporting an entry again after reconnecting is harmless.
	for range 2 {
		if err := database.RecordRuntimeTransition(ctx, marker); err != nil {
			t.Fatal(err)
		}
	}
	page, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{NodeID: testNodeID, Limit: 10})
	if err != nil || len(page.Items) != 1 || !page.HistoryStartedAt.Equal(initialized) {
		t.Fatalf("history after the marker = %+v, %v", page, err)
	}
	conflict := marker
	conflict.Reason = "changed"
	if err := database.RecordRuntimeTransition(ctx, conflict); !errors.Is(err, ErrRuntimeTransitionConflict) {
		t.Fatalf("rewriting a recorded entry = %v, want ErrRuntimeTransitionConflict", err)
	}
	// A recreated node database numbers its history again.
	again := marker
	again.InstanceID, again.OccurredAt, again.UncertainSince = "instance-b", initialized.Add(time.Hour), new(initialized.Add(time.Hour))
	if err := database.RecordRuntimeTransition(ctx, again); err != nil {
		t.Fatal(err)
	}
	for instance, want := range map[string]int64{"instance-a": 1, "instance-b": 1, "instance-c": 0} {
		if latest, err := database.LatestRuntimeTransitionSource(ctx, testNodeID, instance); err != nil || latest != want {
			t.Fatalf("latest source of %s = %d, %v; want %d", instance, latest, err, want)
		}
	}
	if _, err := database.db.ExecContext(ctx, `UPDATE runtime_transitions SET reason = 'changed'`); err == nil {
		t.Fatal("runtime transition update unexpectedly succeeded")
	}
	// Removing a node removes its history.
	if err := database.DeleteNode(ctx, testNodeID); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := database.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_transitions`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("history left after removing the node = %d, %v", remaining, err)
	}
}

func TestRuntimeHistoryPaginationFiltersAndPreceding(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC)
	bundle := "bundle_history"
	observation := runtimeProcess{PID: 4242, StartedAt: now}

	states := []RuntimeTransitionState{RuntimeTransitionRunning, RuntimeTransitionFailed, RuntimeTransitionStopped}
	reasons := []string{"start_succeeded", "unexpected_exit", "stop_succeeded"}
	for index, state := range states {
		input := runtimeTransitionTestInput(int64(index+1), state, reasons[index], bundle, observation, now.Add(time.Duration(index+10)*time.Second))
		if err := database.RecordRuntimeTransition(ctx, input); err != nil {
			t.Fatalf("RecordRuntimeTransition(%s) error = %v", state, err)
		}
	}
	all, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{NodeID: testNodeID, Limit: 10})
	if err != nil || len(all.Items) != 3 {
		t.Fatalf("history = %+v, %v", all, err)
	}
	stored := []RuntimeTransition{all.Items[2], all.Items[1], all.Items[0]}

	page, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{NodeID: testNodeID, ActivationBundleID: bundle, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.Items[0].ID != stored[2].ID || page.Items[1].ID != stored[1].ID || page.Next == nil {
		t.Fatalf("first runtime history page = %+v", page)
	}
	next, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{NodeID: testNodeID, ActivationBundleID: bundle, Cursor: page.Next, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 1 || next.Items[0].ID != stored[0].ID || next.Next != nil {
		t.Fatalf("second runtime history page = %+v", next)
	}

	from := stored[1].OccurredAt
	filtered, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{
		NodeID: testNodeID, From: &from, State: RuntimeTransitionFailed, ActivationBundleID: bundle, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].ID != stored[1].ID || filtered.Preceding != nil {
		t.Fatalf("filtered runtime history = %+v", filtered)
	}

	rangePage, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{NodeID: testNodeID, From: &from, ActivationBundleID: bundle, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if rangePage.Preceding == nil || rangePage.Preceding.ID != stored[0].ID {
		t.Fatalf("runtime history preceding transition = %+v, want ID %d", rangePage.Preceding, stored[0].ID)
	}
	if _, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{Limit: 10}); err == nil {
		t.Fatal("history without a node succeeded")
	}
}

// runtimeProcess identifies the core process a transition records.
type runtimeProcess struct {
	PID       int
	StartedAt time.Time
}

func runtimeTransitionTestInput(
	sourceID int64,
	state RuntimeTransitionState,
	reason string,
	bundleID string,
	observation runtimeProcess,
	occurredAt time.Time,
) RuntimeTransitionInput {
	startedAt := observation.StartedAt
	return RuntimeTransitionInput{
		NodeID:             testNodeID,
		InstanceID:         "instance-a",
		SourceID:           sourceID,
		State:              state,
		Reason:             reason,
		ActivationBundleID: bundleID,
		Generation:         1,
		PID:                observation.PID,
		ProcessStartedAt:   &startedAt,
		OccurredAt:         occurredAt,
	}
}
