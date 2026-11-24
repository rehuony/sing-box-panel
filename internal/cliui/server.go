// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/rehuony/sing-box-panel/internal/procctl"
	"github.com/spf13/cobra"
)

// ServerControl is server stop or server status of the process that runs
// name with the data directory that dataDir reads from its settings.
func ServerControl(name, action string, dataDir func() (string, error), format *Format) *cobra.Command {
	var timeout time.Duration
	short := "Show the state of the " + name + " process started with these settings"
	if action == "stop" {
		short = "Stop a manually started " + name + " and wait for it to finish"
	}
	command := &cobra.Command{
		Use: action, Short: short, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if timeout <= 0 {
				return &Error{Kind: ErrorUsage, Code: "invalid_timeout", Message: "timeout must be positive"}
			}
			directory, err := dataDir()
			if err != nil {
				return Fail(ErrorValidation, "settings_invalid", err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			var status procctl.Status
			if action == "stop" {
				status, err = procctl.Stop(ctx, directory)
			} else {
				status, err = procctl.Inspect(ctx, directory)
			}
			if err != nil {
				kind := ErrorUnavailable
				if errors.Is(err, os.ErrPermission) {
					kind = ErrorPermission
				}
				return Fail(kind, "server_"+action+"_failed", err)
			}
			style := NewStyle(cmd.OutOrStdout(), *format)
			return Write(cmd.OutOrStdout(), *format, status, ProcessText(name, status, style))
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "maximum wait; stopping continues if this expires")
	return command
}

// ProcessText describes a server process's state and what it runs.
func ProcessText(name string, status procctl.Status, style Style) string {
	state := map[string]string{"ready": "running"}[status.State]
	if state == "" {
		state = status.State
	}
	title := name + " is " + state
	switch status.State {
	case "ready":
		title = style.Success(title)
	case "stopped":
		title = style.Title(title)
	default:
		title = style.Warning(title)
	}
	fields := []Field{{Label: "Data directory", Value: style.Path(status.DataDir)}}
	if status.State != "stopped" {
		started := ""
		if !status.StartedAt.IsZero() {
			started = status.StartedAt.Local().Format(time.DateTime)
		}
		pid := ""
		if status.PID > 0 {
			pid = strconv.Itoa(status.PID)
		}
		fields = append([]Field{
			{Label: "PID", Value: pid},
			{Label: "Version", Value: status.Version},
			{Label: "Listen", Value: status.Listen},
			{Label: "Settings", Value: style.Path(status.SettingsPath)},
		}, append(fields, Field{Label: "Managed by", Value: status.ManagedBy}, Field{Label: "Started", Value: started})...)
	}
	return Section(title, Fields(fields...))
}
