// SPDX-License-Identifier: GPL-3.0-or-later

package releases

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	releasecontract "github.com/rehuony/sing-box-panel/internal/release"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/telemetry"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"golang.org/x/mod/semver"
)

// States of the panel's own update.
const (
	// StateIdle: no update runs.
	StateIdle = "idle"
	// StateUpdating: the release is being downloaded, verified and
	// installed or staged.
	StateUpdating = "updating"
	// StateStaged: the system service staged the release for its root
	// update helper, which installs it and restarts the panel.
	StateStaged = "staged"
	// StateRestarting: the release is installed and the panel's unit
	// restarts into it.
	StateRestarting = "restarting"
	// StateRestartRequired: the release is installed and runs once the
	// panel is restarted.
	StateRestartRequired = "restart_required"
)

const (
	checkRecheck = time.Minute
	checkRetry   = 5 * time.Minute
	// resultCheck is how often the panel looks for its root update helper's
	// result.
	resultCheck = 2 * time.Second
)

var (
	// ErrUpdateUnsupported answers updates of a panel that is not a release
	// build on a supported platform.
	ErrUpdateUnsupported = errors.New("self-update needs a release build of the panel on linux/amd64 or linux/arm64")
	// ErrUpdateRunning refuses a second update while one runs or awaits its
	// restart.
	ErrUpdateRunning = errors.New("an update of the panel is already in progress")
	// ErrNoUpdate refuses an update when no newer release is known.
	ErrNoUpdate = errors.New("no newer release of the panel is known; check for updates first")
)

// Attempt is the outcome of an update.
type Attempt = selfupdate.Attempt

// Failure is an update failure with its short code, which operation logs
// may record; the error it wraps may not be safe to show.
type Failure struct {
	Code string
	err  error
}

func (failure *Failure) Error() string { return "update failed: " + failure.Code }

func (failure *Failure) Unwrap() error { return failure.err }

// Failed wraps an update error, or returns nil.
func Failed(err error) error {
	if err == nil {
		return nil
	}
	var known *Failure
	if errors.As(err, &known) {
		return err
	}
	return &Failure{Code: selfupdate.FailureCode(err), err: err}
}

// UpdateStatus is the panel's version, the newest release it knows and the
// state of its update.
type UpdateStatus struct {
	CurrentVersion string `json:"current_version"`
	// Supported reports a release build on a supported platform.
	Supported     bool       `json:"supported"`
	LatestVersion *string    `json:"latest_version"`
	CheckedAt     *time.Time `json:"checked_at"`
	// CheckFailure is the code of the last check's failure.
	CheckFailure    *string  `json:"check_failure"`
	UpdateAvailable bool     `json:"update_available"`
	State           string   `json:"state"`
	TargetVersion   *string  `json:"target_version"`
	LastAttempt     *Attempt `json:"last_attempt"`
}

type updateState struct {
	latest       string
	checkedAt    time.Time
	checkFailure string
	state        string
	target       string
	last         *Attempt
}

func (service *Service) updatesDir() string {
	return filepath.Join(service.dataDir, selfupdate.UpdatesDir)
}

func (service *Service) supported() bool {
	return releasecontract.ValidateVersion(service.build.Version) == nil && selfupdate.SupportedPlatform(runtime.GOOS, runtime.GOARCH)
}

// Status returns the state of the panel's update.
func (service *Service) Status() UpdateStatus {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.statusLocked()
}

func (service *Service) statusLocked() UpdateStatus {
	value := service.update
	status := UpdateStatus{
		CurrentVersion: service.build.Version, Supported: service.supported(), State: value.state, LastAttempt: value.last,
	}
	if value.latest != "" {
		latest := value.latest
		status.LatestVersion = &latest
		status.UpdateAvailable = status.Supported && semver.Compare(latest, service.build.Version) > 0
	}
	if !value.checkedAt.IsZero() {
		checked := value.checkedAt
		status.CheckedAt = &checked
	}
	if value.checkFailure != "" {
		failure := value.checkFailure
		status.CheckFailure = &failure
	}
	if value.target != "" {
		target := value.target
		status.TargetVersion = &target
	}
	return status
}

// Check asks GitHub for the newest stable release.
func (service *Service) Check(ctx context.Context) (UpdateStatus, error) {
	if !service.supported() {
		return service.Status(), ErrUpdateUnsupported
	}
	latest, err := service.updater(selfupdate.Panel).Latest(ctx)
	service.mu.Lock()
	defer service.mu.Unlock()
	service.update.checkedAt = service.options.Now().UTC()
	service.update.checkFailure = ""
	if err != nil {
		service.update.checkFailure = selfupdate.FailureCode(err)
		return service.statusLocked(), err
	}
	service.update.latest = latest
	return service.statusLocked(), nil
}

// Update starts updating the panel to the newest stable release it knows,
// in the background: the system service stages it for its root update
// helper, and a panel that may replace its executable installs it and, under
// a user unit, restarts. Status follows its progress.
func (service *Service) Update(ctx context.Context) (UpdateStatus, error) {
	if !service.supported() {
		return service.Status(), ErrUpdateUnsupported
	}
	service.mu.Lock()
	if service.update.state != StateIdle {
		defer service.mu.Unlock()
		return service.statusLocked(), ErrUpdateRunning
	}
	target := service.update.latest
	if target == "" || semver.Compare(target, service.build.Version) <= 0 {
		defer service.mu.Unlock()
		return service.statusLocked(), ErrNoUpdate
	}
	service.update.state, service.update.target = StateUpdating, target
	lifetime := service.lifetime
	status := service.statusLocked()
	service.mu.Unlock()
	go service.runUpdate(context.WithoutCancel(lifetime), target)
	return status, nil
}

