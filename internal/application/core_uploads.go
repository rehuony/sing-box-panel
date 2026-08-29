// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/store"
)

const coreUploadPrefix = "core-upload-"

// CoreUploadGCResult summarizes one conservative startup garbage-collection
// pass. Aborted means no deletion was attempted because an active task payload
// could not be interpreted safely.
type CoreUploadGCResult struct {
	Deleted  int
	Retained int
	Skipped  int
	Aborted  bool
}

// FinalizeTaskResources releases task-owned files only after the durable task
// has reached a committed terminal state. Cleanup is deliberately best effort:
// a filesystem failure is recorded but never changes the task result.
func (application *Application) FinalizeTaskResources(ctx context.Context, task store.Task) {
	if !terminalTaskStatus(task.Status) {
		return
	}
	if err := application.finalizeTaskResources(task); err != nil {
		application.recordCoreUploadCleanupWarning(
			ctx,
			"task.resource_cleanup_failed",
			"A durable task completed but its staged upload could not be removed",
			map[string]any{"task_id": task.ID, "kind": task.Kind, "error": err.Error()},
		)
	}
}

func (application *Application) finalizeTaskResources(task store.Task) error {
	if task.Kind != store.TaskKindCoreImport {
		return nil
	}
	input, err := decodeCoreImportPayload(task.Payload)
	if err != nil {
		return fmt.Errorf("decode terminal core import payload: %w", err)
	}
	if !input.DeleteSource {
		return nil
	}
	return application.removePrivateUploadedCore(input.SourcePath)
}

// GarbageCollectCoreUploads removes only unreferenced, directly contained
// regular staging files. It first parses every active core-import payload; one
// malformed payload aborts the entire pass so uncertainty can only leak files,
// never delete a task resource that may still be needed for lease reclaim.
func (application *Application) GarbageCollectCoreUploads(ctx context.Context) (CoreUploadGCResult, error) {
	result := CoreUploadGCResult{}
	references, err := application.activeCoreUploadReferences(ctx)
	if err != nil {
		result.Aborted = true
		return result, err
	}

	directory, err := application.privateCoreUploadDirectory()
	if err != nil {
		result.Aborted = true
		return result, err
	}
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		result.Aborted = true
		return result, fmt.Errorf("inspect private core upload directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		result.Aborted = true
		return result, errors.New("private core upload path is not a physical directory")
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		result.Aborted = true
		return result, fmt.Errorf("list private core uploads: %w", err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), coreUploadPrefix) {
			result.Skipped++
			continue
		}
		path := filepath.Join(directory, entry.Name())
		if _, retained := references[path]; retained {
			result.Retained++
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			result.Skipped++
			continue
		}
		fileInfo, infoErr := entry.Info()
		if infoErr != nil {
			if !errors.Is(infoErr, os.ErrNotExist) {
				application.recordCoreUploadCleanupWarning(
					ctx,
					"core_upload.gc_inspection_failed",
					"A staged core upload could not be inspected during startup cleanup",
					map[string]any{"name": entry.Name(), "error": infoErr.Error()},
				)
			}
			result.Skipped++
			continue
		}
		if !fileInfo.Mode().IsRegular() {
			result.Skipped++
			continue
		}
		if removeErr := application.remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			application.recordCoreUploadCleanupWarning(
				ctx,
				"core_upload.gc_delete_failed",
				"An unreferenced staged core upload could not be removed",
				map[string]any{"name": entry.Name(), "error": removeErr.Error()},
			)
			result.Skipped++
			continue
		}
		result.Deleted++
	}
	return result, nil
}

