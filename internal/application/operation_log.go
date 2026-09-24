// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/rehuony/sing-box-panel/internal/corelogs"
	coreruntime "github.com/rehuony/sing-box-panel/internal/runtime"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// OperationLogContext contains only bounded identifiers and timing, never request bodies.
type OperationLogContext struct {
	StartedAt          time.Time
	File               string
	CoreID             string
	StartupArtifactID  string
	ActivationBundleID string
	Generation         int64
}

func (details OperationLogContext) metadata(operationErr error) json.RawMessage {
	metadata := make(map[string]any)
	if !details.StartedAt.IsZero() {
		metadata["duration_ms"] = max(0, time.Since(details.StartedAt).Milliseconds())
	}
	for key, value := range map[string]string{
		"file": details.File, "core_id": details.CoreID,
		"startup_artifact_id": details.StartupArtifactID, "activation_bundle_id": details.ActivationBundleID,
	} {
		// Invalid or oversized request targets must not discard the failure event.
		if value != "" && len(value) <= store.MaximumLogIDBytes {
			metadata[key] = value
		}
	}
	if details.Generation > 0 {
		metadata["generation"] = details.Generation
	}
	if operationErr != nil {
		metadata["error_code"], metadata["error"] = operationLogFailure(operationErr)
	}
	encoded, _ := json.Marshal(metadata) // Only strings and integers are encoded.
	return encoded
}

func operationLogFailure(err error) (string, string) {
	// Only known sentinel descriptions are safe. Wrapped errors can contain
	// configuration bytes, source URLs, credentials, or process output.
	for _, kind := range runtimeErrorKinds {
		if errors.Is(err, kind.cause) {
			return kind.code, kind.cause.Error()
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
		{"file_not_found", os.ErrNotExist, "The target file does not exist."},
		{"permission_denied", os.ErrPermission, "The panel does not have permission to access the target."},
	} {
		if errors.Is(err, kind.cause) {
			return kind.code, kind.message
		}
	}
	return "operation_failed", "The operation failed; no safe diagnostic is available."
}
