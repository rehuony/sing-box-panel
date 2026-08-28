// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaximumLogIDBytes       = 80
	MaximumLogCodeBytes     = 128
	MaximumLogMessageBytes  = 4 << 10
	MaximumLogMetadataBytes = 16 << 10

	maximumLogMetadataDepth = 32
	maximumLogMetadataNodes = 2_048
	redactedLogValue        = "[REDACTED]"
	omittedLogValue         = "[OMITTED]"
)

var (
	ErrLogEntryNotFound = errors.New("log entry not found")

	logIdentifierPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]*$`)
	logCodePattern        = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
	logBearerPattern      = regexp.MustCompile(`(?i)\bbearer[[:space:]]+[A-Za-z0-9._~+/=-]+`)
	logAssignmentPattern  = regexp.MustCompile(`(?i)\b(token|password|passwd|secret|private[_-]?key|authorization|cookie|proxy[_-]?(?:credentials?|authorization|password))[[:space:]]*[:=][[:space:]]*(?:"[^"]*"|'[^']*'|[^[:space:],;]+)`)
	logHeaderPattern      = regexp.MustCompile(`(?i)\b(authorization|cookie|set-cookie|proxy-authorization)[[:blank:]]*:[[:blank:]]*[^\r\n]*`)
	logURLUserInfoPattern = regexp.MustCompile(`(?i)\b(https?|socks5?|http\+unix)://[^/[:space:]@]+@`)
)

type LogSource string

const (
	LogSourcePanel    LogSource = "panel"
	LogSourceCore     LogSource = "core"
	LogSourceTask     LogSource = "task"
	LogSourceSecurity LogSource = "security"
)

type LogLevel string

const (
	LogLevelTrace LogLevel = "trace"
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
	LogLevelFatal LogLevel = "fatal"
)

// LogEntry is sanitized durable event metadata. It is deliberately not a
// journald mirror: messages are bounded summaries and Metadata may not contain
// credentials, full configurations, or subscription bodies.
type LogEntry struct {
	ID       string          `json:"id"`
	Time     time.Time       `json:"time"`
	Source   LogSource       `json:"source"`
	Level    LogLevel        `json:"level"`
	Code     string          `json:"code"`
	Message  string          `json:"message"`
	Metadata json.RawMessage `json:"metadata"`
}

// LogCursor identifies one point in the total (time, id) ordering.
type LogCursor struct {
	Time time.Time `json:"time"`
	ID   string    `json:"id"`
}

// LogListFilter selects a newest-first page. Since is inclusive, Until is
// exclusive, and Cursor is exclusive.
type LogListFilter struct {
	Source LogSource
	Level  LogLevel
	Code   string
	Since  *time.Time
	Until  *time.Time
	Cursor *LogCursor
	Limit  int
}

type LogPage struct {
	Items []LogEntry
	Next  *LogCursor
}

// LogTailFilter selects oldest-first entries after an exclusive cursor. Since
// is an inclusive starting boundary for the first poll.
type LogTailFilter struct {
	Source LogSource
	Level  LogLevel
	Code   string
	Since  *time.Time
	After  *LogCursor
	Limit  int
}

type LogClearFilter struct {
	Source LogSource
	Before *time.Time
}

func (s *Store) AppendLogEntry(ctx context.Context, entry LogEntry) (LogEntry, error) {
	prepared, err := prepareLogEntry(entry)
	if err != nil {
		return LogEntry{}, err
	}
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO log_entries(
            id, occurred_at, source, level, code, message, metadata_json
         ) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		prepared.ID,
		formatTaskTime(prepared.Time),
		string(prepared.Source),
		string(prepared.Level),
		prepared.Code,
		prepared.Message,
		string(prepared.Metadata),
	); err != nil {
		return LogEntry{}, fmt.Errorf("append log entry: %w", err)
	}
	return prepared, nil
}

func (s *Store) GetLogEntry(ctx context.Context, entryID string) (LogEntry, error) {
	if err := validateLogID(entryID); err != nil {
		return LogEntry{}, err
	}
	return getLogEntry(ctx, s.db, entryID)
}