func (application *Application) activeCoreUploadReferences(ctx context.Context) (map[string]struct{}, error) {
	references := make(map[string]struct{})
	for _, status := range []store.TaskStatus{store.TaskStatusQueued, store.TaskStatusRunning} {
		var cursor *store.CreatedAtCursor
		for {
			page, err := application.database.ListTasks(ctx, store.TaskListFilter{
				Status: status, Kind: store.TaskKindCoreImport, Cursor: cursor, Limit: 200,
			})
			if err != nil {
				return nil, fmt.Errorf("list active core import tasks: %w", err)
			}
			for _, task := range page.Items {
				input, err := decodeCoreImportPayload(task.Payload)
				if err != nil {
					return nil, fmt.Errorf("decode active core import task %q: %w", task.ID, err)
				}
				if !input.DeleteSource {
					continue
				}
				if !application.isPrivateUploadedCorePath(input.SourcePath) {
					return nil, fmt.Errorf("active core import task %q has an unsafe staged upload path", task.ID)
				}
				references[input.SourcePath] = struct{}{}
			}
			if page.Next == nil {
				break
			}
			cursor = page.Next
		}
	}
	return references, nil
}

func decodeCoreImportPayload(payload json.RawMessage) (coreImportPayload, error) {
	var input coreImportPayload
	if err := jsonstrict.Decode(payload, 128<<10, &input); err != nil {
		return coreImportPayload{}, err
	}
	if !filepath.IsAbs(input.SourcePath) || filepath.Clean(input.SourcePath) != input.SourcePath {
		return coreImportPayload{}, errors.New("core import source path is not absolute and clean")
	}
	digest, err := coreartifact.ParseSHA256(input.SHA256)
	if err != nil || digest.IsZero() {
		return coreImportPayload{}, errors.New("core import digest is invalid")
	}
	version, err := coreartifact.ParseExactVersion(input.ExactVersion)
	if err != nil || version.IsZero() {
		return coreImportPayload{}, errors.New("core import version is invalid")
	}
	source, err := coreartifact.NewUserSource(input.SourceDescription)
	if err != nil {
		return coreImportPayload{}, err
	}
	if _, err := coreartifact.NewIdentity(
		source,
		digest,
		coreartifact.OperatingSystemLinux,
		coreartifact.Architecture(input.Architecture),
		coreartifact.Variant(input.Variant),
		version,
	); err != nil {
		return coreImportPayload{}, err
	}
	return input, nil
}

func (application *Application) privateCoreUploadDirectory() (string, error) {
	if application.settings.DataDir == "" || !filepath.IsAbs(application.settings.DataDir) ||
		filepath.Clean(application.settings.DataDir) != application.settings.DataDir {
		return "", errors.New("panel data directory is unavailable for private core uploads")
	}
	return filepath.Join(application.settings.DataDir, "imports"), nil
}

func (application *Application) isPrivateUploadedCorePath(path string) bool {
	directory, err := application.privateCoreUploadDirectory()
	if err != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	base := filepath.Base(path)
	return filepath.Dir(path) == directory && strings.HasPrefix(base, coreUploadPrefix) && len(base) > len(coreUploadPrefix)
}

func (application *Application) validatePrivateUploadedCoreFile(path string) error {
	if !application.isPrivateUploadedCorePath(path) {
		return errors.New("core import task upload path is outside the private staging directory")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect private staged core upload: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("private staged core upload is not a regular file")
	}
	return nil
}

func (application *Application) removePrivateUploadedCore(path string) error {
	if !application.isPrivateUploadedCorePath(path) {
		return errors.New("core import task upload path is outside the private staging directory")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect private staged core upload: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("private staged core upload is not a regular file")
	}
	if err := application.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove private staged core upload: %w", err)
	}
	return nil
}

func (application *Application) remove(path string) error {
	if application.removeFile == nil {
		return os.Remove(path)
	}
	return application.removeFile(path)
}

func (application *Application) recordCoreUploadCleanupWarning(
	ctx context.Context,
	code string,
	message string,
	metadata map[string]any,
) {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		encoded = json.RawMessage(`{}`)
	}
	if ctx == nil {
		ctx = context.Background()
	} else {
		ctx = context.WithoutCancel(ctx)
	}
	logContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, _ = application.RecordLog(logContext, LogRecordRequest{
		Source: store.LogSourceTask, Level: store.LogLevelWarn, Code: code,
		Message: message, Metadata: encoded,
	})
}

func terminalTaskStatus(status store.TaskStatus) bool {
	switch status {
	case store.TaskStatusSucceeded, store.TaskStatusFailed, store.TaskStatusCanceled, store.TaskStatusSuperseded:
		return true
	default:
		return false
	}
}
