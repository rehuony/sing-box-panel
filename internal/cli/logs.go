// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

// newDurableLogCommand is kept separate from groups.go so durable log command
// behavior stays independent from server and HTTP transport wiring.
func newDurableLogCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("log", "Inspect sanitized panel, core, task, and security event metadata")
	root.AddCommand(
		newLogListCommand(state, open),
		newLogShowCommand(state, open),
		newLogTailCommand(state, open),
		newLogClearCommand(state, open),
		newLogDeleteCommand(state, open),
	)
	return root
}

func newLogListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var source, level, code, sinceRaw, untilRaw, cursorTimeRaw, cursorID string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List durable sanitized event metadata newest first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			now := time.Now().UTC()
			since, err := parseLogSince(sinceRaw, now)
			if err != nil {
				return logUsageError("log_since_invalid", err)
			}
			until, err := parseOptionalRFC3339(untilRaw, "until")
			if err != nil {
				return logUsageError("log_until_invalid", err)
			}
			cursor, err := parseLogCursor(cursorTimeRaw, cursorID)
			if err != nil {
				return logUsageError("log_cursor_invalid", err)
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			page, err := instance.ListLogs(cmd.Context(), application.LogListRequest{
				Source: store.LogSource(strings.TrimSpace(source)),
				Level:  store.LogLevel(strings.TrimSpace(level)),
				Code:   strings.TrimSpace(code),
				Since:  since,
				Until:  until,
				Cursor: cursor,
				Limit:  limit,
			})
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "log_filter_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, page, logPageText(page))
		},
	}
	addLogFilterFlags(command, &source, &level, &code)
	command.Flags().StringVar(&sinceRaw, "since", "", "inclusive RFC3339 time or relative duration such as 30m")
	command.Flags().StringVar(&untilRaw, "until", "", "exclusive RFC3339 time")
	command.Flags().StringVar(&cursorTimeRaw, "cursor-time", "", "next cursor RFC3339 time (requires --cursor-id)")
	command.Flags().StringVar(&cursorID, "cursor-id", "", "next cursor entry ID (requires --cursor-time)")
	command.Flags().IntVar(&limit, "limit", 50, "maximum entries to return (1-200)")
	return command
}

func newLogShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show one durable sanitized log entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			entry, err := instance.Log(cmd.Context(), args[0])
			if err != nil {
				return classifyLogError("log_read_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, entry, logEntryText(entry, true))
		},
	}
}

func newLogTailCommand(state *options, open openApplicationFunc) *cobra.Command {
	var source, level, code, sinceRaw, cursorTimeRaw, cursorID string
	var follow bool
	var pollInterval time.Duration
	var limit int
	command := &cobra.Command{
		Use:   "tail",
		Short: "Read sanitized event metadata oldest first and optionally follow SQLite",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if pollInterval < 50*time.Millisecond || pollInterval > 10*time.Second {
				return &Error{Kind: ErrorUsage, Code: "log_poll_interval_invalid", Message: "--poll-interval must be between 50ms and 10s"}
			}
			if follow && state.format == outputJSON {
				return &Error{Kind: ErrorUsage, Code: "log_follow_output_invalid", Message: "--follow requires --output text or jsonl"}
			}
			now := time.Now().UTC()
			since, err := parseLogSince(sinceRaw, now)
			if err != nil {
				return logUsageError("log_since_invalid", err)
			}
			cursor, err := parseLogCursor(cursorTimeRaw, cursorID)
			if err != nil {
				return logUsageError("log_cursor_invalid", err)
			}
			if since != nil && cursor != nil {
				return &Error{Kind: ErrorUsage, Code: "log_tail_boundary_conflict", Message: "--since and cursor flags are mutually exclusive"}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			filter := application.LogTailRequest{
				Source: store.LogSource(strings.TrimSpace(source)),
				Level:  store.LogLevel(strings.TrimSpace(level)),
				Code:   strings.TrimSpace(code),
				Since:  since,
				After:  cursor,
				Limit:  limit,
			}
			return runLogTail(cmd.Context(), cmd.OutOrStdout(), state.format, instance, filter, follow, pollInterval, now)
		},
	}
	addLogFilterFlags(command, &source, &level, &code)
	command.Flags().StringVar(&sinceRaw, "since", "", "inclusive RFC3339 time or relative duration such as 30m")
	command.Flags().StringVar(&cursorTimeRaw, "cursor-time", "", "exclusive resume cursor RFC3339 time (requires --cursor-id)")
	command.Flags().StringVar(&cursorID, "cursor-id", "", "exclusive resume cursor entry ID (requires --cursor-time)")
	command.Flags().BoolVarP(&follow, "follow", "f", false, "continue polling the local SQLite event log")
	command.Flags().DurationVar(&pollInterval, "poll-interval", 250*time.Millisecond, "bounded local SQLite polling interval")
	command.Flags().IntVar(&limit, "limit", 100, "maximum entries per read (1-200)")
	return command
}

