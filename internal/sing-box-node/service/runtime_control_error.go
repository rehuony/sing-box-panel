// SPDX-License-Identifier: GPL-3.0-or-later
package service

import (
	"context"
	"errors"

	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/store"
)

// RuntimeControlError classifies a runtime failure by a stable code that the
// panel maps to its problem details.
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
	{"no_configuration", ErrNoConfiguration},
	{"runtime_not_started", ErrRuntimeNotStarted},
	{"revision_conflict", store.ErrRevisionConflict},
	{"core_artifact_in_use", store.ErrCoreArtifactInUse},
	{"configuration_digest", ErrConfigurationDigest},
	{"configuration_invalid", configuration.ErrInvalidDocument},
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
