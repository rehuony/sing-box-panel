// SPDX-License-Identifier: GPL-3.0-or-later

package corecache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
)

const coreUploadPrefix = "core-upload-"

// CoreUploadGCResult summarizes one conservative startup garbage-collection
// pass. It only visits regular files in the private staging directory.
type CoreUploadGCResult struct {
	Deleted  int
	Retained int
	Skipped  int
	Aborted  bool
}

// GarbageCollectCoreUploads runs before the server accepts uploads, while it
// holds the exclusive data-directory lease. Every remaining staged file belongs
// to an interrupted request and can be removed conservatively.
func (application *Service) GarbageCollectCoreUploads(ctx context.Context) (CoreUploadGCResult, error) {
	result := CoreUploadGCResult{}
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
		if entry.Type()&os.ModeSymlink != 0 {
			result.Skipped++
			continue
		}
		fileInfo, infoErr := entry.Info()
		if infoErr != nil {
			if !errors.Is(infoErr, os.ErrNotExist) {
				application.RecordUploadCleanupWarning(
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
			application.RecordUploadCleanupWarning(
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

func (application *Service) privateCoreUploadDirectory() (string, error) {
	if application.dataDir == "" || !filepath.IsAbs(application.dataDir) ||
		filepath.Clean(application.dataDir) != application.dataDir {
		return "", errors.New("panel data directory is unavailable for private core uploads")
	}
	return filepath.Join(application.dataDir, "imports"), nil
}

func (application *Service) isPrivateUploadedCorePath(path string) bool {
	directory, err := application.privateCoreUploadDirectory()
	if err != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	base := filepath.Base(path)
	return filepath.Dir(path) == directory && strings.HasPrefix(base, coreUploadPrefix) && len(base) > len(coreUploadPrefix)
}

// ValidateStagedUpload checks that path is a regular file in the private
// upload staging directory.
func (application *Service) ValidateStagedUpload(path string) error {
	if !application.isPrivateUploadedCorePath(path) {
		return errors.New("core import upload path is outside the private staging directory")
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

// RemoveStagedUpload removes one staged upload.
func (application *Service) RemoveStagedUpload(path string) error {
	if !application.isPrivateUploadedCorePath(path) {
		return errors.New("core import upload path is outside the private staging directory")
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

func (application *Service) remove(path string) error {
	if application.removeFile == nil {
		return os.Remove(path)
	}
	return application.removeFile(path)
}

// RecordUploadCleanupWarning records a staged upload that could not be removed.
func (application *Service) RecordUploadCleanupWarning(
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
	_, _ = application.telemetry.RecordLog(logContext, telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: store.LogLevelWarn, Code: code,
		Message: message, Metadata: encoded,
	})
}
