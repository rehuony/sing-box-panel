// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

func (application *Node) coreLogFiles() (*corelogs.Files, error) {
	application.coreLogsMu.Lock()
	defer application.coreLogsMu.Unlock()
	if application.coreLogs == nil {
		files, err := corelogs.New(application.dataDir)
		if err != nil {
			return nil, err
		}
		application.coreLogs = files
	}
	return application.coreLogs, nil
}
func (application *Node) CoreLogFiles() ([]corelogs.File, error) {
	files, err := application.coreLogFiles()
	if err != nil {
		return nil, err
	}
	return files.List()
}
func (application *Node) CoreLogContent(name string, offset int64, generation string) (corelogs.Chunk, error) {
	files, err := application.coreLogFiles()
	if err != nil {
		return corelogs.Chunk{}, err
	}
	return files.Read(name, offset, generation)
}
func (application *Node) DeleteCoreLogFile(ctx context.Context, name string) (err error) {
	started := time.Now()
	defer func() {
		details := OperationContext{StartedAt: started}
		if !errors.Is(err, corelogs.ErrInvalidFile) {
			details.File = name
		}
		application.recordOperation("core.log.delete", "Core log file deletion", err, details)
	}()
	files, err := application.coreLogFiles()
	if err != nil {
		return err
	}
	return files.Delete(name)
}
func (application *Node) ClearCoreLog(ctx context.Context, name string) (err error) {
	started := time.Now()
	defer func() {
		details := OperationContext{StartedAt: started}
		if !errors.Is(err, corelogs.ErrInvalidFile) {
			details.File = name
		}
		application.recordOperation("core.log.clear", "Core log clearing", err, details)
	}()
	files, err := application.coreLogFiles()
	if err != nil {
		return err
	}
	return files.Clear(name)
}

// SetCoreLogPolicy applies the core log retention the panel sends, removing
// what it no longer keeps, and keeps it for the node's next start. A policy
// already in effect changes nothing. A failure is recorded as well as
// returned; a stored policy that its cleanup could not finish still applies,
// and the next cleanup retries it.
func (application *Node) SetCoreLogPolicy(ctx context.Context, policy corelogs.Policy) (err error) {
	defer func() {
		if err != nil {
			application.record(Event{
				Source: SourceNode, Level: LevelWarn,
				Code: "core.log.policy_failed", Message: "The panel's core log retention could not be applied",
				Metadata: map[string]any{"error": err.Error()},
			})
		}
	}()
	if err := policy.Validate(); err != nil {
		return err
	}
	files, err := application.coreLogFiles()
	if err != nil {
		return err
	}
	if files.Policy() == policy {
		return nil
	}
	if err := application.database.SaveCoreLogPolicy(ctx, store.CoreLogPolicy{
		RetentionDays: policy.RetentionDays, MaxFiles: policy.MaxFiles, MaxFileBytes: policy.MaxFileBytes,
	}, application.now()); err != nil {
		return err
	}
	return files.SetPolicy(policy)
}

// startingCoreLogPolicy is the policy the node starts with: the one the panel
// last sent, or DefaultCoreLogPolicy before it sent one.
func startingCoreLogPolicy(ctx context.Context, database *store.Store) (corelogs.Policy, error) {
	stored, found, err := database.CoreLogPolicy(ctx)
	if err != nil || !found {
		return DefaultCoreLogPolicy, err
	}
	return corelogs.Policy{RetentionDays: stored.RetentionDays, MaxFiles: stored.MaxFiles, MaxFileBytes: stored.MaxFileBytes}, nil
}

// attachCoreLogs attaches the runtime's core log files before requests are
// served.
func (app *Node) attachCoreLogs(files *corelogs.Files) {
	app.coreLogsMu.Lock()
	defer app.coreLogsMu.Unlock()
	app.coreLogs = files
}
func (app *Node) MaintainCoreLogs() error {
	app.coreLogsMu.Lock()
	defer app.coreLogsMu.Unlock()
	if app.coreLogs == nil {
		return nil
	}
	return app.coreLogs.EnsureCurrentFile()
}
