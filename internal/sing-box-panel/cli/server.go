// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

func newServerCommand(state *options, run func(context.Context, string) error) *cobra.Command {
	command := cliui.Group("server", "Run or manage a manually started panel process")
	dataDir := func() (string, error) { return settings.LoadDataDir(state.settingsPath) }
	command.AddCommand(newServerStartCommand(state, run),
		cliui.ServerControl("sing-box-panel", "stop", dataDir, &state.format), cliui.ServerControl("sing-box-panel", "status", dataDir, &state.format))
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
				return &cliui.Error{Kind: cliui.ErrorUnavailable, Code: "server_unavailable", Message: "server runner is unavailable"}
			}
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			// Refuse before creating or opening anything a service account owns.
			guarded := []string{state.settingsPath}
			if dataDir, err := settings.LoadDataDir(state.settingsPath); err == nil {
				guarded = append(guarded, dataDir)
			}
			if err := systemd.CheckRootStart("sing-box-panel", guarded...); err != nil {
				return &cliui.Error{Kind: cliui.ErrorPermission, Code: "root_start_refused", Message: err.Error(), Cause: err}
			}
			configuration, created, err := settings.LoadOrInitialize(cmd.Context(), state.settingsPath)
			if created {
				if outputErr := writeServerInitialization(cmd, state, configuration); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			}
			if err != nil {
				return &cliui.Error{Kind: cliui.ErrorValidation, Code: "settings_invalid", Message: err.Error(), Cause: err}
			}
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			return run(cliui.WithConsole(cmd.Context(), cmd.ErrOrStderr(), state.format != cliui.Text), state.settingsPath)
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
	return cliui.Write(cmd.ErrOrStderr(), state.format, result, text)
}
