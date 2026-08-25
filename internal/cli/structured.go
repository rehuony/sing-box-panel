// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/runtimeidentity"
	"github.com/spf13/cobra"
)

func newConfigRenderCommand(state *options, open openApplicationFunc) *cobra.Command {
	var version, artifactID string
	var allowCompatible, detach bool
	command := &cobra.Command{
		Use:   "render",
		Short: "Project the canonical head for one exact core and check the immutable result",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			rendered, err := instance.RenderStructured(cmd.Context(), application.StructuredRenderRequest{
				CoreVersion: version, CoreArtifactID: artifactID, AllowCompatible: allowCompatible,
			})
			if err != nil {
				return classifyStructuredRenderError(err, version)
			}
			if detach {
				return writeResult(
					cmd.OutOrStdout(), state.format, rendered,
					fmt.Sprintf(
						"rendered %s for %s from %s; queued task %s",
						rendered.Artifact.ID,
						rendered.Resolution.ExactVersion,
						rendered.Resolution.Source,
						rendered.Task.ID,
					),
				)
			}
			completed, err := waitForTaskWithCancellationRequest(
				cmd.Context(), instance, rendered.Task.ID, 250*time.Millisecond,
				"structured_check_wait_failed",
			)
			if err != nil {
				return err
			}
			result := struct {
				Render application.StructuredRender `json:"render"`
				Check  application.Task             `json:"check"`
			}{Render: rendered, Check: completed}
			if err := writeResult(
				cmd.OutOrStdout(), state.format, result,
				fmt.Sprintf(
					"structured candidate %s is %s; core %s from %s",
					rendered.Artifact.ID, completed.Status,
					rendered.Resolution.ExactVersion, rendered.Resolution.Source,
				),
			); err != nil {
				return err
			}
			return terminalTaskError(completed)
		},
	}
	command.Flags().StringVar(&version, "core-version", "", "exact sing-box version; omit to use the actual running version")
	command.Flags().StringVar(&artifactID, "artifact", "", "immutable core artifact ID; required when the exact version is ambiguous")
	command.Flags().BoolVar(&allowCompatible, "allow-compatible", false, "explicitly accept compatible_structured support and its persistent warnings")
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable check task is queued")
	return command
}

func classifyStructuredRenderError(err error, explicitVersion string) error {
	switch {
	case application.IsCompatibleCapabilityNotAccepted(err):
		return &Error{Kind: ErrorConflict, Code: "compatible_capability_not_accepted", Message: err.Error(), Cause: err}
	case application.IsStructuredCapabilityUnavailable(err):
		return &Error{Kind: ErrorUnavailable, Code: "structured_capability_unavailable", Message: err.Error(), Cause: err}
	case application.IsUnsupportedActiveFact(err):
		return &Error{Kind: ErrorConflict, Code: "unsupported_active_fact", Message: err.Error(), Cause: err}
	case strings.TrimSpace(explicitVersion) == "" && application.IsNoRunningCore(err):
		return &Error{Kind: ErrorUnavailable, Code: "core_not_running", Message: "no verified core is running; --core-version was omitted", Cause: err}
	case strings.TrimSpace(explicitVersion) == "" && (errors.Is(err, runtimeidentity.ErrStaleObservation) || errors.Is(err, runtimeidentity.ErrInspectionUnavailable)):
		return &Error{Kind: ErrorUnavailable, Code: "running_core_unavailable", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: "structured_render_failed", Message: err.Error(), Cause: err}
	}
}
