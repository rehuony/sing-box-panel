// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/spf13/cobra"
)

func newConfigCompileCommand(state *options, open openApplicationFunc) *cobra.Command {
	var artifactID, acceptedIgnoredDigest string
	var detach bool
	command := &cobra.Command{
		Use:   "compile",
		Short: "Compile the global configuration for one exact installed core and check it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if artifactID == "" {
				return &Error{Kind: ErrorUsage, Code: "core_artifact_required", Message: "--artifact is required"}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			compiled, err := instance.CompileConfiguration(cmd.Context(), application.ConfigurationCompileRequest{
				CoreArtifactID: artifactID, AcceptedIgnoredDigest: acceptedIgnoredDigest,
			})
			if err != nil {
				return classifyConfigurationCompileError(err)
			}
			if detach {
				return writeResult(cmd.OutOrStdout(), state.format, compiled,
					fmt.Sprintf("compiled %s for %s; queued task %s", compiled.Artifact.ID, compiled.Artifact.ExactCoreVersion, compiled.Task.ID))
			}
			completed, err := waitForTaskWithCancellationRequest(
				cmd.Context(), instance, compiled.Task.ID, 250*time.Millisecond, "configuration_check_wait_failed",
			)
			if err != nil {
				return err
			}
			result := struct {
				Compile application.ConfigurationCompile `json:"compile"`
				Check   application.Task                 `json:"check"`
			}{compiled, completed}
			if err := writeResult(cmd.OutOrStdout(), state.format, result,
				fmt.Sprintf("compiled candidate %s is %s", compiled.Artifact.ID, completed.Status)); err != nil {
				return err
			}
			return terminalTaskError(completed)
		},
	}
	command.Flags().StringVar(&artifactID, "artifact", "", "required immutable core artifact ID")
	command.Flags().StringVar(&acceptedIgnoredDigest, "accept-ignored", "", "exact ignored diagnostic digest shown by preview")
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable check task is queued")
	return command
}

func classifyConfigurationCompileError(err error) error {
	switch {
	case errors.Is(err, configuration.ErrUnsupportedCoreProfile):
		return &Error{Kind: ErrorUnavailable, Code: "core_profile_unsupported", Message: err.Error(), Cause: err}
	case errors.Is(err, configuration.ErrIgnoredNotAccepted):
		return &Error{Kind: ErrorConflict, Code: "ignored_fields_not_accepted", Message: err.Error(), Cause: err}
	case errors.Is(err, configuration.ErrProjection), errors.Is(err, configuration.ErrProjectionBlocked):
		return &Error{Kind: ErrorValidation, Code: "configuration_projection_failed", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: "configuration_compile_failed", Message: err.Error(), Cause: err}
	}
}
