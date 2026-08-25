// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/capability"
)

func TestActivationBundleApplyCommitsOnlyAfterHealthyTaskSuccess(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 13, 0, 0, 0, time.UTC)
	bundle := readyBundleFixture(t, database, now, "a", "")

	intent, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
		TaskID: "apply-a", IdempotencyKey: "apply:" + bundle.SHA256,
		Kind: RuntimeIntentApply, BundleID: bundle.ID, CreatedAt: now.Add(5 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := database.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Hub.AppliedBundleID != "" || state.Hub.DesiredBundleID != bundle.ID || !state.Hub.DesiredRunning {
		t.Fatalf("pre-health hub = %+v", state.Hub)
	}
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "worker", Now: now.Add(6 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != intent.ID {
		t.Fatalf("claimed=%+v err=%v", claimed, err)
	}
	completed, err := database.CompleteTask(ctx, claimed.ID, claimed.LeaseOwner, now.Add(7*time.Second), TaskCompletion{
		Succeeded: true, Result: json.RawMessage(`{"healthy":true}`),
	})
	if err != nil || completed.Status != TaskStatusSucceeded {
		t.Fatalf("completed=%+v err=%v", completed, err)
	}
	state, err = database.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if state.Hub.AppliedBundleID != bundle.ID || state.Hub.RollbackBundleID != "" || !state.Hub.DesiredRunning {
		t.Fatalf("post-health hub = %+v", state.Hub)
	}
	if state.Hub.AppliedAt == nil || !state.Hub.AppliedAt.Equal(now.Add(7*time.Second)) {
		t.Fatalf("applied_at = %v", state.Hub.AppliedAt)
	}

	retry, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
		TaskID: "different-task-id", IdempotencyKey: "apply:" + bundle.SHA256,
		Kind: RuntimeIntentApply, BundleID: bundle.ID, CreatedAt: now.Add(8 * time.Second),
	})
	if err != nil || retry.ID != intent.ID || retry.Generation != intent.Generation {
		t.Fatalf("idempotent retry=%+v err=%v", retry, err)
	}
}

