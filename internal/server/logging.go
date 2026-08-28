// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
)

func startLogRetention(ctx context.Context, commands *application.Application) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				retention, err := commands.EnforceLogRetention(ctx)
				if err != nil {
					recordOperationalLog(commands, application.LogRecordRequest{
						Source: store.LogSourcePanel, Level: store.LogLevelError, Code: "logs.retention_failed",
						Message: "Operational log retention failed", Metadata: json.RawMessage(`{}`),
					})
					continue
				}
				if retention.Deleted > 0 {
					recordOperationalLog(commands, application.LogRecordRequest{
						Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "logs.retention_enforced",
						Message:  "Expired operational log entries were deleted",
						Metadata: mustLogMetadata(map[string]any{"deleted": retention.Deleted}),
					})
				}
			}
		}
	}()
	return done
}

func withTaskLogging(commands *application.Application, next taskHandler) taskHandler {
	return taskHandlerFunc(func(
		ctx context.Context,
		task store.Task,
		control taskExecutionControl,
	) (json.RawMessage, error) {
		metadata := mustLogMetadata(map[string]any{
			"task_id": task.ID,
			"kind":    task.Kind,
			"lane":    task.Lane,
			"attempt": task.Attempt,
		})
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourceTask, Level: store.LogLevelInfo, Code: "task.started",
			Message: "Durable task execution started", Metadata: metadata,
		})
		result, err := next.Handle(ctx, task, control)
		if err != nil {
			recordOperationalLog(commands, application.LogRecordRequest{
				Source: store.LogSourceTask, Level: store.LogLevelError, Code: "task.failed",
				Message: "Durable task execution failed", Metadata: metadata,
			})
			return result, err
		}
		recordOperationalLog(commands, application.LogRecordRequest{
			Source: store.LogSourceTask, Level: store.LogLevelInfo, Code: "task.succeeded",
			Message: "Durable task execution succeeded", Metadata: metadata,
		})
		return result, nil
	})
}

func recordOperationalLog(commands *application.Application, request application.LogRecordRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = commands.RecordLog(ctx, request)
}

func mustLogMetadata(value map[string]any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func stopHTTPServer(server *http.Server, serveResult <-chan error) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	shutdownErr := server.Shutdown(shutdownContext)
	if shutdownErr != nil {
		_ = server.Close()
	}
	serveErr := <-serveResult
	if shutdownErr != nil {
		return fmt.Errorf("shut down panel HTTP server: %w", shutdownErr)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return fmt.Errorf("serve panel HTTP server during shutdown: %w", serveErr)
	}
	return nil
}

func workerID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}
	return fmt.Sprintf("panel/%s/%d", hostname, os.Getpid())
}
