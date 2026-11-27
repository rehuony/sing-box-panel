// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli owns the public command tree and delegates business work.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/nodeapi"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/installation"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/settings"
	"github.com/rehuony/sing-box-panel/internal/sing-box-panel/store"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/sing-box-panel/systemd"
	"github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	CleanupHistoryPath string
	Stdin              io.Reader
	Stdout             io.Writer
	Stderr             io.Writer
	Build              buildinfo.Info
	Update             func(context.Context, string, selfupdate.ProgressFunc) (selfupdate.Result, error)
	RunServer          func(context.Context, string) error
	Systemd            systemdService
}

type options struct {
	settingsPath string
	historyPath  string
	format       cliui.Format
}

func NewRootCommand(deps Dependencies) *cobra.Command {
	// Building the command tree must not load settings or open storage. Only
	// handlers that need instance data resolve these dependencies when executed.
	state := &options{historyPath: deps.CleanupHistoryPath}
	if state.historyPath == "" {
		state.historyPath = installation.DefaultHistoryPath()
	}
	service := deps.Systemd
	if service == nil {
		manager, err := panelSystemd.New(systemd.Options{})
		if err == nil {
			service = manager
		}
	}
	root := cliui.Root(cliui.Group("sing-box-panel", "Manage panel configuration and services"))
	root.SetIn(deps.Stdin)
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	root.PersistentFlags().StringVarP(&state.settingsPath, "config", "c", settings.DefaultPath(), "settings file path")
	cliui.AddOutputFlag(root.PersistentFlags(), &state.format)
	root.AddCommand(
		newInitCommand(state),
		cliui.Version("sing-box-panel", deps.Build, fmt.Sprintf("%d.%d", nodeapi.ProtocolMajor, nodeapi.ProtocolMinor), &state.format),
		newUpdateCommand(state, deps.Build, deps.Update),
		newServerCommand(state, deps.RunServer),
		newConfigCommand(state),
		newNodeCommand(state),
		newSystemCommand(state, service, deps.Build),
		newSystemdCommand(state, service),
		cliui.Completion(root),
	)
	return root
}

func newInitCommand(state *options) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create settings and initialize the data directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) (runErr error) {
			value, err := settings.Initialize(state.settingsPath, force)
			initial := value
			outputAttempted := false
			defer func() {
				// Creation is durable even if storage preparation later fails. Give
				// the operator the one-time credential without claiming readiness.
				if runErr != nil && !outputAttempted {
					runErr = errors.Join(runErr, writeServerInitialization(cmd, state, initial))
				}
			}()
			if err != nil {
				return &cliui.Error{Kind: cliui.ErrorValidation, Code: "initialization_failed", Message: err.Error(), Cause: err}
			}
			value, err = installation.PrepareDataLocation(cmd.Context(), state.settingsPath)
			if err != nil {
				return &cliui.Error{Kind: cliui.ErrorDomain, Code: "data_migration_failed", Message: err.Error(), Cause: err}
			}
			database, err := store.Open(cmd.Context(), filepath.Join(value.DataDir, "panel.db"))
			if err != nil {
				return &cliui.Error{Kind: cliui.ErrorDomain, Code: "database_initialization_failed", Message: err.Error(), Cause: err}
			}
			info, err := database.SchemaInfo(cmd.Context())
			closeErr := database.Close()
			if err != nil {
				return &cliui.Error{Kind: cliui.ErrorDomain, Code: "database_verification_failed", Message: err.Error(), Cause: err}
			}
			if closeErr != nil {
				return &cliui.Error{Kind: cliui.ErrorDomain, Code: "database_close_failed", Message: closeErr.Error(), Cause: closeErr}
			}
			if err := settings.EstablishDataLocation(cmd.Context(), state.settingsPath, value.DataDir); err != nil {
				return err
			}
			result := map[string]any{"settings_path": state.settingsPath, "data_dir": value.DataDir, "schema_version": info.Version, "login_email": value.Auth.Email, "login_password": initial.InitialPassword}
			outputAttempted = true
			return cliui.Write(cmd.OutOrStdout(), state.format, result, initializationText(cmd.OutOrStdout(), state.format, initial))
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing settings file")
	return cmd
}
