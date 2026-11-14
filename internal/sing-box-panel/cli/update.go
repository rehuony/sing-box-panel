// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
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
				return &Error{Kind: ErrorUsage, Code: "invalid_arguments", Message: "--apply-staged and --data-dir go together"}
			}
			if applyStaged {
				return applyStagedRelease(cmd, state, build, dataDir)
			}
			if update == nil {
				return &Error{Kind: ErrorUnavailable, Code: "update_unavailable", Message: "self-update is unavailable"}
			}
			var report selfupdate.ProgressFunc
			var progress *updateProgress
			if state.format == outputText {
				progress = newUpdateProgress(cmd.ErrOrStderr())
				report = progress.report
			}
			result, err := update(cmd.Context(), build.Version, report)
			if progress != nil {
				progress.finish()
			}
			if err != nil {
				return classifyUpdateError(err)
			}
			text := fmt.Sprintf("sing-box-panel %s is already up to date", result.PreviousVersion)
			if result.Updated {
				text = fmt.Sprintf(
					"updated sing-box-panel from %s to %s at %s",
					result.PreviousVersion, result.Version, result.ExecutablePath,
				)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
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
		return writeResult(cmd.OutOrStdout(), state.format, map[string]any{"applied": false}, "no update is staged")
	case attempt != nil && attempt.Succeeded:
		return writeResult(cmd.OutOrStdout(), state.format, attempt, "installed sing-box-panel "+attempt.Version+" and restarted "+panelSystemd.UnitName)
	case attempt != nil:
		return &Error{Kind: ErrorDomain, Code: "update_" + attempt.Code, Message: "the staged update to " + attempt.Version + " failed: " + err.Error(), Cause: err}
	}
	return classifyUpdateError(err)
}

func classifyUpdateError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return err
	case errors.Is(err, fs.ErrPermission):
		return &Error{Kind: ErrorPermission, Code: "update_permission_denied", Message: err.Error(), Cause: err}
	case errors.Is(err, selfupdate.ErrUnsupportedPlatform), errors.Is(err, selfupdate.ErrInvalidVersion):
		return &Error{Kind: ErrorUnavailable, Code: "update_unsupported", Message: err.Error(), Cause: err}
	case errors.Is(err, selfupdate.ErrReleaseUnavailable), errors.Is(err, selfupdate.ErrAssetMissing):
		return &Error{Kind: ErrorUnavailable, Code: "update_release_unavailable", Message: err.Error(), Cause: err}
	case errors.Is(err, selfupdate.ErrVerificationKeyInvalid), errors.Is(err, selfupdate.ErrReleaseInvalid), errors.Is(err, selfupdate.ErrSignatureInvalid), errors.Is(err, selfupdate.ErrChecksumInvalid), errors.Is(err, selfupdate.ErrExecutableInvalid), errors.Is(err, selfupdate.ErrExecutableChanged), errors.Is(err, selfupdate.ErrStagedExecutableInvalid), errors.Is(err, selfupdate.ErrStagedFileInvalid), errors.Is(err, selfupdate.ErrVersionNotNewer):
		return &Error{Kind: ErrorValidation, Code: "update_validation_failed", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: "update_failed", Message: err.Error(), Cause: err}
	}
}
