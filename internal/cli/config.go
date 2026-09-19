// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/store"
	"github.com/spf13/cobra"
)

type openApplicationFunc func(context.Context, string) (*application.Application, error)

// ConfigurationFileName is the logical name shown for the one editable
// configuration. Its bytes live in the panel database, not at a filesystem path.
const ConfigurationFileName = "config.json"

func newConfigShowCommand(state *options, open openApplicationFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the saved configuration file exactly as stored, even when invalid",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			file, err := instance.ConfigurationFile(cmd.Context())
			if err != nil {
				return classifyConfigurationFileError("configuration_file_read_failed", err)
			}
			if state.format == outputText {
				_, err := io.WriteString(cmd.OutOrStdout(), file.Content)
				return err
			}
			return writeResult(cmd.OutOrStdout(), state.format, file, "")
		},
	}
}

func newConfigExportCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath string
	var force bool
	command := &cobra.Command{
		Use:   "export",
		Short: "Write the saved configuration file bytes to a file or stdout",
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
			file, err := instance.ConfigurationFile(cmd.Context())
			if err != nil {
				return classifyConfigurationFileError("configuration_file_export_failed", err)
			}
			if filePath == "-" {
				_, err := io.WriteString(cmd.OutOrStdout(), file.Content)
				return err
			}
			if err := writePrivateExport(filePath, []byte(file.Content), force); err != nil {
				return &Error{Kind: ErrorValidation, Code: "configuration_file_export_failed", Message: err.Error(), Cause: err}
			}
			digest := sha256.Sum256([]byte(file.Content))
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{
				"file": filePath, "revision": file.Revision, "syntax_valid": file.SyntaxValid,
				"canonical_revision_id": file.CanonicalRevisionID, "sha256": hex.EncodeToString(digest[:]),
			}, fmt.Sprintf("exported configuration file revision %d (%s) to %s", file.Revision, syntaxStatusText(file), filePath))
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "destination file, or - for stdout")
	command.Flags().BoolVar(&force, "force", false, "atomically replace an existing destination")
	return command
}

func newConfigImportCommand(state *options, open openApplicationFunc) *cobra.Command {
	var filePath string
	var revision int64
	command := &cobra.Command{
		Use:   "import",
		Short: "Replace the saved configuration file using its numeric revision",
		Long: `Replace the saved configuration file text using a numeric compare-and-swap
revision. Use --revision 0 before the first save; afterwards pass the current
revision reported by "config show --output json". The exact text is stored even
when it is not valid JSON; an invalid draft blocks check, apply, start and
restart until it is corrected and never falls back to older valid content.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !cmd.Flags().Changed("revision") {
				return &Error{Kind: ErrorUsage, Code: "revision_required", Message: "--revision is required; use 0 before the first save"}
			}
			if revision < 0 {
				return &Error{Kind: ErrorUsage, Code: "revision_invalid", Message: "--revision must be a non-negative file revision"}
			}
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdin"}
			}
			raw, err := readConfigurationInput(cmd.InOrStdin(), filePath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "configuration_input_failed", Message: err.Error(), Cause: err}
			}
			instance, err := openApplication(cmd.Context(), state.settingsPath, open)
			if err != nil {
				return err
			}
			defer instance.Close()
			file, err := instance.SaveConfigurationFile(cmd.Context(), application.ConfigurationFileWrite{
				Revision: revision, Content: string(raw),
			})
			if err != nil {
				return classifyConfigurationFileError("configuration_file_save_failed", err)
			}
			text := fmt.Sprintf("saved configuration file revision %d (%s)", file.Revision, syntaxStatusText(file))
			if file.Revision == revision {
				text = fmt.Sprintf("configuration file revision %d is unchanged (%s)", file.Revision, syntaxStatusText(file))
			}
			if !file.SyntaxValid {
				text += "; check, apply, start and restart are blocked until the text is corrected"
			}
			return writeResult(cmd.OutOrStdout(), state.format, file, text)
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "configuration text file, or - for stdin")
	command.Flags().Int64Var(&revision, "revision", 0, "current file revision used as the compare-and-swap base; 0 before the first save")
	return command
}

func syntaxStatusText(file application.ConfigurationFile) string {
	if file.SyntaxValid {
		if file.CanonicalRevisionID != "" {
			return "valid JSON, canonical revision " + file.CanonicalRevisionID
		}
		return "valid JSON"
	}
	return "invalid JSON"
}

func classifyConfigurationFileError(code string, err error) error {
	switch {
	case errors.Is(err, store.ErrConfigurationFileConflict):
		return &Error{Kind: ErrorConflict, Code: "configuration_file_conflict", Message: "configuration file changed; read the current revision with config show and retry", Cause: err}
	case errors.Is(err, store.ErrConfigurationFileInvalid):
		return &Error{Kind: ErrorValidation, Code: "configuration_file_invalid", Message: err.Error(), Cause: err}
	default:
		return &Error{Kind: ErrorDomain, Code: code, Message: err.Error(), Cause: err}
	}
}

func openApplication(
	ctx context.Context,
	settingsPath string,
	open openApplicationFunc,
) (*application.Application, error) {
	if open == nil {
		return nil, &Error{Kind: ErrorUnavailable, Code: "application_unavailable", Message: "application services are unavailable"}
	}
	instance, err := open(ctx, settingsPath)
	if err != nil {
		return nil, &Error{Kind: ErrorValidation, Code: "application_open_failed", Message: err.Error(), Cause: err}
	}
	return instance, nil
}

func readConfigurationInput(stdin io.Reader, filePath string) ([]byte, error) {
	return readInputFile(stdin, filePath, int64(configuration.MaximumBytes), "configuration")
}
