// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/store"
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
}

type LogClearResult struct {
	Deleted int64 `json:"deleted"`
}

type LogDeleteResult struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

// RecordLog creates one sanitized durable event. Store validation and
// redaction are intentionally applied even to trusted in-process callers.
func (application *Application) RecordLog(ctx context.Context, request LogRecordRequest) (store.LogEntry, error) {
	if application == nil || application.database == nil {
		return store.LogEntry{}, errors.New("application database is unavailable")
	}
	id, err := application.newID("log")
	if err != nil {
		return store.LogEntry{}, fmt.Errorf("generate log entry id: %w", err)
	}
	return application.database.AppendLogEntry(ctx, store.LogEntry{
		ID:       id,
		Time:     application.now().UTC(),
		Source:   request.Source,
		Level:    request.Level,
		Code:     request.Code,
		Message:  request.Message,
		Metadata: request.Metadata,
	})
}

func (application *Application) Log(ctx context.Context, entryID string) (store.LogEntry, error) {
	return application.database.GetLogEntry(ctx, entryID)
}

func (application *Application) ListLogs(ctx context.Context, request LogListRequest) (LogPage, error) {
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

func (application *Application) TailLogs(ctx context.Context, request LogTailRequest) ([]store.LogEntry, error) {
	return application.database.TailLogEntries(ctx, store.LogTailFilter{
		Source: request.Source,
		Level:  request.Level,
		Code:   request.Code,
		Since:  request.Since,
		After:  request.After,
		Limit:  request.Limit,
	})
}

// EnforceLogRetention removes entries older than the configured retention
// period. The server calls it at startup and on a bounded periodic schedule.
func (application *Application) EnforceLogRetention(ctx context.Context) (LogClearResult, error) {
	retentionDays := application.settings.Logs.RetentionDays
	if retentionDays < 1 || retentionDays > 3650 {
		return LogClearResult{}, errors.New("log retention setting is unavailable")
	}
	cutoff := application.now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	return application.ClearLogs(ctx, LogClearRequest{Before: &cutoff})
}

// ClearLogs and DeleteLog are transport-neutral explicit deletion primitives.
func (application *Application) ClearLogs(ctx context.Context, request LogClearRequest) (LogClearResult, error) {
	deleted, err := application.database.ClearLogEntries(ctx, store.LogClearFilter{
		Source: request.Source,
		Before: request.Before,
	})
	if err != nil {
		return LogClearResult{}, err
	}
	return LogClearResult{Deleted: deleted}, nil
}

func (application *Application) DeleteLog(ctx context.Context, entryID string) (LogDeleteResult, error) {
	if err := application.database.DeleteLogEntry(ctx, entryID); err != nil {
		return LogDeleteResult{}, err
	}
	return LogDeleteResult{ID: entryID, Deleted: true}, nil
}

func IsLogNotFound(err error) bool {
	return errors.Is(err, store.ErrLogEntryNotFound)
}
