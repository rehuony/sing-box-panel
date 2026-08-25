// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestSignalInterruptedTaskWaitRequestsCancellationAndNamesTask(t *testing.T) {
	cases := []struct {
		name         string
		fallbackCode string
	}{
		{name: "config render", fallbackCode: "structured_check_wait_failed"},
		{name: "manual detach", fallbackCode: "manual_detach_check_wait_failed"},
		{name: "manual replace", fallbackCode: "manual_check_wait_failed"},
		{name: "manual reattach apply", fallbackCode: "manual_reattach_check_wait_failed"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			database, err := store.Open(ctx, filepath.Join(t.TempDir(), "panel.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close() })

			taskID := "task-" + strings.ReplaceAll(testCase.name, " ", "-")
			now := time.Date(2026, time.August, 26, 22, 0, 0, 0, time.UTC)
			if _, err := database.EnqueueTask(ctx, store.EnqueueTaskInput{
				ID: taskID, Lane: store.TaskLaneMaintenance, Kind: "startup-check",
				Payload: []byte(`{}`), CreatedAt: now,
			}); err != nil {
				t.Fatal(err)
			}
			claimed, err := database.ClaimTask(ctx, store.ClaimTaskInput{
				Lane: store.TaskLaneMaintenance, LeaseOwner: "signal-test",
				Now: now.Add(time.Second), LeaseDuration: time.Minute,
			})
			if err != nil || claimed == nil || claimed.ID != taskID {
				t.Fatalf("claim task = %+v, err=%v", claimed, err)
			}

			signalContext, cancelSignalContext := context.WithCancel(ctx)
			cancelSignalContext()
			_, err = waitForTaskWithCancellationRequest(
				signalContext,
				application.FromStore(database),
				taskID,
				250*time.Millisecond,
				testCase.fallbackCode,
			)
			var classified *Error
			if err == nil || !errors.As(err, &classified) ||
				classified.Code != "task_wait_interrupted" || ExitCode(err) != 130 {
				t.Fatalf("wait error=%v classified=%+v exit=%d", err, classified, ExitCode(err))
			}

			persisted, readErr := database.GetTask(ctx, taskID)
			if readErr != nil || persisted.Status != store.TaskStatusRunning || !persisted.CancelRequested {
				t.Fatalf("cancellation request task=%+v err=%v", persisted, readErr)
			}

			var stderr bytes.Buffer
			if writeErr := WriteError(&stderr, nil, err); writeErr != nil {
				t.Fatal(writeErr)
			}
			if !strings.Contains(stderr.String(), taskID) {
				t.Fatalf("stderr does not name task %q: %s", taskID, stderr.String())
			}
		})
	}
}
