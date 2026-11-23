// SPDX-License-Identifier: GPL-3.0-or-later

package service

import (
	"errors"
	"os"
	"time"
)

// Event sources and levels.
const (
	SourceNode = "node"
	SourceCore = "core"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Event is one node log record: an operation outcome or a runtime event. It
// carries only bounded identifiers and safe descriptions, never request
// bodies, configuration text or credentials.
type Event struct {
	Time     time.Time
	Source   string
	Level    string
	Code     string
	Message  string
	Metadata map[string]any
}

// OperationContext identifies what a completed operation acted on.
type OperationContext struct {
	StartedAt          time.Time
	File               string
	CoreID             string
	StartupArtifactID  string
	ActivationBundleID string
	Generation         int64
}

func (application *Node) record(event Event) {
	if application.events == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = application.now().UTC()
	}
	application.events(event)
}

// recordOperation records a completed operation without its inputs. A
// failure is described only by a known, safe classification.
func (application *Node) recordOperation(code, message string, operationErr error, details OperationContext) {
	metadata := map[string]any{}
	if !details.StartedAt.IsZero() {
		metadata["duration_ms"] = max(0, time.Since(details.StartedAt).Milliseconds())
	}
	for key, value := range map[string]string{
		"file": details.File, "core_id": details.CoreID,
		"startup_artifact_id": details.StartupArtifactID, "activation_bundle_id": details.ActivationBundleID,
	} {
		if value != "" && len(value) <= 256 {
			metadata[key] = value
		}
	}
	if details.Generation > 0 {
		metadata["generation"] = details.Generation
	}
	level := LevelInfo
	if operationErr != nil {
		level = LevelError
		code, message = code+".failed", message+" failed"
		metadata["error_code"], metadata["error"] = DescribeFailure(operationErr)
	} else {
		code, message = code+".completed", message+" completed"
	}
	application.record(Event{Source: SourceNode, Level: level, Code: code, Message: message, Metadata: metadata})
}

// DescribeFailure names a failure by a stable code and a message that is
// safe to show: it never quotes configuration, output or credentials.
func DescribeFailure(err error) (string, string) {
	if code, message, ok := ClassifyOperationFailure(err); ok {
		return code, message
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "file_not_found", "The target file does not exist."
	case errors.Is(err, os.ErrPermission):
		return "permission_denied", "The node does not have permission to access the target."
	}
	return "operation_failed", "The operation failed; no safe diagnostic is available."
}
