// SPDX-License-Identifier: GPL-3.0-or-later
package application

import (
	"context"
	"errors"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/store"
)

// RuntimeControlError preserves domain classification across the private socket,
// so local CLI commands keep the same exit categories as in-process calls.
type RuntimeControlError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var runtimeErrorKinds = []struct {
	code  string
	cause error
}{
	{"canceled", context.Canceled},
	{"deadline", context.DeadlineExceeded},
	{"configuration_file_unparsed", store.ErrConfigurationFileUnparsed},
	{"configuration_invalid", configuration.ErrInvalidDocument},
	{"configuration_schema_validation", ErrConfigurationSchemaValidation},
	{"configuration_changed", store.ErrCompiledStartupEvidenceStale},
	{"core_artifact_not_found", store.ErrCoreArtifactNotFound},
	{"core_platform_mismatch", ErrCorePlatformMismatch},
	{"core_not_enabled", store.ErrCoreNotEnabled},
	{"no_applied_bundle", store.ErrNoAppliedBundle},
	{"no_rollback_bundle", store.ErrNoRollbackBundle},
	{"startup_artifact_not_found", store.ErrStartupArtifactNotFound},
	{"startup_artifact_state", store.ErrStartupArtifactState},
	{"activation_bundle_not_ready", store.ErrActivationBundleNotReady},
	{"runtime_intent_stale", store.ErrRuntimeIntentStale},
	{"monitoring_tier_unavailable", ErrMonitoringTierUnavailable},
	{"stale_observation", ErrStaleObservation},
	{"inspection_unavailable", ErrInspectionUnavailable},
}

func NewRuntimeControlError(err error) *RuntimeControlError {
	if err == nil {
		return nil
	}
	result := &RuntimeControlError{Code: "runtime_failed", Message: err.Error()}
	for _, kind := range runtimeErrorKinds {
		if errors.Is(err, kind.cause) {
			result.Code = kind.code
			break
		}
	}
	return result
}
func (e *RuntimeControlError) Error() string { return e.Message }
func (e *RuntimeControlError) Unwrap() error {
	for _, kind := range runtimeErrorKinds {
		if e.Code == kind.code {
			return kind.cause
		}
	}
	return nil
}