func newLogClearCommand(state *options, open openApplicationFunc) *cobra.Command {
	var source, beforeRaw string
	var all bool
	command := &cobra.Command{
		Use:   "clear",
		Short: "Explicitly delete a bounded set of durable log entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(beforeRaw) == "" && !all {
				return &Error{Kind: ErrorUsage, Code: "log_clear_scope_required", Message: "use --before to bound deletion, or --all to acknowledge unbounded deletion"}
			}
			if strings.TrimSpace(beforeRaw) != "" && all {
				return &Error{Kind: ErrorUsage, Code: "log_clear_scope_conflict", Message: "--before and --all are mutually exclusive"}
			}
			before, err := parseLogBefore(beforeRaw, time.Now().UTC())
			if err != nil {
				return logUsageError("log_before_invalid", err)
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.ClearLogs(cmd.Context(), application.LogClearRequest{
				Source: store.LogSource(strings.TrimSpace(source)), Before: before,
			})
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "log_clear_failed", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, fmt.Sprintf("deleted %d log entries", result.Deleted))
		},
	}
	command.Flags().StringVar(&source, "source", "", "limit deletion to panel, core, task, or security")
	command.Flags().StringVar(&beforeRaw, "before", "", "delete entries before an RFC3339 time or relative duration such as 168h")
	command.Flags().BoolVar(&all, "all", false, "acknowledge deletion without a time boundary")
	return command
}

func newLogDeleteCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "delete ID",
		Short: "Explicitly delete one durable log entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.DeleteLog(cmd.Context(), args[0])
			if err != nil {
				return classifyLogError("log_delete_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, "deleted log entry "+result.ID)
		},
	}
}

func runLogTail(
	ctx context.Context,
	writer io.Writer,
	format outputFormat,
	instance *application.Application,
	filter application.LogTailRequest,
	follow bool,
	pollInterval time.Duration,
	startedAt time.Time,
) error {
	if filter.Since == nil && filter.After == nil {
		page, err := instance.ListLogs(ctx, application.LogListRequest{
			Source: filter.Source, Level: filter.Level, Code: filter.Code, Limit: filter.Limit,
		})
		if err != nil {
			return &Error{Kind: ErrorValidation, Code: "log_filter_invalid", Message: err.Error(), Cause: err}
		}
		initial := slices.Clone(page.Items)
		slices.Reverse(initial)
		if err := writeLogTailBatch(writer, format, initial, follow); err != nil {
			return err
		}
		if len(initial) > 0 {
			last := initial[len(initial)-1]
			filter.After = &store.LogCursor{Time: last.Time, ID: last.ID}
		} else {
			filter.Since = &startedAt
		}
	} else {
		entries, err := instance.TailLogs(ctx, filter)
		if err != nil {
			return &Error{Kind: ErrorValidation, Code: "log_filter_invalid", Message: err.Error(), Cause: err}
		}
		if err := writeLogTailBatch(writer, format, entries, follow); err != nil {
			return err
		}
		if len(entries) > 0 {
			last := entries[len(entries)-1]
			filter.After = &store.LogCursor{Time: last.Time, ID: last.ID}
			filter.Since = nil
		}
		if !follow {
			return nil
		}
		if len(entries) == filter.Limit {
			return followLogTail(ctx, writer, format, instance, filter, pollInterval, false)
		}
	}
	if !follow {
		return nil
	}
	return followLogTail(ctx, writer, format, instance, filter, pollInterval, true)
}

