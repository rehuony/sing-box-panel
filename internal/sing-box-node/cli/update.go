// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	nodesystemd "github.com/rehuony/sing-box-panel/internal/sing-box-node/systemd"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

// The root update helper waits this long for the restarted node to stay
// active for helperSettle before it restores the previous version.
const (
	helperTimeout = 30 * time.Second
	helperSettle  = 5 * time.Second
)

// The sources an update comes from: the panel the node is enrolled with,
// which serves its own release so both run one version, or the repository's
// latest published release for a node without a panel.
const (
	sourcePanel  = "panel"
	sourceGitHub = "github"
)

// updateResult is an update's outcome and where its release came from.
type updateResult struct {
	selfupdate.Result
	Source    string `json:"source"`
	Restarted bool   `json:"restarted"`
}

func newUpdateCommand(deps Dependencies, state *options, service systemdService) *cobra.Command {
	var restart, applyStaged bool
	var stagedDataDir string
	command := &cobra.Command{
		Use:   "update",
		Short: "Update sing-box-node to its panel's release, or to the latest release without a panel",
		Long: `Update sing-box-node to the release of the panel it is enrolled with, so the
node runs the panel's version. A node that is not enrolled updates to the
latest published release on GitHub instead. The release is verified with the
key built into this binary before it replaces the binary, which needs write
access to it. Text output reports progress on stderr.

The running node keeps its version until it restarts; --restart restarts its
service now, which briefly interrupts its proxy.

--apply-staged --data-dir DIR is the root update helper of the system service:
it installs the release the service staged in DIR and restarts the service,
restoring the previous binary if the service does not stay active.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if applyStaged != (stagedDataDir != "") {
				return &cliui.Error{Kind: cliui.ErrorUsage, Code: "invalid_arguments", Message: "--apply-staged and --data-dir go together"}
			}
			if applyStaged {
				if restart {
					return &cliui.Error{Kind: cliui.ErrorUsage, Code: "invalid_arguments", Message: "--restart does not apply to --apply-staged, which always restarts the service"}
				}
				return applyStagedRelease(cmd, state, deps.Build, stagedDataDir)
			}
			enrollment, enrolled, err := currentEnrollment(state.settingsPath)
			if err != nil {
				return cliui.Fail(cliui.ErrorValidation, "settings_invalid", err)
			}
			var progress *cliui.Progress
			var report selfupdate.ProgressFunc
			if state.format == cliui.Text {
				progress = cliui.NewProgress(cmd.ErrOrStderr())
				report = progress.Report
			}
			updater := selfupdate.New(selfupdate.Options{Binary: selfupdate.Node})
			result := updateResult{Source: sourceGitHub}
			if enrolled {
				result.Source = sourcePanel
				result.Result, err = updateFromPanel(cmd.Context(), updater, enrollment, deps.Build, report)
			} else {
				result.Result, err = updater.Update(cmd.Context(), deps.Build.Version, report)
			}
			if progress != nil {
				progress.Finish()
			}
			if err != nil {
				return cliui.UpdateError(err)
			}
			style := cliui.NewStyle(cmd.OutOrStdout(), state.format)
			if !result.Updated {
				return cliui.Write(cmd.OutOrStdout(), state.format, result, fmt.Sprintf("sing-box-node %s is already up to date", deps.Build.Version))
			}
			title := fmt.Sprintf("Updated sing-box-node from %s to %s at %s", deps.Build.Version, result.Version, result.ExecutablePath)
			if !restart {
				return cliui.Write(cmd.OutOrStdout(), state.format, result,
					cliui.Section(title, style.Next(cliui.ServiceCommand("sing-box-node", systemd.ScopeAuto, "restart"))))
			}
			if service == nil {
				return &cliui.Error{Kind: cliui.ErrorUnavailable, Code: "systemd_unavailable", Message: "installed the update, but systemd is unavailable to restart the node"}
			}
			restarted, err := service.Control(cmd.Context(), systemd.ScopeAuto, systemd.ActionRestart)
			if err != nil {
				return cliui.SystemdError("system_restart_failed", fmt.Errorf("installed sing-box-node %s, but restarting its service failed: %w", result.Version, err))
			}
			result.Restarted = true
			return cliui.Write(cmd.OutOrStdout(), state.format, result, cliui.Section(title, "Restarted "+restarted.Unit))
		},
	}
	command.Flags().BoolVar(&restart, "restart", false, "restart the node's systemd service after installing")
	command.Flags().BoolVar(&applyStaged, "apply-staged", false, "install the release the system service staged (run by the root update helper)")
	command.Flags().StringVar(&stagedDataDir, "data-dir", "", "the system service's data directory, for --apply-staged")
	return command
}

// currentEnrollment reads the enrollment of the node whose settings are at
// path; a node without settings or an enrollment has none.
func currentEnrollment(path string) (settings.Enrollment, bool, error) {
	dataDir, err := settings.LoadDataDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings.Enrollment{}, false, nil
	}
	if err != nil {
		return settings.Enrollment{}, false, err
	}
	enrollment, err := settings.LoadEnrollment(dataDir)
	if errors.Is(err, settings.ErrNotEnrolled) {
		return settings.Enrollment{}, false, nil
	}
	return enrollment, err == nil, err
}

// updateFromPanel installs the release the enrolled panel serves, staged
// beside the executable.
func updateFromPanel(ctx context.Context, updater *selfupdate.Updater, enrollment settings.Enrollment, build buildinfo.Info, report selfupdate.ProgressFunc) (selfupdate.Result, error) {
	result := selfupdate.Result{PreviousVersion: build.Version, Version: build.Version}
	if err := updater.Ready(build.Version); err != nil {
		return result, err
	}
	emit := func(progress selfupdate.Progress) {
		if report != nil {
			report(progress)
		}
	}
	emit(selfupdate.Progress{Stage: selfupdate.StageCheckRelease})
	executable, err := os.Executable()
	if err == nil {
		executable, err = filepath.EvalSymlinks(executable)
	}
	if err != nil {
		return result, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(executable), ".sing-box-node-update-*")
	if errors.Is(err, fs.ErrPermission) {
		return result, fmt.Errorf("updating needs write access to %s; run the command as its owner, such as with sudo: %w", filepath.Dir(executable), err)
	}
	if err != nil {
		return result, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	version, err := agent.FetchRelease(ctx, enrollment, build, "", stage)
	if err != nil {
		return result, fmt.Errorf("fetch the panel's release: %w", err)
	}
	emit(selfupdate.Progress{Stage: selfupdate.StageVerifyBinary})
	staged, err := updater.Verify(stage, version, build.Version)
	if errors.Is(err, selfupdate.ErrUpToDate) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	emit(selfupdate.Progress{Stage: selfupdate.StageInstall})
	installed, err := updater.Install(ctx, staged)
	if err != nil {
		return result, err
	}
	installed.PreviousVersion = build.Version
	return installed, nil
}

// applyStagedRelease is the root update helper: it installs the release the
// system service staged and restarts the service.
func applyStagedRelease(cmd *cobra.Command, state *options, build buildinfo.Info, dataDir string) error {
	updater := selfupdate.New(selfupdate.Options{Binary: selfupdate.Node})
	attempt, err := updater.ApplyStaged(cmd.Context(), selfupdate.HelperOptions{
		DataDir: filepath.Clean(dataDir), Current: build.Version,
		Restart: func(ctx context.Context) error {
			manager, err := nodesystemd.New(systemd.Options{})
			if err != nil {
				return err
			}
			return manager.RestartAndWait(ctx, systemd.ScopeSystem, helperTimeout, helperSettle)
		},
	})
	switch {
	case attempt == nil && err == nil:
		return cliui.Write(cmd.OutOrStdout(), state.format, map[string]any{"applied": false}, "No update is staged")
	case attempt != nil && attempt.Succeeded:
		return cliui.Write(cmd.OutOrStdout(), state.format, attempt, "Installed sing-box-node "+attempt.Version+" and restarted "+nodesystemd.UnitName)
	case attempt != nil:
		return &cliui.Error{Kind: cliui.ErrorDomain, Code: "update_" + attempt.Code, Message: "the staged update to " + attempt.Version + " failed: " + err.Error(), Cause: err}
	}
	return cliui.UpdateError(err)
}
