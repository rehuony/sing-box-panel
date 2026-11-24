// SPDX-License-Identifier: GPL-3.0-or-later

package cli

import (
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/settings"
	nodeSystemd "github.com/rehuony/sing-box-panel/internal/sing-box-node/systemd"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

// systemdService is the node's systemd lifecycle.
type systemdService = systemd.Service[settings.Settings]

func newSystemdCommand(state *options, service systemdService) *cobra.Command {
	return cliui.Systemd(cliui.SystemdOptions[settings.Settings]{
		Name: "sing-box-node", Service: service, SettingsPath: &state.settingsPath, Format: &state.format,
		Long: "Install or manage the sing-box-node systemd service.\n\n" +
			"System scope runs the node as the dedicated sing-box-node account with its settings in /etc/sing-box-node and its data in " +
			"/var/lib/sing-box-node; user scope runs it as the current user with the selected --config.",
		DataDir: func(path string, now bool) (string, error) {
			dataDir, err := settings.LoadDataDir(path)
			if now && err == nil {
				_, err = settings.Load(path)
			}
			return dataDir, err
		},
		Created: func(cmd *cobra.Command, value settings.Settings) error {
			return writeInitialization(cmd.ErrOrStderr(), state.format, value)
		},
		Status: func(cmd *cobra.Command, status systemd.Status) error {
			style := cliui.NewStyle(cmd.OutOrStdout(), state.format)
			configured, dataDir := "unknown (the unit file does not state one unambiguous --config)", ""
			if status.UnitFileStatePath != "" {
				configured = style.Path(status.UnitFileStatePath)
				if directory, err := settings.LoadDataDir(status.UnitFileStatePath); err == nil {
					dataDir = style.Path(directory)
				}
			}
			fields := append(cliui.UnitFields(status, style),
				cliui.Field{Label: "Settings", Value: configured}, cliui.Field{Label: "Data directory", Value: dataDir})
			text := cliui.Section(cliui.UnitTitle(status, style), cliui.Fields(fields...),
				cliui.UnitNotes("sing-box-node", status, style))
			return cliui.Write(cmd.OutOrStdout(), state.format, status, text)
		},
	})
}

// defaultSystemd is the node's lifecycle on this host, or nil when systemd
// paths cannot be resolved.
func defaultSystemd() systemdService {
	manager, err := nodeSystemd.New(systemd.Options{})
	if err != nil {
		return nil
	}
	return manager
}
