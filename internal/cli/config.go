// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"fmt"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/spf13/cobra"
)

func newConfigCommand(state *options) *cobra.Command {
	root := group("config", "Initialize, show, replace, reset, or verify panel settings")
	root.Long = `Manage the panel settings file selected by --config.

The Web UI, CLI, and manual edits share this same file. These commands do not
open the database. Use the Web UI to manage sing-box configuration.`
	root.AddCommand(newConfigInitCommand(state), newConfigShowCommand(state), newConfigSetCommand(state),
		newConfigValidationCommand(state, "check"), newConfigValidationCommand(state, "verify"), newConfigUnsetCommand(state))
	return root
}

func newConfigInitCommand(state *options) *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:   "init",
		Short: "Generate a default panel settings file with a random login token",
		Long: `Create the settings file selected by --config using the current defaults
and a new random login token. Create missing settings directories with private
permissions. The data directory and database are untouched and no service starts.

An existing file is preserved unless --force is specified. Forced initialization
replaces the settings and rotates the login token; migration recovery records
are preserved. Directories, symlinks, and pending recovery cannot be overwritten.`,
		Example: `  sing-box-panel config init
  sing-box-panel config init --config ./setting.json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			value, err := settings.InitializeFile(cmd.Context(), state.settingsPath, force)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_initialization_failed", Message: err.Error(), Cause: err}
			}
			text := fmt.Sprintf("Default settings created\n  Settings     %s\n  Login token  %s", value.Path(), value.Auth.Token)
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{
				"initialized": true, "settings_path": value.Path(), "login_token": value.Auth.Token,
			}, text)
		},
	}
	command.Flags().BoolVar(&force, "force", false, "replace an existing settings file and generate a new login token")
	return command
}

func newConfigUnsetCommand(state *options) *cobra.Command {
	return &cobra.Command{
		Use:   "unset FIELD [FIELD...]",
		Short: "Restore selected panel settings fields to their defaults",
		Long: `Restore one or more fields or sections to the defaults used by init.
Use dotted names such as server.port or paths such as /server/port. Multiple
fields are reset atomically and the complete result must remain valid. Required
values without a default, such as auth.token, cannot be reset.

Only the shared settings file changes. Startup settings take effect after a
manual restart; resetting data_dir follows the existing data migration workflow.`,
		Example: `  sing-box-panel config unset server.port
  sing-box-panel config unset /subscription/provider
  sing-box-panel config unset server.external_origin auth.secure_cookie`,
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := settings.ResetFields(cmd.Context(), state.settingsPath, args); err != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_reset_failed", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{
				"settings_path": state.settingsPath, "saved": true, "reset_fields": args,
			}, fmt.Sprintf("restored defaults for %s", strings.Join(args, ", ")))
		},
	}
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

func newConfigValidationCommand(state *options, name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: "Validate the panel settings file without opening the database",
		Long: `Validate JSON structure and panel settings values in the selected --config
file. This reads only the file; database and environment checks run at server
startup. It does not check the saved sing-box configuration.`,
		Example: fmt.Sprintf("  sing-box-panel config %s\n  sing-box-panel config %s --config ./setting.json --output json", name, name),
		Args:    cobra.NoArgs,
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
