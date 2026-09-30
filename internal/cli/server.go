// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rehuony/sing-box-panel/internal/console"
	"github.com/rehuony/sing-box-panel/internal/panelprocess"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/spf13/cobra"
)

func newServerCommand(state *options, run func(context.Context, string) error) *cobra.Command {
	command := group("server", "Run or manage a manually started panel process")
	command.AddCommand(newServerStartCommand(state, run), newServerControlCommand(state, "stop"), newServerControlCommand(state, "status"))
	return command
}

func newServerStartCommand(state *options, run func(context.Context, string) error) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Run the panel in this terminal until stopped (Ctrl+C or server stop)",
		Long: "Run the panel in this terminal until stopped (Ctrl+C or server stop).\n" +
			"Create default settings when the selected file is missing and print first-run guidance.\n" +
			"Existing settings are validated and never replaced.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if run == nil {
				return &Error{Kind: ErrorUnavailable, Code: "server_unavailable", Message: "server runner is unavailable"}
			}
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			configuration, created, err := settings.LoadOrInitializeContext(cmd.Context(), state.settingsPath)
			if created {
				if outputErr := writeServerInitialization(cmd, state, configuration); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			}
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_invalid", Message: err.Error(), Cause: err}
			}
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			return run(console.WithOutput(cmd.Context(), cmd.ErrOrStderr(), state.format != outputText), state.settingsPath)
		},
	}
}

func writeServerInitialization(cmd *cobra.Command, state *options, configuration settings.Settings) error {
	if configuration.InitialPassword == "" {
		return nil
	}
	selectedPath := configuration.Path()
	if selectedPath == "" {
		selectedPath = state.settingsPath
	}
	path, err := filepath.Abs(selectedPath)
	if err != nil {
		return err
	}
	panelURL := initialPanelURL(configuration)
	result := struct {
		Event         string `json:"event"`
		SettingsPath  string `json:"settings_path"`
		DataDir       string `json:"data_dir"`
		PanelURL      string `json:"default_panel_url"`
		LoginEmail    string `json:"login_email"`
		LoginPassword string `json:"login_password"`
	}{"settings_initialized", path, configuration.DataDir, panelURL, configuration.Auth.Email, configuration.InitialPassword}
	text := initializationText(cmd.ErrOrStderr(), state.format, configuration)
	return writeResult(cmd.ErrOrStderr(), state.format, result, text)
}

func newServerControlCommand(state *options, action string) *cobra.Command {
	var timeout time.Duration
	short := "Inspect the panel process for the selected data directory"
	if action == "stop" {
		short = "Stop a manually started panel and wait for graceful cleanup"
	}
	command := &cobra.Command{
		Use: action, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if timeout <= 0 {
				return &Error{Kind: ErrorUsage, Code: "invalid_timeout", Message: "timeout must be positive"}
			}
			dataDir, err := settings.LoadDataDir(state.settingsPath)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "settings_invalid", Message: err.Error(), Cause: err}
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			var result panelprocess.Status
			if action == "stop" {
				result, err = panelprocess.Stop(ctx, dataDir)
			} else {
				result, err = panelprocess.Inspect(ctx, dataDir)
			}
			if err != nil {
				kind := ErrorUnavailable
				if errors.Is(err, os.ErrPermission) {
					kind = ErrorPermission
				}
				return &Error{Kind: kind, Code: "server_" + action + "_failed", Message: err.Error(), Cause: err}
			}
			text := fmt.Sprintf("panel %s; data directory: %s", result.State, result.DataDir)
			if result.Listen != "" {
				text += "; listen: " + result.Listen
			}
			if result.ManagedBy != "" {
				text += "; managed by: " + result.ManagedBy
			}
			return writeResult(cmd.OutOrStdout(), state.format, result, text)
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "maximum wait; stopping continues if this expires")
	return command
}
