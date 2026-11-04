// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
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

// SetCoreLogs attaches the server-owned writer before requests are served.
func (app *Node) SetCoreLogs(files *corelogs.Files) {
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