func (s *Store) ListLogEntries(ctx context.Context, filter LogListFilter) (LogPage, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return LogPage{}, err
	}
	clauses, args, err := logFilterClauses(filter.Source, filter.Level, filter.Code, filter.Since, filter.Until)
	if err != nil {
		return LogPage{}, err
	}
	if err := validateLogCursor(filter.Cursor); err != nil {
		return LogPage{}, err
	}
	if filter.Cursor != nil {
		cursorTime := formatTaskTime(filter.Cursor.Time)
		clauses = append(clauses, "(occurred_at < ? OR (occurred_at = ? AND id < ?))")
		args = append(args, cursorTime, cursorTime, filter.Cursor.ID)
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, occurred_at, source, level, code, message, metadata_json
           FROM log_entries
          WHERE `+strings.Join(clauses, " AND ")+`
          ORDER BY occurred_at DESC, id DESC
          LIMIT ?`,
		args...,
	)
	if err != nil {
		return LogPage{}, fmt.Errorf("list log entries: %w", err)
	}
	defer rows.Close()

	items := make([]LogEntry, 0, limit+1)
	for rows.Next() {
		entry, err := scanLogEntry(rows)
		if err != nil {
			return LogPage{}, fmt.Errorf("scan listed log entry: %w", err)
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return LogPage{}, fmt.Errorf("iterate listed log entries: %w", err)
	}
	page := LogPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		last := page.Items[len(page.Items)-1]
		page.Next = &LogCursor{Time: last.Time, ID: last.ID}
	}
	return page, nil
}

func (s *Store) TailLogEntries(ctx context.Context, filter LogTailFilter) ([]LogEntry, error) {
	limit, err := normalizePageLimit(filter.Limit)
	if err != nil {
		return nil, err
	}
	clauses, args, err := logFilterClauses(filter.Source, filter.Level, filter.Code, filter.Since, nil)
	if err != nil {
		return nil, err
	}
	if err := validateLogCursor(filter.After); err != nil {
		return nil, err
	}
	if filter.After != nil {
		cursorTime := formatTaskTime(filter.After.Time)
		clauses = append(clauses, "(occurred_at > ? OR (occurred_at = ? AND id > ?))")
		args = append(args, cursorTime, cursorTime, filter.After.ID)
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, occurred_at, source, level, code, message, metadata_json
           FROM log_entries
          WHERE `+strings.Join(clauses, " AND ")+`
          ORDER BY occurred_at ASC, id ASC
          LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("tail log entries: %w", err)
	}
	defer rows.Close()
	items := make([]LogEntry, 0, limit)
	for rows.Next() {
		entry, err := scanLogEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan tailed log entry: %w", err)
		}
		items = append(items, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tailed log entries: %w", err)
	}
	return items, nil
}

func (s *Store) ClearLogEntries(ctx context.Context, filter LogClearFilter) (int64, error) {
	if filter.Source != "" && !validLogSource(filter.Source) {
		return 0, fmt.Errorf("invalid log source %q", filter.Source)
	}
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 2)
	if filter.Source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, string(filter.Source))
	}
	if filter.Before != nil {
		if filter.Before.IsZero() {
			return 0, errors.New("log clear before time is zero")
		}
		clauses = append(clauses, "occurred_at < ?")
		args = append(args, formatTaskTime(filter.Before.UTC()))
	}
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM log_entries WHERE `+strings.Join(clauses, " AND "),
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("clear log entries: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read cleared log entry count: %w", err)
	}
	return count, nil
}

