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

func newUpdateCommand(deps Dependencies, settingsPath *string, service systemdService) *cobra.Command {
	var restart, applyStaged bool
	var stagedDataDir string
	command := &cobra.Command{
		Use:   "update",
		Short: "Update sing-box-node to its panel's release",
		Long: "Fetch the release of the panel this node is enrolled with, verify it with the release key built into this\n" +
			"binary, and replace the binary, which needs write access to it. The running node keeps its version until\n" +
			"its service restarts; --restart restarts it now, which briefly interrupts its proxy.\n\n" +
			"--apply-staged is the root update helper of the system service: it installs the release the service staged\n" +
			"in its data directory and restarts the service, restoring the previous binary if the service does not\n" +
			"stay active.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if applyStaged {
				if restart {
					return errors.New("--restart does not apply to --apply-staged, which always restarts the service")
				}
				if stagedDataDir == "" {
					return errors.New("--apply-staged needs the service's --data-dir")
				}
				return applyStagedRelease(cmd, deps, stagedDataDir)
			}
			dataDir, err := settings.LoadDataDir(*settingsPath)
			if err != nil {
				return err
			}
			enrollment, err := settings.LoadEnrollment(dataDir)
			if err != nil {
				return err
			}
			updater := selfupdate.New(selfupdate.Options{Binary: selfupdate.Node})
			if err := updater.Ready(deps.Build.Version); err != nil {
				return err
			}
			executable, err := os.Executable()
			if err == nil {
				executable, err = filepath.EvalSymlinks(executable)
			}
			if err != nil {
				return err
			}
			stage, err := os.MkdirTemp(filepath.Dir(executable), ".sing-box-node-update-*")
			if errors.Is(err, fs.ErrPermission) {
				return fmt.Errorf("updating needs write access to %s; run the command as its owner, such as with sudo: %w", filepath.Dir(executable), err)
			}
			if err != nil {
				return err
			}
			defer func() { _ = os.RemoveAll(stage) }()
			version, err := agent.FetchRelease(cmd.Context(), enrollment, deps.Build, "", stage)
			if err != nil {
				return fmt.Errorf("fetch the panel's release: %w", err)
			}
			staged, err := updater.Verify(stage, version, deps.Build.Version)
			if errors.Is(err, selfupdate.ErrUpToDate) {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "sing-box-node %s is the panel's release already.\n", deps.Build.Version)
				return err
			}
			if err != nil {
				return err
			}
			result, err := updater.Install(cmd.Context(), staged)
			if err != nil {
				return err
			}
			if !restart {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed sing-box-node %s at %s, replacing %s. Restart the node to run it: sing-box-node systemd restart\n",
					version, result.ExecutablePath, deps.Build.Version)
				return err
			}
			if service == nil {
				return errors.New("installed the update, but systemd is unavailable to restart the node")
			}
			if _, err := service.Control(cmd.Context(), systemd.ScopeAuto, systemd.ActionRestart); err != nil {
				return fmt.Errorf("installed sing-box-node %s, but restarting its service failed: %w", version, err)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed sing-box-node %s at %s and restarted its service.\n", version, result.ExecutablePath)
			return err
		},
	}
	command.Flags().BoolVar(&restart, "restart", false, "restart the node's systemd service after installing")
	command.Flags().BoolVar(&applyStaged, "apply-staged", false, "install the release the system service staged (run by the root update helper)")
	command.Flags().StringVar(&stagedDataDir, "data-dir", "", "the system service's data directory, for --apply-staged")
	return command
}

// applyStagedRelease is the root update helper: it installs the release the
// system service staged and restarts the service.
func applyStagedRelease(cmd *cobra.Command, deps Dependencies, dataDir string) error {
	updater := selfupdate.New(selfupdate.Options{Binary: selfupdate.Node})
	attempt, err := updater.ApplyStaged(cmd.Context(), selfupdate.HelperOptions{
		DataDir: filepath.Clean(dataDir), Current: deps.Build.Version,
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
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "No update is staged.")
		return err
	case attempt != nil && attempt.Succeeded:
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed sing-box-node %s and restarted %s.\n", attempt.Version, nodesystemd.UnitName)
		return err
	case attempt != nil:
		return fmt.Errorf("the staged update to %s failed (%s): %w", attempt.Version, attempt.Code, err)
	}
	return err
}
