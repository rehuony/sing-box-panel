// SPDX-License-Identifier: GPL-3.0-or-later

package localnode

import (
	"context"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

func (application *Node) coreLogFiles() (*corelogs.Files, error) {
	application.coreLogsMu.Lock()
	defer application.coreLogsMu.Unlock()
	if application.coreLogs == nil {
		files, err := corelogs.New(application.settings.DataDir)
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
		details := telemetry.OperationLogContext{StartedAt: started}
		if !errors.Is(err, corelogs.ErrInvalidFile) {
			details.File = name
		}
		application.telemetry.RecordOperation(ctx, "core.log.delete", "Core log file deletion", err, details)
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
		details := telemetry.OperationLogContext{StartedAt: started}
		if !errors.Is(err, corelogs.ErrInvalidFile) {
			details.File = name
		}
		application.telemetry.RecordOperation(ctx, "core.log.clear", "Core log clearing", err, details)
	}()
	files, err := application.coreLogFiles()
	if err != nil {
		return err
	}
	return files.Clear(name)
}

// SetCoreLogs attaches the server-owned writer before requests are served.
func (app *Node) SetCoreLogs(files *corelogs.Files) {
	app.coreLogsMu.Lock()
	defer app.coreLogsMu.Unlock()
	app.coreLogs = files
}
func coreLogPolicy(value settings.Settings) corelogs.Policy {
	return corelogs.Policy{RetentionDays: value.Logs.CoreRetentionDays, MaxFiles: value.Logs.CoreMaxFiles, MaxFileBytes: int64(value.Logs.CoreMaxFileSizeMiB) << 20}
}
func (app *Node) MaintainCoreLogs() error {
	app.coreLogsMu.Lock()
	defer app.coreLogsMu.Unlock()
	if app.coreLogs == nil {
		return nil
	}
	return app.coreLogs.EnsureCurrentFile()
}

// applyCoreLogPolicy installs the committed core log policy. Cleanup is retried
// by the retention worker; the validated policy is installed even when
// removing an old file temporarily fails.
func (app *Node) applyCoreLogPolicy(value settings.Settings) {
	app.coreLogsMu.Lock()
	files := app.coreLogs
	app.coreLogsMu.Unlock()
	if files != nil {
		_ = files.SetPolicy(coreLogPolicy(value))
	}
}
