// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/manualjson"
	"github.com/spf13/cobra"
)

const maximumReattachDecisionBytes = 1 << 20

func newConfigManualCommand(state *options, open openApplicationFunc) *cobra.Command {
	root := group("manual", "Manage exact-byte manual configuration candidates")
	root.AddCommand(
		newManualListCommand(state, open),
		newManualShowCommand(state, open),
		newManualDetachCommand(state, open),
		newManualPreviewCommand(state, open),
		newManualReplaceCommand(state, open),
		newManualDiscardCommand(state, open),
	)
	reattach := group("reattach", "Reconcile a stale manual candidate")
	reattach.AddCommand(
		newManualReattachPreviewCommand(state, open),
		newManualReattachApplyCommand(state, open),
	)
	root.AddCommand(reattach)
	return root
}

func newManualPreviewCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, baseRevision, version, artifactID string
	var allowCompatible bool
	command := &cobra.Command{
		Use:   "preview",
		Short: "Preview exact-version reverse mapping without writing",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filePath == "" || !cmd.Flags().Changed("base-revision") || strings.TrimSpace(baseRevision) == "" {
				return &Error{Kind: ErrorUsage, Code: "manual_input_required", Message: "--file and --base-revision are required"}
			}
			raw, err := readManualInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "manual_input_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			preview, err := instance.PreviewManualReplace(cmd.Context(), application.ManualReplaceRequest{
				ExpectedHead: baseRevision, CoreVersion: version, CoreArtifactID: artifactID,
				Raw: raw, AllowCompatible: allowCompatible,
			})
			if err != nil {
				return classifyManualError(err, version)
			}
			text := fmt.Sprintf(
				"manual reverse preview for core %s from %s: available=%t, canonical_changed=%t, residual_paths=%d",
				preview.Resolution.ExactVersion, preview.Resolution.Source,
				preview.Reverse.Available, preview.Reverse.CanonicalChanged,
				len(preview.Reverse.ResidualPaths),
			)
			return writeResult(cmd.OutOrStdout(), state.format, preview, text)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "manual JSON/JSONC file, or - for stdin")
	command.Flags().StringVar(&baseRevision, "base-revision", "", "current canonical revision ID used for compare-and-swap")
	command.Flags().StringVar(&version, "core-version", "", "exact sing-box version; omit to use the actual running version")
	command.Flags().StringVar(&artifactID, "artifact", "", "immutable core artifact ID; required when the exact version is ambiguous")
	command.Flags().BoolVar(&allowCompatible, "allow-compatible", false, "accept compatible_structured ownership for this exact preview")
	return command
}

func newManualDetachCommand(state *options, open openApplicationFunc) *cobra.Command {
	var background bool
	command := &cobra.Command{
		Use:   "detach STRUCTURED_STARTUP_ARTIFACT",
		Short: "Copy exact structured bytes into a capability-independent manual candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			saved, err := instance.DetachManualJSON(cmd.Context(), args[0])
			if err != nil {
				return classifyManualError(err, "explicit")
			}
			if background {
				return writeResult(
					cmd.OutOrStdout(), state.format, saved,
					"detached as "+saved.Artifact.ID+"; queued task "+saved.Task.ID,
				)
			}
			completed, err := waitForTaskWithCancellationRequest(
				cmd.Context(), instance, saved.Task.ID, 250*time.Millisecond,
				"manual_detach_check_wait_failed",
			)
			if err != nil {
				return err
			}
			result := struct {
				Save  application.ManualSave `json:"save"`
				Check application.Task       `json:"check"`
			}{Save: saved, Check: completed}
			if err := writeResult(
				cmd.OutOrStdout(), state.format, result,
				"detached manual candidate "+saved.Artifact.ID+" is "+string(completed.Status),
			); err != nil {
				return err
			}
			return terminalTaskError(completed)
		},
	}
	command.Flags().BoolVar(&background, "detach", false, "return after the durable check task is queued")
	return command
}

