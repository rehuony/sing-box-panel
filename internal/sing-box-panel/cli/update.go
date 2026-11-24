// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/sing-box-panel/systemd"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

func newUpdateCommand(
	state *options,
	build buildinfo.Info,
	update func(context.Context, string, selfupdate.ProgressFunc) (selfupdate.Result, error),
) *cobra.Command {
	var applyStaged bool
	var dataDir string
	command := &cobra.Command{
		Use:   "update",
		Short: "Update sing-box-panel to the latest published release",
		Long: `Download, verify, and atomically install the latest published release.
Text output reports update stages and download progress on stderr, with a
percentage bar in an interactive terminal. JSON and JSONL omit progress.

--apply-staged --data-dir DIR is the root update helper of the system
service: it installs the release the service staged in DIR and restarts the
service, restoring the previous binary if the service does not stay active.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if applyStaged != (dataDir != "") {
				return &cliui.Error{Kind: cliui.ErrorUsage, Code: "invalid_arguments", Message: "--apply-staged and --data-dir go together"}
			}
			if applyStaged {
				return applyStagedRelease(cmd, state, build, dataDir)
			}
			if update == nil {
				return &cliui.Error{Kind: cliui.ErrorUnavailable, Code: "update_unavailable", Message: "self-update is unavailable"}
			}
			var report selfupdate.ProgressFunc
			var progress *cliui.Progress
			if state.format == cliui.Text {
				progress = cliui.NewProgress(cmd.ErrOrStderr())
				report = progress.Report
			}
			result, err := update(cmd.Context(), build.Version, report)
			if progress != nil {
				progress.Finish()
			}
			if err != nil {
				return cliui.UpdateError(err)
			}
			text := fmt.Sprintf("sing-box-panel %s is already up to date", result.PreviousVersion)
			if result.Updated {
				text = fmt.Sprintf("Updated sing-box-panel from %s to %s at %s", result.PreviousVersion, result.Version, result.ExecutablePath)
			}
			return cliui.Write(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	command.Flags().BoolVar(&applyStaged, "apply-staged", false, "install the release the system service staged (run by the root update helper)")
	command.Flags().StringVar(&dataDir, "data-dir", "", "the system service's data directory, for --apply-staged")
	return command
}

// The root update helper waits this long for the restarted panel to stay
// active for helperSettle before it restores the previous version.
const (
	helperTimeout = 30 * time.Second
	helperSettle  = 5 * time.Second
)

// applyStagedRelease is the root update helper: it installs the release the
// system service staged and restarts the service.
func applyStagedRelease(cmd *cobra.Command, state *options, build buildinfo.Info, dataDir string) error {
	updater := selfupdate.New(selfupdate.Options{Binary: selfupdate.Panel})
	attempt, err := updater.ApplyStaged(cmd.Context(), selfupdate.HelperOptions{
		DataDir: filepath.Clean(dataDir), Current: build.Version,
		Restart: func(ctx context.Context) error {
			manager, err := panelSystemd.New(systemd.Options{})
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
		return cliui.Write(cmd.OutOrStdout(), state.format, attempt, "Installed sing-box-panel "+attempt.Version+" and restarted "+panelSystemd.UnitName)
	case attempt != nil:
		return &cliui.Error{Kind: cliui.ErrorDomain, Code: "update_" + attempt.Code, Message: "the staged update to " + attempt.Version + " failed: " + err.Error(), Cause: err}
	}
	return cliui.UpdateError(err)
}
