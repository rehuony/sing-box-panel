// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/installation"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
)

func hasCleanupTargets(report instanceFilesReport) bool {
	for _, entry := range report.Entries {
		if entry.State != "missing" && (entry.Cleanup == "remove" || entry.Cleanup == "remove_link" || (entry.Cleanup == "remove_if_empty" && entry.Empty != nil && *entry.Empty)) {
			return true
		}
	}
	for _, file := range report.Service.Files {
		if report.ServiceMatches && file.Managed && !file.Retained && file.State != "missing" {
			return true
		}
	}
	return false
}

// The saved inventory is discovery evidence even if cleanup loses configuration
// halfway through. Rechecking it never grants permission to delete old paths.
func pruneInstance(ctx context.Context, report instanceFilesReport, service panelSystemd.Service) (result installation.CleanupResult, cleanupErr error) {
	result = installation.CleanupResult{Removed: []string{}, Retained: []string{}, Remaining: []string{}, Warnings: []string{}}
	for _, file := range report.Service.Files {
		cleanup := "retain"
		if report.ServiceMatches && file.Managed && !file.Retained {
			cleanup = "remove"
		}
		report.Entries = append(report.Entries, installation.Entry{Path: file.Path, Role: "service", State: file.State, Cleanup: cleanup})
	}
	if !hasCleanupTargets(report) {
		installation.Recheck(report.Report, &result)
		return result, nil
	}
	if err := installation.ValidateCleanup(report.Report); err != nil {
		installation.Recheck(report.Report, &result)
		return result, err
	}
	history := installation.CleanupHistory{SettingsPath: report.SettingsPath, Scope: string(report.Service.Scope), Outcome: "started"}
	if report.DataDir != "" {
		history.DataDirs = []string{report.DataDir}
	}
	for _, file := range report.Service.Files {
		if report.ServiceMatches && file.State != "missing" {
			history.ServicePaths = append(history.ServicePaths, file.Path)
		}
	}
	if err := installation.WriteCleanupHistory(ctx, report.HistoryPath, history); err != nil {
		installation.Recheck(report.Report, &result)
		return result, err
	}
	report.Entries = append(report.Entries, installation.Entry{Path: report.HistoryPath, Role: "cleanup history", State: "file", Cleanup: "retain"})
	defer func() {
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		finalReport, inspectErr := installation.InspectWithHistory(finishCtx, report.SettingsPath, report.HistoryPath)
		report.Entries = append(report.Entries, finalReport.Entries...)
		if inspectErr != nil {
			result.Warnings = append(result.Warnings, inspectErr.Error())
		}
		installation.Recheck(report.Report, &result)
		if len(result.Remaining) > 0 || len(result.Warnings) > 0 {
			cleanupErr = errors.Join(cleanupErr, errors.New("cleanup incomplete; inspect remaining resources and warnings"))
		}
		history.RemovedCount = len(result.Removed)
		history.RemainingCount = len(result.Remaining)
		history.Outcome = "completed"
		if cleanupErr != nil {
			history.Outcome = "interrupted"
			result.Warnings = append(result.Warnings, cleanupErr.Error())
		}
		// Cancellation must not erase the outcome of already completed operations.
		if err := installation.WriteCleanupHistory(finishCtx, report.HistoryPath, history); err != nil {
			result.Warnings = append(result.Warnings, "update cleanup history: "+err.Error())
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}()
	removed, stopErr := stopInstanceForCleanup(ctx, report, service)
	result.Removed = append(result.Removed, removed...)
	cleanupErr = stopErr
	if cleanupErr != nil {
		return result, cleanupErr
	}
	cleaned, err := installation.Clean(ctx, report.Report)
	result.Removed = append(result.Removed, cleaned.Removed...)
	result.Retained = append(result.Retained, cleaned.Retained...)
	result.Warnings = append(result.Warnings, cleaned.Warnings...)
	return result, err
}
