// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/spf13/cobra"
)

func newConfigCommand(state *options) *cobra.Command {
	root := group("config", "Initialize, read, edit, or verify panel settings")
	root.Long = `Manage the panel settings file selected by --config.

The Web UI, CLI, and manual edits share this same file. These commands do not
open the database. Use the Web UI to manage sing-box configuration.`
	root.AddCommand(newConfigInitCommand(state), newConfigShowCommand(state), newConfigGetCommand(state),
		newConfigSetCommand(state), newConfigVerifyCommand(state))
	return root
}

func newConfigInitCommand(state *options) *cobra.Command {
	var force bool
	command := &cobra.Command{
		Use:   "init",
		Short: "Generate a default panel settings file with a random administrator password",
		Long: `Create the settings file selected by --config using the current defaults
and a new random administrator password. Create missing settings directories with private
permissions. The data directory and database are untouched and no service starts.

An existing file is preserved unless --force is specified. Forced initialization
replaces the settings and rotates the administrator password; migration recovery records
are preserved. Directories, symlinks, and pending recovery cannot be overwritten.`,
		Example: `  sing-box-panel config init
  sing-box-panel config init --config ./setting.json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			value, err := settings.InitializeFile(cmd.Context(), state.settingsPath, force)
			if err != nil {
				failure := &Error{Kind: ErrorValidation, Code: "settings_initialization_failed", Message: err.Error(), Cause: err}
				return errors.Join(failure, writeServerInitialization(cmd, state, value))
			}
			text := initializationText(cmd.OutOrStdout(), state.format, value)
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{
				"initialized": true, "settings_path": value.Path(), "login_email": value.Auth.Email, "login_password": value.InitialPassword,
			}, text)
		},
	}
	command.Flags().BoolVar(&force, "force", false, "replace an existing settings file and generate a new administrator password")
	return command
}

func newConfigGetCommand(state *options) *cobra.Command {
	return &cobra.Command{
		Use:   "get FIELD",
		Short: "Read one stored panel setting or section",
		Long: `Read one field or section from the selected --config file using a dotted
name such as server.port. Text output prints strings without JSON quotes and
other values as JSON. Structured output includes settings_path, field, and value.

Only stored values are returned: omitted fields are reported as missing and
relative paths are unchanged. No database is opened and no files are changed.
Credential fields are returned as stored. auth.password is write-only; the file
contains auth.password_hash instead.`,
		Example: `  sing-box-panel config get server.port
  sing-box-panel config get server --output json`,
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := settings.GetField(cmd.Context(), state.settingsPath, args[0])
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_read_failed", Message: err.Error(), Cause: err}
			}
			var compact bytes.Buffer
			if err := json.Compact(&compact, value); err != nil {
				return err
			}
			text := compact.String()
			if len(text) > 0 && text[0] == '"' {
				if err := json.Unmarshal(value, &text); err != nil {
					return err
				}
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{
				"settings_path": state.settingsPath, "field": args[0], "value": value,
			}, text)
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
	var stdin bool
	command := &cobra.Command{
		Use:   "set [FIELD VALUE]",
		Short: "Update one panel setting or replace the complete settings file",
		Long: `Set one field using a dotted name such as server.port.
String values are literal text; other values use JSON, including
numbers, arrays, and objects. Use --stdin instead of VALUE to read a value,
removing only its final line ending. Use an empty string ('') to clear a string,
null for nullable fields, or [] for an empty array. Required values cannot be
cleared. The complete result must validate before the existing file is changed.

Set auth.password to hash a plaintext password with Argon2id before saving it
as auth.password_hash. Use --stdin to keep passwords out of shell history.
Passwords contain 12–128 Unicode characters; spaces are preserved. Setting
auth.password_hash directly requires an already valid hash. Password changes
invalidate existing sessions at their next authentication check without a restart.

Alternatively, read a complete settings document from --file FILE or --file -
for stdin. Validate it before replacing the selected --config file with private
permissions. Relative data_dir paths resolve beside the destination settings
file. Full replacement creates missing settings directories; the data directory
is untouched. --file cannot be combined with FIELD, VALUE, or --stdin.

Restart the panel to reload startup fields such as its listener and data path.
Credentials and panel preferences use this same file at operation boundaries.`,
		Example: `  sing-box-panel config set server.port 9090
  sing-box-panel config set server.external_origin ''
  sing-box-panel config set traffic.quota_gib null
  sing-box-panel config set auth.password --stdin
  sing-box-panel config set --file ./new-setting.json
  sing-box-panel --config ./setting.json config set --file - < ./new-setting.json`,
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("file") {
				if filePath != "" && len(args) == 0 && !cmd.Flags().Changed("stdin") {
					return nil
				}
			} else if (stdin && len(args) == 1) || (!stdin && len(args) == 2) {
				return nil
			}
			return &Error{Kind: ErrorUsage, Code: "settings_set_usage", Message: "use set FIELD VALUE, set FIELD --stdin, or set --file FILE"}
		},
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			var saveErr error
			if filePath != "" {
				data, err := readInputFile(cmd.InOrStdin(), filePath, settings.MaximumBytes, "settings")
				if err != nil {
					return &Error{Kind: ErrorValidation, Code: "settings_input_failed", Message: err.Error(), Cause: err}
				}
				if err := cmd.Context().Err(); err != nil {
					return err
				}
				saveErr = settings.ReplaceContext(cmd.Context(), state.settingsPath, data)
			} else {
				var value string
				if stdin {
					var data []byte
					var err error
					if args[0] == "auth.password" {
						data, err = readPasswordStdin(cmd.InOrStdin())
					} else {
						data, err = readInputFile(cmd.InOrStdin(), "-", settings.MaximumBytes, "settings value")
						data = bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r"))
					}
					defer clear(data)
					if err != nil {
						return &Error{Kind: ErrorValidation, Code: "settings_input_failed", Message: err.Error(), Cause: err}
					}
					value = string(data)
				} else {
					value = args[1]
				}
				saveErr = settings.SetField(cmd.Context(), state.settingsPath, args[0], value)
			}
			if saveErr != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_save_failed", Message: saveErr.Error(), Cause: saveErr}
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]any{"settings_path": state.settingsPath, "saved": true},
				fmt.Sprintf("saved %s; restart the panel to reload startup fields", state.settingsPath))
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "replace from a complete panel settings document, or - for stdin")
	command.Flags().BoolVar(&stdin, "stdin", false, "read one field's value from stdin instead of an argument")
	return command
}

func newConfigVerifyCommand(state *options) *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Validate the panel settings file without opening the database",
		Long: `Validate JSON structure and panel settings values in the selected --config
file. This reads only the file; database and environment checks run at server
startup. It does not check the saved sing-box configuration.`,
		Example: "  sing-box-panel config verify\n  sing-box-panel config verify --config ./setting.json --output json",
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
