// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/spf13/cobra"
)

func newConfigCommand(state *options) *cobra.Command {
	root := group("config", "Show, replace, or check the panel settings file")
	root.Long = `Manage the panel settings file selected by --config.

The Web UI, CLI, and manual edits share this same file. These commands do not
open the database. Use the Web UI to manage sing-box configuration.`
	root.AddCommand(newConfigShowCommand(state), newConfigSetCommand(state), newConfigCheckCommand(state))
	return root
}

func newConfigShowCommand(state *options) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the panel settings file exactly as stored, including credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			data, err := settings.Read(state.settingsPath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_read_failed", Message: err.Error(), Cause: err}
			}
			if state.format == outputText {
				_, err := cmd.OutOrStdout().Write(data)
				return err
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{"settings_path": state.settingsPath, "content": string(data)}, "")
		},
	}
}

func newConfigSetCommand(state *options) *cobra.Command {
	var filePath string
	command := &cobra.Command{
		Use:   "set",
		Short: "Validate and atomically replace the complete panel settings file",
		Long: `Read a complete panel settings document from --file FILE or --file - for
stdin. Validate it before replacing the selected --config file with private
permissions. Relative data_dir paths resolve beside the destination settings
file. Missing settings directories are created; the data directory is untouched.

Restart the panel to reload startup fields such as its listener and data path.
Credentials and panel preferences use this same file at operation boundaries.`,
		Example: `  sing-box-panel config set --file ./new-setting.json
  sing-box-panel --config ./setting.json config set --file - < ./new-setting.json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if filePath == "" {
				return &Error{Kind: ErrorUsage, Code: "file_required", Message: "--file is required; use - for stdin"}
			}
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			data, err := readInputFile(cmd.InOrStdin(), filePath, settings.MaximumBytes, "settings")
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_input_failed", Message: err.Error(), Cause: err}
			}
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			if err := settings.ReplaceContext(cmd.Context(), state.settingsPath, data); err != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_save_failed", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{"settings_path": state.settingsPath, "saved": true},
				fmt.Sprintf("saved %s; restart the panel to reload startup fields", state.settingsPath))
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "complete panel settings document, or - for stdin (required)")
	return command
}

func newConfigCheckCommand(state *options) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate the panel settings file without opening the database",
		Long: `Validate JSON structure and panel settings values in the selected --config
file. This reads only the file; database and environment checks run at server
startup. It does not check the saved sing-box configuration.`,
		Example: `  sing-box-panel config check
  sing-box-panel config check --config ./setting.json --output json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			if _, err := settings.Load(state.settingsPath); err != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{"valid": true, "settings_path": state.settingsPath}, "panel settings are valid")
		},
	}
}