func newManualListCommand(state *options, open openApplicationFunc) *cobra.Command {
	var version, artifactID string
	var limit int
	command := &cobra.Command{
		Use:   "list",
		Short: "List manual candidates for one exact core version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			resolution, artifacts, err := instance.ListManualArtifacts(
				cmd.Context(), version, artifactID, limit,
			)
			if err != nil {
				return classifyManualError(err, version)
			}
			result := map[string]any{"resolution": resolution, "items": artifacts}
			var output strings.Builder
			for _, artifact := range artifacts {
				fmt.Fprintf(
					&output, "%s\t%s\t%s\t%s\t%s\n",
					artifact.ID, artifact.ExactCoreVersion, artifact.CoreArtifactID,
					artifact.State, artifact.ConfigSHA256,
				)
			}
			itemsText := strings.TrimSuffix(output.String(), "\n")
			text := fmt.Sprintf("core %s from %s", resolution.ExactVersion, resolution.Source)
			if itemsText == "" {
				text += "; no manual candidates"
			} else {
				text += "\n" + itemsText
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	command.Flags().StringVar(&version, "core-version", "", "exact sing-box version; omit to use the actual running version")
	command.Flags().StringVar(&artifactID, "artifact", "", "filter by immutable core artifact ID")
	command.Flags().IntVar(&limit, "limit", 50, "maximum candidates to return (1-200)")
	return command
}

func newManualShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show STARTUP_ARTIFACT",
		Short: "Show the exact bytes and binding of one manual candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			artifact, err := instance.ManualArtifact(cmd.Context(), args[0])
			if err != nil {
				return classifyManualError(err, "explicit")
			}
			return writeResult(cmd.OutOrStdout(), state.format, artifact, artifact.Raw)
		},
	}
}

func newManualReplaceCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, baseRevision, version, artifactID string
	var allowCompatible, detach bool
	command := &cobra.Command{
		Use:   "replace",
		Short: "Save exact JSONC bytes and queue a real sing-box check",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filePath == "" || !cmd.Flags().Changed("base-revision") || strings.TrimSpace(baseRevision) == "" {
				return &Error{Kind: ErrorUsage, Code: "manual_input_required", Message: "--file and --base-revision are required"}
			}
			raw, err := readManualInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "manual_input_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			saved, err := instance.ReplaceManualJSON(cmd.Context(), application.ManualReplaceRequest{
				ExpectedHead: baseRevision, CoreVersion: version, CoreArtifactID: artifactID,
				Raw: raw, AllowCompatible: allowCompatible,
			})
			if err != nil {
				return classifyManualError(err, version)
			}
			if detach {
				return writeResult(
					cmd.OutOrStdout(), state.format, saved,
					fmt.Sprintf(
						"saved manual candidate %s for core %s from %s; queued task %s",
						saved.Artifact.ID, saved.Resolution.ExactVersion, saved.Resolution.Source, saved.Task.ID,
					),
				)
			}
			completed, err := waitForTaskWithCancellationRequest(
				cmd.Context(), instance, saved.Task.ID, 250*time.Millisecond,
				"manual_check_wait_failed",
			)
			if err != nil {
				return err
			}
			result := struct {
				Save  application.ManualSave `json:"save"`
				Check application.Task       `json:"check"`
			}{Save: saved, Check: completed}
			if err := writeResult(
				cmd.OutOrStdout(), state.format, result,
				fmt.Sprintf(
					"manual candidate %s is %s; core %s from %s",
					saved.Artifact.ID, completed.Status,
					saved.Resolution.ExactVersion, saved.Resolution.Source,
				),
			); err != nil {
				return err
			}
			return terminalTaskError(completed)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "manual JSON/JSONC file, or - for stdin")
	command.Flags().StringVar(&baseRevision, "base-revision", "", "current canonical revision ID used for compare-and-swap")
	command.Flags().StringVar(&version, "core-version", "", "exact sing-box version; omit to use the actual running version")
	command.Flags().StringVar(&artifactID, "artifact", "", "immutable core artifact ID; required when the exact version is ambiguous")
	command.Flags().BoolVar(&allowCompatible, "allow-compatible", false, "accept compatible_structured ownership after reviewing a preview")
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable check task is queued")
	return command
}

func newManualDiscardCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "discard STARTUP_ARTIFACT",
		Short: "Mark one manual candidate stale",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			artifact, err := instance.DiscardManualArtifact(cmd.Context(), args[0])
			if err != nil {
				return classifyManualError(err, "explicit")
			}
			return writeResult(cmd.OutOrStdout(), state.format, artifact, "discarded manual candidate "+artifact.ID)
		},
	}
}

func newManualReattachPreviewCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "preview STARTUP_ARTIFACT",
		Short: "Preview a pinned-manifest three-way reconciliation without writing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			preview, err := instance.PreviewManualReattach(cmd.Context(), args[0])
			if err != nil {
				return classifyManualError(err, "explicit")
			}
			text := fmt.Sprintf(
				"preview %s against head %s: %d conflicts, %d residual paths",
				preview.Evidence.StartupArtifactID,
				preview.Evidence.CurrentHeadID,
				len(preview.Conflicts),
				len(preview.ResidualPaths),
			)
			return writeResult(cmd.OutOrStdout(), state.format, preview, text)
		},
	}
}

func newManualReattachApplyCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath string
	var detach bool
	command := &cobra.Command{
		Use:   "apply STARTUP_ARTIFACT",
		Short: "Apply explicit three-way decisions and queue an exact-byte check",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath == "" {
				return &Error{
					Kind: ErrorUsage, Code: "reattach_decisions_required",
					Message: "--file is required and must contain preview evidence plus conflict decisions",
				}
			}
			input, err := readManualReattachInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{
					Kind: ErrorValidation, Code: "reattach_decisions_invalid",
					Message: err.Error(), Cause: err,
				}
			}
			input.StartupArtifactID = args[0]
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			saved, err := instance.ApplyManualReattach(cmd.Context(), input)
			if err != nil {
				return classifyManualError(err, "explicit")
			}
			if detach {
				return writeResult(
					cmd.OutOrStdout(), state.format, saved,
					"reattached as "+saved.Artifact.ID+"; queued task "+saved.Task.ID,
				)
			}
			completed, err := waitForTaskWithCancellationRequest(
				cmd.Context(), instance, saved.Task.ID, 250*time.Millisecond,
				"manual_reattach_check_wait_failed",
			)
			if err != nil {
				return err
			}
			result := struct {
				Save  application.ManualReattachSave `json:"save"`
				Check application.Task               `json:"check"`
			}{Save: saved, Check: completed}
			if err := writeResult(
				cmd.OutOrStdout(), state.format, result,
				"reattached manual candidate "+saved.Artifact.ID+" is "+string(completed.Status),
			); err != nil {
				return err
			}
			return terminalTaskError(completed)
		},
	}
	command.Flags().StringVar(
		&filePath,
		"file",
		"",
		"JSON decision file, or - for stdin; contains evidence and decisions",
	)
	command.Flags().BoolVar(&detach, "detach", false, "return after the durable check task is queued")
	return command
}

func readManualInput(stdin io.Reader, filePath string) ([]byte, error) {
	var reader io.Reader = stdin
	var closeFile func() error
	if filePath != "-" {
		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("open manual input: %w", err)
		}
		reader = file
		closeFile = file.Close
	}
	if closeFile != nil {
		defer closeFile()
	}
	raw, err := io.ReadAll(io.LimitReader(reader, manualjson.MaximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read manual input: %w", err)
	}
	if len(raw) > manualjson.MaximumBytes {
		return nil, fmt.Errorf("manual input exceeds %d bytes", manualjson.MaximumBytes)
	}
	return raw, nil
}

func readManualReattachInput(
	stdin io.Reader,
	filePath string,
) (application.ManualReattachApplyRequest, error) {
	var reader io.Reader = stdin
	var closeFile func() error
	if filePath != "-" {
		file, err := os.Open(filePath)
		if err != nil {
			return application.ManualReattachApplyRequest{}, fmt.Errorf("open reattach decisions: %w", err)
		}
		reader = file
		closeFile = file.Close
	}
	if closeFile != nil {
		defer closeFile()
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maximumReattachDecisionBytes+1))
	if err != nil {
		return application.ManualReattachApplyRequest{}, fmt.Errorf("read reattach decisions: %w", err)
	}
	if len(raw) > maximumReattachDecisionBytes {
		return application.ManualReattachApplyRequest{}, fmt.Errorf(
			"reattach decisions exceed %d bytes",
			maximumReattachDecisionBytes,
		)
	}
	var input application.ManualReattachApplyRequest
	if err := jsonstrict.Decode(raw, maximumReattachDecisionBytes, &input); err != nil {
		return application.ManualReattachApplyRequest{}, fmt.Errorf("decode reattach decisions: %w", err)
	}
	return input, nil
}

func classifyManualError(err error, explicitVersion string) error {
	if classified := classifyOmittedCoreVersionError(err, explicitVersion); classified != nil {
		return classified
	}
	switch {
	case application.IsRevisionConflict(err):
		return &Error{Kind: ErrorConflict, Code: "canonical_revision_conflict", Message: err.Error(), Cause: err}
	case application.IsManualReattachPreviewStale(err):
		return &Error{Kind: ErrorConflict, Code: "manual_reattach_preview_stale", Message: err.Error(), Cause: err}
	case application.IsManualReattachUnresolved(err):
		return &Error{Kind: ErrorConflict, Code: "manual_reattach_conflicts_unresolved", Message: err.Error(), Cause: err}
	case application.IsManualReattachUnavailable(err):
		return &Error{Kind: ErrorUnavailable, Code: "manual_reattach_unavailable", Message: err.Error(), Cause: err}
	case application.IsInvalidManualJSON(err):
		return &Error{Kind: ErrorValidation, Code: "manual_json_invalid", Message: err.Error(), Cause: err}
	case application.IsStartupArtifactNotFound(err):
		return &Error{Kind: ErrorDomain, Code: "startup_artifact_not_found", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: "manual_operation_failed", Message: err.Error(), Cause: err}
	}
}
