// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/agent"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/httpapi"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	nodeweb "github.com/rehuony/sing-box-panel/web/apps/node"
	"github.com/spf13/cobra"
)

// supervisorMarker is set to systemd by the node's own units.
const supervisorMarker = "SING_BOX_NODE_SUPERVISOR"

func newServerCommand(deps Dependencies, state *options) *cobra.Command {
	command := cliui.Group("server", "Run or manage a manually started node")
	dataDir := func() (string, error) { return settings.LoadDataDir(state.settingsPath) }
	command.AddCommand(newServerStartCommand(deps, state),
		cliui.ServerControl("sing-box-node", "stop", dataDir, &state.format), cliui.ServerControl("sing-box-node", "status", dataDir, &state.format))
	return command
}

func newServerStartCommand(deps Dependencies, state *options) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Run the node and its console in this terminal until stopped (Ctrl+C or server stop)",
		Long: "Run the node and its console in this terminal until stopped (Ctrl+C or server stop).\n\n" +
			"Missing settings are created first, with a generated console password that is printed once. " +
			"An unenrolled node waits to be enrolled from its console or with sing-box-node enroll.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Refuse before creating or opening anything a service account owns.
			guarded := []string{state.settingsPath}
			if dataDir, err := settings.LoadDataDir(state.settingsPath); err == nil {
				guarded = append(guarded, dataDir)
			}
			if err := systemd.CheckRootStart("sing-box-node", guarded...); err != nil {
				return cliui.Fail(cliui.ErrorPermission, "root_start_refused", err)
			}
			value, created, err := settings.LoadOrInitialize(cmd.Context(), state.settingsPath)
			if created {
				if outputErr := writeInitialization(cmd.ErrOrStderr(), state.format, value); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			}
			if err != nil {
				return cliui.Fail(cliui.ErrorValidation, "settings_invalid", err)
			}
			// A running node holds the console port too; name it rather than the port.
			if running, err := procctl.LeaseHeld(value.DataDir); err == nil && running {
				return &cliui.Error{Kind: cliui.ErrorConflict, Code: "server_running",
					Message: "a node already runs with the data directory " + value.DataDir + "\nInspect: sing-box-node server status"}
			}
			link, err := agent.OpenLink(value.DataDir, deps.Build)
			if err != nil {
				return cliui.Fail(cliui.ErrorValidation, "enrollment_invalid", err)
			}
			listener, err := httpapi.Listen(value.Listen())
			if err != nil {
				return &cliui.Error{Kind: cliui.ErrorUnavailable, Code: "console_listen_failed", Cause: err,
					Message: "console: " + err.Error() + "\nNext: sing-box-node config set server.port PORT"}
			}
			defer listener.Close()
			output := cliui.ConsoleFrom(cliui.WithConsole(cmd.Context(), cmd.ErrOrStderr(), state.format != cliui.Text))
			log := func(level, message string) { output.Event(time.Now(), level, "", message) }
			options := agent.Options{
				Log: log,
				Control: &procctl.Status{
					Version: deps.Build.Version, SettingsPath: value.Path(), Listen: listener.Addr().String(),
					ManagedBy: procctl.ManagedBy(supervisorMarker),
				},
				Console: func(ctx context.Context, readings agent.ReadingsSource) error {
					handler := httpapi.NewHandler(httpapi.Options{
						SettingsPath: value.Path(), Link: link, Readings: readings, Build: deps.Build, Assets: nodeweb.Assets(),
						Logf: func(format string, args ...any) { log("info", fmt.Sprintf(format, args...)) },
					})
					return httpapi.Serve(ctx, listener, handler)
				},
				Ready: func() {
					style := output.Style()
					output.Ready("sing-box-node is running", []cliui.Field{
						{Label: "Console URL", Value: style.Accent(value.ConsoleURL())},
						{Label: "Settings", Value: style.Path(value.Path())},
						{Label: "Data directory", Value: style.Path(value.DataDir)},
					}, "Press Ctrl+C to stop.", map[string]string{
						"event": "server_ready", "console_url": value.ConsoleURL(), "settings_path": value.Path(), "data_dir": value.DataDir,
					})
				},
			}
			return agent.Run(cmd.Context(), link, options)
		},
	}
}

// writeInitialization reports settings created with a generated console
// password, which is shown only here.
func writeInitialization(output io.Writer, format cliui.Format, value settings.Settings) error {
	if value.InitialPassword == "" {
		return nil
	}
	style := cliui.NewStyle(output, format)
	text := cliui.Notice(cliui.Section(style.Success("sing-box-node settings created"), cliui.Fields(
		cliui.Field{Label: "Console URL", Value: style.Accent(value.ConsoleURL())},
		cliui.Field{Label: "Initial password", Value: value.InitialPassword},
		cliui.Field{Label: "Settings", Value: style.Path(value.Path())},
		cliui.Field{Label: "Data directory", Value: style.Path(value.DataDir)},
	), "Save this password now; only its hash is stored."))
	return cliui.Write(output, format, map[string]string{
		"event": "settings_initialized", "settings_path": value.Path(), "data_dir": value.DataDir,
		"console_url": value.ConsoleURL(), "console_password": value.InitialPassword,
	}, text)
}
