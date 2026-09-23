// SPDX-License-Identifier: GPL-3.0-or-later

// Package cli owns the public command tree and delegates business work.
package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/installation"
	"github.com/rehuony/sing-box-panel/internal/selfupdate"
	"github.com/rehuony/sing-box-panel/internal/settings"
	"github.com/rehuony/sing-box-panel/internal/store"
	panelSystemd "github.com/rehuony/sing-box-panel/internal/systemd"
	"github.com/spf13/cobra"
)

type Dependencies struct {
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	Build           buildinfo.Info
	Update          func(context.Context, string, selfupdate.ProgressFunc) (selfupdate.Result, error)
	RunServer       func(context.Context, string) error
	OpenApplication func(context.Context, string) (*application.Application, error)
	Systemd         panelSystemd.Service
}

type options struct {
	settingsPath string
	format       outputFormat
}

func NewRootCommand(deps Dependencies) *cobra.Command {
	// Building the command tree must not load settings or open storage. Only
	// handlers that need instance data resolve these dependencies when executed.
	state := &options{}
	systemdService := deps.Systemd
	if systemdService == nil {
		manager, err := panelSystemd.New(panelSystemd.Options{})
		if err == nil {
			systemdService = manager
		}
	}
	root := &cobra.Command{
		Use:           "sing-box-panel",
		Short:         "Manage sing-box artifacts, configuration, runtime, and subscriptions",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetIn(deps.Stdin)
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	root.SetUsageTemplate(usageTemplate)
	root.PersistentFlags().StringVarP(&state.settingsPath, "config", "c", settings.DefaultPath(), "settings file path")
	root.PersistentFlags().VarP(newOutputValue(&state.format), "output", "o", "output format: text, json, or jsonl")
	root.AddCommand(
		newInitCommand(state),
		newVersionCommand(state, deps.Build),
		newUpdateCommand(state, deps.Build, deps.Update),
		newServerCommand(state, deps.RunServer),
		newCoreCommand(state, deps.OpenApplication),
		newConfigCommand(state),
		newSubscriptionChannelCommand(state, deps.OpenApplication),
		newSubscriptionSourceCommand(state, deps.OpenApplication),
		newSubscriptionTokenCommand(state, deps.OpenApplication),
		newDurableLogCommand(state, deps.OpenApplication),
		newMetricsCommand(state, deps.OpenApplication),
		newSystemCommand(state, systemdService),
		newSystemdCommand(state, systemdService),
		newCompletionCommand(root),
	)
	return root
}

type outputValue struct{ target *outputFormat }

func newOutputValue(target *outputFormat) *outputValue {
	*target = outputText
	return &outputValue{target: target}
}

func (value *outputValue) String() string {
	if value == nil || value.target == nil {
		return string(outputText)
	}
	return string(*value.target)
}

func (value *outputValue) Set(raw string) error {
	format := outputFormat(strings.ToLower(strings.TrimSpace(raw)))
	switch format {
	case outputText, outputJSON, outputJSONL:
		*value.target = format
		return nil
	default:
		return &Error{Kind: ErrorUsage, Code: "invalid_output", Message: "output must be text, json, or jsonl"}
	}
}

func (value *outputValue) Type() string { return "format" }

func newInitCommand(state *options) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create settings and initialize the data directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			value, err := settings.Initialize(state.settingsPath, force)
			if err != nil {
				return &Error{Kind: ErrorValidation, Code: "initialization_failed", Message: err.Error(), Cause: err}
			}
			value, err = installation.PrepareDataLocation(cmd.Context(), state.settingsPath)
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "data_migration_failed", Message: err.Error(), Cause: err}
			}
			database, err := store.Open(cmd.Context(), filepath.Join(value.DataDir, "panel.db"))
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "database_initialization_failed", Message: err.Error(), Cause: err}
			}
			info, err := database.SchemaInfo(cmd.Context())
			closeErr := database.Close()
			if err != nil {
				return &Error{Kind: ErrorDomain, Code: "database_verification_failed", Message: err.Error(), Cause: err}
			}
			if closeErr != nil {
				return &Error{Kind: ErrorDomain, Code: "database_close_failed", Message: closeErr.Error(), Cause: closeErr}
			}
			if err := settings.EstablishDataLocation(cmd.Context(), state.settingsPath, value.DataDir); err != nil {
				return err
			}
			result := map[string]any{"settings_path": state.settingsPath, "data_dir": value.DataDir, "schema_version": info.Version}
			return writeResult(cmd.OutOrStdout(), state.format, result, fmt.Sprintf("initialized %s", state.settingsPath))
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing settings file")
	return cmd
}

func newVersionCommand(state *options, info buildinfo.Info) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print sing-box-panel build information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			version := info.Version
			switch version {
			case "", "dev", "(devel)":
				version = "unknown"
			}
			text := "sing-box-panel " + version
			return writeResult(cmd.OutOrStdout(), state.format, info, text)
		},
	}
}
