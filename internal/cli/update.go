// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/spf13/cobra"
)

func newUpdateCommand(
	state *options,
	build buildinfo.Info,
	update func(context.Context, string) (selfupdate.Result, error),
) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update sing-box-panel to the latest published release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if update == nil {
				return &Error{Kind: ErrorUnavailable, Code: "update_unavailable", Message: "self-update is unavailable"}
			}
			result, err := update(cmd.Context(), build.Version)
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
	case errors.Is(err, selfupdate.ErrVerificationKeyInvalid), errors.Is(err, selfupdate.ErrReleaseInvalid), errors.Is(err, selfupdate.ErrSignatureInvalid), errors.Is(err, selfupdate.ErrChecksumInvalid), errors.Is(err, selfupdate.ErrExecutableInvalid), errors.Is(err, selfupdate.ErrExecutableChanged), errors.Is(err, selfupdate.ErrStagedExecutableInvalid):
		return &Error{Kind: ErrorValidation, Code: "update_validation_failed", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: "update_failed", Message: err.Error(), Cause: err}
	}
}
