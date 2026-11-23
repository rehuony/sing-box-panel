// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/webauth"
	"github.com/spf13/cobra"
)

func newConsoleCommand(dataDir *string) *cobra.Command {
	command := &cobra.Command{
		Use:   "console",
		Short: "Configure the local console that enrolls this node with a panel",
		Long: "Configure the local console that enrolls this node with a panel.\n\n" +
			"The console is off by default. When enabled, sing-box-node run serves it on a loopback address only; " +
			"reach it from another machine through an SSH tunnel.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	command.AddCommand(newConsoleStatusCommand(dataDir), newConsoleEnableCommand(dataDir), newConsoleDisableCommand(dataDir), newConsolePasswordCommand(dataDir))
	return command
}

type consoleStatus struct {
	Enabled     bool   `json:"enabled"`
	Listen      string `json:"listen"`
	PasswordSet bool   `json:"password_set"`
}

func newConsoleStatusCommand(dataDir *string) *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "status",
		Short: "Print the console settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			console, err := settings.LoadConsole(*dataDir)
			if err != nil {
				return err
			}
			status := consoleStatus{Enabled: console.Enabled, Listen: console.Listen, PasswordSet: console.PasswordHash != ""}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(status)
			}
			state, password := "disabled", "not set"
			if status.Enabled {
				state = "enabled"
			}
			if status.PasswordSet {
				password = "set"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Console:  %s\nAddress:  http://%s\nPassword: %s\n", state, status.Listen, password)
			return err
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print the settings as JSON")
	return command
}

func newConsoleEnableCommand(dataDir *string) *cobra.Command {
	var listen string
	command := &cobra.Command{
		Use:   "enable",
		Short: "Enable the console, generating a password if it has none",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			console, err := settings.LoadConsole(*dataDir)
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("listen") {
				if err := settings.ValidateConsoleListen(listen); err != nil {
					return fmt.Errorf("--listen: %w", err)
				}
				console.Listen = listen
			}
			generated := ""
			if console.PasswordHash == "" {
				if generated, err = webauth.GeneratePassword(); err != nil {
					return err
				}
				if console.PasswordHash, err = webauth.HashPassword(cmd.Context(), generated); err != nil {
					return err
				}
			}
			console.Enabled = true
			if err := settings.SaveConsole(*dataDir, console); err != nil {
				return err
			}
			output := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(output, "The console is enabled at http://%s. It starts with the node; restart a running node to apply.\n", console.Listen); err != nil {
				return err
			}
			if generated != "" {
				_, err = fmt.Fprintf(output, "Console password (shown once): %s\nChange it with: sing-box-node console password --stdin\n", generated)
			}
			return err
		},
	}
	command.Flags().StringVar(&listen, "listen", settings.DefaultConsoleListen, "loopback IP address and port to listen on")
	return command
}

func newConsoleDisableCommand(dataDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable the console",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			console, err := settings.LoadConsole(*dataDir)
			if err != nil {
				return err
			}
			console.Enabled = false
			if err := settings.SaveConsole(*dataDir, console); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "The console is disabled. A running node refuses its requests at once and stops listening when it restarts.")
			return err
		},
	}
}

func newConsolePasswordCommand(dataDir *string) *cobra.Command {
	var fromStdin bool
	command := &cobra.Command{
		Use:   "password",
		Short: "Set the console password, or generate one",
		Long: "Set the console password from standard input with --stdin, or generate and print a new one.\n\n" +
			"The change applies at once and signs every console session out.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			console, err := settings.LoadConsole(*dataDir)
			if err != nil {
				return err
			}
			password, generated := "", false
			if fromStdin {
				raw, err := webauth.ReadPassword(cmd.InOrStdin())
				if err != nil {
					return err
				}
				password = string(raw)
				clear(raw)
			} else {
				if password, err = webauth.GeneratePassword(); err != nil {
					return err
				}
				generated = true
			}
			if console.PasswordHash, err = webauth.HashPassword(cmd.Context(), password); err != nil {
				return err
			}
			if err := settings.SaveConsole(*dataDir, console); err != nil {
				return err
			}
			output := cmd.OutOrStdout()
			if generated {
				if _, err := fmt.Fprintf(output, "Console password (shown once): %s\n", password); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintln(output, "The console password is changed; console sessions are signed out.")
			return err
		},
	}
	command.Flags().BoolVar(&fromStdin, "stdin", false, "read the password from standard input")
	return command
}
