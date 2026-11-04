// SPDX-License-Identifier: GPL-3.0-or-later

package localnode

import (
	"errors"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/corelogs"
	coreruntime "github.com/rehuony/sing-box-panel/internal/sing-box-node/runtime"
)

func IsNoRunningCore(err error) bool {
	return errors.Is(err, ErrNoRunningCore)
}

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
	} {
		if errors.Is(err, kind.cause) {
			return kind.code, kind.message, true
		}
	}
	return "", "", false
}
