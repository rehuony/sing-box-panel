// SPDX-License-Identifier: GPL-3.0-or-later
package telemetry

import (
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
)

// OperationLogContext contains only bounded identifiers and timing, never request bodies.
type OperationLogContext struct {
	StartedAt          time.Time
	NodeID             string
	File               string
	CoreID             string
	StartupArtifactID  string
	ActivationBundleID string
	Generation         int64
}

func (details OperationLogContext) metadata(operationErr error, classify Classifier) json.RawMessage {
	metadata := make(map[string]any)
	if !details.StartedAt.IsZero() {
		metadata["duration_ms"] = max(0, time.Since(details.StartedAt).Milliseconds())
	}
	for key, value := range map[string]string{
		"node_id": details.NodeID, "file": details.File, "core_id": details.CoreID,
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
		metadata["error_code"], metadata["error"] = operationLogFailure(operationErr, classify)
	}
	encoded, _ := json.Marshal(metadata) // Only strings and integers are encoded.
	return encoded
}

// Classifier names a failure that a service knows to be safe to describe. It
// returns ok false for errors it does not recognize.
type Classifier func(error) (code, message string, ok bool)

func operationLogFailure(err error, classify Classifier) (string, string) {
	// Only known sentinel descriptions are safe. Wrapped errors can contain
	// configuration bytes, source URLs, credentials, or process output.
	if classify != nil {
		if code, message, ok := classify(err); ok {
			return code, message
		}
	}
	for _, kind := range []struct {
		code    string
		cause   error
		message string
	}{
		{"file_not_found", os.ErrNotExist, "The target file does not exist."},
		{"permission_denied", os.ErrPermission, "The panel does not have permission to access the target."},
	} {
		if errors.Is(err, kind.cause) {
			return kind.code, kind.message
		}
	}
	return "operation_failed", "The operation failed; no safe diagnostic is available."
}
