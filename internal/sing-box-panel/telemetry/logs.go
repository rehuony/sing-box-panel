// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/database"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

type LogEntry = store.LogEntry
type LogCursor = store.LogCursor

type LogRecordRequest struct {
	Source   store.LogSource
	Level    store.LogLevel
	Code     string
	Message  string
	Metadata json.RawMessage
}

type LogListRequest struct {
	Source store.LogSource
	Level  store.LogLevel
	Code   string
	Since  *time.Time
	Until  *time.Time
	Cursor *store.LogCursor
	Limit  int
}

type LogPage struct {
	Items []store.LogEntry `json:"items"`
	Next  *store.LogCursor `json:"next,omitempty"`
}

type LogTailRequest struct {
	Source store.LogSource
	Level  store.LogLevel
	Code   string
	Since  *time.Time
	After  *store.LogCursor
	Limit  int
}

type LogClearRequest struct {
	Source store.LogSource
	Before *time.Time
	// Scope limits the clearing to the panel's own entries or to one node's.
	Scope store.LogScope
}

type LogClearResult struct {
	Deleted int64 `json:"deleted"`
	// FreedBytes is how much smaller the log's files on disk became.
	FreedBytes int64 `json:"freed_bytes"`
}

type LogUsage = store.LogUsage

type LogDeleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// RecordLog creates one sanitized durable event. Store validation and
// redaction are intentionally applied even to trusted in-process callers.
func (application *Service) RecordLog(ctx context.Context, request LogRecordRequest) (store.LogEntry, error) {
	if application == nil || application.database == nil {
		return store.LogEntry{}, errors.New("application database is unavailable")
	}
	id, err := database.NewID(application.random, "log")
	if err != nil {
		return store.LogEntry{}, fmt.Errorf("generate log entry id: %w", err)
	}
	entry, err := application.database.AppendLogEntry(ctx, store.LogEntry{
		ID:       id,
		Time:     application.now().UTC(),
		Source:   request.Source,
		Level:    request.Level,
		Code:     request.Code,
		Message:  request.Message,
		Metadata: request.Metadata,
	})
	if err == nil && application.logObserver != nil {
		application.logObserver(entry)
	}
	return entry, err
}

func (application *Service) Log(ctx context.Context, entryID string) (store.LogEntry, error) {
	return application.database.GetLogEntry(ctx, entryID)
}

func (application *Service) ListLogs(ctx context.Context, request LogListRequest) (LogPage, error) {
	page, err := application.database.ListLogEntries(ctx, store.LogListFilter{
		Source: request.Source,
		Level:  request.Level,
		Code:   request.Code,
		Since:  request.Since,
		Until:  request.Until,
		Cursor: request.Cursor,
		Limit:  request.Limit,
	})
	if err != nil {
		return LogPage{}, err
	}
	return LogPage{Items: page.Items, Next: page.Next}, nil
}

func (application *Service) TailLogs(ctx context.Context, request LogTailRequest) ([]store.LogEntry, error) {
	return application.database.TailLogEntries(ctx, store.LogTailFilter{
		Source: request.Source,
		Level:  request.Level,
		Code:   request.Code,
		Since:  request.Since,
		After:  request.After,
		Limit:  request.Limit,
	})
}

// ClearLogs deletes the entries the request selects, shrinks the log's files
// on disk and records one entry saying how many it deleted and how many bytes
// that freed, about the node whose entries it cleared, so the record shows
// where the clearing was asked for. ClearLogs and DeleteLog are
// transport-neutral explicit deletion primitives.
func (application *Service) ClearLogs(ctx context.Context, request LogClearRequest) (LogClearResult, error) {
	before, err := application.database.LogUsage(ctx, store.LogScope{})
	if err != nil {
		return LogClearResult{}, err
	}
	deleted, err := application.database.ClearLogEntries(ctx, store.LogClearFilter{
		Source: request.Source,
		Before: request.Before,
		Scope:  request.Scope,
	})
	if err != nil {
		return LogClearResult{}, err
	}
	if err := application.database.CompactLogs(ctx); err != nil {
		return LogClearResult{}, err
	}
	after, err := application.database.LogUsage(ctx, store.LogScope{})
	if err != nil {
		return LogClearResult{}, err
	}
	result := LogClearResult{Deleted: deleted, FreedBytes: max(0, before.Bytes-after.Bytes)}
	metadata := map[string]any{"deleted": result.Deleted, "freed_bytes": result.FreedBytes}
	if request.Scope.NodeID != "" {
		metadata["node_id"] = request.Scope.NodeID
	}
	application.RecordEvent(LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelInfo, Code: "logs.cleared", Message: "Panel log cleared",
		Metadata: LogMetadata(metadata),
	})
	return result, nil
}

// LogUsage reports the entries the scope selects and the size of the whole
// log's files on disk.
func (application *Service) LogUsage(ctx context.Context, scope store.LogScope) (LogUsage, error) {
	return application.database.LogUsage(ctx, scope)
}

func (application *Service) DeleteLog(ctx context.Context, entryID string) (LogDeleteResult, error) {
	if err := application.database.DeleteLogEntry(ctx, entryID); err != nil {
		return LogDeleteResult{}, err
	}
	return LogDeleteResult{ID: entryID, Deleted: true}, nil
}

func IsLogNotFound(err error) bool {
	return errors.Is(err, store.ErrLogEntryNotFound)
}

func (application *Service) PanelLogs(ctx context.Context, filter store.PanelLogFilter) (store.PanelLogPage, error) {
	return application.database.ListPanelLogs(ctx, filter)
}

// RecordEvent records request on its own short deadline, so a lifecycle event
// is kept while its caller is stopping. A failure to record is dropped: the
// log must not change the outcome it reports.
func (application *Service) RecordEvent(request LogRecordRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = application.RecordLog(ctx, request)
}

// LogMetadata encodes log metadata, or an empty object if value cannot be
// encoded.
func LogMetadata(value map[string]any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

// RecordOperation records a completed action without exposing inputs or credentials.
func (application *Service) RecordOperation(ctx context.Context, code, message string, operationErr error, details OperationLogContext) {
	level := store.LogLevelInfo
	if operationErr != nil {
		level = store.LogLevelError
		message += " failed"
		code += ".failed"
	} else {
		message += " completed"
		code += ".completed"
	}
	logCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_, _ = application.RecordLog(logCtx, LogRecordRequest{Source: store.LogSourcePanel, Level: level, Code: code, Message: message, Metadata: details.metadata(operationErr, application.classify)})
}