func (service *Service) runUpdate(ctx context.Context, version string) {
	started := service.options.Now()
	state, err := service.deliver(ctx, version)
	attempt := &Attempt{Version: version, Succeeded: err == nil, FinishedAt: service.options.Now().UTC()}
	if err != nil {
		attempt.Code = selfupdate.FailureCode(err)
	}
	if service.records != nil {
		service.records.RecordOperation(ctx, "panel.update", "Panel update", Failed(err), telemetry.OperationLogContext{StartedAt: started})
	}
	service.mu.Lock()
	service.update.state = state
	if err != nil {
		service.update.state, service.update.target = StateIdle, ""
	}
	// A staged release reports its result through the helper.
	if state != StateStaged {
		service.update.last = attempt
		_ = selfupdate.RecordAttempt(service.updatesDir(), *attempt)
	}
	service.mu.Unlock()
	if state == StateRestarting {
		if err := service.restart(ctx); err != nil {
			service.mu.Lock()
			service.update.state = StateRestartRequired
			service.mu.Unlock()
		}
	}
}

// deliver fetches, verifies and installs or stages version, and returns the
// state it leaves.
func (service *Service) deliver(ctx context.Context, version string) (string, error) {
	updater := service.updater(selfupdate.Panel)
	incoming, err := selfupdate.IncomingDir(service.updatesDir())
	if err != nil {
		return StateIdle, err
	}
	defer func() { _ = os.RemoveAll(incoming) }()
	if err := updater.Fetch(ctx, version, []string{updater.AssetName()}, incoming); err != nil {
		return StateIdle, err
	}
	staged, err := updater.Verify(incoming, version, service.build.Version)
	if err != nil {
		return StateIdle, err
	}
	scope, supervised := systemd.RunningScope()
	delivery, err := updater.Deliver(ctx, staged, service.updatesDir(), supervised && scope == systemd.ScopeSystem)
	switch {
	case err != nil:
		return StateIdle, err
	case delivery == selfupdate.DeliveryStaged:
		return StateStaged, nil
	case supervised:
		return StateRestarting, nil
	default:
		return StateRestartRequired, nil
	}
}

func (service *Service) restart(ctx context.Context) error {
	if service.options.Restart == nil {
		return errors.New("the panel cannot restart its unit")
	}
	return service.options.Restart(ctx)
}

// Run reads the result of an update the root helper made since the panel
// last ran, records it, clears finished staging, and checks for a new
// release on the catalog refresh interval until ctx ends.
func (service *Service) Run(ctx context.Context) {
	service.mu.Lock()
	service.lifetime = ctx
	service.mu.Unlock()
	attempt, fresh, err := selfupdate.TakeAttempt(service.updatesDir())
	if err == nil && attempt != nil {
		service.mu.Lock()
		service.update.last = attempt
		service.mu.Unlock()
		if fresh {
			service.recordHelperAttempt(*attempt)
		}
	}
	_ = selfupdate.CleanUpdates(service.updatesDir())
	if !service.supported() {
		return
	}
	var changes <-chan struct{}
	if service.settings != nil {
		var unsubscribe func()
		changes, unsubscribe = service.settings.SettingsChanges()
		defer unsubscribe()
	}
	results := time.NewTicker(resultCheck)
	defer results.Stop()
	timer := time.NewTimer(service.checkStep(ctx))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-results.C:
			service.takeHelperResult()
			continue
		case <-changes:
		case <-timer.C:
		}
		timer.Reset(service.checkStep(ctx))
	}
}

// takeHelperResult records a result the root update helper wrote while the
// panel runs, such as its refusal of the staged release, or withdraws a
// staged release the helper never took.
func (service *Service) takeHelperResult() {
	attempt, err := selfupdate.TakeFreshAttempt(service.updatesDir())
	if err == nil && attempt == nil {
		attempt, err = selfupdate.WithdrawUnclaimed(service.updatesDir(), service.options.Now())
	}
	if err != nil || attempt == nil {
		return
	}
	service.mu.Lock()
	service.update.last = attempt
	if service.update.state == StateStaged {
		service.update.state, service.update.target = StateIdle, ""
		if attempt.Succeeded {
			service.update.state, service.update.target = StateRestarting, attempt.Version
		}
	}
	service.mu.Unlock()
	service.recordHelperAttempt(*attempt)
}

func (service *Service) checkStep(ctx context.Context) time.Duration {
	interval := 12 * time.Hour
	if service.settings != nil {
		if value, err := service.settings.EffectiveSettings(ctx); err == nil {
			interval = time.Duration(value.GitHub.CatalogRefreshIntervalHours) * time.Hour
		}
	}
	service.mu.Lock()
	checkedAt := service.update.checkedAt
	service.mu.Unlock()
	if !checkedAt.IsZero() && service.options.Now().Before(checkedAt.Add(interval)) {
		return checkRecheck
	}
	if _, err := service.Check(ctx); err != nil && ctx.Err() == nil {
		return checkRetry
	}
	return checkRecheck
}

// recordHelperAttempt logs the update the root helper applied while the
// panel was stopped.
func (service *Service) recordHelperAttempt(attempt Attempt) {
	if service.records == nil {
		return
	}
	level, code, message := store.LogLevelInfo, "panel.update.applied", "The panel update helper installed a new version"
	if !attempt.Succeeded {
		level, code, message = store.LogLevelError, "panel.update.rejected", "The panel update helper could not install a new version"
	}
	service.records.RecordEvent(telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: level, Code: code, Message: message,
		Metadata: telemetry.LogMetadata(map[string]any{"version": attempt.Version, "error_code": attempt.Code}),
	})
}
