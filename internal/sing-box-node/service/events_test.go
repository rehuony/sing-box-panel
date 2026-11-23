// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	coreruntime "github.com/rehuony/sing-box-panel/internal/sing-box-node/runtime"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

// recordedEvents collects the events a node records.
type recordedEvents struct {
	mu     sync.Mutex
	events []Event
}

func (recorded *recordedEvents) record(event Event) {
	recorded.mu.Lock()
	defer recorded.mu.Unlock()
	recorded.events = append(recorded.events, event)
}

func (recorded *recordedEvents) withPrefix(prefix string) []Event {
	recorded.mu.Lock()
	defer recorded.mu.Unlock()
	var matched []Event
	for _, event := range recorded.events {
		if strings.HasPrefix(event.Code, prefix) {
			matched = append(matched, event)
		}
	}
	return matched
}

func TestOperationEventsCarryContextAndOnlySafeFailures(t *testing.T) {
	database, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	events := &recordedEvents{}
	app.events = events.record
	for _, test := range []struct {
		name string
		err  error
		code string
	}{
		{"success", nil, ""},
		{"known", fmt.Errorf("password=secret configuration payload: %w", store.ErrCoreNotEnabled), "core_not_enabled"},
		{"unknown", errors.New("https://user:secret@example.com/private-subscription-token"), "operation_failed"},
		{"canceled", context.Canceled, "canceled"},
		{"file", corelogs.ErrCurrentFile, "core_log_current"},
		{"check", fmt.Errorf("configuration payload password=secret: %w", coreruntime.ErrCheckFailed), "core_check_failed"},
		{"health", errors.Join(coreruntime.ErrRuntime, coreruntime.ErrHealthFailed, errors.New("private-subscription-token")), "core_health_failed"},
		{"version", fmt.Errorf("secret binary output: %w", coreruntime.ErrVersionMismatch), "core_version_mismatch"},
		{"termination", errors.Join(coreruntime.ErrRuntime, coreruntime.ErrTermination), "core_termination_failed"},
		{"check_canceled", errors.Join(coreruntime.ErrCheckFailed, context.Canceled), "canceled"},
		{"health_deadline", errors.Join(coreruntime.ErrHealthFailed, context.DeadlineExceeded), "deadline"},
	} {
		t.Run(test.name, func(t *testing.T) {
			app.recordOperation("test."+test.name, "Test operation", test.err, OperationContext{
				StartedAt: time.Now(), CoreID: "core-test", Generation: 3,
				StartupArtifactID: "startup-test", ActivationBundleID: "bundle-test",
			})
			matched := events.withPrefix("test." + test.name + ".")
			if len(matched) != 1 {
				t.Fatalf("operation events = %+v", matched)
			}
			details := matched[0].Metadata
			if details["core_id"] != "core-test" || details["generation"] != int64(3) || details["startup_artifact_id"] != "startup-test" || details["activation_bundle_id"] != "bundle-test" {
				t.Fatalf("context: %v", details)
			}
			if duration, ok := details["duration_ms"].(int64); !ok || duration < 0 {
				t.Fatalf("duration missing: %v", details)
			}
			if test.code == "" && details["error_code"] != nil || test.code != "" && details["error_code"] != test.code {
				t.Fatalf("classification: %v", details)
			}
			if raw := fmt.Sprint(details); strings.Contains(raw, "secret") || strings.Contains(raw, "payload") || strings.Contains(raw, "private-subscription") {
				t.Fatalf("unsafe context: %s", raw)
			}
		})
	}
}

func TestCoreLogActionsRecordOnlyManagedTargetAndDuration(t *testing.T) {
	ctx := t.Context()
	database, err := store.Open(ctx, filepath.Join(t.TempDir(), "node.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	app := newApplication(database)
	app.dataDir = t.TempDir()
	events := &recordedEvents{}
	app.events = events.record
	files, err := app.coreLogFiles()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := files.Writer().Write([]byte("INFO sample\n")); err != nil {
		t.Fatal(err)
	}
	listed, err := files.List()
	if err != nil || len(listed) != 1 {
		t.Fatalf("files: %v, %v", listed, err)
	}
	if err := app.ClearCoreLog(ctx, listed[0].Name); err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteCoreLogFile(ctx, listed[0].Name); !errors.Is(err, corelogs.ErrCurrentFile) {
		t.Fatal(err)
	}
	if err := app.ClearCoreLog(ctx, "../../secret"); !errors.Is(err, corelogs.ErrInvalidFile) {
		t.Fatal(err)
	}
	matched := events.withPrefix("core.log.")
	if len(matched) != 3 {
		t.Fatalf("events: %+v", matched)
	}
	for _, event := range matched {
		details := event.Metadata
		if details["duration_ms"] == nil {
			t.Fatalf("missing duration: %v", details)
		}
		if details["error_code"] == "core_log_invalid" {
			if details["file"] != nil {
				t.Fatalf("invalid target recorded: %v", details)
			}
		} else if details["file"] != listed[0].Name {
			t.Fatalf("target not recorded: %v", details)
		}
	}
}
