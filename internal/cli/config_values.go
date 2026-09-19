// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

const baseRevisionUsage = "canonical revision ID of the current valid saved file (config show --output json) used as the compare-and-swap base"

func newConfigGetCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "get JSON_POINTER",
		Short: "Read one value from the valid saved file using an RFC 6901 JSON pointer",
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
		Short: "Set one value in the valid saved file from a JSON file or stdin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			expectedHead, err := requiredConfigBaseRevision(cmd, baseRevision)
			if err != nil {
				return err
			}
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdin"}
			}
			raw, err := readConfigurationInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "canonical_value_input_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			return renderConfigurationEdit(cmd, state, "canonical_set_failed", func(ctx context.Context) (application.CanonicalSave, error) {
				return instance.SetCanonicalValue(ctx, expectedHead, args[0], raw)
			})
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "JSON value file, or - for stdin")
	command.Flags().StringVar(&baseRevision, "base-revision", "", baseRevisionUsage)
	return command
}

func newConfigUnsetCommand(state *options, open openApplicationFunc) *cobra.Command {
	var baseRevision string
	command := &cobra.Command{
		Use:   "unset JSON_POINTER",
		Short: "Remove one value from the valid saved file using an RFC 6901 JSON pointer",
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
			return renderConfigurationEdit(cmd, state, "canonical_unset_failed", func(ctx context.Context) (application.CanonicalSave, error) {
				return instance.UnsetCanonicalValue(ctx, expectedHead, args[0])
			})
		},
	}
	command.Flags().StringVar(&baseRevision, "base-revision", "", baseRevisionUsage)
	return command
}

func newConfigValidateCommand(state *options) *cobra.Command {
	var filePath string
	command := &cobra.Command{
		Use:   "validate",
		Short: "Check JSON structure in a file or stdin without saving it",
		Long: `Read --file FILE (or --file - for stdin) and check for a strict JSON object
within size, nesting, and value-count limits.

This does not load panel settings or the database, save the input, or run
sing-box. It does not validate sing-box field semantics; valid JSON alone
does not mean the core will accept the configuration.`,
		Example: `  sing-box-panel config validate --file ./config.json
  sing-box-panel config validate --file - < ./config.json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdin"}
			}
			raw, err := readConfigurationInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "configuration_input_failed", Message: err.Error(), Cause: err}
			}
			document, err := configuration.Parse(raw)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "configuration_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{
				"valid": true, "canonical_bytes": len(document.CanonicalJSON()),
			}, "configuration document is valid JSON")
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "JSON document to validate without saving; use - for stdin (required)")
	return command
}

// renderConfigurationEdit reports only the result of this write. A later read
// of the saved file could already belong to another writer.
func renderConfigurationEdit(
	cmd *cobra.Command,
	state *options,
	code string,
	edit func(context.Context) (application.CanonicalSave, error),
) error {
	save, err := edit(cmd.Context())
	if err != nil {
		return classifyCanonicalValueError(code, err)
	}
	return writeResult(cmd.OutOrStdout(), state.format, save, configurationEditText(save))
}

func requiredConfigBaseRevision(cmd *cobra.Command, raw string) (string, error) {
	if !cmd.Flags().Changed("base-revision") {
		return "", &Error{Kind: ErrorUsage, Code: "base_revision_required", Message: "--base-revision is required; read canonical_revision_id from config show --output json"}
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", &Error{Kind: ErrorUsage, Code: "base_revision_invalid", Message: "--base-revision must be the current canonical revision ID"}
	}
	return value, nil
}

func configurationEditText(result application.CanonicalSave) string {
	if result.NoChange {
		return fmt.Sprintf("canonical revision #%d %s is unchanged", result.Revision.Sequence, result.Revision.ID)
	}
	return fmt.Sprintf("saved canonical revision #%d %s", result.Revision.Sequence, result.Revision.ID)
}

func classifyCanonicalValueError(code string, err error) error {
	switch {
	case application.IsRevisionConflict(err):
		return &Error{Kind: ErrorConflict, Code: "canonical_revision_conflict", Message: err.Error(), Cause: err}
	case errors.Is(err, store.ErrConfigurationFileUnparsed):
		return &Error{Kind: ErrorValidation, Code: "configuration_file_unparsed", Message: "saved configuration is not valid JSON; correct it with config import before editing fields", Cause: err}
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
