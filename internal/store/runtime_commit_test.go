// SPDX-License-Identifier: GPL-3.0-or-later
package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeCompletionCommitsEvidenceAndHubAtomically(t *testing.T) {
	ctx := context.Background()
	database := openTestStore(t, ctx)
	initial, err := database.LatestRuntimeTransition(ctx)
	if err != nil {
		t.Fatal(err)
	}
	old, observation := seedAppliedRuntime(t, ctx, database, initial.OccurredAt.Add(time.Minute))
	bundle, err := database.SaveActivationBundle(ctx, ActivationBundle{ID: "new-bundle", StartupArtifactID: old.StartupArtifactID, MonitoringTier: MonitoringLimited, CreatedAt: observation.ObservedAt.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{Kind: RuntimeIntentApply, BundleID: bundle.ID, CreatedAt: observation.ObservedAt.Add(2 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	next := observation
	next.PID++
	next.ProcessStartToken = "new-incarnation"
	next.ActivationBundleID = bundle.ID
	next.StartedAt = observation.ObservedAt.Add(3 * time.Second)
	next.ObservedAt = next.StartedAt
	transition := runtimeTransitionTestInput("atomic-running", RuntimeTransitionRunning, "apply_succeeded", bundle.ID, next, next.ObservedAt)
	transition.Generation = intent.Generation
	commit := &RuntimeCommit{ExpectedObservation: &observation, Observation: &next, Transitions: []RuntimeTransitionInput{transition}}
	for _, trigger := range []string{
		`CREATE TRIGGER reject_atomic_write BEFORE INSERT ON runtime_transitions WHEN NEW.reason='apply_succeeded' BEGIN SELECT RAISE(ABORT,'injected history failure'); END`,
		`CREATE TRIGGER reject_atomic_write BEFORE UPDATE OF applied_bundle_id ON hub_state BEGIN SELECT RAISE(ABORT,'injected hub failure'); END`,
	} {
		if _, err := database.db.ExecContext(ctx, trigger); err != nil {
			t.Fatal(err)
		}
		if err := database.CompleteRuntimeIntent(ctx, intent, true, commit, next.ObservedAt); err == nil {
			t.Fatal("injected failure was ignored")
		}
		assertRuntimeCommitUnchanged(t, ctx, database, observation, old.ID, "apply_succeeded")
		if _, err := database.db.ExecContext(ctx, `DROP TRIGGER reject_atomic_write`); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.CompleteRuntimeIntent(ctx, intent, true, commit, next.ObservedAt); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil || bootstrap.Hub.AppliedBundleID != bundle.ID || bootstrap.Hub.RollbackBundleID != old.ID {
		t.Fatalf("committed hub: %+v, %v", bootstrap, err)
	}
	stored, err := database.RuntimeObservation(ctx)
	if err != nil || stored.PID != next.PID {
		t.Fatalf("observation: %+v %v", stored, err)
	}
	history, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{Reason: "apply_succeeded"})
	if err != nil || len(history.Items) != 1 || history.Items[0].Generation != intent.Generation {
		t.Fatalf("history: %+v %v", history, err)
	}
}

func TestRuntimeCompletionRejectsStaleOrMissingEvidence(t *testing.T) {
	for _, scenario := range []string{"stale-generation", "new-incarnation", "wrong-transition-generation", "wrong-process", "missing-evidence"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			database := openTestStore(t, ctx)
			initial, err := database.LatestRuntimeTransition(ctx)
			if err != nil {
				t.Fatal(err)
			}
			bundle, observation := seedAppliedRuntime(t, ctx, database, initial.OccurredAt.Add(time.Minute))
			intent, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{Kind: RuntimeIntentStop, CreatedAt: observation.ObservedAt.Add(time.Second)})
			if err != nil {
				t.Fatal(err)
			}
			transition := runtimeTransitionTestInput("stop-fenced", RuntimeTransitionStopped, "stop_succeeded", bundle.ID, observation, observation.ObservedAt.Add(2*time.Second))
			transition.Generation = intent.Generation
			commit := &RuntimeCommit{ExpectedObservation: &observation, ClearObservation: true, Transitions: []RuntimeTransitionInput{transition}}
			expected := observation
			switch scenario {
			case "stale-generation":
				if _, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{Kind: RuntimeIntentRestart, CreatedAt: transition.OccurredAt}); err != nil {
					t.Fatal(err)
				}
			case "new-incarnation":
				expected.PID++
				expected.ProcessStartToken = "newer-incarnation"
				expected.StartedAt = transition.OccurredAt
				expected.ObservedAt = transition.OccurredAt
				if _, err := database.RecordRuntimeObservation(ctx, expected); err != nil {
					t.Fatal(err)
				}
			case "wrong-transition-generation":
				commit.Transitions[0].Generation++
			case "wrong-process":
				commit.Transitions[0].ProcessStartToken = "other-process"
			case "missing-evidence":
				commit = nil
			}
			if err := database.CompleteRuntimeIntent(ctx, intent, true, commit, transition.OccurredAt); err == nil {
				t.Fatal("invalid completion accepted")
			}
			assertRuntimeCommitUnchanged(t, ctx, database, expected, bundle.ID, "stop_succeeded")
		})
	}
}