func (s *Store) DeleteLogEntry(ctx context.Context, entryID string) error {
	if err := validateLogID(entryID); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM log_entries WHERE id = ?`, entryID)
	if err != nil {
		return fmt.Errorf("delete log entry: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted log entry count: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w: %s", ErrLogEntryNotFound, entryID)
	}
	return nil
}

func prepareLogEntry(entry LogEntry) (LogEntry, error) {
	if err := validateLogID(entry.ID); err != nil {
		return LogEntry{}, err
	}
	if entry.Time.IsZero() {
		return LogEntry{}, errors.New("log entry time is required")
	}
	if !validLogSource(entry.Source) {
		return LogEntry{}, fmt.Errorf("invalid log source %q", entry.Source)
	}
	if !validLogLevel(entry.Level) {
		return LogEntry{}, fmt.Errorf("invalid log level %q", entry.Level)
	}
	if len(entry.Code) == 0 || len(entry.Code) > MaximumLogCodeBytes || !logCodePattern.MatchString(entry.Code) {
		return LogEntry{}, errors.New("log code must be a lowercase dotted identifier of at most 128 bytes")
	}
	message, err := sanitizeLogMessage(entry.Message)
	if err != nil {
		return LogEntry{}, err
	}
	metadata, err := sanitizeLogMetadata(entry.Metadata)
	if err != nil {
		return LogEntry{}, fmt.Errorf("sanitize log metadata: %w", err)
	}
	entry.Time = entry.Time.UTC()
	entry.Message = message
	entry.Metadata = metadata
	return entry, nil
}

func validateLogID(value string) error {
	if len(value) == 0 || len(value) > MaximumLogIDBytes || !utf8.ValidString(value) || !logIdentifierPattern.MatchString(value) {
		return errors.New("log entry id is invalid")
	}
	return nil
}

func validLogSource(value LogSource) bool {
	switch value {
	case LogSourcePanel, LogSourceCore, LogSourceTask, LogSourceSecurity:
		return true
	default:
		return false
	}
}

func validLogLevel(value LogLevel) bool {
	switch value {
	case LogLevelTrace, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelFatal:
		return true
	default:
		return false
	}
}

func logFilterClauses(
	source LogSource,
	level LogLevel,
	code string,
	since *time.Time,
	until *time.Time,
) ([]string, []any, error) {
	if source != "" && !validLogSource(source) {
		return nil, nil, fmt.Errorf("invalid log source %q", source)
	}
	if level != "" && !validLogLevel(level) {
		return nil, nil, fmt.Errorf("invalid log level %q", level)
	}
	if code != "" && (len(code) > MaximumLogCodeBytes || !logCodePattern.MatchString(code)) {
		return nil, nil, errors.New("log code filter is invalid")
	}
	if since != nil && since.IsZero() {
		return nil, nil, errors.New("log since time is zero")
	}
	if until != nil && until.IsZero() {
		return nil, nil, errors.New("log until time is zero")
	}
	if since != nil && until != nil && !since.Before(*until) {
		return nil, nil, errors.New("log since time must be before until time")
	}
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 8)
	if source != "" {
		clauses = append(clauses, "source = ?")
		args = append(args, string(source))
	}
	if level != "" {
		clauses = append(clauses, "level = ?")
		args = append(args, string(level))
	}
	if code != "" {
		clauses = append(clauses, "code = ?")
		args = append(args, code)
	}
	if since != nil {
		clauses = append(clauses, "occurred_at >= ?")
		args = append(args, formatTaskTime(since.UTC()))
	}
	if until != nil {
		clauses = append(clauses, "occurred_at < ?")
		args = append(args, formatTaskTime(until.UTC()))
	}
	return clauses, args, nil
}

func validateLogCursor(cursor *LogCursor) error {
	if cursor == nil {
		return nil
	}
	if cursor.Time.IsZero() {
		return errors.New("log cursor time is zero")
	}
	if err := validateLogID(cursor.ID); err != nil {
		return fmt.Errorf("log cursor: %w", err)
	}
	return nil
}

func getLogEntry(ctx context.Context, q queryRower, entryID string) (LogEntry, error) {
	entry, err := scanLogEntry(q.QueryRowContext(
		ctx,
		`SELECT id, occurred_at, source, level, code, message, metadata_json
           FROM log_entries WHERE id = ?`,
		entryID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return LogEntry{}, fmt.Errorf("%w: %s", ErrLogEntryNotFound, entryID)
	}
	if err != nil {
		return LogEntry{}, fmt.Errorf("get log entry: %w", err)
	}
	return entry, nil
}

func scanLogEntry(row taskScanner) (LogEntry, error) {
	var entry LogEntry
	var occurredAt, source, level, metadata string
	if err := row.Scan(
		&entry.ID,
		&occurredAt,
		&source,
		&level,
		&entry.Code,
		&entry.Message,
		&metadata,
	); err != nil {
		return LogEntry{}, err
	}
	parsed, err := parseTaskTime(occurredAt)
	if err != nil {
		return LogEntry{}, fmt.Errorf("parse log entry time: %w", err)
	}
	entry.Time = parsed
	entry.Source = LogSource(source)
	entry.Level = LogLevel(level)
	entry.Metadata = append(json.RawMessage(nil), metadata...)
	return entry, nil
}
