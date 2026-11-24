// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

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

func newServerCommand(deps Dependencies, settingsPath *string) *cobra.Command {
	command := &cobra.Command{
		Use:   "server",
		Short: "Run or manage a manually started node",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	command.AddCommand(newServerStartCommand(deps, settingsPath), newServerControlCommand(settingsPath, "stop"), newServerControlCommand(settingsPath, "status"))
	return command
}

func newServerStartCommand(deps Dependencies, settingsPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Run the node and its console in this terminal until stopped (Ctrl+C or server stop)",
		Long: "Run the node and its console in this terminal until stopped (Ctrl+C or server stop).\n\n" +
			"Missing settings are created first, with a generated console password that is printed once. " +
			"An unenrolled node waits to be enrolled from its console or with sing-box-node enroll.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Refuse before creating or opening anything a service account owns.
			guarded := []string{*settingsPath}
			if dataDir, err := settings.LoadDataDir(*settingsPath); err == nil {
				guarded = append(guarded, dataDir)
			}
			if err := systemd.CheckRootStart("sing-box-node", guarded...); err != nil {
				return err
			}
			value, created, err := settings.LoadOrInitialize(cmd.Context(), *settingsPath)
			if created {
				if outputErr := writeInitialization(cmd.ErrOrStderr(), value); outputErr != nil {
					return errors.Join(err, outputErr)
				}
			}
			if err != nil {
				return err
			}
			// A running node holds the console port too; name it rather than the port.
			if running, err := procctl.LeaseHeld(value.DataDir); err == nil && running {
				return fmt.Errorf("a node already runs with the data directory %s; see sing-box-node server status", value.DataDir)
			}
			link, err := agent.OpenLink(value.DataDir, deps.Build)
			if err != nil {
				return err
			}
			listener, err := httpapi.Listen(value.Listen())
			if err != nil {
				return fmt.Errorf("console: %w; change server.port with sing-box-node config set server.port PORT", err)
			}
			defer listener.Close()
			logger := log.New(cmd.ErrOrStderr(), "", log.LstdFlags)
			options := agent.Options{
				Logf: logger.Printf,
				Control: &procctl.Status{
					Version: deps.Build.Version, SettingsPath: value.Path(), Listen: listener.Addr().String(),
					ManagedBy: procctl.ManagedBy(supervisorMarker),
				},
				Console: func(ctx context.Context, readings agent.ReadingsSource) error {
					handler := httpapi.NewHandler(httpapi.Options{
						SettingsPath: value.Path(), Link: link, Readings: readings, Build: deps.Build, Assets: nodeweb.Assets(), Logf: logger.Printf,
					})
					return httpapi.Serve(ctx, listener, handler)
				},
				Ready: func() {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "sing-box-node is running\n\n  Console   %s\n  Settings  %s\n  Data Dir  %s\n\nPress Ctrl+C to stop.\n",
						value.ConsoleURL(), value.Path(), value.DataDir)
				},
			}
			return agent.Run(cmd.Context(), link, options)
		},
	}
}

// writeInitialization reports settings created with a generated console
// password, which is shown only here.
func writeInitialization(output io.Writer, value settings.Settings) error {
	if value.InitialPassword == "" {
		return nil
	}
	_, err := fmt.Fprintf(output, "\nsing-box-node settings created\n\n  Console           %s\n  Initial Password  %s\n  Settings          %s\n  Data Dir          %s\n\n"+
		"  Save this password now; only its hash is stored. Change it with: sing-box-node config set auth.password --stdin\n\n",
		value.ConsoleURL(), value.InitialPassword, value.Path(), value.DataDir)
	return err
}

func newServerControlCommand(settingsPath *string, action string) *cobra.Command {
	var timeout time.Duration
	var asJSON bool
	short := "Show the state of the node started with these settings"
	if action == "stop" {
		short = "Stop a manually started node and wait for it to finish"
	}
	command := &cobra.Command{
		Use: action, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if timeout <= 0 {
				return errors.New("--timeout must be positive")
			}
			dataDir, err := settings.LoadDataDir(*settingsPath)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			var result procctl.Status
			if action == "stop" {
				result, err = procctl.Stop(ctx, dataDir)
			} else {
				result, err = procctl.Inspect(ctx, dataDir)
			}
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			text := fmt.Sprintf("node %s; data directory: %s", result.State, result.DataDir)
			if result.Listen != "" {
				text += "; console: " + result.Listen
			}
			if result.ManagedBy != "" {
				text += "; managed by: " + result.ManagedBy
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), text)
			return err
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "maximum wait; stopping continues if this expires")
	command.Flags().BoolVar(&asJSON, "json", false, "print the state as JSON")
	return command
}
