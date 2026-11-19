// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

func TestRetentionKeepsWhatIsInUse(t *testing.T) {
	ctx := testContext(t)
	database := openTestStore(t, ctx)
	now := time.Date(2026, time.November, 18, 9, 0, 0, 0, time.UTC)

	// Revisions: the newest two and the one the node reports applied stay.
	revisions := make([]CanonicalRevision, 0, 5)
	for i := 1; i <= 5; i++ {
		revision, err := saveTestConfiguration(ctx, database, int64(i-1), NewCanonicalRevision{
			ID: fmt.Sprintf("revision-%d", i), SchemaVersion: configuration.SchemaVersion,
			Document:  json.RawMessage(fmt.Sprintf(`{"experimental":{"value":%d}}`, i)),
			CommandID: fmt.Sprintf("command-%d", i), CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatal(err)
		}
		revisions = append(revisions, revision)
	}
	if _, err := database.db.ExecContext(ctx, `UPDATE nodes SET applied_revision_id = ? WHERE id = ?`, revisions[0].ID, testNodeID); err != nil {
		t.Fatal(err)
	}
	if deleted, err := database.PruneCanonicalRevisions(ctx, 2); err != nil || deleted != 2 {
		t.Fatalf("pruned revisions = %d, %v; want the two unused older ones", deleted, err)
	}
	for index, revision := range revisions {
		_, err := database.GetCanonicalRevision(ctx, revision.ID)
		if kept := index == 0 || index >= 3; kept != (err == nil) {
			t.Fatalf("revision %d kept = %t (%v)", index+1, err == nil, err)
		}
	}
	if _, err := database.SaveConfigurationFile(ctx, testNodeID, 5, `{"experimental":{"value":6}}`, NewCanonicalRevision{
		ID: "revision-6", SchemaVersion: configuration.SchemaVersion, Document: json.RawMessage(`{"experimental":{"value":6}}`),
		CommandID: "command-6", CreatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("saving after pruning = %v", err)
	}

	// Runtime history: old entries go, except each instance's newest.
	old := now.Add(-100 * 24 * time.Hour)
	for _, transition := range []RuntimeTransitionInput{
		{InstanceID: "instance-a", SourceID: 1, OccurredAt: old},
		{InstanceID: "instance-a", SourceID: 2, OccurredAt: old.Add(time.Hour)},
		{InstanceID: "instance-b", SourceID: 1, OccurredAt: old.Add(2 * time.Hour)},
	} {
		transition.NodeID, transition.State, transition.Reason = testNodeID, RuntimeTransitionStopped, "stop_succeeded"
		if err := database.RecordRuntimeTransition(ctx, transition); err != nil {
			t.Fatal(err)
		}
	}
	if deleted, err := database.DeleteRuntimeTransitionsBefore(ctx, now.Add(-90*24*time.Hour)); err != nil || deleted != 1 {
		t.Fatalf("deleted runtime transitions = %d, %v", deleted, err)
	}
	if after, err := database.LatestRuntimeTransitionSource(ctx, testNodeID, "instance-a"); err != nil || after != 2 {
		t.Fatalf("instance-a resumes after %d, %v; want its newest entry kept", after, err)
	}

	// Log entries: only the expired one goes.
	for id, at := range map[string]time.Time{"log_old": old, "log_new": now} {
		if _, err := database.AppendLogEntry(ctx, LogEntry{ID: id, Time: at, Source: LogSourcePanel, Level: LogLevelInfo, Code: "test.entry", Message: "Entry"}); err != nil {
			t.Fatal(err)
		}
	}
	if deleted, err := database.DeleteLogEntriesBefore(ctx, now.Add(-90*24*time.Hour)); err != nil || deleted != 1 {
		t.Fatalf("deleted log entries = %d, %v", deleted, err)
	}
	if _, err := database.GetLogEntry(ctx, "log_old"); !errors.Is(err, ErrLogEntryNotFound) {
		t.Fatalf("expired log entry = %v", err)
	}
}
