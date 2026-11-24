// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/rehuony/sing-box-panel/internal/settingsfile"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/webauth"
	"github.com/spf13/cobra"
)

func newConfigCommand(settingsPath *string) *cobra.Command {
	var asJSON bool
	root := &cobra.Command{
		Use:   "config",
		Short: "Initialize, read, edit or verify the node settings",
		Long: "Manage the node settings file selected by --config: where the console listens, the data directory and the " +
			"console password's hash. These commands open no database.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	root.PersistentFlags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	write := func(cmd *cobra.Command, value any, text string) error {
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
		}
		_, err := fmt.Fprintln(cmd.OutOrStdout(), text)
		return err
	}

	var force bool
	initialize := &cobra.Command{
		Use:   "init",
		Short: "Write default node settings with a generated console password",
		Long: "Write default node settings to the --config file with a generated console password, which is printed once. " +
			"An existing file is kept unless --force replaces it, which also replaces the password. The data directory is untouched.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			value, err := settings.Initialize(cmd.Context(), *settingsPath, force)
			if err != nil {
				return errors.Join(err, writeInitialization(cmd.ErrOrStderr(), value))
			}
			if asJSON {
				return write(cmd, map[string]any{"initialized": true, "settings_path": value.Path(), "console_password": value.InitialPassword}, "")
			}
			return writeInitialization(cmd.OutOrStdout(), value)
		},
	}
	initialize.Flags().BoolVar(&force, "force", false, "replace an existing settings file and generate a new console password")

	show := &cobra.Command{
		Use:   "show",
		Short: "Print the settings file exactly as stored",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := settingsfile.Read(*settingsPath)
			if err != nil {
				return err
			}
			if asJSON {
				return write(cmd, map[string]any{"settings_path": *settingsPath, "content": string(data)}, "")
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}

	get := &cobra.Command{
		Use:   "get FIELD",
		Short: "Read one stored setting or section, such as server.port",
		Long: "Read one field or section using a dotted name such as server.port. Strings print without quotes, other values as JSON. " +
			"auth.password is write-only; the file holds auth.password_hash.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := settings.GetField(cmd.Context(), *settingsPath, args[0])
			if err != nil {
				return err
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
			return write(cmd, map[string]any{"settings_path": *settingsPath, "field": args[0], "value": value}, text)
		},
	}

	var filePath string
	var stdin bool
	set := &cobra.Command{
		Use:   "set [FIELD VALUE]",
		Short: "Update one setting or replace the settings file",
		Long: "Set one field using a dotted name such as server.port. Strings are literal; other values are JSON. " +
			"--stdin reads the value from standard input instead, without its final line ending.\n\n" +
			"Set auth.password to save a new console password's Argon2id hash; use --stdin to keep it out of shell history. " +
			"It applies at once and signs every console session out.\n\n" +
			"--file FILE, or --file - for standard input, replaces the whole file after validating it.\n\n" +
			"Restart the node to apply server and data_dir changes.",
		Example: "  sing-box-node config set server.port 8080\n" +
			"  sing-box-node config set server.host 0.0.0.0\n" +
			"  sing-box-node config set server.external_origin https://node.example.com\n" +
			"  sing-box-node config set auth.password --stdin",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("file") {
				if filePath != "" && len(args) == 0 && !stdin {
					return nil
				}
			} else if (stdin && len(args) == 1) || (!stdin && len(args) == 2) {
				return nil
			}
			return errors.New("use set FIELD VALUE, set FIELD --stdin, or set --file FILE")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if filePath != "" {
				data, err := readInput(cmd.InOrStdin(), filePath)
				if err != nil {
					return err
				}
				if err := settings.Replace(cmd.Context(), *settingsPath, data); err != nil {
					return err
				}
			} else {
				value := ""
				if stdin {
					var data []byte
					var err error
					if args[0] == "auth.password" {
						data, err = webauth.ReadPassword(cmd.InOrStdin())
					} else {
						data, err = readInput(cmd.InOrStdin(), "-")
						data = bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r"))
					}
					defer clear(data)
					if err != nil {
						return err
					}
					value = string(data)
				} else {
					value = args[1]
				}
				if err := settings.SetField(cmd.Context(), *settingsPath, args[0], value); err != nil {
					return err
				}
			}
			return write(cmd, map[string]any{"settings_path": *settingsPath, "saved": true},
				fmt.Sprintf("Saved %s. Restart the node to apply server and data_dir changes.", *settingsPath))
		},
	}
	set.Flags().StringVar(&filePath, "file", "", "replace the file with a complete settings document, or - for standard input")
	set.Flags().BoolVar(&stdin, "stdin", false, "read the field's value from standard input")

	verify := &cobra.Command{
		Use:   "verify",
		Short: "Validate the settings file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := settings.Load(*settingsPath); err != nil {
				return err
			}
			return write(cmd, map[string]any{"valid": true, "settings_path": *settingsPath}, "The node settings are valid.")
		},
	}
	root.AddCommand(initialize, show, get, set, verify)
	return root
}

// readInput reads a settings document or value from a file, or standard
// input for "-", bounded like a settings file.
func readInput(stdin io.Reader, path string) ([]byte, error) {
	reader := stdin
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open settings input: %w", err)
		}
		defer file.Close()
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, settingsfile.MaximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read settings input: %w", err)
	}
	if len(data) > settingsfile.MaximumBytes {
		return nil, fmt.Errorf("settings input exceeds %d bytes", settingsfile.MaximumBytes)
	}
	return data, nil
}
