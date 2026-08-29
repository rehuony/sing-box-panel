// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/spf13/cobra"
)

func newConfigGetCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "get JSON_POINTER",
		Short: "Read one canonical value using an RFC 6901 JSON pointer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.CanonicalValueAt(cmd.Context(), args[0])
			if err != nil {
				return classifyCanonicalValueError("canonical_get_failed", err)
			}
			pretty, err := json.MarshalIndent(result.Value, "", "  ")
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "canonical_encode_failed", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, string(pretty))
		},
	}
}

func newConfigSetCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, baseRevision string
	command := &cobra.Command{
		Use:   "set JSON_POINTER",
		Short: "Set one canonical value from a JSON file or stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expectedHead, err := requiredConfigBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdin"}
			}
			raw, err := readCanonicalInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "canonical_value_input_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.SetCanonicalValue(cmd.Context(), expectedHead, args[0], raw)
			if err != nil {
				return classifyCanonicalValueError("canonical_set_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, canonicalSaveText(result))
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "JSON value file, or - for stdin")
	command.Flags().StringVar(&baseRevision, "base-revision", "", "revision ID used as the compare-and-swap base")
	return command
}

func newConfigUnsetCommand(state *options, open openApplicationFunc) *cobra.Command {
	var baseRevision string
	command := &cobra.Command{
		Use:   "unset JSON_POINTER",
		Short: "Remove one canonical value using an RFC 6901 JSON pointer",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expectedHead, err := requiredConfigBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.UnsetCanonicalValue(cmd.Context(), expectedHead, args[0])
			if err != nil {
				return classifyCanonicalValueError("canonical_unset_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, canonicalSaveText(result))
		},
	}
	command.Flags().StringVar(&baseRevision, "base-revision", "", "revision ID used as the compare-and-swap base")
	return command
}

func newConfigExportCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath string
	var force bool
	command := &cobra.Command{
		Use:   "export",
		Short: "Export the canonical document to a file or stdout",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdout"}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			head, err := instance.CanonicalHead(cmd.Context())
			if err != nil {
				return classifyCanonicalValueError("canonical_export_failed", err)
			}
			if head == nil {
				return &Error{Kind: ErrorDomain, Code: "canonical_not_initialized", Message: "no canonical revision has been saved"}
			}
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, head.Document, "", "  "); err != nil {
				return &Error{Kind: ErrorDomain, Code: "canonical_encode_failed", Message: err.Error(), Cause: err}
			}
			pretty.WriteByte('\n')
			if filePath == "-" {
				_, err := cmd.OutOrStdout().Write(pretty.Bytes())
				return err
			}
			if err := writePrivateExport(filePath, pretty.Bytes(), force); err != nil {
				return &Error{Kind: ErrorValidation, Code: "canonical_export_failed", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{
				"file": filePath, "revision": head.ID, "sha256": head.SHA256,
			}, "exported canonical revision "+head.ID+" to "+filePath)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "destination file, or - for stdout")
	command.Flags().BoolVar(&force, "force", false, "atomically replace an existing destination")
	return command
}

func newConfigImportCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath, baseRevision string
	command := &cobra.Command{
		Use:   "import",
		Short: "Import a complete canonical document using revision CAS",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			expectedHead, err := requiredConfigBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdin"}
			}
			raw, err := readCanonicalInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "canonical_import_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.ReplaceCanonical(cmd.Context(), expectedHead, raw)
			if err != nil {
				return classifyCanonicalValueError("canonical_import_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, canonicalSaveText(result))
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "canonical JSON file, or - for stdin")
	command.Flags().StringVar(&baseRevision, "base-revision", "", "revision ID used as the compare-and-swap base, or none")
	return command
}

func newConfigValidateCommand(state *options) *cobra.Command {
	var filePath string
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate a complete canonical document without saving it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdin"}
			}
			raw, err := readCanonicalInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "canonical_input_failed", Message: err.Error(), Cause: err}
			}
			document, err := configuration.Parse(raw)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "canonical_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{
				"valid": true, "canonical_bytes": len(document.CanonicalJSON()),
			}, "canonical document is valid")
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "canonical JSON file, or - for stdin")
	return command
}

func newConfigDiffCommand(state *options, open openApplicationFunc) *cobra.Command {
	var from, to string
	command := &cobra.Command{
		Use:   "diff",
		Short: "Diff two immutable canonical revisions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
				return &Error{Kind: ErrorUsage, Code: "revision_references_required", Message: "--from and --to are required"}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			result, err := instance.DiffCanonicalRevisions(cmd.Context(), from, to)
			if err != nil {
				return classifyCanonicalValueError("canonical_diff_failed", err)
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, fmt.Sprintf("%d canonical changes", len(result.Changes)))
		},
	}
	command.Flags().StringVar(&from, "from", "", "source revision ID or #sequence")
	command.Flags().StringVar(&to, "to", "", "target revision ID or #sequence")
	return command
}

func requiredConfigBaseRevision(cmd *cobra.Command, raw string) (string, error) {
	if !cmd.Flags().Changed("base-revision") {
		return "", &Error{Kind: ErrorUsage, Code: "base_revision_required", Message: "--base-revision is required"}
	}
	value := strings.TrimSpace(raw)
	if value == "none" {
		return "", nil
	}
	if value == "" {
		return "", &Error{Kind: ErrorUsage, Code: "base_revision_invalid", Message: "--base-revision must be a revision ID or none"}
	}
	return value, nil
}

func canonicalSaveText(result application.CanonicalSave) string {
	if result.NoChange {
		return fmt.Sprintf("canonical revision #%d %s is unchanged", result.Revision.Sequence, result.Revision.ID)
	}
	return fmt.Sprintf("saved canonical revision #%d %s", result.Revision.Sequence, result.Revision.ID)
}

func classifyCanonicalValueError(code string, err error) error {
	switch {
	case application.IsRevisionConflict(err):
		return &Error{Kind: ErrorConflict, Code: "canonical_revision_conflict", Message: err.Error(), Cause: err}
	case errors.Is(err, configuration.ErrInvalidDocument), errors.Is(err, configuration.ErrPointerNotFound):
		return &Error{Kind: ErrorValidation, Code: "canonical_invalid", Message: err.Error(), Cause: err}
	case application.IsRevisionNotFound(err):
		return &Error{Kind: ErrorDomain, Code: "revision_not_found", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
}

func writePrivateExport(path string, data []byte, force bool) error {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return errors.New("export destination is invalid")
	}
	if !force {
		file, err := os.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		complete := false
		defer func() {
			_ = file.Close()
			if !complete {
				_ = os.Remove(clean)
			}
		}()
		if _, err := file.Write(data); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		if err := syncExportDirectory(filepath.Dir(clean)); err != nil {
			return err
		}
		complete = true
		return nil
	}
	directory := filepath.Dir(clean)
	temporary, err := os.CreateTemp(directory, ".sing-box-panel-export-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, clean); err != nil {
		return err
	}
	return syncExportDirectory(directory)
}

func syncExportDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