func TestCoreArtifactRestrictionFencesChecksRuntimeAndObservation(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 13, 30, 0, 0, time.UTC)
	bundle := readyBundleFixture(t, database, now, "f", "")
	startup, err := database.GetStartupArtifact(ctx, bundle.StartupArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	core, err := database.GetCoreArtifact(ctx, startup.CoreArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	check, err := database.EnqueueTask(ctx, EnqueueTaskInput{
		ID: "restriction-check", Lane: TaskLaneMaintenance, Kind: "startup-check",
		StartupArtifactID: startup.ID, Payload: json.RawMessage(`{}`), CreatedAt: now.Add(5 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	intent, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
		TaskID: "restriction-apply", Kind: RuntimeIntentApply, BundleID: bundle.ID,
		CreatedAt: now.Add(6 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "restriction-worker",
		Now: now.Add(7 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != intent.ID {
		t.Fatalf("claimed runtime intent = %+v, error = %v", claimed, err)
	}

	restricted, err := database.RestrictCoreArtifactVerification(
		ctx,
		core.ID,
		CoreArtifactRevoked,
		now.Add(8*time.Second),
	)
	if err != nil || restricted.VerificationState != CoreArtifactRevoked {
		t.Fatalf("restricted artifact = %+v, error = %v", restricted, err)
	}
	canceledCheck, err := database.GetTask(ctx, check.ID)
	if err != nil || canceledCheck.Status != TaskStatusCanceled || !canceledCheck.CancelRequested {
		t.Fatalf("startup check after restriction = %+v, error = %v", canceledCheck, err)
	}
	runningIntent, err := database.GetTask(ctx, intent.ID)
	if err != nil || !runningIntent.CancelRequested {
		t.Fatalf("runtime intent after restriction = %+v, error = %v", runningIntent, err)
	}
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.Hub.DesiredRunning || bootstrap.Hub.TargetGeneration <= intent.Generation || bootstrap.Hub.AppliedBundleID != "" {
		t.Fatalf("hub after restriction = %+v", bootstrap.Hub)
	}
	if _, err := database.RecordRuntimeObservation(ctx, RuntimeObservation{
		PID: 4343, ProcessStartToken: "restriction-process", CoreArtifactID: core.ID,
		ActivationBundleID: bundle.ID, ExactCoreVersion: core.ExactVersion,
		ArchiveSHA256: core.ArchiveSHA256, BinarySHA256: core.BinarySHA256,
		StartedAt: now.Add(9 * time.Second), ObservedAt: now.Add(10 * time.Second),
	}); !errors.Is(err, ErrRuntimeIdentityMismatch) {
		t.Fatalf("restricted runtime observation error = %v, want ErrRuntimeIdentityMismatch", err)
	}
	completed, err := database.CompleteTask(
		ctx,
		claimed.ID,
		claimed.LeaseOwner,
		now.Add(11*time.Second),
		TaskCompletion{Succeeded: true, Result: json.RawMessage(`{"healthy":true}`)},
	)
	if err != nil || completed.Status != TaskStatusSuperseded {
		t.Fatalf("late restricted runtime completion = %+v, error = %v", completed, err)
	}
	bootstrap, err = database.Bootstrap(ctx)
	if err != nil || bootstrap.Hub.AppliedBundleID != "" || bootstrap.Hub.DesiredRunning {
		t.Fatalf("late completion changed hub = %+v, error = %v", bootstrap.Hub, err)
	}
}

func TestSecondApplyFreezesRollbackAndRuntimeObservationIsFenced(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 14, 0, 0, 0, time.UTC)
	first := readyBundleFixture(t, database, now, "a", "")
	completeRuntimeIntent(t, database, now.Add(5*time.Second), RuntimeIntentInput{
		TaskID: "apply-a", Kind: RuntimeIntentApply, BundleID: first.ID,
	})
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second := readyBundleFixture(t, database, now.Add(10*time.Second), "b", bootstrap.Hub.HeadRevisionID)
	completeRuntimeIntent(t, database, now.Add(15*time.Second), RuntimeIntentInput{
		TaskID: "apply-b", Kind: RuntimeIntentApply, BundleID: second.ID,
	})
	bootstrap, err = database.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.Hub.AppliedBundleID != second.ID || bootstrap.Hub.RollbackBundleID != first.ID {
		t.Fatalf("second applied hub = %+v", bootstrap.Hub)
	}

	startup, err := database.GetStartupArtifact(ctx, second.StartupArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	core, err := database.GetCoreArtifact(ctx, startup.CoreArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := database.RecordRuntimeObservation(ctx, RuntimeObservation{
		PID: 4242, ProcessStartToken: "proc-start-1", CoreArtifactID: core.ID,
		ActivationBundleID: second.ID, ExactCoreVersion: core.ExactVersion,
		ArchiveSHA256: core.ArchiveSHA256, BinarySHA256: core.BinarySHA256,
		StartedAt: now.Add(16 * time.Second), ObservedAt: now.Add(17 * time.Second),
	})
	if err != nil || observation.ActivationBundleID != second.ID {
		t.Fatalf("observation=%+v err=%v", observation, err)
	}
	cleared, err := database.ClearRuntimeObservation(ctx, observation.PID, "old-incarnation")
	if err != nil || cleared {
		t.Fatalf("stale clear=%v err=%v", cleared, err)
	}
	if _, err := database.RuntimeObservation(ctx); err != nil {
		t.Fatalf("stale clear removed observation: %v", err)
	}
	cleared, err = database.ClearRuntimeObservation(ctx, observation.PID, observation.ProcessStartToken)
	if err != nil || !cleared {
		t.Fatalf("matching clear=%v err=%v", cleared, err)
	}
}

func TestRollbackUsesFrozenBundleWithoutReprojection(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 14, 30, 0, 0, time.UTC)
	first := readyBundleFixture(t, database, now, "c", "")
	completeRuntimeIntent(t, database, now.Add(5*time.Second), RuntimeIntentInput{
		TaskID: "apply-c", Kind: RuntimeIntentApply, BundleID: first.ID,
	})
	bootstrap, err := database.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second := readyBundleFixture(t, database, now.Add(10*time.Second), "d", bootstrap.Hub.HeadRevisionID)
	completeRuntimeIntent(t, database, now.Add(15*time.Second), RuntimeIntentInput{
		TaskID: "apply-d", Kind: RuntimeIntentApply, BundleID: second.ID,
	})

	rollback, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
		TaskID: "rollback-c", Kind: RuntimeIntentRollback, CreatedAt: now.Add(20 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rollback.ActivationBundleID != first.ID {
		t.Fatalf("rollback bundle = %q, want frozen %q", rollback.ActivationBundleID, first.ID)
	}
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "rollback-worker",
		Now: now.Add(21 * time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != rollback.ID {
		t.Fatalf("claimed rollback = %+v, err=%v", claimed, err)
	}
	if _, err := database.CompleteTask(
		ctx, claimed.ID, claimed.LeaseOwner, now.Add(22*time.Second),
		TaskCompletion{Succeeded: true, Result: json.RawMessage(`{"healthy":true}`)},
	); err != nil {
		t.Fatal(err)
	}
	bootstrap, err = database.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.Hub.AppliedBundleID != first.ID || bootstrap.Hub.RollbackBundleID != second.ID {
		t.Fatalf("post-rollback hub = %+v", bootstrap.Hub)
	}
	if bootstrap.Hub.AppliedAt == nil || !bootstrap.Hub.AppliedAt.Equal(now.Add(22*time.Second)) {
		t.Fatalf("rollback applied_at = %v", bootstrap.Hub.AppliedAt)
	}
}

func TestActivationBundleFailureRollsBackNewSubscriptionSnapshot(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 15, 30, 0, 0, time.UTC)
	first := readyBundleFixture(t, database, now, "e", "")
	firstSnapshot, err := database.GetSubscriptionSnapshot(ctx, first.SubscriptionSnapshotID)
	if err != nil {
		t.Fatal(err)
	}

	// Activation identity deliberately excludes caller-chosen row IDs. These
	// inputs therefore produce the first bundle's SHA under different IDs and
	// fail its UNIQUE constraint after the snapshot insert is attempted.
	_, err = database.SaveActivationBundle(ctx, SubscriptionSnapshot{
		ID:                  "subscription-unique-sha-collision",
		CanonicalRevisionID: firstSnapshot.CanonicalRevisionID,
		StartupArtifactID:   firstSnapshot.StartupArtifactID,
		Content:             firstSnapshot.Content,
		CreatedAt:           now.Add(10 * time.Second),
	}, ActivationBundle{
		ID:                     "bundle-unique-sha-collision",
		StartupArtifactID:      first.StartupArtifactID,
		SubscriptionSnapshotID: "subscription-unique-sha-collision",
		PublicAddresses:        first.PublicAddresses,
		SourceSnapshots:        first.SourceSnapshots,
		MonitoringTier:         first.MonitoringTier,
		CreatedAt:              now.Add(11 * time.Second),
	})
	if err == nil {
		t.Fatal("duplicate activation identity was accepted")
	}
	if _, lookupErr := database.GetSubscriptionSnapshot(ctx, "subscription-unique-sha-collision"); !errors.Is(lookupErr, ErrSubscriptionSnapshotNotFound) {
		t.Fatalf("failed bundle left a half snapshot: %v", lookupErr)
	}
}

func TestActivationBundleRejectsStructuredStartupAfterCapabilityPinMoves(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 16, 0, 0, 0, time.UTC)
	fixture := readyStructuredActivationFixture(t, database, now, "moved-pin", 'a', '1', 'b')

	if _, err := database.UpsertCapabilityPin(ctx, CapabilityPin{
		ExactCoreVersion: fixture.Core.ExactVersion,
		Repository:       capability.ManifestRepository,
		CommitSHA:        strings.Repeat("2", 40),
		ManifestSHA256:   strings.Repeat("c", 64),
		SupportLevel:     capability.SupportNativeStructured,
		PinnedAt:         now.Add(4 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	const snapshotID = "subscription-structured-pin"
	const bundleID = "bundle-structured-pin"
	_, err := database.SaveActivationBundle(ctx, SubscriptionSnapshot{
		ID: snapshotID, CanonicalRevisionID: fixture.Revision.ID, StartupArtifactID: fixture.Startup.ID,
		Content: json.RawMessage(`{"nodes":[]}`), CreatedAt: now.Add(5 * time.Second),
	}, ActivationBundle{
		ID: bundleID, StartupArtifactID: fixture.Startup.ID, SubscriptionSnapshotID: snapshotID,
		PublicAddresses: json.RawMessage(`{}`), SourceSnapshots: json.RawMessage(`[]`),
		MonitoringTier: MonitoringProcessOnly, CreatedAt: now.Add(6 * time.Second),
	})
	assertActivationSaveRejected(t, database, err, snapshotID, bundleID)
}

func TestActivationBundleRejectsCurrentStructuredCapabilityQuarantine(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 26, 16, 10, 0, 0, time.UTC)
	fixture := readyStructuredActivationFixture(t, database, now, "quarantine", 'b', '3', 'd')
	if _, err := database.UpsertCapabilityQuarantine(ctx, CapabilityQuarantine{
		ManifestSHA256: fixture.Pin.ManifestSHA256, ReasonCode: "projection_failed",
		Diagnostics: json.RawMessage(`{"fixture":"activation"}`), QuarantinedAt: now.Add(4 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}

	const snapshotID = "subscription-structured-quarantine"
	const bundleID = "bundle-structured-quarantine"
	_, err := database.SaveActivationBundle(ctx, SubscriptionSnapshot{
		ID: snapshotID, CanonicalRevisionID: fixture.Revision.ID, StartupArtifactID: fixture.Startup.ID,
		Content: json.RawMessage(`{"nodes":[]}`), CreatedAt: now.Add(5 * time.Second),
	}, ActivationBundle{
		ID: bundleID, StartupArtifactID: fixture.Startup.ID, SubscriptionSnapshotID: snapshotID,
		PublicAddresses: json.RawMessage(`{}`), SourceSnapshots: json.RawMessage(`[]`),
		MonitoringTier: MonitoringProcessOnly, CreatedAt: now.Add(6 * time.Second),
	})
	assertActivationSaveRejected(t, database, err, snapshotID, bundleID)
}

func TestActivationBundleSaveRechecksCurrentCanonicalHead(t *testing.T) {
	t.Run("structured", func(t *testing.T) {
		ctx := testContext(t)
		database := openTestStore(t, ctx)
		now := time.Date(2026, time.August, 26, 16, 20, 0, 0, time.UTC)
		fixture := readyStructuredActivationFixture(t, database, now, "head", 'c', '4', 'e')
		if _, err := database.SaveCanonicalRevisionAndTask(ctx, fixture.Revision.ID, NewCanonicalRevision{
			ID: "revision-after-structured", SchemaVersion: 1,
			Document:  json.RawMessage(`{"schema_version":1,"global":{}}`),
			CommandID: "command-after-structured", CreatedAt: now.Add(4 * time.Second),
		}, NewTask{ID: "canonical-task-after-structured", Lane: TaskLaneMaintenance, Kind: "canonical-saved"}); err != nil {
			t.Fatal(err)
		}
		current, err := database.GetStartupArtifact(ctx, fixture.Startup.ID)
		if err != nil || current.State != StartupArtifactReady {
			t.Fatalf("structured startup after head move = %+v, %v; want ready to exercise transactional head fence", current, err)
		}
		const snapshotID = "subscription-old-structured-head"
		const bundleID = "bundle-old-structured-head"
		_, err = database.SaveActivationBundle(ctx, SubscriptionSnapshot{
			ID: snapshotID, CanonicalRevisionID: fixture.Revision.ID, StartupArtifactID: fixture.Startup.ID,
			Content: json.RawMessage(`{"nodes":[]}`), CreatedAt: now.Add(5 * time.Second),
		}, ActivationBundle{
			ID: bundleID, StartupArtifactID: fixture.Startup.ID, SubscriptionSnapshotID: snapshotID,
			PublicAddresses: json.RawMessage(`{}`), SourceSnapshots: json.RawMessage(`[]`),
			MonitoringTier: MonitoringProcessOnly, CreatedAt: now.Add(6 * time.Second),
		})
		assertActivationSaveRejected(t, database, err, snapshotID, bundleID)
	})

	t.Run("manual", func(t *testing.T) {
		ctx := testContext(t)
		database := openTestStore(t, ctx)
		now := time.Date(2026, time.August, 26, 16, 30, 0, 0, time.UTC)
		original := readyBundleFixture(t, database, now, "d", "")
		startup, err := database.GetStartupArtifact(ctx, original.StartupArtifactID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.SaveCanonicalRevisionAndTask(ctx, startup.CanonicalRevisionID, NewCanonicalRevision{
			ID: "revision-after-manual", SchemaVersion: 1,
			Document:  json.RawMessage(`{"schema_version":1,"global":{}}`),
			CommandID: "command-after-manual", CreatedAt: now.Add(5 * time.Second),
		}, NewTask{ID: "canonical-task-after-manual", Lane: TaskLaneMaintenance, Kind: "canonical-saved"}); err != nil {
			t.Fatal(err)
		}
		const snapshotID = "subscription-old-manual-head"
		const bundleID = "bundle-old-manual-head"
		_, err = database.SaveActivationBundle(ctx, SubscriptionSnapshot{
			ID: snapshotID, CanonicalRevisionID: startup.CanonicalRevisionID, StartupArtifactID: startup.ID,
			Content: json.RawMessage(`{"nodes":[]}`), CreatedAt: now.Add(6 * time.Second),
		}, ActivationBundle{
			ID: bundleID, StartupArtifactID: startup.ID, SubscriptionSnapshotID: snapshotID,
			PublicAddresses: json.RawMessage(`{}`), SourceSnapshots: json.RawMessage(`[]`),
			MonitoringTier: MonitoringProcessOnly, CreatedAt: now.Add(7 * time.Second),
		})
		assertActivationSaveRejected(t, database, err, snapshotID, bundleID)
	})
}

func TestRuntimeApplyRechecksStructuredPinAndQuarantineButStartAndRollbackUseAppliedBundles(t *testing.T) {
	t.Run("pin moved", func(t *testing.T) {
		ctx := testContext(t)
		database := openTestStore(t, ctx)
		now := time.Date(2026, time.August, 26, 16, 40, 0, 0, time.UTC)
		fixture := readyStructuredActivationFixture(t, database, now, "apply-pin", 'd', '5', 'a')
		bundle := saveStructuredActivationFixture(t, database, fixture, "apply-pin", now.Add(4*time.Second))
		if _, err := database.UpsertCapabilityPin(ctx, CapabilityPin{
			ExactCoreVersion: fixture.Core.ExactVersion, Repository: capability.ManifestRepository,
			CommitSHA: strings.Repeat("6", 40), ManifestSHA256: strings.Repeat("b", 64),
			SupportLevel: capability.SupportNativeStructured, PinnedAt: now.Add(5 * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
		_, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
			TaskID: "apply-after-pin-move", Kind: RuntimeIntentApply,
			BundleID: bundle.ID, CreatedAt: now.Add(6 * time.Second),
		})
		if !errors.Is(err, ErrActivationBundleNotReady) {
			t.Fatalf("RequestRuntimeIntent(apply after pin move) error = %v, want ErrActivationBundleNotReady", err)
		}
	})

	t.Run("quarantine blocks new apply but not applied start or rollback", func(t *testing.T) {
		ctx := testContext(t)
		database := openTestStore(t, ctx)
		now := time.Date(2026, time.August, 26, 16, 50, 0, 0, time.UTC)
		fixture := readyStructuredActivationFixture(t, database, now, "apply-quarantine", 'e', '7', 'c')
		firstBundle := saveStructuredActivationFixture(t, database, fixture, "apply-quarantine", now.Add(4*time.Second))
		secondBundle, err := database.SaveActivationBundle(ctx, SubscriptionSnapshot{
			ID: "subscription-structured-apply-quarantine-second", CanonicalRevisionID: fixture.Revision.ID,
			StartupArtifactID: fixture.Startup.ID, Content: json.RawMessage(`{"nodes":[{"tag":"second"}]}`),
			CreatedAt: now.Add(6 * time.Second),
		}, ActivationBundle{
			ID: "bundle-structured-apply-quarantine-second", StartupArtifactID: fixture.Startup.ID,
			SubscriptionSnapshotID: "subscription-structured-apply-quarantine-second",
			PublicAddresses:        json.RawMessage(`{}`), SourceSnapshots: json.RawMessage(`[]`),
			MonitoringTier: MonitoringProcessOnly, CreatedAt: now.Add(7 * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		completeRuntimeIntent(t, database, now.Add(5*time.Second), RuntimeIntentInput{
			TaskID: "initial-structured-apply", Kind: RuntimeIntentApply, BundleID: firstBundle.ID,
		})
		completeRuntimeIntent(t, database, now.Add(8*time.Second), RuntimeIntentInput{
			TaskID: "second-structured-apply", Kind: RuntimeIntentApply, BundleID: secondBundle.ID,
		})
		if _, err := database.UpsertCapabilityQuarantine(ctx, CapabilityQuarantine{
			ManifestSHA256: fixture.Pin.ManifestSHA256, ReasonCode: "post_apply_review",
			Diagnostics: json.RawMessage(`{}`), QuarantinedAt: now.Add(11 * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
		_, err = database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
			TaskID: "apply-after-quarantine", Kind: RuntimeIntentApply,
			BundleID: secondBundle.ID, CreatedAt: now.Add(12 * time.Second),
		})
		if !errors.Is(err, ErrActivationBundleNotReady) {
			t.Fatalf("RequestRuntimeIntent(apply after quarantine) error = %v, want ErrActivationBundleNotReady", err)
		}
		started, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
			TaskID: "start-applied-after-quarantine", Kind: RuntimeIntentStart,
			CreatedAt: now.Add(13 * time.Second),
		})
		if err != nil || started.ActivationBundleID != secondBundle.ID {
			t.Fatalf("RequestRuntimeIntent(start applied after quarantine) = %+v, %v", started, err)
		}
		rollback, err := database.RequestRuntimeIntent(ctx, RuntimeIntentInput{
			TaskID: "rollback-after-quarantine", Kind: RuntimeIntentRollback,
			CreatedAt: now.Add(14 * time.Second),
		})
		if err != nil || rollback.ActivationBundleID != firstBundle.ID {
			t.Fatalf("RequestRuntimeIntent(rollback after quarantine) = %+v, %v", rollback, err)
		}
	})
}

type structuredActivationFixture struct {
	Core     CoreArtifact
	Revision CanonicalRevision
	Startup  StartupArtifact
	Pin      CapabilityPin
}

func readyStructuredActivationFixture(
	t *testing.T,
	database *Store,
	now time.Time,
	suffix string,
	coreDigestCharacter byte,
	commitCharacter byte,
	manifestDigestCharacter byte,
) structuredActivationFixture {
	t.Helper()
	ctx := testContext(t)
	core := testCoreArtifact(
		"core-structured-"+suffix,
		900+int64(coreDigestCharacter),
		coreDigestCharacter,
		"amd64",
		now,
	)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	revision, err := database.SaveCanonicalRevisionAndTask(ctx, "", NewCanonicalRevision{
		ID: "revision-structured-" + suffix, SchemaVersion: 1,
		Document:  json.RawMessage(`{"schema_version":1}`),
		CommandID: "command-structured-" + suffix, CreatedAt: now,
	}, NewTask{
		ID:   "canonical-task-structured-" + suffix,
		Lane: TaskLaneMaintenance, Kind: "canonical-saved",
	})
	if err != nil {
		t.Fatal(err)
	}
	pin, err := database.UpsertCapabilityPin(ctx, CapabilityPin{
		ExactCoreVersion: core.ExactVersion,
		Repository:       capability.ManifestRepository,
		CommitSHA:        strings.Repeat(string(commitCharacter), 40),
		ManifestSHA256:   strings.Repeat(string(manifestDigestCharacter), 64),
		SupportLevel:     capability.SupportNativeStructured,
		PinnedAt:         now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(ctx, StartupArtifact{
		ID: "startup-structured-" + suffix, Kind: StartupArtifactStructured,
		CanonicalRevisionID: revision.ID, ExactCoreVersion: core.ExactVersion,
		CapabilityCommit: pin.CommitSHA, CapabilityDigest: pin.ManifestSHA256,
		RendererVersion: "capability-projector-v1", CoreArtifactID: core.ID,
		ConfigBytes: []byte(`{"route":{}}`), CreatedAt: now.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, nil, now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return structuredActivationFixture{Core: core, Revision: revision, Startup: startup, Pin: pin}
}

func saveStructuredActivationFixture(
	t *testing.T,
	database *Store,
	fixture structuredActivationFixture,
	suffix string,
	createdAt time.Time,
) ActivationBundle {
	t.Helper()
	ctx := testContext(t)
	snapshotID := "subscription-structured-" + suffix
	bundle, err := database.SaveActivationBundle(ctx, SubscriptionSnapshot{
		ID: snapshotID, CanonicalRevisionID: fixture.Revision.ID, StartupArtifactID: fixture.Startup.ID,
		Content: json.RawMessage(`{"nodes":[]}`), CreatedAt: createdAt,
	}, ActivationBundle{
		ID: "bundle-structured-" + suffix, StartupArtifactID: fixture.Startup.ID,
		SubscriptionSnapshotID: snapshotID, PublicAddresses: json.RawMessage(`{}`),
		SourceSnapshots: json.RawMessage(`[]`), MonitoringTier: MonitoringProcessOnly,
		CreatedAt: createdAt.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func assertActivationSaveRejected(
	t *testing.T,
	database *Store,
	err error,
	snapshotID string,
	bundleID string,
) {
	t.Helper()
	ctx := testContext(t)
	if !errors.Is(err, ErrActivationBundleNotReady) {
		t.Fatalf("SaveActivationBundle() error = %v, want ErrActivationBundleNotReady", err)
	}
	if _, lookupErr := database.GetSubscriptionSnapshot(ctx, snapshotID); !errors.Is(lookupErr, ErrSubscriptionSnapshotNotFound) {
		t.Fatalf("rejected activation left subscription snapshot: %v", lookupErr)
	}
	if _, lookupErr := database.GetActivationBundle(ctx, bundleID); !errors.Is(lookupErr, ErrActivationBundleNotFound) {
		t.Fatalf("rejected activation left bundle: %v", lookupErr)
	}
}

func readyBundleFixture(
	t *testing.T,
	database *Store,
	now time.Time,
	suffix string,
	expectedHead string,
) ActivationBundle {
	t.Helper()
	ctx := testContext(t)
	core := testCoreArtifact("core-"+suffix, int64(800)+int64(suffix[0]), suffix[0], "amd64", now)
	if _, err := database.UpsertCoreArtifact(ctx, core); err != nil {
		t.Fatal(err)
	}
	revision, err := database.SaveCanonicalRevisionAndTask(ctx, expectedHead, NewCanonicalRevision{
		ID: "revision-" + suffix, SchemaVersion: 1,
		Document: json.RawMessage(`{"schema_version":1}`), CommandID: "command-" + suffix, CreatedAt: now,
	}, NewTask{ID: "canonical-task-" + suffix, Lane: TaskLaneMaintenance, Kind: "canonical-saved"})
	if err != nil {
		t.Fatal(err)
	}
	startup, err := database.CreateStartupArtifact(ctx, StartupArtifact{
		ID: "startup-" + suffix, Kind: StartupArtifactManual, CanonicalRevisionID: revision.ID,
		ExactCoreVersion: core.ExactVersion, RendererVersion: "manual-v1", CoreArtifactID: core.ID,
		ConfigBytes: []byte("{\n  // frozen\n}\n"), CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	startup, err = database.CompleteStartupArtifactCheck(ctx, startup.ID, true, nil, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := database.SaveActivationBundle(ctx, SubscriptionSnapshot{
		ID: "subscription-" + suffix, CanonicalRevisionID: revision.ID, StartupArtifactID: startup.ID,
		Content: json.RawMessage(`{"nodes":[]}`), CreatedAt: now.Add(3 * time.Second),
	}, ActivationBundle{
		ID: "bundle-" + suffix, StartupArtifactID: startup.ID, SubscriptionSnapshotID: "subscription-" + suffix,
		PublicAddresses: json.RawMessage(`{}`), SourceSnapshots: json.RawMessage(`[]`),
		MonitoringTier: MonitoringProcessOnly, CreatedAt: now.Add(4 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func completeRuntimeIntent(t *testing.T, database *Store, now time.Time, input RuntimeIntentInput) Task {
	t.Helper()
	ctx := testContext(t)
	input.CreatedAt = now
	queued, err := database.RequestRuntimeIntent(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := database.ClaimTask(ctx, ClaimTaskInput{
		Lane: TaskLaneRuntime, LeaseOwner: "worker", Now: now.Add(time.Second), LeaseDuration: time.Minute,
	})
	if err != nil || claimed == nil || claimed.ID != queued.ID {
		t.Fatalf("claim runtime intent=%+v err=%v", claimed, err)
	}
	completed, err := database.CompleteTask(ctx, claimed.ID, claimed.LeaseOwner, now.Add(2*time.Second), TaskCompletion{Succeeded: true})
	if err != nil {
		t.Fatal(err)
	}
	return completed
}
