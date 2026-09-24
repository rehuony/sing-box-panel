// SPDX-License-Identifier: GPL-3.0-or-later
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var errRuntimeEvidenceUnavailable = errors.New("runtime process evidence is unavailable")

type runtimeExecutionGuard interface{ SafePoint(context.Context) error }

type runtimeResult struct{ Runtime *store.RuntimeCommit }
type runtimeGuard struct {
	database   *store.Store
	generation int64
}

func (g runtimeGuard) SafePoint(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return g.database.CheckRuntimeGeneration(ctx, g.generation)
}

func (s *runtimeServices) ExecuteRuntime(ctx context.Context, input application.RuntimeRequest) (result application.RuntimeResponse, runErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if s.lifetime != nil {
		stop := context.AfterFunc(s.lifetime, cancel)
		defer stop()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var intent store.RuntimeIntent
	started := time.Now()
	defer func() {
		details := application.OperationLogContext{
			StartedAt: started, CoreID: input.CoreID, StartupArtifactID: input.StartupArtifactID,
			ActivationBundleID: input.BundleID, Generation: intent.Generation,
		}
		if intent.ActivationBundleID != "" {
			details.ActivationBundleID = intent.ActivationBundleID
		}
		if intent.StartupArtifactID != "" {
			details.StartupArtifactID = intent.StartupArtifactID
		}
		s.commands.RecordOperation(ctx, "runtime."+input.Action, "Core "+input.Action, runErr, details)
	}()
	var err error
	switch input.Action {
	case "enable":
		intent, err = s.commands.PrepareCoreEnable(ctx, input.CoreID)
	case "disable":
		intent, err = s.commands.PrepareCoreDisable(ctx, input.CoreID)
	case "start":
		intent, err = s.commands.PrepareConfigurationRuntime(ctx, "", store.RuntimeIntentStart)
	case "restart":
		intent, err = s.commands.PrepareConfigurationRuntime(ctx, "", store.RuntimeIntentRestart)
	case "stop":
		intent, err = s.commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentStop, "")
	case "rollback":
		intent, err = s.commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentRollback, input.BundleID)
	case "activate":
		prepared, prepareErr := s.commands.PrepareActivationBundle(ctx, input.StartupArtifactID, input.MonitoringTier)
		if prepareErr != nil {
			return result, prepareErr
		}
		summary := prepared.Summary()
		result.Activation = &summary
		intent, err = s.commands.PrepareRuntimeIntent(ctx, store.RuntimeIntentApply, prepared.Bundle.ID)
	case "check":
		summary, checkErr := s.checkStartup(ctx, input.StartupArtifactID)
		result.Startup = &summary
		return result, checkErr
	default:
		return result, errors.New("unsupported runtime action")
	}
	if err != nil {
		return result, err
	}
	if err = s.executeIntent(ctx, &intent); err != nil {
		return result, err
	}
	result.Status, err = s.commands.RuntimeStatus(ctx)
	return result, err
}

// executeIntent runs under the runtime mutex. Completion has its own short
// deadline so cancellation cannot discard evidence of a process already changed.
// The caller retains the checked target for operation logging, even on failure.
func (s *runtimeServices) executeIntent(ctx context.Context, intent *store.RuntimeIntent) error {
	guard := runtimeGuard{database: s.database, generation: intent.Generation}
	if err := guard.SafePoint(ctx); err != nil {
		return err
	}
	if intent.StartupArtifactID != "" && intent.ActivationBundleID == "" {
		if intent.Kind == store.RuntimeIntentStart && s.manager.ObserveLiveIdentity().Running {
			return errors.New("core is already running; restart to load saved configuration")
		}
		checked, err := s.checkConfigurationForIntent(ctx, *intent, guard)
		if err != nil {
			return err
		}
		*intent = checked
	}
	result, runErr := s.performRuntimeIntent(ctx, *intent, guard)
	completionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	completeErr := s.database.CompleteRuntimeIntent(completionCtx, *intent, runErr == nil, result.Runtime, time.Now().UTC())
	if completeErr != nil && runErr == nil && s.manager.ObserveLiveIdentity().Running {
		observation, readErr := s.captureRuntimeObservation(completionCtx)
		commit, stopErr := s.stopAfterLostIntent(observation, *intent, intent.ActivationBundleID, store.RuntimeTransitionFailed, "runtime_commit_failed")
		if readErr == nil && stopErr == nil {
			stopErr = s.database.CompleteRuntimeIntent(completionCtx, *intent, false, commit, time.Now().UTC())
		}
		completeErr = errors.Join(completeErr, readErr, stopErr)
	}
	if runErr == nil && completeErr == nil && s.syncEnabledCore != nil {
		completeErr = s.syncEnabledCore(completionCtx)
	}
	return errors.Join(runErr, completeErr)
}

func (s *runtimeServices) checkStartup(ctx context.Context, id string) (application.StartupArtifactSummary, error) {
	material, err := s.commands.LoadStartupCheckMaterial(ctx, id)
	if err != nil {
		return application.StartupArtifactSummary{}, err
	}
	checkErr := s.manager.Check(ctx, material.Bundle)
	completionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	completed, err := s.commands.CompleteStartupCheck(completionCtx, id, checkErr == nil)
	if err != nil {
		return application.StartupArtifactSummary{}, errors.Join(checkErr, err)
	}
	return application.StartupArtifactSummary{ID: completed.ID, CanonicalRevisionID: completed.CanonicalRevisionID, ExactCoreVersion: completed.ExactCoreVersion, CoreArtifactID: completed.CoreArtifactID, ConfigSHA256: completed.ConfigSHA256, State: completed.State, CheckedAt: completed.CheckedAt, CreatedAt: completed.CreatedAt}, checkErr
}

func (s *runtimeServices) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var input application.RuntimeRequest
	body, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
	if err == nil {
		err = jsonstrict.Decode(body, 64<<10, &input)
	}
	var result application.RuntimeResponse
	if err == nil {
		result, err = s.ExecuteRuntime(r.Context(), input)
	}
	response := struct {
		Result application.RuntimeResponse      `json:"result"`
		Error  *application.RuntimeControlError `json:"error,omitempty"`
	}{Result: result}
	if err != nil {
		response.Error = application.NewRuntimeControlError(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}
