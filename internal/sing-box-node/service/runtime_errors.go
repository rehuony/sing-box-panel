// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"errors"
	"io/fs"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/artifacts"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/host"
	coreruntime "github.com/rehuony/sing-box-panel/internal/sing-box-node/runtime"
)

// ClassifyOperationFailure names runtime and core failures that are safe to
// record in operation outcomes.
func ClassifyOperationFailure(err error) (string, string, bool) {
	for _, kind := range runtimeErrorKinds {
		if errors.Is(err, kind.cause) {
			return kind.code, kind.cause.Error(), true
		}
	}
	for _, kind := range []struct {
		code    string
		cause   error
		message string
	}{
		{"core_check_failed", coreruntime.ErrCheckFailed, "The core rejected the startup configuration."},
		{"core_health_failed", coreruntime.ErrHealthFailed, "The core failed its health check."},
		{"core_version_mismatch", coreruntime.ErrVersionMismatch, "The core version does not match the selected artifact."},
		{"core_termination_failed", coreruntime.ErrTermination, "The core process could not be terminated."},
		{"core_log_invalid", corelogs.ErrInvalidFile, "Select a managed core log file."},
		{"core_log_current", corelogs.ErrCurrentFile, "Log files from the current UTC day cannot be deleted."},
		{"filesystem_invalid", host.ErrInvalidPath, "Check the path, selection mode and pagination parameters."},
		{"filesystem_forbidden", fs.ErrPermission, "The node process cannot access this path."},
		{"filesystem_not_found", host.ErrBrokenLink, "The path or symbolic link target no longer exists."},
		{"filesystem_not_found", fs.ErrNotExist, "The path or symbolic link target no longer exists."},
		{"filesystem_type_mismatch", host.ErrPathType, "The target has the wrong type for this field."},
		{"filesystem_directory_too_large", host.ErrDirectoryTooLarge, "This directory exceeds the 20000-entry browsing limit. Enter a subdirectory path directly."},
	} {
		if errors.Is(err, kind.cause) {
			return kind.code, kind.message, true
		}
	}
	var install *artifacts.Failure
	if errors.As(err, &install) {
		return "core_install_failed", "The core archive failed verification on the node (" + install.Code + ").", true
	}
	return "", "", false
}