func followLogTail(
	ctx context.Context,
	writer io.Writer,
	format outputFormat,
	instance *application.Application,
	filter application.LogTailRequest,
	pollInterval time.Duration,
	waitBeforeFirst bool,
) error {
	wait := waitBeforeFirst
	for {
		if wait {
			timer := time.NewTimer(pollInterval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return context.Cause(ctx)
			case <-timer.C:
			}
		}
		entries, err := instance.TailLogs(ctx, filter)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return &Error{Kind: ErrorDomain, Code: "log_tail_failed", Message: err.Error(), Cause: err}
		}
		if err := writeLogTailBatch(writer, format, entries, true); err != nil {
			return err
		}
		if len(entries) > 0 {
			last := entries[len(entries)-1]
			filter.After = &store.LogCursor{Time: last.Time, ID: last.ID}
			filter.Since = nil
		}
		wait = len(entries) < filter.Limit
	}
}

func writeLogTailBatch(writer io.Writer, format outputFormat, entries []store.LogEntry, streaming bool) error {
	switch format {
	case outputText:
		for _, entry := range entries {
			if _, err := fmt.Fprintln(writer, logEntryText(entry, false)); err != nil {
				return err
			}
		}
		return nil
	case outputJSONL:
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)
		for _, entry := range entries {
			if err := encoder.Encode(entry); err != nil {
				return err
			}
		}
		return nil
	case outputJSON:
		if streaming {
			return &Error{Kind: ErrorUsage, Code: "log_follow_output_invalid", Message: "streaming logs require text or jsonl output"}
		}
		return writeResult(writer, outputJSON, entries, "")
	default:
		return &Error{Kind: ErrorUsage, Code: "invalid_output", Message: "output must be text, json, or jsonl"}
	}
}

func addLogFilterFlags(command *cobra.Command, source, level, code *string) {
	command.Flags().StringVar(source, "source", "", "filter panel, core, task, or security events")
	command.Flags().StringVar(level, "level", "", "filter trace, debug, info, warn, error, or fatal events")
	command.Flags().StringVar(code, "code", "", "filter one exact stable event code")
}

func parseLogSince(raw string, now time.Time) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if duration, err := time.ParseDuration(raw); err == nil {
		if duration <= 0 {
			return nil, errors.New("relative since duration must be positive")
		}
		value := now.Add(-duration).UTC()
		return &value, nil
	}
	return parseOptionalRFC3339(raw, "since")
}

func parseLogBefore(raw string, now time.Time) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if duration, err := time.ParseDuration(raw); err == nil {
		if duration <= 0 {
			return nil, errors.New("relative before duration must be positive")
		}
		value := now.Add(-duration).UTC()
		return &value, nil
	}
	return parseOptionalRFC3339(raw, "before")
}

func parseOptionalRFC3339(raw, field string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, fmt.Errorf("%s must be RFC3339: %w", field, err)
	}
	value = value.UTC()
	return &value, nil
}

func parseLogCursor(timeRaw, entryID string) (*store.LogCursor, error) {
	timeRaw = strings.TrimSpace(timeRaw)
	entryID = strings.TrimSpace(entryID)
	if timeRaw == "" && entryID == "" {
		return nil, nil
	}
	if timeRaw == "" || entryID == "" {
		return nil, errors.New("--cursor-time and --cursor-id must be provided together")
	}
	value, err := time.Parse(time.RFC3339Nano, timeRaw)
	if err != nil {
		return nil, fmt.Errorf("cursor time must be RFC3339: %w", err)
	}
	return &store.LogCursor{Time: value.UTC(), ID: entryID}, nil
}

func logPageText(page application.LogPage) string {
	if len(page.Items) == 0 {
		return "no log entries"
	}
	lines := make([]string, len(page.Items))
	for index, entry := range page.Items {
		lines[index] = logEntryText(entry, false)
	}
	return strings.Join(lines, "\n")
}

func logEntryText(entry store.LogEntry, includeMetadata bool) string {
	text := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s", entry.Time.Format(time.RFC3339Nano), entry.ID, entry.Source, entry.Level, entry.Code, entry.Message)
	if includeMetadata {
		text += "\t" + string(entry.Metadata)
	}
	return text
}

func classifyLogError(code string, err error) error {
	if application.IsLogNotFound(err) {
		return &Error{Kind: ErrorDomain, Code: "log_not_found", Message: err.Error(), Cause: err}
	}
	return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
}

func logUsageError(code string, err error) error {
	return &Error{Kind: ErrorUsage, Code: code, Message: err.Error(), Cause: err}
}
