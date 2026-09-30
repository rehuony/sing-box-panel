// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/rehuony/sing-box-panel/internal/auth"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newConfigHashPasswordCommand(state *options) *cobra.Command {
	var stdin bool
	command := &cobra.Command{
		Use:   "hash-password",
		Short: "Generate an Argon2id password hash without changing settings",
		Long:  "Read and confirm a password without terminal echo, or use --stdin to read one line.\nPrint only the hash in text mode, or password_hash in structured output.\nPasswords contain 12–128 Unicode characters; spaces are preserved.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			var password []byte
			if stdin {
				var err error
				password, err = readPasswordStdin(cmd.InOrStdin())
				if err != nil {
					return err
				}
			} else {
				input, ok := cmd.InOrStdin().(interface{ Fd() uintptr })
				if !ok || !term.IsTerminal(int(input.Fd())) {
					return &Error{Kind: ErrorUsage, Code: "password_input_required", Message: "use --stdin for non-interactive password input"}
				}
				fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
				var err error
				password, err = term.ReadPassword(int(input.Fd()))
				fmt.Fprintln(cmd.ErrOrStderr())
				if err != nil {
					return errors.New("could not read password")
				}
				fmt.Fprint(cmd.ErrOrStderr(), "Confirm password: ")
				confirmation, err := term.ReadPassword(int(input.Fd()))
				fmt.Fprintln(cmd.ErrOrStderr())
				if err != nil {
					return errors.New("could not read password confirmation")
				}
				matches := bytes.Equal(password, confirmation)
				clear(confirmation)
				if !matches {
					clear(password)
					return &Error{Kind: ErrorValidation, Code: "password_mismatch", Message: "passwords do not match"}
				}
			}
			defer clear(password)
			hash, err := auth.HashPassword(cmd.Context(), string(password))
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "password_invalid", Message: err.Error(), Cause: err}
			}
			return writeResult(cmd.OutOrStdout(), state.format, map[string]string{"password_hash": hash}, hash)
		},
	}
	command.Flags().BoolVar(&stdin, "stdin", false, "read the password from stdin instead of prompting")
	return command
}

// readPasswordStdin removes only the final line ending, never password spaces.
func readPasswordStdin(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 515))
	if err != nil {
		clear(data)
		return nil, errors.New("could not read password")
	}
	if len(data) > 514 {
		clear(data)
		return nil, auth.ErrPassword
	}
	return bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r")), nil
}

func newConfigResetPasswordCommand(state *options) *cobra.Command {
	var stdin bool
	command := &cobra.Command{
		Use:     "reset-password",
		Short:   "Reset the administrator password without changing other settings",
		Long:    "Generate and display a new random password, or use --stdin to supply one without echo.\nOnly the password hash in the existing --config file changes. No database is opened.\nExisting sessions become invalid at their next authentication check; no restart is needed.",
		Example: "  sing-box-panel config reset-password\n  sing-box-panel --config /etc/sing-box-panel/setting.json config reset-password",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			var password []byte
			if stdin {
				var err error
				password, err = readPasswordStdin(cmd.InOrStdin())
				if err != nil {
					return &Error{Kind: ErrorValidation, Code: "password_invalid", Message: err.Error(), Cause: err}
				}
			} else {
				generated, err := auth.GeneratePassword()
				if err != nil {
					return errors.New("could not generate password")
				}
				password = []byte(generated)
			}
			defer clear(password)
			value, err := settings.ResetPassword(cmd.Context(), state.settingsPath, string(password))
			if err != nil {
				kind := ErrorValidation
				if errors.Is(err, os.ErrPermission) {
					kind = ErrorPermission
				}
				if errors.Is(err, settings.ErrPending) {
					kind = ErrorConflict
				}
				return &Error{Kind: kind, Code: "password_reset_failed", Message: err.Error(), Cause: err}
			}
			result := map[string]any{"password_reset": true, "settings_path": value.Path(), "login_email": value.Auth.Email}
			style := newFileTreeStyle(cmd.OutOrStdout(), state.format)
			text := fmt.Sprintf("%s\n\n  Settings      %s\n  Email         %s", style.paint("1;32", "Administrator password reset"), style.path(value.Path()), value.Auth.Email)
			if !stdin {
				result["login_password"] = string(password)
				text += fmt.Sprintf("\n  New Password  %s\n\n  Save this password now; only its hash is stored.", password)
			}
			text += "\n\n  Sign in with the new password; no restart is needed."
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	command.Flags().BoolVar(&stdin, "stdin", false, "read a custom password from stdin instead of generating one")
	return command
}
