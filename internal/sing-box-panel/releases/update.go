// SPDX-License-Identifier: GPL-3.0-or-later

package releases

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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
	// StateRestarting: the panel restarts into the release: its unit
	// restarts it once it installed the release in place, or the system
	// service stops for its unit to install the release it staged and
	// start it again.
	StateRestarting = "restarting"
	// StateRestartRequired: the release is installed, or staged for the
	// system unit's install step, and runs once the panel is restarted.
	StateRestartRequired = "restart_required"
)

const (
	checkRecheck = time.Minute
	checkRetry   = 5 * time.Minute
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
	// ErrNodeUpdatesRunning refuses the panel's update while nodes are
	// being updated: the panel's restart would interrupt them.
	ErrNodeUpdatesRunning = errors.New("nodes are being updated; update the panel once they finish")
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
	if service.nodeUpdates > 0 {
		defer service.mu.Unlock()
		return service.statusLocked(), ErrNodeUpdatesRunning
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
	state, staged, err := service.deliver(ctx, version)
	attempt := &Attempt{Version: version, Succeeded: err == nil, FinishedAt: service.options.Now().UTC()}
	if err != nil {
		attempt.Code = selfupdate.FailureCode(err)
	}
	if service.records != nil {
		service.records.RecordOperation(ctx, "panel.update", "Panel update", Failed(err), telemetry.OperationLogContext{StartedAt: started})
	}
	if staged && service.options.Exit == nil {
		state = StateRestartRequired
	}
	service.mu.Lock()
	service.update.state = state
	if err != nil {
		service.update.state, service.update.target = StateIdle, ""
	}
	// The install step records a staged release's outcome, which the panel
	// reads when it starts again.
	if !staged {
		service.update.last = attempt
		_ = selfupdate.RecordAttempt(service.updatesDir(), *attempt)
	}
	service.mu.Unlock()
	switch {
	case state != StateRestarting:
	case staged:
		service.options.Exit()
	default:
		if err := service.restart(ctx); err != nil {
			service.mu.Lock()
			service.update.state = StateRestartRequired
			service.mu.Unlock()
		}
	}
}

// deliver fetches, verifies and installs or stages version. It returns the
// state it leaves and whether a system service staged the release for its
// unit's install step.
func (service *Service) deliver(ctx context.Context, version string) (string, bool, error) {
	updater := service.updater(selfupdate.Panel)
	incoming, err := selfupdate.IncomingDir(service.updatesDir())
	if err != nil {
		return StateIdle, false, err
	}
	defer func() { _ = os.RemoveAll(incoming) }()
	if err := updater.Fetch(ctx, version, []string{updater.AssetName()}, incoming); err != nil {
		return StateIdle, false, err
	}
	staged, err := updater.Verify(incoming, version, service.build.Version)
	if err != nil {
		return StateIdle, false, err
	}
	scope, supervised := systemd.RunningScope()
	delivery, err := updater.Deliver(ctx, staged, service.updatesDir(), supervised && scope == systemd.ScopeSystem)
	switch {
	case err != nil:
		return StateIdle, false, err
	case delivery == selfupdate.DeliveryStaged:
		return StateRestarting, true, nil
	case supervised:
		return StateRestarting, false, nil
	default:
		return StateRestartRequired, false, nil
	}
}

// BeginNodeUpdate admits a node update unless the panel's own update is
// under way, and holds off the panel's update until end is called.
func (service *Service) BeginNodeUpdate() (end func(), err error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.update.state != StateIdle {
		return nil, ErrUpdateRunning
	}
	service.nodeUpdates++
	var once sync.Once
	return func() {
		once.Do(func() {
			service.mu.Lock()
			defer service.mu.Unlock()
			service.nodeUpdates--
		})
	}, nil
}

// NodeUpdatesRunning reports whether nodes are being updated.
func (service *Service) NodeUpdatesRunning() bool {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.nodeUpdates > 0
}

func (service *Service) restart(ctx context.Context) error {
	if service.options.Restart == nil {
		return errors.New("the panel cannot restart its unit")
	}
	return service.options.Restart(ctx)
}

// Run reads the outcome of an update that the system unit's install step
// made before the panel started, records it, withdraws a staged release the
// unit did not install, clears finished staging, and checks for a new
// release on the catalog refresh interval until ctx ends.
func (service *Service) Run(ctx context.Context) {
	service.mu.Lock()
	service.lifetime = ctx
	service.mu.Unlock()
	attempt, fresh, err := selfupdate.TakeAttempt(service.updatesDir())
	if withdrawn, withdrawErr := selfupdate.WithdrawUnapplied(service.updatesDir(), service.options.Now()); withdrawErr == nil && withdrawn != nil {
		attempt, fresh, err = withdrawn, true, nil
	}
	if err == nil && attempt != nil {
		service.mu.Lock()
		service.update.last = attempt
		service.mu.Unlock()
		if fresh {
			service.recordInstallAttempt(*attempt)
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
	timer := time.NewTimer(service.checkStep(ctx))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
		case <-timer.C:
		}
		timer.Reset(service.checkStep(ctx))
	}
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

// recordInstallAttempt logs the update the system unit's install step
// applied, or the staged release it did not install, before the panel
// started.
func (service *Service) recordInstallAttempt(attempt Attempt) {
	if service.records == nil {
		return
	}
	level, code, message := store.LogLevelInfo, "panel.update.applied", "The panel's unit installed a new version"
	if !attempt.Succeeded {
		level, code, message = store.LogLevelError, "panel.update.rejected", "The panel's unit could not install a new version"
	}
	service.records.RecordEvent(telemetry.LogRecordRequest{
		Source: store.LogSourcePanel, Level: level, Code: code, Message: message,
		Metadata: telemetry.LogMetadata(map[string]any{"version": attempt.Version, "error_code": attempt.Code}),
	})
}