func TestRuntimeCompletionRecordsFailedAndStoppedIncarnations(t *testing.T) {
	for _, succeeded := range []bool{false, true} {
		t.Run(map[bool]string{true: "stopped", false: "failed"}[succeeded], func(t *testing.T) {
			ctx := context.Background()
			database := openTestStore(t, ctx)
			initial, err := database.LatestRuntimeTransition(ctx)
			if err != nil {
				t.Fatal(err)
			}
			bundle, observation := seedAppliedRuntime(t, ctx, database, initial.OccurredAt.Add(time.Minute))
			intent, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{Kind: RuntimeIntentStop, CreatedAt: observation.ObservedAt.Add(time.Second)})
			if err != nil {
				t.Fatal(err)
			}
			state := RuntimeTransitionFailed
			if succeeded {
				state = RuntimeTransitionStopped
			}
			transition := runtimeTransitionTestInput("finished-stop", state, "stop_finished", bundle.ID, observation, observation.ObservedAt.Add(2*time.Second))
			transition.Generation = intent.Generation
			if err := database.CompleteRuntimeIntent(ctx, intent, succeeded, &RuntimeCommit{ExpectedObservation: &observation, ClearObservation: true, Transitions: []RuntimeTransitionInput{transition}}, transition.OccurredAt); err != nil {
				t.Fatal(err)
			}
			if _, err := database.RuntimeObservation(ctx); !errors.Is(err, ErrRuntimeObservationNotFound) {
				t.Fatalf("observation not cleared: %v", err)
			}
			history, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{Reason: "stop_finished"})
			if err != nil || len(history.Items) != 1 || history.Items[0].State != state {
				t.Fatalf("history: %+v %v", history, err)
			}
		})
	}
}

func assertRuntimeCommitUnchanged(t *testing.T, ctx context.Context, database *Store, expected RuntimeObservation, bundleID, reason string) {
	t.Helper()
	observation, err := database.RuntimeObservation(ctx)
	if err != nil || observation.PID != expected.PID || observation.ProcessStartToken != expected.ProcessStartToken {
		t.Fatalf("process evidence changed: %+v %v", observation, err)
	}
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil || bootstrap.Hub.AppliedBundleID != bundleID {
		t.Fatalf("applied bundle changed: %+v %v", bootstrap, err)
	}
	history, err := database.ListRuntimeTransitions(ctx, RuntimeHistoryFilter{Reason: reason})
	if err != nil || len(history.Items) != 0 {
		t.Fatalf("history partially committed: %+v %v", history, err)
	}
}
